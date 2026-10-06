// Copyright 2026 Som Samantray and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

type rhythmWeek struct {
	WeekStart  string `json:"week_start"`
	Minutes    int64  `json:"minutes"`
	ActiveDays int    `json:"active_days"`
}

type rhythmMonth struct {
	Month   string `json:"month"`
	Minutes int64  `json:"minutes"`
}

type rhythmRank struct {
	Day          string `json:"day"`
	Rank         int64  `json:"rank"`
	Participants int64  `json:"participants"`
	Streak       int64  `json:"streak"`
}

type rhythmView struct {
	Weeks         []rhythmWeek  `json:"weeks"`
	Months        []rhythmMonth `json:"months"`
	RankTrend     []rhythmRank  `json:"rank_trend"`
	DaysRecorded  int           `json:"days_recorded"`
	FirstDay      string        `json:"first_day,omitempty"`
	LongestStreak int64         `json:"longest_streak"`
	Refreshed     bool          `json:"refreshed"`
	RefreshError  string        `json:"refresh_error,omitempty"`
	Note          string        `json:"note,omitempty"`
}

func newNovelRhythmCmd(flags *rootFlags) *cobra.Command {
	var weeks int
	var dbPath string
	var noRefresh bool
	cmd := &cobra.Command{
		Use:   "rhythm",
		Short: "Weekly and monthly reading minutes, longest streak and rank-vs-friends trend.",
		Long: "Weekly and monthly reading minutes, longest streak and rank-vs-friends trend.\n\n" +
			"Philonet only reports the last week of reading, so each run records those days in the local store and this command aggregates everything recorded so far.\n" +
			"Use this command for trends and history. Do NOT use it for today's live status; use 'today' instead.",
		Example: "  philonet-pp-cli rhythm --weeks 4 --agent",
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "auto",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "rhythm")
			}
			if weeks < 1 || weeks > 104 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--weeks must be between 1 and 104"))
			}
			refresh, strict, err := pnRefreshMode(flags, noRefresh)
			if err != nil {
				return err
			}
			uid, err := pnUserID(flags)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := pnOpenStore(ctx, dbPath, uid)
			if err != nil {
				return err
			}
			defer db.Close()

			view := rhythmView{}
			now := time.Now().In(pnStoredLocation(ctx, db.DB(), uid))
			if refresh {
				loc, rerr := rhythmRefresh(ctx, flags, db.DB(), uid, strict)
				if loc != nil {
					now = now.In(loc) // week boundaries follow the account's own time zone
				}
				if rerr != nil {
					if strict {
						return apiErr(fmt.Errorf("--data-source live: reading-stats refresh failed: %w", rerr))
					}
					view.RefreshError = rerr.Error()
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: live refresh failed (%v); using recorded history only\n", rerr)
				} else {
					view.Refreshed = true
				}
			}
			if err := rhythmAggregate(ctx, db.DB(), uid, weeks, now, &view); err != nil {
				return err
			}
			if view.DaysRecorded == 0 && view.RefreshError != "" {
				return apiErr(fmt.Errorf("could not load reading stats and nothing is recorded yet: %s", view.RefreshError))
			}
			if view.DaysRecorded == 0 {
				view.Note = "no reading days recorded yet; read something on Philonet and run again"
			} else {
				view.Note = fmt.Sprintf("history begins %s; Philonet exposes only the last week, so older days are unknown", view.FirstDay)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Reading rhythm (%d day(s) recorded, longest streak %d)\n", view.DaysRecorded, view.LongestStreak)
			for _, wk := range view.Weeks {
				fmt.Fprintf(w, "  week of %s  %4dm  %d active day(s)\n", wk.WeekStart, wk.Minutes, wk.ActiveDays)
			}
			for _, r := range view.RankTrend {
				fmt.Fprintf(w, "  %s  rank %d of %d\n", r.Day, r.Rank, r.Participants)
			}
			fmt.Fprintln(w, view.Note)
			return nil
		},
	}
	cmd.Flags().IntVar(&weeks, "weeks", 4, "number of recent weeks to report (zero-minute weeks included)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "skip the live fetch and aggregate only what is already recorded (same as --data-source local)")
	return cmd
}

// rhythmRefresh fetches the last week of reading and the friends standing and
// records both in ONE transaction, so a failure never leaves half a refresh
// behind. It returns the account's time zone when the profile reports one.
func rhythmRefresh(ctx context.Context, flags *rootFlags, db *sql.DB, uid string, strict bool) (*time.Location, error) {
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	if strict {
		c.NoCache = true // --data-source live means the API, never the response cache
	}
	prof, err := pnGet(ctx, c, "/v1/room/myprofilestats", map[string]string{"userId": uid})
	if err != nil {
		return nil, err
	}
	loc := time.Local
	tzName := ""
	if tz := pnStr(pnDig(prof, "data", "timezone")); tz != "" {
		if l, lerr := time.LoadLocation(tz); lerr == nil {
			loc, tzName = l, tz
		}
	}
	// A failed standings call stores NULL (never 0) so a good rank captured
	// earlier the same day is kept by the COALESCE below. Under strict mode a
	// failed standings call is an error: live means no silent partial refresh.
	var rank, parts any
	stand, serr := pnGet(ctx, c, "/v1/room/friendsstandings", nil)
	if serr == nil {
		rank = pnInt(pnDig(stand, "data", "my_rank"))
		parts = pnInt(pnDig(stand, "data", "total_participants"))
	} else if strict {
		return loc, fmt.Errorf("friends standings: %w", serr)
	}

	stamp := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return loc, err
	}
	for _, d := range pnList(pnDig(prof, "data", "daily_last_7")) {
		dm, ok := d.(map[string]any)
		if !ok || pnStr(dm["date"]) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO pn_reading_days(uid,day,seconds,captured_at) VALUES(?,?,?,?)
			ON CONFLICT(uid, day) DO UPDATE SET seconds=excluded.seconds, captured_at=excluded.captured_at`,
			uid, pnStr(dm["date"]), pnInt(dm["seconds"]), stamp); err != nil {
			_ = tx.Rollback()
			return loc, err
		}
	}
	if tzName != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO pn_account_meta(uid,timezone) VALUES(?,?)
			ON CONFLICT(uid) DO UPDATE SET timezone=excluded.timezone`, uid, tzName); err != nil {
			_ = tx.Rollback()
			return loc, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO pn_snapshots(uid,day,captured_at,streak_current,streak_max,my_rank,participants) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(uid, day) DO UPDATE SET captured_at=excluded.captured_at, streak_current=excluded.streak_current,
		streak_max=excluded.streak_max, my_rank=COALESCE(excluded.my_rank, pn_snapshots.my_rank),
		participants=COALESCE(excluded.participants, pn_snapshots.participants)`,
		uid, time.Now().In(loc).Format("2006-01-02"), stamp, pnInt(pnDig(prof, "data", "streak", "current")), pnInt(pnDig(prof, "data", "streak", "max")), rank, parts); err != nil {
		_ = tx.Rollback()
		return loc, err
	}
	return loc, tx.Commit()
}

// rhythmAggregate fills view from recorded days. It drains each result set
// before running the next query (single SQLite connection rule).
func rhythmAggregate(ctx context.Context, db *sql.DB, uid string, weeks int, now time.Time, view *rhythmView) error {
	type dayRow struct {
		day     string
		seconds int64
	}
	rows, err := db.QueryContext(ctx, `SELECT day, seconds FROM pn_reading_days WHERE uid = ? ORDER BY day`, uid)
	if err != nil {
		return fmt.Errorf("reading recorded days: %w", err)
	}
	var days []dayRow
	for rows.Next() {
		var d dayRow
		if err := rows.Scan(&d.day, &d.seconds); err != nil {
			_ = rows.Close()
			return err
		}
		days = append(days, d)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	byWeek := map[string]*rhythmWeek{}
	weekSecs := map[string]int64{}
	byMonthSecs := map[string]int64{}
	for _, d := range days {
		t, err := time.ParseInLocation("2006-01-02", d.day, now.Location())
		if err != nil {
			continue
		}
		ws := pnWeekStart(t).Format("2006-01-02")
		w := byWeek[ws]
		if w == nil {
			w = &rhythmWeek{WeekStart: ws}
			byWeek[ws] = w
		}
		weekSecs[ws] += d.seconds
		if d.seconds > 0 {
			w.ActiveDays++
		}
		byMonthSecs[t.Format("2006-01")] += d.seconds
	}
	for ws, w := range byWeek {
		w.Minutes = weekSecs[ws] / 60
	}
	view.DaysRecorded = len(days)
	if len(days) > 0 {
		view.FirstDay = days[0].day
	}
	view.Weeks = make([]rhythmWeek, 0, weeks)
	cur := pnWeekStart(now)
	for i := weeks - 1; i >= 0; i-- {
		ws := cur.AddDate(0, 0, -7*i).Format("2006-01-02")
		if w := byWeek[ws]; w != nil {
			view.Weeks = append(view.Weeks, *w)
		} else {
			view.Weeks = append(view.Weeks, rhythmWeek{WeekStart: ws})
		}
	}
	view.Months = make([]rhythmMonth, 0, len(byMonthSecs))
	for _, m := range pnSortedKeys(byMonthSecs) {
		view.Months = append(view.Months, rhythmMonth{Month: m, Minutes: byMonthSecs[m] / 60})
	}

	srows, err := db.QueryContext(ctx, `SELECT day, COALESCE(my_rank,0), COALESCE(participants,0), COALESCE(streak_current,0)
		FROM pn_snapshots WHERE uid = ? AND my_rank IS NOT NULL ORDER BY day DESC LIMIT ?`, uid, weeks*7)
	if err != nil {
		return fmt.Errorf("reading snapshots: %w", err)
	}
	view.RankTrend = make([]rhythmRank, 0)
	for srows.Next() {
		var r rhythmRank
		if err := srows.Scan(&r.Day, &r.Rank, &r.Participants, &r.Streak); err != nil {
			_ = srows.Close()
			return err
		}
		view.RankTrend = append(view.RankTrend, r)
	}
	if err := srows.Err(); err != nil {
		_ = srows.Close()
		return err
	}
	_ = srows.Close()
	var longest sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(streak_max) FROM pn_snapshots WHERE uid = ?`, uid).Scan(&longest); err != nil {
		return fmt.Errorf("reading longest streak: %w", err)
	}
	view.LongestStreak = longest.Int64
	for i, j := 0, len(view.RankTrend)-1; i < j; i, j = i+1, j-1 { // oldest first
		view.RankTrend[i], view.RankTrend[j] = view.RankTrend[j], view.RankTrend[i]
	}
	return nil
}
