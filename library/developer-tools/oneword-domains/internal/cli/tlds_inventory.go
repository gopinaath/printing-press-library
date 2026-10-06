// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: how many available one-word domains each TLD
// still has for a category/length/price filter (lifetime-pass count route),
// with the TLD's cheapest registration price.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil"
	"github.com/spf13/cobra"
)

func newNovelTldsInventoryCmd(flags *rootFlags) *cobra.Command {
	var category, price, search, typ, sortKey string
	var minLen, maxLen, limit, maxChecks int
	var maxPrice float64

	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Compare how many available one-word domains each TLD still has for a category/length/price filter, with its cheapest price",
		Long: strings.TrimSpace(`
Use this command to compare how many available one-word domains each TLD still has for a category/length/price filter, with the TLD's cheapest registration price. min_price comes from the TLD list and is always populated; cheapest_registrar comes from the latest detail fetch or price snapshot ('tlds get', 'check', 'compare', 'tlds drift --registrars') and is null for TLDs never fetched in detail. Do NOT use this command to list the words themselves; use 'domains intersect' instead. Do NOT use it for registrations, top-10M presence, views, or registrar prices alone; use 'tlds list' or 'tlds get' instead.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli tlds inventory --category positive --max-checks 3 --json
  oneword-domains-pp-cli tlds inventory --category tech --min-len 4 --max-len 6 --price available --sort count
  oneword-domains-pp-cli tlds inventory --type ccTld --max-price 20 --sort price --limit 10 --json
  oneword-domains-pp-cli tlds inventory --search art --sort top10m
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "--category=positive --max-checks=3",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "tlds inventory")
			}
			if err := owdSourceErr(cmd, flags); err != nil {
				return err
			}
			if !slices.Contains(owdInventorySorts, sortKey) {
				return usageErr(fmt.Errorf("--sort must be one of %s", strings.Join(owdInventorySorts, ", ")))
			}
			typ = strings.TrimSpace(typ)
			if typ != "" && !slices.ContainsFunc(owdInventoryTypes, func(t string) bool { return strings.EqualFold(t, typ) }) {
				return usageErr(fmt.Errorf("--type must be one of %s", strings.Join(owdInventoryTypes, ", ")))
			}
			if price != "" && price != "available" {
				return usageErr(fmt.Errorf("--price accepts only 'available'"))
			}
			if minLen < 0 || maxLen < 0 || (maxLen > 0 && minLen > maxLen) {
				return usageErr(fmt.Errorf("--min-len and --max-len must be zero or positive, with --max-len at least --min-len"))
			}
			var maxPricePtr *float64
			if cmd.Flags().Changed("max-price") {
				if maxPrice < 0 {
					return usageErr(fmt.Errorf("--max-price must be zero or positive"))
				}
				maxPricePtr = &maxPrice
			}
			if limit < 0 || maxChecks < 0 {
				return usageErr(fmt.Errorf("--limit and --max-checks must be zero or positive"))
			}
			maxChecks = owdDogfoodCap(maxChecks, owdDogfoodTLDs)

			ctx, cancel := owdLoopCtx(cmd, flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			db, _, err := owdOpenStore(ctx)
			if err != nil {
				return err
			}
			defer db.Close()

			tlds, _, err := owdFetchTLDs(ctx, c, db, false, cmd.ErrOrStderr())
			if err != nil {
				return owdAPIErr(cmd, flags, err)
			}
			cands := owdInventoryFilter(tlds, typ, maxPricePtr)
			out := owdInventoryOutput{Rows: make([]owdInventoryRow, 0), Matched: len(cands), FetchFailures: make([]owdFailure, 0)}
			if maxChecks > 0 && len(cands) > maxChecks {
				out.Note = fmt.Sprintf("%d TLDs matched but only the first %d were counted (--max-checks %d); raise --max-checks or narrow with --type/--max-price", len(cands), maxChecks, maxChecks)
				cands = cands[:maxChecks]
			}
			if len(cands) == 0 {
				out.Note = "no TLD matched --type/--max-price; run 'oneword-domains-pp-cli tlds list' to see the 93 tracked TLDs"
				return owdInventoryPrint(cmd, flags, out, sortKey)
			}
			candSlugs := make([]string, 0, len(cands))
			for _, t := range cands {
				candSlugs = append(candSlugs, t.Slug)
			}
			cheapest := owdInventoryCheapestFromStore(ctx, db, candSlugs)

			base := owdDirectoryParams(category, price, search, minLen, maxLen)
			counts := map[string]int{}
			failures := make([]cliutil.FanoutError, 0)

			// Probe one TLD before fanning out so a missing lifetime-pass
			// session fails once with the fix instead of 93 times.
			first := cands[0]
			if n, err := owdInventoryCount(ctx, c, first.Slug, base); err != nil {
				if sessionErr, ok := owdIsSessionError(err); ok {
					return owdTypedErr(cmd, flags, sessionErr)
				}
				failures = append(failures, cliutil.FanoutError{Source: first.Slug, Err: err})
			} else {
				counts[first.Slug] = n
			}
			if len(cands) > 1 {
				results, errs := cliutil.FanoutRun(ctx, cands[1:],
					func(t owdTLD) string { return t.Slug },
					func(ctx context.Context, t owdTLD) (int, error) {
						return owdInventoryCount(ctx, c, t.Slug, base)
					}, cliutil.WithConcurrency(owdDefaultConcurrency))
				if err := owdFirstSessionErr(errs); err != nil {
					return owdTypedErr(cmd, flags, err)
				}
				for _, r := range results {
					counts[r.Source] = r.Value
				}
				failures = append(failures, errs...)
			}
			out.FetchFailures = owdFailures(failures)
			owdWarnFailuresListed(cmd.ErrOrStderr(), failures, len(cands), "TLD counts")
			if len(counts) == 0 && len(out.FetchFailures) > 0 {
				return owdAPIErr(cmd, flags, fmt.Errorf("every count request failed; first error: %s", out.FetchFailures[0].Error))
			}

			for _, t := range cands {
				n, ok := counts[t.Slug]
				if !ok {
					continue
				}
				out.Rows = append(out.Rows, owdInventoryRow{TLD: t.Slug, Type: t.Type, AvailableCount: n, MinPrice: t.MinPrice, CheapestRegistrar: cheapest[t.Slug], Top10m: t.Top10m, TotalReg: t.TotalReg})
			}
			out.Checked = len(out.Rows)
			owdInventorySort(out.Rows, sortKey)
			if limit > 0 && len(out.Rows) > limit {
				out.Rows = out.Rows[:limit]
			}
			return owdInventoryPrint(cmd, flags, out, sortKey)
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "Word category slug, e.g. positive, tech, nouns")
	cmd.Flags().IntVar(&minLen, "min-len", 0, "Minimum word length")
	cmd.Flags().IntVar(&maxLen, "max-len", 0, "Maximum word length")
	cmd.Flags().StringVar(&price, "price", "", "Price filter: available (standard registration price only)")
	cmd.Flags().StringVar(&search, "search", "", "Substring the word must contain")
	cmd.Flags().StringVar(&typ, "type", "", "Only TLDs of this type: gTld or ccTld")
	cmd.Flags().Float64Var(&maxPrice, "max-price", 0, "Only TLDs whose cheapest registration is at most this price")
	cmd.Flags().StringVar(&sortKey, "sort", "count", "Order rows by: count (desc), price (asc, by min_price, which is always populated), top10m (desc)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Print at most this many rows (0 = all)")
	cmd.Flags().IntVar(&maxChecks, "max-checks", owdTLDTotal, "Maximum TLDs to count per run")
	return cmd
}

// owdInventoryPrint emits the wrapper as JSON or a table with a stderr summary.
func owdInventoryPrint(cmd *cobra.Command, flags *rootFlags, out owdInventoryOutput, sortKey string) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), out, flags)
	}
	if len(out.Rows) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No TLD inventory to show (%d TLDs matched, %d counted).\n", out.Matched, out.Checked)
		if out.Note != "" {
			fmt.Fprintln(cmd.OutOrStdout(), out.Note)
		}
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%d of %d TLDs counted, sorted by %s\n", out.Checked, out.Matched, sortKey)
	if out.Note != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), out.Note)
	}
	return owdHumanTable(cmd, flags, out.Rows)
}
