// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: re-run every previously checked word/TLD pair
// (or a word file) and report which fields changed since the last snapshot.
// pp:data-source auto

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/client"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
	"github.com/spf13/cobra"
)

// owdRecheckPairsFromFile reads --file and crosses its bare words with
// tldList, validating --tld against the tracked TLDs the way `check` does
// (owdReadWordLines rejects a pinned TLD outside that list by line number).
// c is nil under --data-source local, where an unavailable TLD list only
// skips that validation.
func owdRecheckPairsFromFile(ctx context.Context, cmd *cobra.Command, flags *rootFlags, c *client.Client, db *store.Store, file string, tldList []string) ([]owdPair, error) {
	r, err := owdOpenWordFile(cmd, file)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var known map[string]owdTLD
	var slugs []string
	if tldRows, _, err := owdFetchTLDs(ctx, c, db, false, cmd.ErrOrStderr()); err != nil {
		if c != nil {
			return nil, owdAPIErr(cmd, flags, err)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: TLD list unavailable, --tld is not validated: %v\n", err)
	} else {
		known, slugs = owdIndexTLDs(tldRows)
		if err := owdRequireKnownTLDs(tldList, known); err != nil {
			return nil, err
		}
	}
	lines, err := owdReadWordLines(r, slugs)
	if err != nil {
		return nil, usageErr(fmt.Errorf("reading --file: %w", err))
	}
	return owdWordPairs(lines, tldList), nil
}

func newNovelRecheckCmd(flags *rootFlags) *cobra.Command {
	var since, file, tlds string
	var limit, maxChecks int

	cmd := &cobra.Command{
		Use:   "recheck",
		Short: "Re-run every word/TLD pair you previously checked and report what changed (availability, premium, price, popularity)",
		Long: strings.TrimSpace(`
Use this command to re-run every word/TLD pair you previously checked and report what changed (availability, premium, price, popularity). Do NOT use this command for a first check of new words; use 'check' instead. Do NOT use it for aftermarket auction changes; use 'listings watch' instead.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli recheck --json
  oneword-domains-pp-cli recheck --since 7d --max-checks 50
  oneword-domains-pp-cli recheck --file words.txt --tld com,ai --json
  oneword-domains-pp-cli recheck --data-source local --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "auto",
			"pp:happy-args":  "--since=7d",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "recheck")
			}
			useFile := strings.TrimSpace(file) != ""
			useSince := strings.TrimSpace(since) != ""
			var windowDur time.Duration
			if useSince {
				d, err := owdParseWindowFlag("since", since)
				if err != nil {
					return err
				}
				windowDur = d
			}
			if limit < 0 || maxChecks < 0 {
				return usageErr(fmt.Errorf("--limit and --max-checks must be zero or positive"))
			}
			maxChecks = owdDogfoodCap(maxChecks, owdDogfoodChecks)
			local := flags.dataSource == "local"

			ctx, cancel := owdLoopCtx(cmd, flags)
			defer cancel()
			db, _, err := owdOpenStore(ctx)
			if err != nil {
				return err
			}
			defer db.Close()
			now := owdNow()

			var c *client.Client
			var pairs []owdPair
			if useFile {
				tldList := owdParseCSVList(tlds)
				if len(tldList) == 0 {
					return usageErr(fmt.Errorf("--tld needs at least one TLD when --file is used"))
				}
				if !local {
					if c, err = flags.newClient(); err != nil {
						return err
					}
				}
				if pairs, err = owdRecheckPairsFromFile(ctx, cmd, flags, c, db, file, tldList); err != nil {
					return err
				}
			} else {
				var tldFilter []string
				if cmd.Flags().Changed("tld") {
					tldFilter = owdParseCSVList(tlds)
				}
				pairs, err = owdRecheckHistory(ctx, db, windowDur, now, tldFilter)
				if err != nil {
					return err
				}
			}

			out := owdRecheckNewResult()
			if len(pairs) == 0 {
				if useFile {
					fmt.Fprintf(cmd.ErrOrStderr(), "hint: %s has no words to check\n", file)
				} else if useSince || cmd.Flags().Changed("tld") {
					fmt.Fprintln(cmd.ErrOrStderr(), "hint: no prior checks match the filter; drop --since/--tld or run 'oneword-domains-pp-cli check <word>' first")
				} else {
					fmt.Fprintln(cmd.ErrOrStderr(), "hint: no prior checks; run 'oneword-domains-pp-cli check <word>' first")
				}
				return owdRecheckPrint(cmd, flags, out)
			}
			if limit > 0 && len(pairs) > limit {
				pairs = pairs[:limit]
			}

			if local {
				out, err = owdRecheckLocal(ctx, db, pairs)
				if err != nil {
					return err
				}
				if out.Checked == 0 {
					fmt.Fprintln(cmd.ErrOrStderr(), "hint: no stored snapshots for these pairs; run without --data-source local to check them live")
				}
				return owdRecheckPrint(cmd, flags, out)
			}

			if maxChecks > 0 && len(pairs) > maxChecks {
				out.Note = fmt.Sprintf("%d pairs matched but only the first %d were re-checked (--max-checks %d); raise --max-checks or narrow with --since/--tld", len(pairs), maxChecks, maxChecks)
				pairs = pairs[:maxChecks]
			}

			// Read every prior snapshot before the first live call so the diff
			// compares against the state as it was, not the row we are about to write.
			priors := owdLatestChecks(ctx, db, owdPairDomains(pairs))

			if c == nil {
				if c, err = flags.newClient(); err != nil {
					return err
				}
			}
			c.NoCache = true // a cached answer would hide exactly the changes we look for
			var snap owdSnapshotErrs
			results, errs, err := owdChecksByDomain(ctx, c, db, owdPairDomains(pairs), owdDefaultConcurrency, now, &snap)
			if err != nil {
				return owdTypedErr(cmd, flags, err)
			}
			if len(results) == 0 && len(errs) > 0 {
				return owdAPIErr(cmd, flags, errs[0].Err)
			}
			for _, r := range results {
				out.Checked++
				prior, had := priors[r.Source]
				if !had {
					out.Baseline++
					continue
				}
				out.record(owdRecheckDiff(r.Source, prior, r.Value, now))
			}
			out.FetchFailures = owdFailures(errs)
			owdWarnFailuresListed(cmd.ErrOrStderr(), errs, len(pairs), "re-checks")
			snap.warn(cmd.ErrOrStderr())
			return owdRecheckPrint(cmd, flags, out)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Only pairs first checked within this window, e.g. 7d, 48h, 1w (default: all)")
	cmd.Flags().StringVar(&file, "file", "", "Re-check the words in this file instead of the history: one word or word.tld per line ('-' for stdin); word.tld pins that TLD, bare words use --tld")
	cmd.Flags().StringVar(&tlds, "tld", "com", "TLDs for bare --file words, comma-separated; without --file, restricts the history to these TLDs")
	cmd.Flags().IntVar(&limit, "limit", 0, "Stop after this many pairs (0 = all)")
	cmd.Flags().IntVar(&maxChecks, "max-checks", owdDefaultMaxChecks, "Maximum live availability checks per run (0 = unlimited)")
	return cmd
}

// owdRecheckPrint emits the result object as JSON, or a short human summary plus a change table.
func owdRecheckPrint(cmd *cobra.Command, flags *rootFlags, out owdRecheckResult) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), out, flags)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Re-checked %d domain(s): %d changed field(s), %d unchanged, %d first snapshot, %d failed\n", out.Checked, len(out.Changed), out.Unchanged, out.Baseline, len(out.FetchFailures))
	if out.Note != "" {
		fmt.Fprintln(w, out.Note)
	}
	if len(out.Changed) == 0 {
		return nil
	}
	return owdHumanTable(cmd, flags, out.Changed)
}
