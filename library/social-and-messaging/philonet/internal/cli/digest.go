// Copyright 2026 Som Samantray and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/internal/cliutil"
)

type digestThought struct {
	ArticleTitle string `json:"article_title"`
	ArticleURL   string `json:"article_url"`
	Thought      string `json:"thought"`
	Reactions    int64  `json:"reactions"`
	FirstSeen    string `json:"first_seen"`
}

type digestPerson struct {
	Name     string          `json:"name"`
	UserID   string          `json:"user_id"`
	IsFriend bool            `json:"is_friend"`
	Thoughts []digestThought `json:"thoughts"`
}

type digestView struct {
	Since         string         `json:"since"`
	People        []digestPerson `json:"people"`
	CardsScanned  int            `json:"cards_scanned"`
	FetchFailures []string       `json:"fetch_failures,omitempty"`
	Note          string         `json:"note,omitempty"`
}

func newNovelDigestCmd(flags *rootFlags) *cobra.Command {
	var since, dbPath string
	var all, noRefresh bool
	var limit, maxScanPages int
	cmd := &cobra.Command{
		Use:   "digest",
		Short: "What your top friends thought and read recently, grouped by friend.",
		Long: "What your top friends thought and read recently, grouped by friend.\n\n" +
			"Each run pulls the newest feed pages into the local store; 'recently' means first seen by this CLI within --since.\n" +
			"Use this command to see what top friends said recently. Do NOT use it to find people by credential or topic; use 'voices' instead.",
		Example: "  philonet-pp-cli digest --since 24h --agent",
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "auto",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "digest")
			}
			if limit < 1 || maxScanPages < 1 || maxScanPages > 20 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit must be at least 1 and --max-scan-pages between 1 and 20"))
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--since must be a duration such as 24h or 7d"))
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

			view := digestView{Since: since}
			if refresh {
				c, cerr := flags.newClient()
				if cerr != nil {
					return cerr
				}
				if strict {
					c.NoCache = true // --data-source live means the API, never the response cache
				}
				_, view.FetchFailures = pnRefreshFeeds(ctx, c, db.DB(), uid, []string{"friends", "forme"}, maxScanPages, strict)
				if strict && len(view.FetchFailures) > 0 {
					return apiErr(fmt.Errorf("--data-source live: feed refresh failed: %s", view.FetchFailures[0]))
				}
				if len(view.FetchFailures) > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d feed page(s) failed to refresh; showing what is already stored\n", len(view.FetchFailures))
				}
			}
			if err := digestBuild(ctx, db.DB(), uid, time.Now().Add(-window), all, limit, &view); err != nil {
				return err
			}
			if len(view.People) == 0 && len(view.FetchFailures) > 0 && view.CardsScanned == 0 {
				return apiErr(fmt.Errorf("could not load the feed and nothing is stored yet: %s", view.FetchFailures[0]))
			}
			if len(view.People) == 0 {
				if all {
					view.Note = "no thoughts seen in this window; raise --since or --max-scan-pages"
				} else {
					view.Note = "no friend thoughts seen in this window; add --all to include everyone in your feed"
				}
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			for _, p := range view.People {
				fmt.Fprintf(w, "%s (%d)\n", p.Name, len(p.Thoughts))
				for _, t := range p.Thoughts {
					fmt.Fprintf(w, "  - %s\n      on: %s\n", pnTrunc(t.Thought, 140), pnTrunc(t.ArticleTitle, 80))
				}
			}
			if view.Note != "" {
				fmt.Fprintln(w, view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "window of newly seen thoughts, e.g. 24h or 7d")
	cmd.Flags().BoolVar(&all, "all", false, "include thoughts from people who are not your friends")
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum people to return")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 3, "feed pages to pull per feed before reading the store")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "skip the live feed pull and read only the local store (same as --data-source local)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func digestBuild(ctx context.Context, db *sql.DB, uid string, cutoff time.Time, all bool, limit int, view *digestView) error {
	q := `SELECT starter_id, starter_name, starter_is_friend, article_title, article_url, thought_text, insightful, first_seen
		FROM pn_feed_cards WHERE uid = ? AND first_seen >= ? AND starter_id <> ''`
	if !all {
		q += ` AND starter_is_friend = 1`
	}
	q += ` ORDER BY first_seen DESC, insightful DESC, card_key`
	rows, err := db.QueryContext(ctx, q, uid, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("reading stored cards: %w", err)
	}
	type raw struct {
		id, name, title, url, text, seen string
		friend                           int
		react                            int64
	}
	var rs []raw
	for rows.Next() {
		var r raw
		var title, url, text sql.NullString
		if err := rows.Scan(&r.id, &r.name, &r.friend, &title, &url, &text, &r.react, &r.seen); err != nil {
			_ = rows.Close()
			return err
		}
		r.title, r.url, r.text = title.String, url.String, text.String
		rs = append(rs, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	view.CardsScanned = len(rs)
	view.People = make([]digestPerson, 0)
	idx := map[string]int{}
	for _, r := range rs {
		i, ok := idx[r.id]
		if !ok {
			if limit > 0 && len(view.People) >= limit {
				continue
			}
			view.People = append(view.People, digestPerson{Name: r.name, UserID: r.id, IsFriend: r.friend == 1, Thoughts: []digestThought{}})
			i = len(view.People) - 1
			idx[r.id] = i
		}
		view.People[i].Thoughts = append(view.People[i].Thoughts, digestThought{
			ArticleTitle: r.title, ArticleURL: r.url, Thought: r.text, Reactions: r.react, FirstSeen: r.seen,
		})
	}
	return nil
}
