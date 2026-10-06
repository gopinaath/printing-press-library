// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored root-level `compare`: several full domains side by side in
// one fan-out, each joined with its TLD's min price and cheapest registrar,
// plus the cheapest available pick. Shares the row shape with `check`.
// pp:data-source live

package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil"
	"github.com/spf13/cobra"
)

// owdCompareResult is the `compare` output envelope.
type owdCompareResult struct {
	Rows              []owdCheckRow `json:"rows"`
	CheapestAvailable string        `json:"cheapest_available"`
	FetchFailures     []owdFailure  `json:"fetch_failures"`
}

func newNovelCompareCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compare <domain> [<domain>...]",
		Short: "Compare several word.tld domains side by side and name the cheapest available one",
		Long: strings.TrimSpace(`
Check a handful of full domains (smart.com smart.io oasis.ai) in one call.
Every row carries availability, premium and aftermarket flags, the word's
popularity across the 93 TLDs, the TLD's min price and cheapest registrar,
and the envelope names the cheapest available domain. Words outside the
dictionary get an error row instead of a failed request.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli compare smart.com smart.io oasis.ai
  oneword-domains-pp-cli compare smart.com smart.io --json --select rows.domain,rows.available,cheapest_available
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "domain=smart.com",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "compare")
			}
			if err := owdSourceErr(cmd, flags); err != nil {
				return err
			}
			if len(args) == 0 {
				return owdMissingArgErr(cmd, flags, "<domain> [<domain>...]")
			}
			// Validate every argument before any request is made.
			domains := owdDedupe(args)
			for _, d := range domains {
				if err := owdValidateDomain(d); err != nil {
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
			known, slugs := owdIndexTLDs(tlds)
			targets := make([]owdPair, 0, len(domains))
			for _, d := range domains {
				w, t, err := owdSplitKnownDomain(d, slugs)
				if err != nil {
					return usageErr(err)
				}
				if err := owdRequireKnownTLDs([]string{t}, known); err != nil {
					return err
				}
				targets = append(targets, owdPair{Word: w, TLD: t, Domain: w + "." + t})
			}

			// Dictionary pre-check per distinct word.
			words := owdPairWords(targets)
			dict, err := owdDictPrecheck(ctx, c, db, words, owdDefaultConcurrency)
			if err != nil {
				return owdTypedErr(cmd, flags, err)
			}

			out := owdCompareResult{Rows: make([]owdCheckRow, 0, len(targets)), FetchFailures: make([]owdFailure, 0)}
			checkable := make([]owdPair, 0, len(targets))
			for _, t := range targets {
				if msg, failed := dict.Failed[t.Word]; failed {
					out.FetchFailures = append(out.FetchFailures, owdFailure{Source: t.Domain, Error: "dictionary lookup failed: " + msg})
					continue
				}
				if !dict.InDict[t.Word] {
					out.Rows = append(out.Rows, owdCheckRow{Word: t.Word, TLD: t.TLD, Domain: t.Domain, MinPrice: known[t.TLD].MinPrice, Error: "not in dictionary", Suggestions: dict.suggest(ctx, c, t.Word, 5)})
					continue
				}
				checkable = append(checkable, t)
			}

			var snap owdSnapshotErrs
			at := owdNow()
			checks, checkErrs, err := owdChecksByDomain(ctx, c, db, owdPairDomains(checkable), owdDefaultConcurrency, at, &snap)
			if err != nil {
				return owdTypedErr(cmd, flags, err)
			}
			checkByDomain, _ := owdIndexChecks(checks, checkErrs)
			out.FetchFailures = append(out.FetchFailures, owdFailures(checkErrs)...)
			distinct := owdPairTLDs(checkable)
			detailByTLD, detailErrs := owdDetailsByTLD(ctx, c, db, distinct, owdDefaultConcurrency, at, &snap)
			out.FetchFailures = append(out.FetchFailures, owdFailures(detailErrs)...)
			for _, t := range checkable {
				dc, ok := checkByDomain[t.Domain]
				if !ok {
					continue
				}
				out.Rows = append(out.Rows, owdBuildCheckRow(dc, t.Word, t.TLD, known[t.TLD].MinPrice, detailByTLD[t.TLD]))
			}
			owdSortCheckRows(out.Rows)
			out.CheapestAvailable = owdCheapestAvailable(out.Rows)
			allErrs := slices.Concat(dict.Errs, checkErrs, detailErrs)
			owdWarnFailuresListed(cmd.ErrOrStderr(), allErrs, len(words)+len(checkable)+len(distinct), "fetches")
			snap.warn(cmd.ErrOrStderr())
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			if len(out.Rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No domains could be checked.")
				return nil
			}
			owdPrintCheckRowsTable(cmd.OutOrStdout(), out.Rows)
			if out.CheapestAvailable != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\ncheapest available: %s\n", cliutil.ScrubTerminal(out.CheapestAvailable))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "\nnone of these domains is available")
			}
			return nil
		},
	}
	return cmd
}
