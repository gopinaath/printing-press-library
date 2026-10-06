// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored `listings watch`: fetch the aftermarket listings, diff them
// against the local owd_listing_seen table (new / changed / gone), and flag
// auctions ending soon. Each run updates the sighting table.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// owdEndingSoon is one listing whose auction ends inside the window.
type owdEndingSoon struct {
	Domain   string `json:"domain"`
	Price    string `json:"price"`
	BidCount int    `json:"bid_count"`
	EndDate  string `json:"end_date"`
}

// owdWatchResult is the `listings watch` output envelope.
type owdWatchResult struct {
	CheckedAt  string             `json:"checked_at"`
	Total      int                `json:"total"`
	New        []owdNewListing    `json:"new"`
	Changed    []owdListingChange `json:"changed"`
	Gone       []string           `json:"gone"`
	EndingSoon []owdEndingSoon    `json:"ending_soon"`
	Unchanged  int                `json:"unchanged"`
	Expired    int                `json:"expired"`
}

// owdEndingSoonRows keeps listings ending within the window, soonest first.
func owdEndingSoonRows(listings []owdListing, window time.Duration, now time.Time) []owdEndingSoon {
	out := make([]owdEndingSoon, 0)
	for _, l := range listings {
		if owdEndsWithin(l.EndDate, window, now) {
			out = append(out, owdEndingSoon{Domain: l.Domain, Price: l.Price, BidCount: l.BidCount, EndDate: l.EndDate})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].EndDate != out[j].EndDate {
			return out[i].EndDate < out[j].EndDate
		}
		return out[i].Domain < out[j].Domain
	})
	return out
}

func newNovelListingsWatchCmd(flags *rootFlags) *cobra.Command {
	var tld, endingWithin string
	var maxPages int
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Diff the aftermarket listings against the last run: new, changed, gone, and auctions ending soon",
		Long: strings.TrimSpace(`
Fetch the current auction and buy-now listings (optionally for one TLD) and
compare them with what the previous run recorded locally. New listings, price
or bid changes, and listings that vanished are reported separately, together
with auctions ending inside --ending-within. The first run records everything
as new; run it again later (or on a schedule) to see movement. A vanished
listing is reported once and counts as new if it comes back.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli listings watch
  oneword-domains-pp-cli listings watch --tld co --json
  oneword-domains-pp-cli listings watch --ending-within 6h --max-pages 10 --json --select ending_soon
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "--tld=co",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "listings watch")
			}
			if len(args) > 0 {
				return usageErr(fmt.Errorf("listings watch takes no positional arguments"))
			}
			if err := owdSourceErr(cmd, flags); err != nil {
				return err
			}
			window, err := owdParseWindowFlag("ending-within", endingWithin)
			if err != nil {
				return err
			}
			if maxPages < 1 {
				return usageErr(fmt.Errorf("--max-pages must be at least 1"))
			}
			tld = owdNormTLD(tld)
			ctx, cancel := owdLoopCtx(cmd, flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			c.NoCache = true // a cached page would hide exactly the changes we diff for
			db, _, err := owdOpenStore(ctx)
			if err != nil {
				return err
			}
			defer db.Close()
			var prior int
			_ = db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM owd_listing_seen`).Scan(&prior)
			pageCap := owdDogfoodCap(maxPages, owdDogfoodPages)
			listings, pages, capped, err := owdFetchListings(ctx, c, map[string]string{"tld": tld}, pageCap)
			// Only a scan that saw the feed's end may declare listings gone:
			// an unfetched page is not a vanished listing.
			complete := err == nil && !capped
			switch {
			case err != nil && len(listings) == 0:
				return owdAPIErr(cmd, flags, err)
			case err != nil:
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: stopped after %d page(s): %v; the fetched listings are recorded but 'gone' is not computed from a partial scan\n", pages, err)
			case capped:
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: stopped at the --max-pages cap (%d page(s)) with a full last page; the fetched listings are recorded but 'gone' is not computed from a partial scan; raise --max-pages to see the whole feed\n", pages)
			}
			listings = owdDedupeListings(listings)
			now := owdNow()
			diff, err := owdRecordListings(ctx, db, listings, tld, now, complete)
			if err != nil {
				return owdTypedErr(cmd, flags, fmt.Errorf("recording listings: %w", err))
			}
			out := owdWatchResult{
				CheckedAt:  now.Format(time.RFC3339),
				Total:      len(listings),
				New:        diff.New,
				Changed:    diff.Changed,
				Gone:       diff.Gone,
				EndingSoon: owdEndingSoonRows(listings, window, now),
				Unchanged:  diff.Unchanged,
			}
			for _, l := range listings {
				if t, perr := time.Parse(time.RFC3339, l.EndDate); perr == nil && t.Before(now) {
					out.Expired++
				}
			}
			if out.Expired > 0 && out.Expired == len(listings) {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: every listing's end date has already passed (the site's aftermarket feed is stale); ending_soon cannot populate until the feed refreshes\n")
			}
			if prior == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: first run; all %d listings are new. Run again later to see changes.\n", len(listings))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%d listings at %s: %d new, %d changed, %d gone, %d unchanged, %d ending within %s\n",
				out.Total, out.CheckedAt, len(out.New), len(out.Changed), len(out.Gone), out.Unchanged, len(out.EndingSoon), endingWithin)
			if len(out.New) > 0 {
				fmt.Fprintln(w, "\nnew:")
				if err := owdHumanTable(cmd, flags, out.New); err != nil {
					return err
				}
			}
			if len(out.Changed) > 0 {
				fmt.Fprintln(w, "\nchanged:")
				if err := owdHumanTable(cmd, flags, out.Changed); err != nil {
					return err
				}
			}
			if len(out.Gone) > 0 {
				fmt.Fprintf(w, "\ngone: %s\n", strings.Join(out.Gone, ", "))
			}
			if len(out.EndingSoon) > 0 {
				fmt.Fprintln(w, "\nending soon:")
				if err := owdHumanTable(cmd, flags, out.EndingSoon); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&tld, "tld", "", "Only watch listings under this TLD")
	cmd.Flags().StringVar(&endingWithin, "ending-within", "24h", "Flag auctions ending inside this window (6h, 24h, 3d)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 5, "Listing pages (100 each) to fetch")
	return cmd
}
