// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored helpers for the per-domain check row that `check` and
// `compare` share: building, costing, ordering and printing rows, plus the
// input resolution for `check`.

package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil"
	"github.com/spf13/cobra"
)

// owdDefaultCheckTLDs is how many of the most-viewed TLDs `check` scans when
// --tld is omitted (owdDogfoodTLDs inside the live test matrix).
const owdDefaultCheckTLDs = 12

// owdCheckRow is the per-domain shape shared by `check` and `compare`.
type owdCheckRow struct {
	Word              string             `json:"word"`
	TLD               string             `json:"tld"`
	Domain            string             `json:"domain"`
	Available         *bool              `json:"available"`
	Premium           bool               `json:"premium"`
	Price             *string            `json:"price"`
	Aftermarket       bool               `json:"aftermarket"`
	TldCount          int                `json:"tld_count"`
	PopularityPct     float64            `json:"popularity_pct"`
	MinPrice          string             `json:"min_price"`
	CheapestRegistrar *owdRegistrarPrice `json:"cheapest_registrar"`
	Error             string             `json:"error,omitempty"`
	Suggestions       []string           `json:"suggestions,omitempty"`
}

// owdBuildCheckRow joins one availability response with the TLD list's
// min price and the TLD detail's cheapest registrar.
func owdBuildCheckRow(dc *owdDomainCheck, word, tld, minPrice string, detail *owdTLDDetail) owdCheckRow {
	row := owdCheckRow{Word: word, TLD: tld, Domain: word + "." + tld, MinPrice: minPrice}
	if dc != nil {
		avail := dc.Available
		row.Available = &avail
		row.Premium = dc.Premium
		row.Price = dc.Price
		row.Aftermarket = dc.Aftermarket
		row.TldCount = dc.TldCount
		row.PopularityPct = owdRoundPct(owdPopularityPct(dc.TldCount))
		if dc.Slug != "" {
			row.Domain = dc.Slug
		}
	}
	if detail != nil {
		row.CheapestRegistrar = owdCloneRegistrar(detail.CheapestRegistrar)
	}
	return row
}

// owdRowCost is the price a buyer would pay for the row: the domain's own
// (premium/aftermarket) price when the site quotes one, else the cheapest
// registrar's standard price, else the TLD list's min price.
func owdRowCost(r owdCheckRow) (float64, bool) {
	if r.Price != nil {
		if v, ok := owdPriceFloat(*r.Price); ok {
			return v, true
		}
	}
	if r.CheapestRegistrar != nil {
		if v, ok := owdPriceFloat(r.CheapestRegistrar.Price); ok {
			return v, true
		}
	}
	return owdPriceFloat(r.MinPrice)
}

// owdSortCheckRows orders available rows first (cheapest first, then domain),
// then unavailable rows by domain, then rows that carry an error.
func owdSortCheckRows(rows []owdCheckRow) {
	rank := func(r owdCheckRow) int {
		switch {
		case r.Error != "":
			return 2
		case r.Available != nil && *r.Available:
			return 0
		default:
			return 1
		}
	}
	type key struct {
		rank int
		cost owdPrice
	}
	owdSortKeyed(rows, func(r owdCheckRow) key {
		k := key{rank: rank(r)}
		if k.rank == 0 {
			k.cost.v, k.cost.ok = owdRowCost(r)
		}
		return k
	}, func(x, y owdKeyed[owdCheckRow, key]) bool {
		a, b := x.k, y.k
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.rank == 0 {
			switch {
			case a.cost.ok && b.cost.ok && a.cost.v != b.cost.v:
				return a.cost.v < b.cost.v
			case a.cost.ok != b.cost.ok:
				return a.cost.ok
			}
		}
		return x.v.Domain < y.v.Domain
	})
}

// owdCheapestAvailable returns the available row with the lowest cost.
func owdCheapestAvailable(rows []owdCheckRow) string {
	best, bestCost, found := "", 0.0, false
	for _, r := range rows {
		if r.Error != "" || r.Available == nil || !*r.Available {
			continue
		}
		c, ok := owdRowCost(r)
		if !ok {
			continue
		}
		if !found || c < bestCost || (c == bestCost && r.Domain < best) {
			best, bestCost, found = r.Domain, c, true
		}
	}
	return best
}

// owdPrintCheckRowsTable renders check/compare rows as a fixed-column table so
// taken domains and blank prices stay visible (the generic card renderer hides
// false booleans and reorders fields). Every cell is scrubbed: domains, prices
// and registrar names come from the site and must not steer the terminal.
func owdPrintCheckRowsTable(w io.Writer, rows []owdCheckRow) {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "DOMAIN\tAVAILABLE\tPREMIUM\tPRICE\tPOPULARITY\tMIN_PRICE\tREGISTRAR\tNOTE")
	for _, r := range rows {
		avail := "?"
		if r.Available != nil {
			if *r.Available {
				avail = "yes"
			} else {
				avail = "taken"
			}
		}
		price := "-"
		if r.Price != nil && *r.Price != "" {
			price = *r.Price
		}
		reg := "-"
		if r.CheapestRegistrar != nil && r.CheapestRegistrar.Name != "" {
			reg = r.CheapestRegistrar.Name + " " + r.CheapestRegistrar.Price
		}
		minPrice := r.MinPrice
		if minPrice == "" {
			minPrice = "-"
		}
		note := r.Error
		if note == "" && r.Aftermarket {
			note = "aftermarket"
		}
		premium := "no"
		if r.Premium {
			premium = "yes"
		}
		pop := "-"
		if r.Available != nil {
			pop = fmt.Sprintf("%.1f%%", r.PopularityPct)
		}
		cells := []string{r.Domain, avail, premium, price, pop, minPrice, reg, note}
		for i := range cells {
			cells[i] = cliutil.ScrubTerminal(cells[i])
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	_ = tw.Flush()
}

// owdCheckInputs resolves what `check` looks at: one bare word, one word.tld,
// or a file of either, plus the TLD set the bare words are crossed with
// (--tld, 'all', or the most-viewed default). Pinned and --tld TLDs are
// validated against the tracked list before any request is made.
func owdCheckInputs(cmd *cobra.Command, arg, file, tldFlag string, tlds []owdTLD) ([]owdWordLine, []string, error) {
	known, slugs := owdIndexTLDs(tlds)
	var lines []owdWordLine
	switch {
	case arg != "" && strings.Contains(arg, "."):
		w, t, err := owdSplitKnownDomain(arg, slugs)
		if err != nil {
			return nil, nil, usageErr(err)
		}
		if tldFlag != "" {
			return nil, nil, usageErr(fmt.Errorf("%s already names a TLD; drop --tld or pass the bare word", arg))
		}
		lines = []owdWordLine{{Word: w, TLD: t}}
	case arg != "":
		lines = []owdWordLine{{Word: arg}}
	default:
		r, err := owdOpenWordFile(cmd, file)
		if err != nil {
			return nil, nil, err
		}
		defer r.Close()
		lines, err = owdReadWordLines(r, slugs)
		if err != nil {
			return nil, nil, usageErr(fmt.Errorf("--file %s: %w", file, err))
		}
		if len(lines) == 0 {
			return nil, nil, usageErr(fmt.Errorf("--file %s contains no words", file))
		}
	}
	pinned := make([]string, 0)
	bare := false
	for _, l := range lines {
		if l.TLD != "" {
			pinned = append(pinned, l.TLD)
		} else {
			bare = true
		}
	}
	if err := owdRequireKnownTLDs(owdDedupe(pinned), known); err != nil {
		return nil, nil, err
	}
	// TLD set for the bare words.
	var tldSet []string
	switch {
	case !bare:
		// Every line pins its own TLD.
	case strings.TrimSpace(tldFlag) == "":
		tldSet = owdTopTLDs(tlds, owdDogfoodCap(owdDefaultCheckTLDs, owdDogfoodTLDs))
	case strings.EqualFold(strings.TrimSpace(tldFlag), "all"):
		tldSet = owdTopTLDs(tlds, owdDogfoodCap(len(tlds), owdDogfoodTLDs))
	default:
		tldSet = owdParseCSVList(tldFlag)
		if err := owdRequireKnownTLDs(tldSet, known); err != nil {
			return nil, nil, err
		}
	}
	if bare && len(tldSet) == 0 {
		return nil, nil, usageErr(fmt.Errorf("--tld lists no TLDs"))
	}
	return lines, tldSet, nil
}
