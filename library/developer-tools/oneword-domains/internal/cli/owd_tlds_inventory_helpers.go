// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Pure helpers for tlds inventory: the bare-integer count parser, the
// cheapest-registrar reader, TLD filtering, and ordering.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/client"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
)

var owdInventorySorts = []string{"count", "price", "top10m"}
var owdInventoryTypes = []string{"gTld", "ccTld"}

// owdInventoryRow is one TLD's available inventory for the filter.
type owdInventoryRow struct {
	TLD               string             `json:"tld"`
	Type              string             `json:"type"`
	AvailableCount    int                `json:"available_count"`
	MinPrice          string             `json:"min_price"`
	CheapestRegistrar *owdRegistrarPrice `json:"cheapest_registrar"`
	Top10m            int                `json:"top10m"`
	TotalReg          int                `json:"total_reg"`
}

// owdInventoryOutput wraps the rows with the fan-out accounting.
type owdInventoryOutput struct {
	Rows          []owdInventoryRow `json:"rows"`
	Matched       int               `json:"matched"`
	Checked       int               `json:"checked"`
	FetchFailures []owdFailure      `json:"fetch_failures"`
	Note          string            `json:"note,omitempty"`
}

// owdInventoryParseCount reads /api/domains/count, which answers a bare
// integer (sometimes quoted). An object with a numeric "count" is accepted too.
func owdInventoryParseCount(raw []byte) (int, error) {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return n, nil
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		for _, key := range []string{"count", "total", "available"} {
			if n, ok := cliutil.ExtractInt(obj, key); ok {
				return int(n), nil
			}
		}
	}
	return 0, fmt.Errorf("decoding /api/domains/count: expected an integer, got %q", strings.TrimSpace(string(raw)))
}

// owdInventoryCount calls GET /api/domains/count for one TLD.
func owdInventoryCount(ctx context.Context, c *client.Client, tld string, base map[string]string) (int, error) {
	params := map[string]string{"tld": tld}
	for k, v := range base {
		if v != "" {
			params[k] = v
		}
	}
	data, err := c.Get(ctx, "/api/domains/count", params)
	if err != nil {
		return 0, err
	}
	return owdInventoryParseCount(data)
}

// owdInventoryCheapest extracts the cheapest registrar from a stored TLD row:
// the detail route's cheapestRegistrar when present, else the lowest of the
// per-registrar price fields.
func owdInventoryCheapest(data string) *owdRegistrarPrice {
	var row struct {
		CheapestRegistrar *owdRegistrarPrice  `json:"cheapestRegistrar"`
		Registrars        []owdRegistrarPrice `json:"registrars"`
		Namecheap         *string             `json:"namecheap"`
		Godaddy           *string             `json:"godaddy"`
		Ionos             *string             `json:"ionos"`
		Porkbun           *string             `json:"porkbun"`
		Gandi             *string             `json:"gandi"`
		Oneohone          *string             `json:"oneohone"`
		Google            *string             `json:"google"`
	}
	if json.Unmarshal([]byte(data), &row) != nil {
		return nil
	}
	if row.CheapestRegistrar != nil && row.CheapestRegistrar.Name != "" {
		if _, ok := owdPriceFloat(row.CheapestRegistrar.Price); ok {
			return owdCloneRegistrar(row.CheapestRegistrar)
		}
	}
	candidates := append([]owdRegistrarPrice{}, row.Registrars...)
	for name, p := range map[string]*string{"namecheap": row.Namecheap, "godaddy": row.Godaddy, "ionos": row.Ionos, "porkbun": row.Porkbun, "gandi": row.Gandi, "101 domains": row.Oneohone, "google": row.Google} {
		if p != nil {
			candidates = append(candidates, owdRegistrarPrice{Name: name, Price: *p})
		}
	}
	var best *owdRegistrarPrice
	bestPrice := 0.0
	for i := range candidates {
		p, ok := owdPriceFloat(candidates[i].Price)
		if !ok {
			continue
		}
		if best == nil || p < bestPrice || (p == bestPrice && candidates[i].Name < best.Name) {
			c := candidates[i]
			best, bestPrice = &c, p
		}
	}
	return best
}

// owdInventoryCheapestFromStore maps slug to cheapest registrar for the
// stored TLDs among slugs: the typed tlds row first, then the latest
// registrar snapshot in owd_tld_prices for TLDs whose stored row carries no
// registrar prices. Only the requested rows are read and decoded.
func owdInventoryCheapestFromStore(ctx context.Context, db *store.Store, slugs []string) map[string]*owdRegistrarPrice {
	out := map[string]*owdRegistrarPrice{}
	if db == nil {
		return out
	}
	for chunk := range slices.Chunk(slugs, owdLookupChunk) {
		in, args := owdInArgs(chunk)
		rows, err := db.DB().QueryContext(ctx, `SELECT slug, data FROM tlds WHERE slug IN (`+in+`)`, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var slug string
			var data any
			if err := rows.Scan(&slug, &data); err != nil {
				continue
			}
			if cr := owdInventoryCheapest(owdScanString(data)); cr != nil {
				out[slug] = cr
			}
		}
		_ = rows.Close()
	}
	for slug, cr := range owdInventoryCheapestFromSnapshots(ctx, db) {
		if _, has := out[slug]; !has {
			out[slug] = cr
		}
	}
	return out
}

// owdInventoryCheapestFromSnapshots picks, per TLD, the lowest-priced
// registrar row at the latest non-"min" snapshot in owd_tld_prices (ties go
// to the registrar that sorts first).
func owdInventoryCheapestFromSnapshots(ctx context.Context, db *store.Store) map[string]*owdRegistrarPrice {
	out := map[string]*owdRegistrarPrice{}
	rows, err := db.DB().QueryContext(ctx, `SELECT p.tld, p.registrar, p.price FROM owd_tld_prices p
		JOIN (SELECT tld, MAX(snapshot_at) AS latest FROM owd_tld_prices WHERE registrar <> 'min' AND price IS NOT NULL GROUP BY tld) m
		ON m.tld = p.tld AND p.snapshot_at = m.latest
		WHERE p.registrar <> 'min' AND p.price IS NOT NULL
		ORDER BY p.tld, p.price, p.registrar`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var tld, reg string
		var price float64
		if err := rows.Scan(&tld, &reg, &price); err != nil {
			continue
		}
		if _, has := out[tld]; has {
			continue
		}
		out[tld] = &owdRegistrarPrice{Name: reg, Price: strconv.FormatFloat(price, 'f', -1, 64)}
	}
	return out
}

// owdInventoryFilter keeps TLDs of the requested type and, when maxPrice is
// set, those whose min price parses and is at most that ceiling.
func owdInventoryFilter(tlds []owdTLD, typ string, maxPrice *float64) []owdTLD {
	out := make([]owdTLD, 0, len(tlds))
	for _, t := range tlds {
		if typ != "" && !strings.EqualFold(t.Type, typ) {
			continue
		}
		if maxPrice != nil {
			p, ok := owdPriceFloat(t.MinPrice)
			if !ok || p > *maxPrice {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

// owdInventorySort orders rows by count (desc), price (asc, unknown last), or top10m (desc).
func owdInventorySort(rows []owdInventoryRow, key string) {
	owdSortKeyed(rows, func(r owdInventoryRow) owdPrice { return owdParsePrice(r.MinPrice) }, func(x, y owdKeyed[owdInventoryRow, owdPrice]) bool {
		a, b := x.v, y.v
		pa, pb := x.k, y.k
		switch key {
		case "price":
			if pa.ok != pb.ok {
				return pa.ok
			}
			if pa.ok && pa.v != pb.v {
				return pa.v < pb.v
			}
		case "top10m":
			if a.Top10m != b.Top10m {
				return a.Top10m > b.Top10m
			}
		default: // count
			if a.AvailableCount != b.AvailableCount {
				return a.AvailableCount > b.AvailableCount
			}
			if pa.ok && pb.ok && pa.v != pb.v {
				return pa.v < pb.v
			}
		}
		return a.TLD < b.TLD
	})
}
