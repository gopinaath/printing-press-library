// Copyright 2026 Som Samantray and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/internal/cliutil"
)

type owedThread struct {
	ThoughtID    string `json:"thought_id"`
	ArticleID    string `json:"article_id"`
	ArticleTitle string `json:"article_title,omitempty"`
	MyThought    string `json:"my_thought"`
	LastReplyBy  string `json:"last_reply_by"`
	LastReply    string `json:"last_reply"`
	WaitingSince string `json:"waiting_since"`
	WaitingHours int64  `json:"waiting_hours"`
}

type owedView struct {
	Owed             []owedThread `json:"owed"`
	ScannedThoughts  int          `json:"scanned_thoughts"`
	MaxScan          int          `json:"max_scan"`
	UnreadDiscussion int64        `json:"unread_discussions"`
	FetchFailures    []pnFailure  `json:"fetch_failures,omitempty"`
	Note             string       `json:"note,omitempty"`
}

func newNovelOwedCmd(flags *rootFlags) *cobra.Command {
	var limit, maxScan int
	cmd := &cobra.Command{
		Use:   "owed",
		Short: "Threads where someone replied and you have not answered yet.",
		Long: "Threads where someone replied and you have not answered yet.\n\n" +
			"Scans your most recent thoughts that have replies, opens each thread, and lists those whose newest reply is from someone else. " +
			"Also reports the unread count of your discussions inbox.",
		Example: "  philonet-pp-cli owed --agent",
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "owed")
			}
			if err := pnRequireLive(flags); err != nil {
				return err
			}
			if maxScan < 1 || maxScan > 100 || limit < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--limit must be at least 1 and --max-scan between 1 and 100"))
			}
			if cliutil.IsDogfoodEnv() && maxScan > 5 {
				maxScan = 5
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
			view := owedView{Owed: []owedThread{}, MaxScan: maxScan}

			mine, err := pnGet(ctx, c, "/v1/room/conversationsnew", map[string]string{"page": "1", "limit": strconv.Itoa(maxScan)})
			if err != nil {
				return apiErr(fmt.Errorf("listing your thoughts: %w", err))
			}
			thoughts := pnList(mine["data"])
			view.ScannedThoughts = len(thoughts)

			type job struct {
				id, article, title, text string
			}
			var jobs []job
			for _, t := range thoughts {
				tm, _ := t.(map[string]any)
				if tm == nil || pnInt(pnFirst(tm, "reply_count", "thread_count")) == 0 {
					continue
				}
				art, _ := tm["article"].(map[string]any)
				jobs = append(jobs, job{
					id:      pnStr(pnFirst(tm, "conversation_id", "comment_id", "id")),
					article: pnStr(pnFirst(tm, "article_id", "articleId")),
					title:   pnStr(pnFirst(art, "title", "headline")),
					text:    pnStr(pnFirst(tm, "content", "quote_headline", "quote")),
				})
				if jobs[len(jobs)-1].article == "" && art != nil {
					jobs[len(jobs)-1].article = pnStr(pnFirst(art, "id", "article_id"))
				}
			}

			type result struct {
				job     job
				replies []any
				err     error
			}
			results := make([]result, len(jobs))
			sem := make(chan struct{}, 4)
			var wg sync.WaitGroup
			for i, j := range jobs {
				wg.Add(1)
				go func(i int, j job) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					aid, aerr := strconv.Atoi(j.article)
					pid, perr := strconv.Atoi(j.id)
					if aerr != nil || perr != nil {
						results[i] = result{job: j, err: fmt.Errorf("thought %q has no usable ids", j.id)}
						return
					}
					m, err := pnPost(ctx, c, "/v1/room/subcommentsnewthreadedv2", map[string]any{
						"articleId": aid, "parentCommentId": pid, "limit": 100, "includeParentDetails": false,
					})
					if err != nil {
						results[i] = result{job: j, err: err}
						return
					}
					results[i] = result{job: j, replies: pnList(m["comments"])}
				}(i, j)
			}
			wg.Wait()

			now := time.Now()
			scanned := 0
			for _, r := range results {
				if r.err != nil {
					view.FetchFailures = append(view.FetchFailures, pnFailure{Source: "thread " + r.job.id, Error: r.err.Error()})
					continue
				}
				scanned++
				if t, ok := owedFromReplies(r.job.id, r.job.article, r.job.title, r.job.text, r.replies, uid, now); ok {
					view.Owed = append(view.Owed, t)
				}
			}
			if len(jobs) > 0 && scanned == 0 {
				return apiErr(fmt.Errorf("could not open any of %d thread(s): %s", len(jobs), view.FetchFailures[0].Error))
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d thread fetches failed; checked %d threads\n", len(view.FetchFailures), len(results), scanned)
			}
			sort.SliceStable(view.Owed, func(i, j int) bool { return view.Owed[i].WaitingHours > view.Owed[j].WaitingHours })
			if len(view.Owed) > limit {
				view.Owed = view.Owed[:limit]
			}

			if inbox, ierr := pnPost(ctx, c, "/v1/room/conversation/inboxnew", map[string]any{"limit": 1, "offset": 0, "sort": "recent", "unread": true, "muted": "all"}); ierr == nil {
				view.UnreadDiscussion = pnInt(pnDig(inbox, "counts", "total_unread"))
			} else {
				view.FetchFailures = append(view.FetchFailures, pnFailure{Source: "discussions inbox", Error: ierr.Error()})
			}
			if len(view.Owed) == 0 && len(view.FetchFailures) == 0 {
				view.Note = fmt.Sprintf("nothing owed among your %d most recent thought(s) with replies; raise --max-scan to look further back", view.ScannedThoughts)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			for _, t := range view.Owed {
				fmt.Fprintf(w, "%s replied %dh ago on %q\n    them: %s\n", t.LastReplyBy, t.WaitingHours, pnTrunc(t.ArticleTitle, 60), pnTrunc(t.LastReply, 120))
			}
			if view.Note != "" {
				fmt.Fprintln(w, view.Note)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum owed threads to return")
	cmd.Flags().IntVar(&maxScan, "max-scan", 15, "how many of your recent thoughts to scan for replies")
	return cmd
}

// owedFromReplies decides whether a thread is waiting on me: the newest reply
// must exist and must not be mine.
func owedFromReplies(thoughtID, articleID, title, myText string, replies []any, uid string, now time.Time) (owedThread, bool) {
	var last map[string]any
	var lastAt time.Time
	for _, r := range replies {
		rm, _ := r.(map[string]any)
		if rm == nil {
			continue
		}
		at, err := time.Parse(time.RFC3339, pnStr(rm["created_at"]))
		if err != nil {
			if pnStr(rm["user_id"]) == uid {
				return owedThread{}, false // cannot order my own reply: assume answered
			}
			continue
		}
		if last == nil || at.After(lastAt) {
			last, lastAt = rm, at
		}
	}
	if last == nil || pnStr(last["user_id"]) == uid {
		return owedThread{}, false
	}
	return owedThread{
		ThoughtID:    thoughtID,
		ArticleID:    articleID,
		ArticleTitle: title,
		MyThought:    pnTrunc(myText, 140),
		LastReplyBy:  pnStr(last["user_name"]),
		LastReply:    pnTrunc(pnStr(pnFirst(last, "content", "minimessage")), 200),
		WaitingSince: lastAt.UTC().Format(time.RFC3339),
		WaitingHours: max(int64(now.Sub(lastAt).Hours()), 0),
	}, true
}
