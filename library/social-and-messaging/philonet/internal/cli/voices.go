// Copyright 2026 Som Samantray and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/internal/cliutil"
)

type voicePerson struct {
	Name       string   `json:"name"`
	UserID     string   `json:"user_id"`
	Thoughts   int      `json:"thoughts"`
	Reactions  int64    `json:"reactions"`
	AlmaMater  string   `json:"alma_mater,omitempty"`
	Employer   string   `json:"employer,omitempty"`
	TopTags    []string `json:"top_tags"`
	IsFriend   bool     `json:"is_friend"`
	SampleText string   `json:"sample_thought,omitempty"`
}

type voicesView struct {
	Topic         string        `json:"topic,omitempty"`
	Badge         string        `json:"badge,omitempty"`
	Voices        []voicePerson `json:"voices"`
	CardsScanned  int           `json:"cards_scanned"`
	FetchFailures []string      `json:"fetch_failures,omitempty"`
	Note          string        `json:"note,omitempty"`
}

func newNovelVoicesCmd(flags *rootFlags) *cobra.Command {
	var topic, badge, dbPath string
	var limit, maxScanPages int
	var noRefresh bool
	cmd := &cobra.Command{
		Use:   "voices",
		Short: "Rank people by thoughts on a topic, filtered by alma mater or employer badge.",
		Long: "Rank people by thoughts on a topic, filtered by alma mater or employer badge.\n\n" +
			"Works over the feed cards this CLI has stored; each run tops the store up from your For You feed.\n" +
			"Use this command to rank people by credential and topic from your synced data. Do NOT use it for a keyword search across thoughts; use 'find thoughts' instead.",
		Example: "  philonet-pp-cli voices --topic ai --badge professional --agent",
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "auto",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "voices")
			}
			if limit < 1 || maxScanPages < 1 || maxScanPages > 20 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit must be at least 1 and --max-scan-pages between 1 and 20"))
			}
			switch badge {
			case "", "any", "alma_mater", "professional":
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--badge must be one of: any, alma_mater, professional"))
			}
			if cliutil.IsDogfoodEnv() && maxScanPages > 1 {
				maxScanPages = 1
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

			view := voicesView{Topic: topic, Badge: badge}
			if refresh {
				c, cerr := flags.newClient()
				if cerr != nil {
					return cerr
				}
				if strict {
					c.NoCache = true // --data-source live means the API, never the response cache
				}
				_, view.FetchFailures = pnRefreshFeeds(ctx, c, db.DB(), uid, []string{"forme"}, maxScanPages, strict)
				if strict && len(view.FetchFailures) > 0 {
					return apiErr(fmt.Errorf("--data-source live: feed refresh failed: %s", view.FetchFailures[0]))
				}
				if len(view.FetchFailures) > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d feed page(s) failed to refresh; ranking stored cards only\n", len(view.FetchFailures))
				}
			}
			if err := voicesBuild(ctx, db.DB(), uid, topic, badge, limit, &view); err != nil {
				return err
			}
			if len(view.Voices) == 0 && len(view.FetchFailures) > 0 && view.CardsScanned == 0 {
				return apiErr(fmt.Errorf("could not load the feed and nothing is stored yet: %s", view.FetchFailures[0]))
			}
			if len(view.Voices) == 0 {
				view.Note = "no matching people in stored cards; widen --topic/--badge or raise --max-scan-pages"
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			for i, v := range view.Voices {
				cred := strings.Trim(strings.Join([]string{v.AlmaMater, v.Employer}, " / "), " /")
				fmt.Fprintf(w, "%2d. %s  thoughts %d  reactions %d  %s\n", i+1, v.Name, v.Thoughts, v.Reactions, cred)
			}
			if view.Note != "" {
				fmt.Fprintln(w, view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&topic, "topic", "", "keep thoughts whose tags, article title or text mention this")
	cmd.Flags().StringVar(&badge, "badge", "", "require a verification badge: any, alma_mater or professional")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum people to return")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 3, "feed pages to pull before ranking")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "skip the live feed pull and rank only what is stored (same as --data-source local)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func voicesBuild(ctx context.Context, db *sql.DB, uid, topic, badge string, limit int, view *voicesView) error {
	rows, err := db.QueryContext(ctx, `SELECT starter_id, starter_name, starter_is_friend, alma_mater, employer, tags, article_title, thought_text, insightful
		FROM pn_feed_cards WHERE uid = ? AND starter_id <> '' ORDER BY first_seen DESC, card_key`, uid)
	if err != nil {
		return fmt.Errorf("reading stored cards: %w", err)
	}
	type agg struct {
		p    voicePerson
		tags map[string]int
	}
	byID := map[string]*agg{}
	needle := strings.ToLower(strings.TrimSpace(topic))
	scanned := 0
	for rows.Next() {
		var id, name, tags, title, text sql.NullString
		var alma, emp sql.NullString
		var friend int
		var react int64
		if err := rows.Scan(&id, &name, &friend, &alma, &emp, &tags, &title, &text, &react); err != nil {
			_ = rows.Close()
			return err
		}
		scanned++
		if needle != "" && !voicesMatchTopic(needle, tags.String, title.String, text.String) {
			continue
		}
		switch badge {
		case "alma_mater":
			if alma.String == "" {
				continue
			}
		case "professional":
			if emp.String == "" {
				continue
			}
		case "any":
			if alma.String == "" && emp.String == "" {
				continue
			}
		}
		a := byID[id.String]
		if a == nil {
			a = &agg{p: voicePerson{Name: name.String, UserID: id.String, AlmaMater: alma.String, Employer: emp.String, IsFriend: friend == 1, TopTags: []string{}}, tags: map[string]int{}}
			byID[id.String] = a
		}
		a.p.Thoughts++
		if a.p.AlmaMater == "" {
			a.p.AlmaMater = alma.String
		}
		if a.p.Employer == "" {
			a.p.Employer = emp.String
		}
		a.p.Reactions += react
		if a.p.SampleText == "" {
			a.p.SampleText = pnTrunc(text.String, 140)
		}
		for _, t := range strings.Split(tags.String, "|") {
			if t != "" {
				a.tags[t]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	view.CardsScanned = scanned
	view.Voices = make([]voicePerson, 0, len(byID))
	for _, a := range byID {
		keys := pnSortedKeys(a.tags)
		sort.SliceStable(keys, func(i, j int) bool { return a.tags[keys[i]] > a.tags[keys[j]] })
		if len(keys) > 3 {
			keys = keys[:3]
		}
		a.p.TopTags = keys
		view.Voices = append(view.Voices, a.p)
	}
	sort.SliceStable(view.Voices, func(i, j int) bool {
		if view.Voices[i].Thoughts != view.Voices[j].Thoughts {
			return view.Voices[i].Thoughts > view.Voices[j].Thoughts
		}
		if view.Voices[i].Reactions != view.Voices[j].Reactions {
			return view.Voices[i].Reactions > view.Voices[j].Reactions
		}
		if view.Voices[i].Name != view.Voices[j].Name {
			return view.Voices[i].Name < view.Voices[j].Name
		}
		return view.Voices[i].UserID < view.Voices[j].UserID
	})
	if limit > 0 && len(view.Voices) > limit {
		view.Voices = view.Voices[:limit]
	}
	return nil
}

// voicesMatchTopic matches the topic as a whole word (or exact tag) so that
// "ai" does not match "said" or "training". Multi-word topics match as a phrase.
func voicesMatchTopic(needle, tags, title, text string) bool {
	for _, t := range strings.Split(tags, "|") {
		if strings.EqualFold(strings.TrimSpace(t), needle) {
			return true
		}
	}
	for _, s := range []string{tags, title, text} {
		if wordContains(strings.ToLower(s), needle) {
			return true
		}
	}
	return false
}

func wordContains(hay, needle string) bool {
	isWord := func(r byte) bool {
		return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 0x80
	}
	for i := 0; ; {
		j := strings.Index(hay[i:], needle)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(needle)
		if (start == 0 || !isWord(hay[start-1])) && (end == len(hay) || !isWord(hay[end])) {
			return true
		}
		i = start + 1
	}
}
