// Copyright 2026 Som Samantray and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

type todayView struct {
	Date   string `json:"date"`
	Streak struct {
		Current    int64  `json:"current"`
		Max        int64  `json:"max"`
		LastActive string `json:"last_active,omitempty"`
		AtRisk     bool   `json:"at_risk"`
	} `json:"streak"`
	Reading struct {
		TodaySeconds int64 `json:"today_seconds"`
		Last7Seconds int64 `json:"last_7_days_seconds"`
	} `json:"reading"`
	Friends struct {
		ReadToday []string `json:"read_today"`
		Online    []string `json:"online_now"`
		MyRank    int64    `json:"my_rank"`
		OutOf     int64    `json:"out_of"`
	} `json:"friends"`
	Unread struct {
		Total         int64 `json:"total"`
		Notifications int64 `json:"notifications"`
		Invitations   int64 `json:"invitations"`
	} `json:"unread"`
	PendingFriendRequests int64       `json:"pending_friend_requests"`
	FetchFailures         []pnFailure `json:"fetch_failures,omitempty"`
}

func newNovelTodayCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "today",
		Short: "One snapshot of your streak, reading time, friends thinking now, unread counts and pending requests.",
		Long: "One snapshot of your streak, reading time, friends thinking now, unread counts and pending requests.\n\n" +
			"Use this command for a single morning status snapshot. Do NOT use it for reading history over time; use 'rhythm' instead.",
		Example: "  philonet-pp-cli today --agent",
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "today")
			}
			if err := pnRequireLive(flags); err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			uid, err := pnUserID(flags)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if flags.dataSource == "live" {
				c.NoCache = true // --data-source live means the API, never the response cache
			}

			var (
				wg                                   sync.WaitGroup
				prof, stand, think, unread, requests map[string]any
				errs                                 [5]error
			)
			run := func(i int, fn func() (map[string]any, error), dst *map[string]any) {
				defer wg.Done()
				*dst, errs[i] = fn()
			}
			wg.Add(5)
			go run(0, func() (map[string]any, error) {
				return pnGet(ctx, c, "/v1/room/myprofilestats", map[string]string{"userId": uid})
			}, &prof)
			go run(1, func() (map[string]any, error) { return pnGet(ctx, c, "/v1/room/friendsstandings", nil) }, &stand)
			go run(2, func() (map[string]any, error) { return pnGet(ctx, c, "/v1/room/friendsthinkinglist", nil) }, &think)
			go run(3, func() (map[string]any, error) { return pnGet(ctx, c, "/v1/room/unread", nil) }, &unread)
			go run(4, func() (map[string]any, error) {
				return pnGet(ctx, c, "/v1/friend/requests", map[string]string{"type": "received", "limit": "1", "offset": "0"})
			}, &requests)
			wg.Wait()

			names := []string{"myprofilestats", "friendsstandings", "friendsthinkinglist", "unread", "friend requests"}
			var view todayView
			for i, e := range errs {
				if e != nil {
					view.FetchFailures = append(view.FetchFailures, pnFailure{Source: names[i], Error: e.Error()})
				}
			}
			if len(view.FetchFailures) == len(errs) {
				return apiErr(fmt.Errorf("all Philonet calls failed: %s", view.FetchFailures[0].Error))
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d calls failed; sections below are partial\n", len(view.FetchFailures), len(errs))
			}
			fillToday(&view, prof, stand, think, unread, requests, time.Now())

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Today %s\n", view.Date)
			risk := ""
			if view.Streak.AtRisk {
				risk = "  (at risk: nothing read yet today)"
			}
			fmt.Fprintf(w, "  Streak: %d day(s), best %d%s\n", view.Streak.Current, view.Streak.Max, risk)
			fmt.Fprintf(w, "  Read today: %dm   last 7 days: %dm\n", view.Reading.TodaySeconds/60, view.Reading.Last7Seconds/60)
			fmt.Fprintf(w, "  Friends: rank %d of %d; read today %d; online %d\n", view.Friends.MyRank, view.Friends.OutOf, len(view.Friends.ReadToday), len(view.Friends.Online))
			fmt.Fprintf(w, "  Unread: %d (notifications %d, invitations %d); pending friend requests %d\n",
				view.Unread.Total, view.Unread.Notifications, view.Unread.Invitations, view.PendingFriendRequests)
			return nil
		},
	}
	return cmd
}

// fillToday maps raw API envelopes into the briefing. Nil inputs (failed calls)
// leave their section at zero values.
func fillToday(v *todayView, prof, stand, think, unread, requests map[string]any, now time.Time) {
	loc := now.Location()
	if tz := pnStr(pnDig(prof, "data", "timezone")); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	today := now.In(loc).Format("2006-01-02")
	v.Date = today
	v.Streak.Current = pnInt(pnDig(prof, "data", "streak", "current"))
	v.Streak.Max = pnInt(pnDig(prof, "data", "streak", "max"))
	v.Streak.LastActive = pnStr(pnDig(prof, "data", "streak", "last_active_date"))
	v.Reading.Last7Seconds = pnInt(pnDig(prof, "data", "reading", "last_7_days_seconds"))
	for _, d := range pnList(pnDig(prof, "data", "daily_last_7")) {
		if dm, ok := d.(map[string]any); ok && pnStr(dm["date"]) == today {
			v.Reading.TodaySeconds = pnInt(dm["seconds"])
		}
	}
	v.Streak.AtRisk = v.Streak.Current > 0 && v.Reading.TodaySeconds == 0

	v.Friends.ReadToday = []string{}
	v.Friends.Online = []string{}
	v.Friends.MyRank = pnInt(pnDig(stand, "data", "my_rank"))
	v.Friends.OutOf = pnInt(pnDig(stand, "data", "total_participants"))
	for _, e := range pnList(pnDig(stand, "data", "entries")) {
		if em, ok := e.(map[string]any); ok && !pnBool(em["is_me"]) && pnInt(em["thinking_time_seconds"]) > 0 {
			v.Friends.ReadToday = append(v.Friends.ReadToday, pnStr(em["name"]))
		}
	}
	for _, u := range pnList(think["users"]) {
		if um, ok := u.(map[string]any); ok && pnBool(um["is_friend"]) && pnBool(pnDig(um, "presence", "isOnline")) {
			v.Friends.Online = append(v.Friends.Online, pnStr(um["name"]))
		}
	}
	v.Unread.Total = pnInt(unread["total_unread"])
	v.Unread.Notifications = pnInt(unread["unread_notif_count"])
	v.Unread.Invitations = pnInt(unread["pending_invitations_count"])
	v.PendingFriendRequests = pnInt(requests["total_received"])
}
