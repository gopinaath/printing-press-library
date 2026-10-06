// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: rank live auctions by word demand (TLDs taken
// of 93), price versus fresh registration, and bid pace.
// pp:data-source auto

package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/client"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
	"github.com/spf13/cobra"
)

// owdRankPopularity resolves each candidate's tldCount (the site's count of
// TLDs where the word is still free): the latest local snapshot first, then
// one live availability check per remaining listing, skipped under
// --data-source local where unknown listings stay unscored. Live results are
// snapshotted; per-listing failures come back as fetch_failures rows.
func owdRankPopularity(ctx context.Context, cmd *cobra.Command, c *client.Client, db *store.Store, cands []owdListing, local bool, now time.Time) (map[string]int, []owdFailure, error) {
	known := map[string]int{}
	need := make([]owdListing, 0)
	domains := make([]string, 0, len(cands))
	for _, l := range cands {
		domains = append(domains, strings.ToLower(l.Domain))
	}
	latest := owdLatestChecks(ctx, db, domains)
	for _, l := range cands {
		d := strings.ToLower(l.Domain)
		if dc, ok := latest[d]; ok {
			known[d] = dc.TldCount
		} else if !local {
			need = append(need, l)
		}
	}
	if len(need) == 0 {
		return known, make([]owdFailure, 0), nil
	}
	needDomains := make([]string, 0, len(need))
	for _, l := range need {
		needDomains = append(needDomains, strings.ToLower(l.Domain))
	}
	var snap owdSnapshotErrs
	results, errs, err := owdChecksByDomain(ctx, c, db, needDomains, owdDefaultConcurrency, now, &snap)
	if err != nil {
		return known, nil, err
	}
	for _, r := range results {
		known[r.Source] = r.Value.TldCount
	}
	owdWarnFailuresListed(cmd.ErrOrStderr(), errs, len(need), "availability checks")
	snap.warn(cmd.ErrOrStderr())
	return known, owdFailures(errs), nil
}

func newNovelListingsRankCmd(flags *rootFlags) *cobra.Command {
	var tld, ending, sortKey string
	var maxBids, maxChecks, maxPages, limit int

	cmd := &cobra.Command{
		Use:   "rank",
		Short: "Rank live auctions by word demand (TLDs taken of 93), price versus fresh registration cost, and bid pace",
		Long: strings.TrimSpace(`
Use this command to rank live auctions by word demand (TLDs taken of 93), price versus fresh registration cost, and bid pace. Do NOT use this command to see which listings are new, changed, or ending since the last run; use 'listings watch' instead. Do NOT use it for the raw server-sorted list; use 'listings list' instead.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli listings rank --tld co --max-checks 5 --json
  oneword-domains-pp-cli listings rank --tld co --ending 48h --max-bids 3 --sort ending
  oneword-domains-pp-cli listings rank --tld io --sort price-x --limit 10 --json --select domain,price,price_x,taken_of_93
  oneword-domains-pp-cli listings rank --tld co --data-source local --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "auto",
			"pp:happy-args":  "--tld=co;--max-checks=5",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "listings rank")
			}
			if !slices.Contains(owdRankSorts, sortKey) {
				return usageErr(fmt.Errorf("--sort must be one of %s", strings.Join(owdRankSorts, ", ")))
			}
			var endingDur time.Duration
			if strings.TrimSpace(ending) != "" {
				d, err := owdParseWindowFlag("ending", ending)
				if err != nil {
					return err
				}
				endingDur = d
			}
			var maxBidsCap *int
			if cmd.Flags().Changed("max-bids") {
				if maxBids < 0 {
					return usageErr(fmt.Errorf("--max-bids must be zero or positive"))
				}
				maxBidsCap = &maxBids
			}
			if maxChecks < 0 || maxPages <= 0 || limit < 0 {
				return usageErr(fmt.Errorf("--max-checks and --limit must be zero or positive and --max-pages at least 1"))
			}
			tld = owdNormTLD(tld)
			maxChecks = owdDogfoodCap(maxChecks, owdDogfoodChecks)
			maxPages = owdDogfoodCap(maxPages, owdDogfoodPages)
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
			if !local {
				if c, err = flags.newClient(); err != nil {
					return err
				}
			}

			out := owdRankOutput{Rows: make([]owdRankRow, 0), FetchFailures: make([]owdFailure, 0)}
			var listings []owdListing
			capped := false
			if local {
				maybeEmitSyncHints(cmd, db, "listings", flags.maxAge)
				if listings, err = owdRankLocalListings(ctx, db); err != nil {
					return owdTypedErr(cmd, flags, err)
				}
			} else {
				params := map[string]string{}
				if tld != "" {
					params["tld"] = tld
				}
				listings, out.ScannedPages, capped, err = owdFetchListings(ctx, c, params, maxPages)
				if err != nil {
					if len(listings) == 0 {
						return owdAPIErr(cmd, flags, err)
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: listings scan stopped after %d page(s): %v\n", out.ScannedPages, err)
				}
			}
			out.ScannedListings = len(listings)

			var knownTLDs map[string]owdTLD
			if tldRows, _, err := owdFetchTLDs(ctx, c, db, false, cmd.ErrOrStderr()); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: TLD prices unavailable, price_x will be null: %v\n", err)
			} else {
				knownTLDs, _ = owdIndexTLDs(tldRows)
			}
			firstSeenBy, err := owdFirstSeen(ctx, db)
			if err != nil {
				return owdTypedErr(cmd, flags, err)
			}

			cands := owdRankFilter(listings, tld, endingDur, maxBidsCap, now)
			out.Matched = len(cands)
			owdRankPreorder(cands, sortKey, now)
			if maxChecks > 0 && len(cands) > maxChecks {
				out.Note = fmt.Sprintf("%d listings matched but only %d were scored (--max-checks %d); raise --max-checks to score more", len(cands), maxChecks, maxChecks)
				cands = cands[:maxChecks]
			}
			if len(cands) == 0 && capped {
				out.Note = fmt.Sprintf("no listings matched within %d page(s) (--max-pages); raise --max-pages or loosen --ending/--max-bids", maxPages)
			}

			known, failures, err := owdRankPopularity(ctx, cmd, c, db, cands, local, now)
			if err != nil {
				return owdTypedErr(cmd, flags, err)
			}
			out.FetchFailures = failures
			for _, l := range cands {
				d := strings.ToLower(l.Domain)
				count, ok := known[d]
				if !ok && !local {
					continue // its failure is listed under fetch_failures
				}
				var tldCount *int
				if ok {
					tldCount = &count
					out.Checked++
				}
				var firstSeen *time.Time
				if fs, seen := firstSeenBy[l.Domain]; seen {
					firstSeen = &fs
				}
				out.Rows = append(out.Rows, owdRankScore(l, tldCount, knownTLDs, firstSeen, now))
			}
			owdRankSort(out.Rows, sortKey)
			if limit > 0 && len(out.Rows) > limit {
				out.Rows = out.Rows[:limit]
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			if len(out.Rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No auctions to rank (%d listings scanned, %d matched the filters).\n", out.ScannedListings, out.Matched)
				if out.Note != "" {
					fmt.Fprintln(cmd.OutOrStdout(), out.Note)
				}
				return nil
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "%d listings scanned, %d matched, %d scored, sorted by %s\n", out.ScannedListings, out.Matched, out.Checked, sortKey)
			if out.Note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), out.Note)
			}
			return owdHumanTable(cmd, flags, out.Rows)
		},
	}
	cmd.Flags().StringVar(&tld, "tld", "", "Only auctions on this TLD, e.g. co")
	cmd.Flags().StringVar(&ending, "ending", "", "Only auctions ending within this window, e.g. 48h or 7d")
	cmd.Flags().IntVar(&maxBids, "max-bids", 0, "Only listings with at most this many bids")
	cmd.Flags().StringVar(&sortKey, "sort", "popularity", "Order rows by: popularity, price-x, bids-per-day, ending")
	cmd.Flags().IntVar(&maxChecks, "max-checks", 100, "Maximum live popularity lookups (listings without a stored snapshot)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 5, "Maximum listing pages to scan (100 listings per page)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Print at most this many rows (0 = all)")
	return cmd
}
