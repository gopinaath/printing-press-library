// Copyright 2026 Som Samantray and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type queueItem struct {
	Title          string `json:"title"`
	URL            string `json:"url"`
	Source         string `json:"source"`
	Minutes        int64  `json:"minutes"`
	FriendThoughts int64  `json:"thoughts"`
	Discussed      bool   `json:"discussed"`
}

type queueView struct {
	Items         []queueItem `json:"items"`
	Scanned       int         `json:"scanned_items"`
	Fits          int         `json:"fits_minutes,omitempty"`
	FetchFailures []pnFailure `json:"fetch_failures,omitempty"`
	Note          string      `json:"note,omitempty"`
}

func newNovelQueueCmd(flags *rootFlags) *cobra.Command {
	var fits, limit int
	var includeUnknown bool
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Read-later and bookmarks that fit your free minutes, flagged when friends discussed them.",
		Long: "Read-later and bookmarks that fit your free minutes, flagged when friends discussed them.\n\n" +
			"Merges your read-later list and bookmarked thoughts. Items already discussed rank first, then shortest reads.",
		Example: "  philonet-pp-cli queue --fits 10 --agent",
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "queue")
			}
			if err := pnRequireLive(flags); err != nil {
				return err
			}
			if fits < 0 || limit < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--fits must be >= 0 and --limit >= 1"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if flags.dataSource == "live" {
				c.NoCache = true // --data-source live means the API, never the response cache
			}
			view := queueView{Items: []queueItem{}, Fits: fits}
			var all []queueItem
			rl, rerr := pnGet(ctx, c, "/v1/room/readlater", map[string]string{"limit": "50"})
			if rerr != nil {
				view.FetchFailures = append(view.FetchFailures, pnFailure{Source: "readlater", Error: rerr.Error()})
			} else {
				all = append(all, queueItemsFrom(pnList(pnDig(rl, "data", "results")), "read_later")...)
			}
			bm, berr := pnGet(ctx, c, "/v1/room/mybookmarks", map[string]string{"limit": "50"})
			if berr != nil {
				view.FetchFailures = append(view.FetchFailures, pnFailure{Source: "bookmarks", Error: berr.Error()})
			} else {
				all = append(all, queueItemsFrom(pnList(bm["data"]), "bookmark")...)
			}
			if rerr != nil && berr != nil {
				return apiErr(fmt.Errorf("could not load read-later or bookmarks: %v", rerr))
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of 2 lists failed to load\n", len(view.FetchFailures))
			}
			view.Scanned = len(all)
			view.Items = queueSelect(all, fits, includeUnknown, limit)
			if len(view.Items) == 0 {
				view.Note = "nothing in read-later or bookmarks matches; save items on philonet.ai, or relax --fits"
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			for _, it := range view.Items {
				flag := " "
				if it.Discussed {
					flag = "*"
				}
				fmt.Fprintf(w, "%s %3dm  %s\n        %s\n", flag, it.Minutes, pnTrunc(it.Title, 80), it.URL)
			}
			if view.Note != "" {
				fmt.Fprintln(w, view.Note)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&fits, "fits", 0, "only items readable within this many minutes (0 = no limit)")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum items to return")
	cmd.Flags().BoolVar(&includeUnknown, "include-unknown", false, "with --fits, keep items whose reading time is unknown")
	return cmd
}

func queueItemsFrom(list []any, source string) []queueItem {
	out := make([]queueItem, 0, len(list))
	for _, raw := range list {
		m, _ := raw.(map[string]any)
		if m == nil {
			continue
		}
		art, _ := m["article"].(map[string]any)
		if art == nil {
			art = m
		}
		it := queueItem{
			Title:          pnStr(pnFirst(art, "title", "headline", "quote_headline")),
			URL:            pnStr(pnFirst(art, "url")),
			Source:         source,
			Minutes:        pnInt(pnFirstOf(pnDig(art, "reading_time", "minutes"), pnDig(m, "reading_time", "minutes"), art["reading_minutes"], art["read_time_minutes"])),
			FriendThoughts: pnInt(pnFirstOf(art["total_thoughts"], m["thoughts_count"], art["thoughts_count"], m["reply_count"])),
		}
		it.Discussed = it.FriendThoughts > 0
		if it.Title == "" && it.URL == "" {
			continue
		}
		out = append(out, it)
	}
	return out
}

// queueSelect applies the time filter, drops duplicate URLs, and ranks items
// already discussed first, then by shortest reading time.
func queueSelect(items []queueItem, fits int, includeUnknown bool, limit int) []queueItem {
	idx := map[string]int{}
	merged := make([]queueItem, 0, len(items))
	for _, it := range items {
		key := it.URL
		if key == "" {
			key = it.Title
		}
		if i, ok := idx[key]; ok { // same article in both lists: keep the richer record
			if it.FriendThoughts > merged[i].FriendThoughts {
				merged[i].FriendThoughts, merged[i].Discussed = it.FriendThoughts, it.Discussed
			}
			if merged[i].Minutes == 0 {
				merged[i].Minutes = it.Minutes
			}
			continue
		}
		idx[key] = len(merged)
		merged = append(merged, it)
	}
	out := make([]queueItem, 0, len(merged))
	for _, it := range merged {
		if fits > 0 {
			if it.Minutes == 0 && !includeUnknown {
				continue
			}
			if it.Minutes > int64(fits) {
				continue
			}
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Discussed != out[j].Discussed {
			return out[i].Discussed
		}
		return out[i].Minutes < out[j].Minutes
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
