// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Pure helpers for the brainstorm command: filtering, pricing, sorting, and
// the owd_generations batch writer.

package cli

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/client"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
)

var owdBrainstormSorts = []string{"price", "length", "name"}

const (
	// owdBrainstormLiveWordBudget caps the live dictionary lookups one run
	// spends on the real-word flag when the local words table is thin.
	owdBrainstormLiveWordBudget = 20
	// owdDogfoodBrainstormTLDs is the TLD count under the live dogfood
	// matrix: one DomainsGPT call per TLD spends the shared quota.
	owdDogfoodBrainstormTLDs = 1
)

// owdBrainstormRow is one kept DomainsGPT name with its registration economics.
type owdBrainstormRow struct {
	Domain            string             `json:"domain"`
	TLD               string             `json:"tld"`
	Name              string             `json:"name"`
	Available         bool               `json:"available"`
	RealWord          *bool              `json:"real_word"`
	MinPrice          string             `json:"min_price"`
	CheapestRegistrar *owdRegistrarPrice `json:"cheapest_registrar"`
	Batch             string             `json:"batch"`
}

// owdBrainstormMeta is the request context stored next to every generated row.
type owdBrainstormMeta struct {
	Type     string
	Context  string
	Word     string
	Position string
}

// owdBrainstormBatchID returns "<RFC3339 UTC>-<8 hex chars>" so batches sort by time.
func owdBrainstormBatchID(at time.Time) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return at.UTC().Format(time.RFC3339) + "-" + fmt.Sprintf("%08x", at.UnixNano()&0xffffffff)
	}
	return at.UTC().Format(time.RFC3339) + "-" + hex.EncodeToString(b[:])
}

// owdBrainstormFilter keeps available names (unless includeTaken), drops the
// excluded domains, and de-duplicates by domain while keeping stream order.
func owdBrainstormFilter(names []owdGenerated, includeTaken bool, exclude []string) []owdGenerated {
	skip := map[string]bool{}
	for _, e := range exclude {
		skip[strings.ToLower(strings.TrimSpace(e))] = true
	}
	seen := map[string]bool{}
	out := make([]owdGenerated, 0, len(names))
	for _, n := range names {
		d := strings.ToLower(strings.TrimSpace(n.Domain))
		if d == "" || seen[d] || skip[d] {
			continue
		}
		if !includeTaken && !n.Available {
			continue
		}
		seen[d] = true
		out = append(out, owdGenerated{Domain: d, Available: n.Available})
	}
	return out
}

// owdBrainstormMinPrice derives the cheapest registration price for a TLD from
// its detail route (cheapest registrar, else the lowest registrar price) and
// falls back to the list route's minPrice.
func owdBrainstormMinPrice(d *owdTLDDetail, listMin string) string {
	if d != nil {
		if d.CheapestRegistrar != nil {
			if _, ok := owdPriceFloat(d.CheapestRegistrar.Price); ok {
				return strings.TrimSpace(d.CheapestRegistrar.Price)
			}
		}
		best, bestStr := 0.0, ""
		for _, r := range d.Registrars {
			if p, ok := owdPriceFloat(r.Price); ok && (bestStr == "" || p < best) {
				best, bestStr = p, strings.TrimSpace(r.Price)
			}
		}
		if bestStr != "" {
			return bestStr
		}
	}
	if _, ok := owdPriceFloat(listMin); ok {
		return strings.TrimSpace(listMin)
	}
	return ""
}

// owdBrainstormSort orders rows by price (numeric, unknown last), length, or name.
func owdBrainstormSort(rows []owdBrainstormRow, key string) {
	owdSortKeyed(rows, func(r owdBrainstormRow) owdPrice { return owdParsePrice(r.MinPrice) },
		func(x, y owdKeyed[owdBrainstormRow, owdPrice]) bool {
			a, b := x.v, y.v
			switch key {
			case "length":
				if len(a.Name) != len(b.Name) {
					return len(a.Name) < len(b.Name)
				}
			case "name":
			default: // price
				pa, pb := x.k, y.k
				if pa.ok != pb.ok {
					return pa.ok
				}
				if pa.ok && pa.v != pb.v {
					return pa.v < pb.v
				}
			}
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.Domain < b.Domain
		})
}

// owdRealWords decides brainstorm's real-word flags. Names found in the local
// typed words table (looked up in one batch up front) are real words; with a
// populated local dictionary a miss is not one. Otherwise GET /api/words is
// asked until the live budget is exhausted, after which the flag is unknown.
// Live answers are memoized by name, so a name generated for several TLDs
// costs one lookup.
type owdRealWords struct {
	c          *client.Client
	local      map[string]bool
	localIsBig bool
	budget     int
	memo       map[string]bool
}

// owdNewRealWords prepares the real-word lookup for names in one local query.
func owdNewRealWords(ctx context.Context, c *client.Client, db *store.Store, names []string, localIsBig bool, liveBudget int) *owdRealWords {
	return &owdRealWords{c: c, local: owdLocalWords(ctx, db, names), localIsBig: localIsBig, budget: liveBudget, memo: map[string]bool{}}
}

// lookup returns the real-word flag for name (nil when unknown).
func (r *owdRealWords) lookup(ctx context.Context, name string) *bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil
	}
	if r.local[name] {
		t := true
		return &t
	}
	if r.localIsBig {
		f := false
		return &f
	}
	if ok, seen := r.memo[name]; seen {
		return &ok
	}
	if r.c == nil || r.budget <= 0 {
		return nil
	}
	r.budget--
	ok, _, err := owdLiveWordLookup(ctx, r.c, name)
	if err != nil {
		return nil
	}
	r.memo[name] = ok
	return &ok
}

// owdBrainstormSave writes every kept row into owd_generations in one transaction.
func owdBrainstormSave(ctx context.Context, db *store.Store, rows []owdBrainstormRow, meta owdBrainstormMeta, at time.Time) error {
	if db == nil || len(rows) == 0 {
		return nil
	}
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO owd_generations (created_at, batch, type, context, word, position, tld, domain, available, real_word, min_price, cheapest_registrar, cheapest_price) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	ts := at.UTC().Format(time.RFC3339)
	for _, r := range rows {
		var realWord any
		if r.RealWord != nil {
			realWord = boolInt(*r.RealWord)
		}
		var cheapName, cheapPrice sql.NullString
		if r.CheapestRegistrar != nil {
			cheapName = sql.NullString{String: r.CheapestRegistrar.Name, Valid: true}
			cheapPrice = sql.NullString{String: r.CheapestRegistrar.Price, Valid: true}
		}
		if _, err := stmt.ExecContext(ctx, ts, r.Batch, meta.Type, meta.Context, meta.Word, meta.Position, r.TLD, r.Domain, boolInt(r.Available), realWord, r.MinPrice, cheapName, cheapPrice); err != nil {
			return err
		}
	}
	return tx.Commit()
}
