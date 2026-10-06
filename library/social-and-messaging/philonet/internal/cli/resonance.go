// Copyright 2026 Som Samantray and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/internal/cliutil"
)

type resonanceThought struct {
	ThoughtID string `json:"thought_id"`
	Article   string `json:"article"`
	Text      string `json:"text"`
	Score     int64  `json:"score"`
	Insights  int64  `json:"insightful"`
	Stars     int64  `json:"stars"`
	Resonated int64  `json:"resonated"`
	Replies   int64  `json:"replies"`
	Tag       string `json:"tag,omitempty"`
	Week      string `json:"week,omitempty"`
}

type resonanceGroup struct {
	Key      string `json:"key"`
	Thoughts int    `json:"thoughts"`
	Score    int64  `json:"score"`
}

type resonanceView struct {
	By              string             `json:"by"`
	Top             []resonanceThought `json:"top"`
	Groups          []resonanceGroup   `json:"groups,omitempty"`
	ScannedThoughts int                `json:"scanned_thoughts"`
	StarsReceived   int64              `json:"total_stars_received"`
	FetchFailures   []pnFailure        `json:"fetch_failures,omitempty"`
	Note            string             `json:"note,omitempty"`
}

func newNovelResonanceCmd(flags *rootFlags) *cobra.Command {
	var by string
	var limit, maxScanPages int
	cmd := &cobra.Command{
		Use:   "resonance",
		Short: "Which of your thoughts earned stars and insightful reactions, by tag and week.",
		Long: "Which of your thoughts earned stars and insightful reactions, by tag and week.\n\n" +
			"Score = insightful reactions + stars + resonates on each of your thoughts. --by groups the totals by tag or by week.",
		Example: "  philonet-pp-cli resonance --by week --agent",
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "resonance")
			}
			if err := pnRequireLive(flags); err != nil {
				return err
			}
			switch by {
			case "", "none", "tag", "week":
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--by must be one of: tag, week, none"))
			}
			if by == "" {
				by = "none"
			}
			if limit < 1 || maxScanPages < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit and --max-scan-pages must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
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
			view := resonanceView{By: by, Top: []resonanceThought{}}
			var thoughts []resonanceThought
			for page := 1; page <= maxScanPages; page++ {
				m, err := pnGet(ctx, c, "/v1/room/conversationsnew", map[string]string{"page": strconv.Itoa(page), "limit": "20"})
				if err != nil {
					return apiErr(fmt.Errorf("listing your thoughts (page %d): %w", page, err))
				}
				list := pnList(m["data"])
				thoughts = append(thoughts, resonanceFrom(list)...)
				if !pnBool(pnDig(m, "pagination", "has_next")) || len(list) == 0 {
					break
				}
			}
			view.ScannedThoughts = len(thoughts)
			if aw, aerr := pnGet(ctx, c, "/v1/room/myawards", nil); aerr == nil {
				view.StarsReceived = pnInt(pnDig(aw, "meta", "total_stars_received"))
			} else {
				view.FetchFailures = append(view.FetchFailures, pnFailure{Source: "myawards", Error: aerr.Error()})
			}
			view.Top, view.Groups = resonanceRank(thoughts, by, limit)
			if len(thoughts) == 0 {
				view.Note = "you have no thoughts yet; post one with 'post thought' and run this again"
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			for _, g := range view.Groups {
				fmt.Fprintf(w, "%-24s %3d thought(s)  score %d\n", g.Key, g.Thoughts, g.Score)
			}
			for _, t := range view.Top {
				fmt.Fprintf(w, "%4d  %s\n", t.Score, pnTrunc(t.Text, 100))
			}
			if view.Note != "" {
				fmt.Fprintln(w, view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "none", "group totals by: tag, week or none")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum thoughts to list")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "pages of your thoughts to scan (20 per page)")
	return cmd
}

func resonanceFrom(list []any) []resonanceThought {
	loc := time.Now().Location()
	out := make([]resonanceThought, 0, len(list))
	for _, raw := range list {
		m, _ := raw.(map[string]any)
		if m == nil {
			continue
		}
		art, _ := m["article"].(map[string]any)
		t := resonanceThought{
			ThoughtID: pnStr(pnFirst(m, "conversation_id", "comment_id", "id")),
			Article:   pnStr(pnFirst(art, "title", "headline")),
			Text:      pnTrunc(pnStr(pnFirst(m, "content", "quote_headline", "quote")), 160),
			Insights:  pnInt(pnFirst(m, "insights_count", "insightful_count")),
			Stars:     pnInt(pnFirst(m, "star_count", "stars")),
			Resonated: pnInt(m["resonate_count"]),
			Replies:   pnInt(pnFirst(m, "reply_count", "thread_count")),
		}
		t.Score = t.Insights + t.Stars + t.Resonated
		if tags := pnList(art["tags"]); len(tags) > 0 {
			t.Tag = pnStr(tags[0])
		} else if art != nil {
			t.Tag = pnStr(pnFirst(art, "category", "reading_label"))
		}
		if at, err := time.Parse(time.RFC3339, pnStr(m["created_at"])); err == nil {
			t.Week = pnWeekStart(at.In(loc)).Format("2006-01-02")
		}
		out = append(out, t)
	}
	return out
}

func resonanceRank(ts []resonanceThought, by string, limit int) ([]resonanceThought, []resonanceGroup) {
	sorted := make([]resonanceThought, len(ts))
	copy(sorted, ts)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	var groups []resonanceGroup
	if by == "tag" || by == "week" {
		agg := map[string]*resonanceGroup{}
		for _, t := range ts {
			key := t.Tag
			if by == "week" {
				key = t.Week
			}
			if key == "" {
				key = "unknown"
			}
			g := agg[key]
			if g == nil {
				g = &resonanceGroup{Key: key}
				agg[key] = g
			}
			g.Thoughts++
			g.Score += t.Score
		}
		for _, k := range pnSortedKeys(agg) {
			groups = append(groups, *agg[k])
		}
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Score > groups[j].Score })
	}
	return sorted, groups
}
