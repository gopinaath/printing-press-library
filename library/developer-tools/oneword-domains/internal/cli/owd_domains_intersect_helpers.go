// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Pure helpers for domains intersect: the paged availability scan and the
// set arithmetic over TLDs.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/client"
)

// owdIntersectEntry is what the lifetime-pass domain database says about one word on one TLD.
type owdIntersectEntry struct {
	Premium  bool    `json:"premium"`
	Price    *string `json:"price"`
	TldCount int     `json:"-"`
}

// owdIntersectWord is one word free on every requested TLD.
type owdIntersectWord struct {
	Word     string                       `json:"word"`
	TLDs     map[string]owdIntersectEntry `json:"tlds"`
	TldCount int                          `json:"tld_count"`
}

// owdIntersectOutput wraps the words with the scan accounting.
type owdIntersectOutput struct {
	Words        []owdIntersectWord `json:"words"`
	ScannedPages int                `json:"scanned_pages"`
	PerTLDCounts map[string]int     `json:"per_tld_counts"`
	TakenOn      []string           `json:"taken_on"`
	Note         string             `json:"note,omitempty"`
}

// owdIntersectScanResult is one TLD's availability set plus how far the scan got.
type owdIntersectScanResult struct {
	Words  map[string]owdIntersectEntry
	Pages  int
	Capped bool
}

// owdIntersectParseRows decodes one /api/domains page (a bare array, or an
// envelope the generated paginator understands).
func owdIntersectParseRows(data json.RawMessage) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err == nil {
		return items, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("decoding /api/domains: %w", err)
	}
	if items, ok := extractPaginatedItems(obj, "/api/domains"); ok {
		return items, nil
	}
	return nil, fmt.Errorf("decoding /api/domains: no rows in response")
}

// owdIntersectScan pages GET /api/domains for one TLD until a short page or the cap.
func owdIntersectScan(ctx context.Context, c *client.Client, tld string, base map[string]string, maxPages int) (owdIntersectScanResult, error) {
	res := owdIntersectScanResult{Words: map[string]owdIntersectEntry{}}
	for page := 1; page <= maxPages; page++ {
		params := map[string]string{"tld": tld, "page": strconv.Itoa(page)}
		for k, v := range base {
			if v != "" {
				params[k] = v
			}
		}
		data, err := c.Get(ctx, "/api/domains", params)
		if err != nil {
			return res, err
		}
		items, err := owdIntersectParseRows(data)
		if err != nil {
			return res, err
		}
		res.Pages++
		for _, it := range items {
			var row struct {
				Slug     string  `json:"slug"`
				TldCount int     `json:"tldCount"`
				Premium  bool    `json:"premium"`
				Price    *string `json:"price"`
			}
			if json.Unmarshal(it, &row) != nil || row.Slug == "" {
				continue
			}
			word := strings.ToLower(row.Slug)
			if w, _, err := owdSplitDomain(row.Slug); err == nil {
				word = w
			}
			res.Words[word] = owdIntersectEntry{Premium: row.Premium, Price: row.Price, TldCount: row.TldCount}
		}
		if len(items) < owdPageSize {
			return res, nil
		}
	}
	res.Capped = true
	return res, nil
}

// owdIntersectWords keeps the words present in every "free" set and absent
// from every "taken-on" set (a word still listed as free there is dropped).
func owdIntersectWords(sets map[string]map[string]owdIntersectEntry, free, takenOn []string) []owdIntersectWord {
	out := make([]owdIntersectWord, 0)
	if len(free) == 0 {
		return out
	}
words:
	for word, first := range sets[free[0]] {
		tlds := map[string]owdIntersectEntry{free[0]: first}
		for _, t := range free[1:] {
			e, has := sets[t][word]
			if !has {
				continue words
			}
			tlds[t] = e
		}
		for _, t := range takenOn {
			if _, stillFree := sets[t][word]; stillFree {
				continue words
			}
		}
		out = append(out, owdIntersectWord{Word: word, TLDs: tlds, TldCount: first.TldCount})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Word < out[j].Word })
	return out
}
