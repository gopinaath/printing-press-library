// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored `tlds drift`: registration-price drift per TLD and registrar,
// computed from the local owd_tld_prices snapshots that this command (and
// every TLD detail fetch) appends to. Live mode records today's snapshot
// first; --data-source local only reads.
// pp:data-source auto

package cli

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
	"github.com/spf13/cobra"
)

// owdPriceSample is one owd_tld_prices row.
type owdPriceSample struct {
	TLD       string
	Registrar string
	Price     float64
	At        time.Time
}

// owdDriftRow is one (tld, registrar) whose price moved across the window.
type owdDriftRow struct {
	TLD       string  `json:"tld"`
	Registrar string  `json:"registrar"`
	PriceThen float64 `json:"price_then"`
	PriceNow  float64 `json:"price_now"`
	Delta     float64 `json:"delta"`
	DeltaPct  float64 `json:"delta_pct"`
	ThenAt    string  `json:"then_at"`
	NowAt     string  `json:"now_at"`
}

// owdDriftRows compares, per (tld, registrar), the latest sample with the
// latest sample at or before cutoff and keeps the pairs whose price changed,
// sorted by |delta_pct| descending.
func owdDriftRows(samples []owdPriceSample, cutoff time.Time) []owdDriftRow {
	type key struct{ tld, reg string }
	groups := map[key][]owdPriceSample{}
	order := make([]key, 0)
	for _, s := range samples {
		k := key{s.TLD, s.Registrar}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], s)
	}
	rows := make([]owdDriftRow, 0)
	for _, k := range order {
		g := groups[k]
		sort.SliceStable(g, func(i, j int) bool { return g[i].At.Before(g[j].At) })
		now := g[len(g)-1]
		var then *owdPriceSample
		for i := len(g) - 1; i >= 0; i-- {
			if !g[i].At.After(cutoff) {
				s := g[i]
				then = &s
				break
			}
		}
		if then == nil || then.At.Equal(now.At) || then.Price == now.Price {
			continue
		}
		delta := now.Price - then.Price
		pct := 0.0
		if then.Price != 0 {
			pct = delta / then.Price * 100
		}
		rows = append(rows, owdDriftRow{
			TLD: k.tld, Registrar: k.reg,
			PriceThen: then.Price, PriceNow: now.Price,
			Delta:    owdRound2(delta),
			DeltaPct: owdRoundPct(pct),
			ThenAt:   then.At.UTC().Format(time.RFC3339),
			NowAt:    now.At.UTC().Format(time.RFC3339),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ai, aj := math.Abs(rows[i].DeltaPct), math.Abs(rows[j].DeltaPct)
		if ai != aj {
			return ai > aj
		}
		if rows[i].TLD != rows[j].TLD {
			return rows[i].TLD < rows[j].TLD
		}
		return rows[i].Registrar < rows[j].Registrar
	})
	return rows
}

// owdLoadPriceSamples drains owd_tld_prices into memory.
func owdLoadPriceSamples(ctx context.Context, db *store.Store) ([]owdPriceSample, error) {
	out := make([]owdPriceSample, 0)
	rows, err := db.DB().QueryContext(ctx, `SELECT tld, registrar, price, snapshot_at FROM owd_tld_prices WHERE price IS NOT NULL ORDER BY snapshot_at, id`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var s owdPriceSample
		var at any
		if err := rows.Scan(&s.TLD, &s.Registrar, &s.Price, &at); err != nil {
			_ = rows.Close()
			return out, err
		}
		s.At = owdScanTime(at)
		if s.At.IsZero() {
			continue
		}
		out = append(out, s)
	}
	_ = rows.Close()
	return out, rows.Err()
}

// owdSnapshotDates counts the distinct calendar days with a snapshot.
func owdSnapshotDates(samples []owdPriceSample) int {
	days := map[string]bool{}
	for _, s := range samples {
		days[s.At.Format("2006-01-02")] = true
	}
	return len(days)
}

// owdMinSnapshotTLDsToday counts the TLDs that already have a "min" price row
// since the start of today (UTC). Registrar rows written by check/compare do
// not count, so a full daily snapshot is still taken after them.
func owdMinSnapshotTLDsToday(ctx context.Context, db *store.Store, now time.Time) int {
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	var n int
	_ = db.DB().QueryRowContext(ctx, `SELECT COUNT(DISTINCT tld) FROM owd_tld_prices WHERE registrar='min' AND snapshot_at >= ?`, dayStart).Scan(&n)
	return n
}

// owdSnapshotTLDsNeeded is how many TLDs a complete daily "min" snapshot
// holds: owdTLDListComplete by default, lowered to the number of locally
// stored TLDs whose list price parses once the local list is complete (six
// of the 93 carry no min price, so 87 is the most a snapshot can record).
func owdSnapshotTLDsNeeded(ctx context.Context, db *store.Store) int {
	needed := owdTLDListComplete
	tlds, _, err := owdFetchTLDs(ctx, nil, db, false, nil)
	if err != nil || len(tlds) < owdTLDListComplete {
		return needed
	}
	priceable := 0
	for _, t := range tlds {
		if _, ok := owdPriceFloat(t.MinPrice); ok {
			priceable++
		}
	}
	if priceable > 0 && priceable < needed {
		needed = priceable
	}
	return needed
}

func newNovelTldsDriftCmd(flags *rootFlags) *cobra.Command {
	var since string
	var snapshot, registrars bool
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Show which TLD registration prices moved since a past snapshot, per registrar",
		Long: strings.TrimSpace(`
Compare today's TLD prices with the latest local snapshot taken at or before
--since ago. Snapshots accumulate in the local store: this command records the
min price of all 93 TLDs whenever today's snapshot is missing (or with
--snapshot), --registrars always also records every registrar's price, and
'check', 'compare' and 'brainstorm' add rows for the TLDs they touch as a
side effect.
Run it on different days to see drift; --data-source local only reads what is
already recorded.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli tlds drift
  oneword-domains-pp-cli tlds drift --since 7d --json
  oneword-domains-pp-cli tlds drift --snapshot --registrars
  oneword-domains-pp-cli tlds drift --since 90d --data-source local --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "auto",
			"pp:happy-args":  "--since=30d",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "tlds drift")
			}
			if len(args) > 0 {
				return usageErr(fmt.Errorf("tlds drift takes no positional arguments"))
			}
			window, err := owdParseWindowFlag("since", since)
			if err != nil {
				return err
			}
			ctx, cancel := owdLoopCtx(cmd, flags)
			defer cancel()
			db, _, err := owdOpenStore(ctx)
			if err != nil {
				return err
			}
			defer db.Close()
			now := owdNow()
			local := flags.dataSource == "local"
			todayTLDs := owdMinSnapshotTLDsToday(ctx, db, now)
			switch {
			case local && snapshot:
				fmt.Fprintln(cmd.ErrOrStderr(), "note: --data-source local skips the snapshot fetch; reading recorded prices only")
			case !local && (snapshot || registrars || todayTLDs < owdSnapshotTLDsNeeded(ctx, db)):
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				tlds, _, err := owdFetchTLDs(ctx, c, db, true, cmd.ErrOrStderr())
				if err != nil {
					return owdAPIErr(cmd, flags, err)
				}
				var snap owdSnapshotErrs
				n, err := owdRecordMinPrices(db, tlds, now)
				snap.add(err)
				fmt.Fprintf(cmd.ErrOrStderr(), "snapshot: recorded min prices for %d TLDs at %s\n", n, now.Format(time.RFC3339))
				if registrars {
					targets := owdTopTLDs(tlds, owdDogfoodCap(len(tlds), owdDogfoodTLDs))
					details, errs := owdDetailsByTLD(ctx, c, db, targets, owdDefaultConcurrency, now, &snap)
					owdWarnFailuresInline(cmd.ErrOrStderr(), errs, len(targets), "TLD detail fetches")
					fmt.Fprintf(cmd.ErrOrStderr(), "snapshot: recorded registrar prices for %d TLDs\n", len(details))
				}
				snap.warn(cmd.ErrOrStderr())
			}
			samples, err := owdLoadPriceSamples(ctx, db)
			if err != nil {
				return owdTypedErr(cmd, flags, err)
			}
			rows := make([]owdDriftRow, 0)
			days := owdSnapshotDates(samples)
			if days < 2 {
				if days == 0 {
					hintIfUnsynced(cmd, db, "tlds")
					fmt.Fprintln(cmd.ErrOrStderr(), "note: no price snapshot recorded yet; run without --data-source local to take one")
				} else {
					fmt.Fprintln(cmd.ErrOrStderr(), "note: only one snapshot recorded; run again later or with --snapshot on another day")
				}
			} else {
				rows = owdDriftRows(samples, now.Add(-window))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No price changes across %d snapshot day(s) within the last %s.\n", days, since)
				return nil
			}
			return owdHumanTable(cmd, flags, rows)
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d", "Compare against the latest snapshot at or before this long ago (7d, 30d, 12h)")
	cmd.Flags().BoolVar(&snapshot, "snapshot", false, "Record a fresh price snapshot before reporting, even if one exists for today")
	cmd.Flags().BoolVar(&registrars, "registrars", false, "Also record every registrar's price (one TLD detail fetch per TLD)")
	return cmd
}
