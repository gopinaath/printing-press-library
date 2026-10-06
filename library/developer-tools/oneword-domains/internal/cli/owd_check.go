// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored root-level `check`: one dictionary word across many TLDs in
// one call, with the dictionary pre-check the site lacks (it answers HTTP 500
// for unknown words), the cheapest registrar joined in, and every result
// appended to the local owd_domain_checks snapshot table.
// pp:data-source live

package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelCheckCmd(flags *rootFlags) *cobra.Command {
	var tldFlag, file string
	var availableOnly bool
	var maxPrice float64
	var concurrency, maxChecks int
	cmd := &cobra.Command{
		Use:   "check <word>|<word.tld>",
		Short: "Check one dictionary word across many TLDs at once, with the cheapest registrar and a dictionary pre-check",
		Long: strings.TrimSpace(`
Check a word's availability across a TLD set in one call. Without --tld the
12 most-viewed TLDs are scanned; --tld all scans all 93. Words are verified
against the dictionary first, because the site answers HTTP 500 for words it
does not know; unknown words get suggestions instead of a failed request.
Each row joins the TLD's min price and cheapest registrar, and every result
is appended to the local snapshot table that 'recheck' reads later. A batch
is capped at --max-checks word/TLD pairs per run.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli check smart
  oneword-domains-pp-cli check smart --tld com,io,ai --json
  oneword-domains-pp-cli check oasis.ai
  oneword-domains-pp-cli check smart --tld all --available-only --max-price 20
  oneword-domains-pp-cli check --file words.txt --tld com,co --concurrency 4 --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "word=smart;--tld=com,io",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "check")
			}
			if err := owdSourceErr(cmd, flags); err != nil {
				return err
			}
			if len(args) > 1 {
				return usageErr(fmt.Errorf("check takes one word; put a batch in --file (one word or word.tld per line)"))
			}
			if len(args) == 0 && file == "" {
				return owdMissingArgErr(cmd, flags, "<word> [--tld com,io] | --file words.txt")
			}
			if len(args) == 1 && file != "" {
				return usageErr(fmt.Errorf("pass either a word or --file, not both"))
			}
			if maxPrice < 0 {
				return usageErr(fmt.Errorf("--max-price must be zero or positive"))
			}
			if maxChecks < 0 {
				return usageErr(fmt.Errorf("--max-checks must be zero or positive"))
			}
			concurrency = min(max(concurrency, 1), owdMaxConcurrency)
			maxChecks = owdDogfoodCap(maxChecks, owdDogfoodChecks)
			// Validate the argument before any request is made.
			single := len(args) == 1
			arg := ""
			if single {
				arg = strings.ToLower(strings.TrimSpace(args[0]))
				if err := owdValidateLabels("word", arg, 1); err != nil {
					return err
				}
			}
			ctx, cancel := owdLoopCtx(cmd, flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			db, closeDB := owdOpenStoreOptional(ctx, cmd.ErrOrStderr())
			defer closeDB()
			tlds, _, err := owdFetchTLDs(ctx, c, db, false, cmd.ErrOrStderr())
			if err != nil {
				return owdAPIErr(cmd, flags, err)
			}
			known, _ := owdIndexTLDs(tlds)
			lines, tldSet, err := owdCheckInputs(cmd, arg, file, tldFlag, tlds)
			if err != nil {
				return err
			}
			pairs := owdWordPairs(lines, tldSet)
			if maxChecks > 0 && len(pairs) > maxChecks {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %d word/TLD pairs requested but only the first %d are checked (--max-checks %d); raise --max-checks or narrow --tld/--file\n", len(pairs), maxChecks, maxChecks)
				pairs = pairs[:maxChecks]
			}
			words := owdPairWords(pairs)

			// Dictionary pre-check (the site answers 500 for unknown words).
			dict, err := owdDictPrecheck(ctx, c, db, words, concurrency)
			if err != nil {
				return owdTypedErr(cmd, flags, err)
			}
			rows := make([]owdCheckRow, 0, len(pairs))
			unknownWords := make([]string, 0)
			for _, w := range words {
				if msg, failed := dict.Failed[w]; failed {
					rows = append(rows, owdCheckRow{Word: w, Error: "dictionary lookup failed: " + msg})
					continue
				}
				if !dict.InDict[w] {
					unknownWords = append(unknownWords, w)
					rows = append(rows, owdCheckRow{Word: w, Error: "not in dictionary", Suggestions: dict.suggest(ctx, c, w, 5)})
				}
			}
			if single && len(unknownWords) == 1 {
				sugg := rows[0].Suggestions
				msg := fmt.Sprintf("%q is not in the One Word Domains dictionary (the site answers 500 for unknown words)", unknownWords[0])
				if len(sugg) > 0 {
					msg += "; did you mean " + strings.Join(sugg, ", ") + "?"
				} else {
					msg += "; try 'oneword-domains-pp-cli words list --prefix " + unknownWords[0][:min(3, len(unknownWords[0]))] + "' or 'gpt generate' for invented names"
				}
				if flags.asJSON {
					_ = printJSONFiltered(cmd.OutOrStdout(), map[string]any{"error": msg, "code": 3, "word": unknownWords[0], "suggestions": sugg}, flags)
				}
				return notFoundErr(fmt.Errorf("%s", msg))
			}

			// Fan out the checkable pairs, then record them sequentially.
			checkable := make([]owdPair, 0, len(pairs))
			for _, p := range pairs {
				if dict.InDict[p.Word] {
					checkable = append(checkable, p)
				}
			}
			var snap owdSnapshotErrs
			at := owdNow()
			checks, checkErrs, err := owdChecksByDomain(ctx, c, db, owdPairDomains(checkable), concurrency, at, &snap)
			if err != nil {
				return owdTypedErr(cmd, flags, err)
			}
			// One TLD detail per distinct TLD (cheapest registrar).
			distinct := owdPairTLDs(checkable)
			detailByTLD, detailErrs := owdDetailsByTLD(ctx, c, db, distinct, concurrency, at, &snap)
			checkByDomain, failedByDomain := owdIndexChecks(checks, checkErrs)
			for _, p := range checkable {
				if msg, failed := failedByDomain[p.Domain]; failed {
					rows = append(rows, owdCheckRow{Word: p.Word, TLD: p.TLD, Domain: p.Domain, MinPrice: known[p.TLD].MinPrice, Error: msg})
					continue
				}
				rows = append(rows, owdBuildCheckRow(checkByDomain[p.Domain], p.Word, p.TLD, known[p.TLD].MinPrice, detailByTLD[p.TLD]))
			}
			allErrs := slices.Concat(dict.Errs, checkErrs, detailErrs)
			owdWarnFailuresInline(cmd.ErrOrStderr(), allErrs, len(words)+len(checkable)+len(distinct), "fetches")
			snap.warn(cmd.ErrOrStderr())
			if len(unknownWords) > 0 {
				// Only --file input reaches here (a single unknown argument
				// returned above); file lines are never echoed, so the note is
				// count-only and the rows carry the words.
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %d word(s) not in the dictionary, skipped (their rows carry \"not in dictionary\")\n", len(unknownWords))
			}

			// Filters and order.
			filtered := make([]owdCheckRow, 0, len(rows))
			for _, r := range rows {
				if availableOnly && (r.Available == nil || !*r.Available) {
					continue
				}
				if maxPrice > 0 && r.Error == "" {
					if cost, ok := owdRowCost(r); ok && cost > maxPrice {
						continue
					}
				}
				filtered = append(filtered, r)
			}
			owdSortCheckRows(filtered)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), filtered, flags)
			}
			if len(filtered) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No domains matched (try dropping --available-only or raising --max-price).")
				return nil
			}
			owdPrintCheckRowsTable(cmd.OutOrStdout(), filtered)
			return nil
		},
	}
	cmd.Flags().StringVar(&tldFlag, "tld", "", "TLDs to check, comma-separated (default: the 12 most-viewed; 'all' for every TLD)")
	cmd.Flags().StringVar(&file, "file", "", "Read words from a file, one word or word.tld per line ('-' for stdin); word.tld pins that TLD, bare words use --tld")
	cmd.Flags().BoolVar(&availableOnly, "available-only", false, "Keep only available domains")
	cmd.Flags().Float64Var(&maxPrice, "max-price", 0, "Drop rows whose cheapest price exceeds this budget (USD)")
	cmd.Flags().IntVar(&concurrency, "concurrency", owdDefaultConcurrency, "Parallel availability and TLD detail requests (1-16)")
	cmd.Flags().IntVar(&maxChecks, "max-checks", owdDefaultMaxChecks, "Maximum word/TLD pairs to check per run (0 = unlimited)")
	return cmd
}
