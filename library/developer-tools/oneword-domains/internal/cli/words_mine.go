// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: filter the local dictionary by several
// categories at once, length, prefix, or glob, with an optional full refresh.
// pp:data-source local

package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelWordsMineCmd(flags *rootFlags) *cobra.Command {
	var category, prefix, glob string
	var minLen, maxLen, limit, maxPages int
	var anyCat, refresh bool

	cmd := &cobra.Command{
		Use:   "mine",
		Short: "Filter the local dictionary by several categories at once, length, prefix, or glob pattern",
		Long: strings.TrimSpace(`
Use this command to filter the local dictionary by several categories at once, length, prefix, or glob pattern. Do NOT use this command to find which words are available on a TLD; use 'domains intersect' (lifetime pass) or pipe its output into 'check --file' instead.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli words mine --category tech --limit 5 --json
  oneword-domains-pp-cli words mine --category adjectives,positive --min-len 4 --max-len 6
  oneword-domains-pp-cli words mine --category tech,nouns --any --prefix sm --json
  oneword-domains-pp-cli words mine --category positive,tech --any --json
  oneword-domains-pp-cli words mine --glob 's*o' --json
  oneword-domains-pp-cli words mine --refresh --max-pages 300 --category verbs --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "local",
			"pp:happy-args":  "--category=tech;--limit=5",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "words mine")
			}
			if err := owdSourceErr(cmd, flags); err != nil {
				return err
			}
			cats := owdParseCSVList(category)
			for _, c := range cats {
				if !slices.Contains(owdMineCategories, c) {
					return usageErr(fmt.Errorf("unknown category %q; use one of %s", c, strings.Join(owdMineCategories, ", ")))
				}
			}
			if anyCat && len(cats) < 2 {
				return usageErr(fmt.Errorf("--any needs two or more --category values"))
			}
			if minLen < 0 || maxLen < 0 || (maxLen > 0 && minLen > maxLen) {
				return usageErr(fmt.Errorf("--min-len and --max-len must be zero or positive, with --max-len at least --min-len"))
			}
			if limit < 0 || maxPages <= 0 {
				return usageErr(fmt.Errorf("--limit must be zero or positive and --max-pages at least 1"))
			}

			ctx, cancel := owdLoopCtx(cmd, flags)
			defer cancel()
			db, _, err := owdOpenStore(ctx)
			if err != nil {
				return err
			}
			defer db.Close()

			var localWords int // the local dictionary size, counted once
			if refresh {
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				maxPages = owdDogfoodCap(maxPages, owdDogfoodPages)
				stored, pages, complete, err := owdMineRefresh(ctx, c, db, maxPages)
				if err != nil && stored == 0 {
					return owdAPIErr(cmd, flags, err)
				}
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: refresh stopped after %d page(s): %v\n", pages, err)
				}
				localWords = owdWordsTableCount(ctx, db)
				if complete {
					if err := db.SaveSyncState("words", "", localWords); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: recording the refresh failed: %v\n", err)
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "refreshed the whole dictionary: %d words over %d page(s)\n", stored, pages)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "refreshed %d words over %d page(s); raise --max-pages to fetch the rest of the dictionary\n", stored, pages)
				}
			}

			if !refresh {
				localWords = owdWordsTableCount(ctx, db)
			}
			if localWords < owdMinLocalWords && !refresh {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: the local dictionary holds only %d words; run 'oneword-domains-pp-cli words mine --refresh' (or 'sync --resources words') to load all of it\n", localWords)
			}
			maybeEmitSyncHints(cmd, db, "words", flags.maxAge)

			query, qargs := owdMineQuery(owdMineOpts{Categories: cats, Any: anyCat, MinLen: minLen, MaxLen: maxLen, Prefix: strings.TrimSpace(prefix), Glob: strings.TrimSpace(glob), Limit: limit})
			rows, err := owdMineRun(ctx, db, query, qargs)
			if err != nil {
				return owdTypedErr(cmd, flags, fmt.Errorf("querying the local words table: %w", err))
			}

			if len(rows) == 0 && len(cats) >= 2 && !anyCat {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: no word carries all of %s; add --any to match words with at least one of them\n", strings.Join(cats, ", "))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No words match (local dictionary: %d words).\n", localWords)
				return nil
			}
			return owdHumanTable(cmd, flags, rows)
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "Category slugs the word must carry, comma-separated (all of them unless --any)")
	cmd.Flags().BoolVar(&anyCat, "any", false, "Match words in any of the --category values instead of all")
	cmd.Flags().IntVar(&minLen, "min-len", 0, "Minimum word length")
	cmd.Flags().IntVar(&maxLen, "max-len", 0, "Maximum word length")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Words starting with this prefix")
	cmd.Flags().StringVar(&glob, "glob", "", "SQLite GLOB pattern the word must match, e.g. 's*o' or 'b?t'")
	cmd.Flags().IntVar(&limit, "limit", 100, "Print at most this many words (0 = all)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Page the whole dictionary from the API into the local store first")
	cmd.Flags().IntVar(&maxPages, "max-pages", 300, "Maximum dictionary pages to fetch with --refresh (100 words per page)")
	return cmd
}
