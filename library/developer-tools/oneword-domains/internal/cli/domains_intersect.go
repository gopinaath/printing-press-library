// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: dictionary words free on every listed TLD at
// once (lifetime-pass domain database), optionally only those already taken
// on other TLDs.
// pp:data-source live

package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelDomainsIntersectCmd(flags *rootFlags) *cobra.Command {
	var tld, takenOn, category, price, search string
	var minLen, maxLen, maxPages, limit int

	cmd := &cobra.Command{
		Use:   "intersect",
		Short: "Find dictionary words available on every listed TLD at once, or free on one TLD but taken on another (lifetime pass)",
		Long: strings.TrimSpace(`
Use this command to find dictionary words available on every listed TLD at once, or free on one TLD but taken on another (lifetime pass). Do NOT use this command to filter the dictionary by category or length without availability; use 'words mine' instead. Do NOT use it to verify a word list you already have; use 'check --file' instead. Do NOT use it to count inventory per TLD; use 'tlds inventory' instead.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli domains intersect --tld com,ai --max-pages 1 --json
  oneword-domains-pp-cli domains intersect --tld com,ai,io --category positive --min-length 4 --max-length 6
  oneword-domains-pp-cli domains intersect --tld ai --taken-on com,io --price available --max-pages 20 --limit 50 --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "--tld=com,ai;--max-pages=1",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "domains intersect")
			}
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := owdSourceErr(cmd, flags); err != nil {
				return err
			}
			tldList := owdParseCSVList(tld)
			takenList := owdParseCSVList(takenOn)
			if len(tldList) == 0 {
				return usageErr(fmt.Errorf("--tld is required: two or more TLDs (e.g. --tld com,ai), or one TLD plus --taken-on"))
			}
			if len(tldList) < 2 && len(takenList) == 0 {
				return usageErr(fmt.Errorf("--tld needs at least two TLDs to intersect (e.g. --tld com,ai); with a single TLD add --taken-on <tld>"))
			}
			for _, t := range takenList {
				if slices.Contains(tldList, t) {
					return usageErr(fmt.Errorf("TLD %q cannot be in both --tld and --taken-on", t))
				}
			}
			if len(takenList) > 0 && !cmd.Flags().Changed("max-pages") {
				return usageErr(fmt.Errorf("--taken-on needs an explicit --max-pages: a word beyond the page cap on a --taken-on TLD would be reported as taken although it is free"))
			}
			if price != "" && price != "available" {
				return usageErr(fmt.Errorf("--price accepts only 'available'"))
			}
			if minLen < 0 || maxLen < 0 || (maxLen > 0 && minLen > maxLen) {
				return usageErr(fmt.Errorf("--min-length and --max-length must be zero or positive, with --max-length at least --min-length"))
			}
			if maxPages <= 0 || limit < 0 {
				return usageErr(fmt.Errorf("--max-pages must be at least 1 and --limit zero or positive"))
			}
			maxPages = owdDogfoodCap(maxPages, owdDogfoodPages)

			ctx, cancel := owdLoopCtx(cmd, flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			base := owdDirectoryParams(category, price, search, minLen, maxLen)

			out := owdIntersectOutput{Words: make([]owdIntersectWord, 0), PerTLDCounts: map[string]int{}, TakenOn: takenList}
			sets := map[string]map[string]owdIntersectEntry{}
			cappedFree, cappedTaken := make([]string, 0), make([]string, 0)
			for _, t := range slices.Concat(tldList, takenList) {
				res, err := owdIntersectScan(ctx, c, t, base, maxPages)
				out.ScannedPages += res.Pages
				if err != nil {
					return owdAPIErr(cmd, flags, err)
				}
				sets[t] = res.Words
				out.PerTLDCounts[t] = len(res.Words)
				if res.Capped {
					if slices.Contains(takenList, t) {
						cappedTaken = append(cappedTaken, t)
					} else {
						cappedFree = append(cappedFree, t)
					}
				}
			}
			out.Words = owdIntersectWords(sets, tldList, takenList)
			notes := make([]string, 0)
			if len(out.Words) == 0 && len(cappedFree) > 0 {
				notes = append(notes, fmt.Sprintf("page cap reached on %s (--max-pages %d) before the sets overlapped; raise --max-pages or narrow with --category/--search", strings.Join(cappedFree, ","), maxPages))
			} else if len(cappedFree) > 0 {
				notes = append(notes, fmt.Sprintf("page cap reached on %s (--max-pages %d); more words may exist beyond the scanned pages", strings.Join(cappedFree, ","), maxPages))
			}
			if len(cappedTaken) > 0 {
				notes = append(notes, fmt.Sprintf("the --taken-on scan for %s was capped at %d page(s); words beyond the cap may be reported as taken although they are free", strings.Join(cappedTaken, ","), maxPages))
			}
			out.Note = strings.Join(notes, "; ")
			if limit > 0 && len(out.Words) > limit {
				out.Words = out.Words[:limit]
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			if len(out.Words) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No words are free on all of %s after %d page(s).\n", strings.Join(tldList, ", "), out.ScannedPages)
				if out.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), out.Note)
				}
				return nil
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%d word(s) free on %s (%d page(s) scanned)\n", len(out.Words), strings.Join(tldList, "+"), out.ScannedPages)
			if out.Note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), out.Note)
			}
			rows := make([]map[string]any, 0, len(out.Words))
			for _, w := range out.Words {
				row := map[string]any{"word": w.Word, "tld_count": w.TldCount}
				for t, e := range w.TLDs {
					v := "free"
					if e.Premium {
						v = "premium"
					}
					if e.Price != nil && *e.Price != "" {
						v += " $" + *e.Price
					}
					row[t] = v
				}
				rows = append(rows, row)
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().StringVar(&tld, "tld", "", "TLDs the word must be free on, comma-separated (two or more, or one with --taken-on)")
	cmd.Flags().StringVar(&takenOn, "taken-on", "", "TLDs the word must already be registered on, comma-separated (requires an explicit --max-pages)")
	cmd.Flags().StringVar(&category, "category", "", "Word category slug, e.g. positive, tech, nouns")
	cmd.Flags().StringVar(&price, "price", "", "Price filter: available (standard registration price only)")
	cmd.Flags().IntVar(&minLen, "min-length", 0, "Minimum word length")
	cmd.Flags().IntVar(&maxLen, "max-length", 0, "Maximum word length")
	cmd.Flags().StringVar(&search, "search", "", "Substring the word must contain")
	cmd.Flags().IntVar(&maxPages, "max-pages", 20, "Maximum pages to scan per TLD (100 words per page); must be set explicitly with --taken-on")
	cmd.Flags().IntVar(&limit, "limit", 0, "Print at most this many words (0 = all)")
	return cmd
}
