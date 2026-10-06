// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: DomainsGPT names filtered to the available
// ones, joined with registration price, cheapest registrar, and a real-word
// flag, and saved to the local owd_generations table.
// pp:data-source live

package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelBrainstormCmd(flags *rootFlags) *cobra.Command {
	var typ, ctxText, word, position, tlds, exclude, sortKey string
	var minLen, maxLen, limit int
	var includeTaken bool

	cmd := &cobra.Command{
		Use:   "brainstorm",
		Short: "Generate DomainsGPT names and keep only the available ones with registration price, cheapest registrar, and a real-word flag",
		Long: strings.TrimSpace(`
Use this command to generate DomainsGPT names and keep only the available ones with registration price, cheapest registrar, and a real-word flag, saved locally. Do NOT use this command for one raw DomainsGPT call with the unfiltered stream; use 'gpt generate' instead. Do NOT use it to check words you already have; use 'check' instead.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli brainstorm --type brandable --context "a terminal tool for domain search" --tld ai --min-length 6 --max-length 10 --json
  oneword-domains-pp-cli brainstorm --type portmanteau --word open --position prefix --tld com,ai --sort length --limit 10
  oneword-domains-pp-cli brainstorm --type random --tld com --include-taken --exclude openlumix.com,openvance.com --sort name
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "--type=brandable;--tld=ai;--min-length=6;--max-length=10",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "brainstorm")
			}
			if err := owdSourceErr(cmd, flags); err != nil {
				return err
			}
			gptReq := owdGPTRequest{Type: typ, Context: ctxText, MinLen: minLen, MaxLen: maxLen, Word: word, Position: position}
			if err := gptReq.validate(); err != nil {
				return err
			}
			if !slices.Contains(owdBrainstormSorts, sortKey) {
				return usageErr(fmt.Errorf("--sort must be one of %s", strings.Join(owdBrainstormSorts, ", ")))
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be zero or positive"))
			}
			tldList := owdParseCSVList(tlds)
			if len(tldList) == 0 {
				return usageErr(fmt.Errorf("--tld needs at least one TLD, e.g. --tld com,ai"))
			}
			liveWordBudget := owdDogfoodCap(owdBrainstormLiveWordBudget, owdDogfoodChecks)
			if n := owdDogfoodCap(len(tldList), owdDogfoodBrainstormTLDs); n < len(tldList) {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: dogfood run; generating for %s only\n", tldList[0])
				tldList = tldList[:n]
			}
			excludeList := owdParseCSVList(exclude)

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

			// Validate --tld against the tracked list before spending DomainsGPT quota.
			tldRows, _, err := owdFetchTLDs(ctx, c, db, false, cmd.ErrOrStderr())
			if err != nil {
				return owdAPIErr(cmd, flags, err)
			}
			known, _ := owdIndexTLDs(tldRows)
			if err := owdRequireKnownTLDs(tldList, known); err != nil {
				return err
			}

			if used, quota, ok, err := owdUsage(ctx, c); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not read the DomainsGPT quota: %v\n", err)
			} else if ok && quota-used < len(tldList) {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: DomainsGPT quota is %d of %d used (%d left) but %d TLD(s) were requested; sign in with 'oneword-domains-pp-cli auth login --chrome' or set ONEWORD_DOMAINS_GPT_TOKEN for a larger quota\n", used, quota, quota-used, len(tldList))
			}

			generated := make([]owdGenerated, 0, owdGPTNamesPerCall*len(tldList))
			var genErr error
			gptCfg := owdGPTConfig(flags)
			gptReq.Exclude = excludeList
			for _, tld := range tldList {
				gptReq.TLD = tld
				names, _, err := owdGenerate(ctx, flags, gptCfg, owdGPTBody(gptReq))
				if err != nil && len(names) == 0 {
					genErr = err
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: DomainsGPT failed for .%s: %v\n", tld, err)
					continue
				}
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v (keeping the %d names parsed before the cut for .%s)\n", err, len(names), tld)
				}
				generated = append(generated, names...)
			}
			if len(generated) == 0 && genErr != nil {
				return owdTypedErr(cmd, flags, genErr)
			}

			kept := owdBrainstormFilter(generated, includeTaken, excludeList)

			localIsBig := owdWordsTableCount(ctx, db) >= owdMinLocalWords

			now := owdNow()
			var snap owdSnapshotErrs
			details, detailErrs := owdDetailsByTLD(ctx, c, db, tldList, owdDefaultConcurrency, now, &snap)
			owdWarnFailuresInline(cmd.ErrOrStderr(), detailErrs, len(tldList), "TLD detail fetches")

			names := make([]string, 0, len(kept))
			for _, g := range kept {
				if name, _, err := owdSplitDomain(g.Domain); err == nil {
					names = append(names, name)
				}
			}
			realWords := owdNewRealWords(ctx, c, db, names, localIsBig, liveWordBudget)

			batch := owdBrainstormBatchID(now)
			rows := make([]owdBrainstormRow, 0, len(kept))
			for _, g := range kept {
				name, tld, err := owdSplitDomain(g.Domain)
				if err != nil {
					continue
				}
				d := details[tld]
				row := owdBrainstormRow{
					Domain:    g.Domain,
					TLD:       tld,
					Name:      name,
					Available: g.Available,
					RealWord:  realWords.lookup(ctx, name),
					MinPrice:  owdBrainstormMinPrice(d, known[tld].MinPrice),
					Batch:     batch,
				}
				if d != nil {
					row.CheapestRegistrar = owdCloneRegistrar(d.CheapestRegistrar)
				}
				rows = append(rows, row)
			}
			owdBrainstormSort(rows, sortKey)
			if err := owdBrainstormSave(ctx, db, rows, owdBrainstormMeta{Type: typ, Context: ctxText, Word: word, Position: position}, now); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: saving batch %s locally failed: %v\n", batch, err)
			}
			snap.warn(cmd.ErrOrStderr())
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "DomainsGPT returned %d names but none were available on %s; try --include-taken, another --type, or a different --tld.\n", len(generated), strings.Join(tldList, ","))
				return nil
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "batch %s: %d of %d generated names kept\n", batch, len(rows), len(generated))
			return owdHumanTable(cmd, flags, rows)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "random", "Name style: portmanteau, combination, brandable, nonenglish, alternate, random")
	cmd.Flags().StringVar(&ctxText, "context", "", "What the business or product does, to steer the names")
	cmd.Flags().StringVar(&word, "word", "", "A word to include in every name")
	cmd.Flags().StringVar(&position, "position", "prefix", "Where the included word goes: prefix, suffix, anywhere")
	cmd.Flags().StringVar(&tlds, "tld", "com", "TLDs to generate for, comma-separated (one DomainsGPT call each)")
	cmd.Flags().IntVar(&minLen, "min-length", 7, "Minimum name length")
	cmd.Flags().IntVar(&maxLen, "max-length", 12, "Maximum name length")
	cmd.Flags().StringVar(&exclude, "exclude", "", "Domains to avoid (also sent to DomainsGPT), comma-separated")
	cmd.Flags().BoolVar(&includeTaken, "include-taken", false, "Keep names whose domain is already registered")
	cmd.Flags().StringVar(&sortKey, "sort", "price", "Order rows by: price, length, name")
	cmd.Flags().IntVar(&limit, "limit", 0, "Print at most this many rows (0 = all)")
	return cmd
}
