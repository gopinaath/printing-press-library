// Copyright 2026 Som Samantray and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPnUIDFromAuthHeader(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"uid":"abc123","exp":1}`))
	if got, err := pnUIDFromAuthHeader("Bearer h." + payload + ".s"); err != nil || got != "abc123" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "Bearer notajwt", "Bearer a.!!!.c"} {
		if _, err := pnUIDFromAuthHeader(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
	none := base64.RawURLEncoding.EncodeToString([]byte(`{"x":1}`))
	if _, err := pnUIDFromAuthHeader("Bearer h." + none + ".s"); err == nil {
		t.Fatal("expected error for token without uid")
	}
}

const feedItemJSON = `{"article":{"id":28806,"url":"https://e.com/a","title":"Weight loss tech","category":"Health","tags":["obesity","telemetry"],"reading_time":{"minutes":4}},
"conversation_starters":[{"id":26272,"content":"Do we need all this?","insightful_count":2,"star_count":1,"resonate_count":3,"created_at":"2026-09-24T12:59:07Z",
"starter":{"user_id":"u1","name":"Rohan Pai","is_friend":true,"verifications":{"alma_mater":{"entity_name":"XLRI"},"professional":{"entity_name":"Sony"}}}}]}`

func TestPnCardsFromItem(t *testing.T) {
	cards := pnCardsFromItem(mustJSON(t, feedItemJSON))
	if len(cards) != 1 {
		t.Fatalf("want 1 card, got %d", len(cards))
	}
	c := cards[0]
	if c.Key != "28806:26272" || c.StarterName != "Rohan Pai" || !c.StarterFriend || c.Insightful != 6 || c.ReadingMinutes != 4 || c.AlmaMater != "XLRI" || c.Employer != "Sony" || len(c.Tags) != 2 {
		t.Fatalf("unexpected card: %+v", c)
	}
	if got := pnCardsFromItem(mustJSON(t, `{"article":{"id":1}}`)); len(got) != 0 {
		t.Fatalf("item without starters must yield no cards, got %d", len(got))
	}
	if got := pnCardsFromItem(mustJSON(t, `{}`)); got != nil {
		t.Fatal("item without article must yield nil")
	}
}

func TestFillToday(t *testing.T) {
	now := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	prof := mustJSON(t, `{"data":{"timezone":"UTC","streak":{"current":5,"max":9},"reading":{"last_7_days_seconds":1200},"daily_last_7":[{"date":"2026-09-29","seconds":600},{"date":"2026-09-30","seconds":0}]}}`)
	stand := mustJSON(t, `{"data":{"my_rank":2,"total_participants":3,"entries":[{"is_me":true,"name":"Me","thinking_time_seconds":0},{"is_me":false,"name":"Ann","thinking_time_seconds":300},{"is_me":false,"name":"Bob","thinking_time_seconds":0}]}}`)
	think := mustJSON(t, `{"users":[{"name":"Ann","is_friend":true,"presence":{"isOnline":true}},{"name":"Stranger","is_friend":false,"presence":{"isOnline":true}}]}`)
	unread := mustJSON(t, `{"total_unread":4,"unread_notif_count":3,"pending_invitations_count":1}`)
	reqs := mustJSON(t, `{"total_received":2}`)
	var v todayView
	fillToday(&v, prof, stand, think, unread, reqs, now)
	if !v.Streak.AtRisk || v.Reading.TodaySeconds != 0 || v.Streak.Current != 5 {
		t.Fatalf("streak/at-risk wrong: %+v", v.Streak)
	}
	if len(v.Friends.ReadToday) != 1 || v.Friends.ReadToday[0] != "Ann" || len(v.Friends.Online) != 1 || v.Friends.MyRank != 2 {
		t.Fatalf("friends wrong: %+v", v.Friends)
	}
	if v.Unread.Total != 4 || v.PendingFriendRequests != 2 {
		t.Fatalf("unread wrong: %+v %d", v.Unread, v.PendingFriendRequests)
	}
	// Failed calls leave zero values rather than panicking.
	var empty todayView
	fillToday(&empty, nil, nil, nil, nil, nil, now)
	if empty.Streak.AtRisk {
		t.Fatal("no data must not report at-risk")
	}
}

func TestRhythmAggregateZeroWeeksAndHistory(t *testing.T) {
	ctx := context.Background()
	db, err := pnOpenStore(ctx, filepath.Join(t.TempDir(), "t.db"), "u1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) // Wednesday
	var empty rhythmView
	if err := rhythmAggregate(ctx, db.DB(), "u1", 3, now, &empty); err != nil {
		t.Fatal(err)
	}
	if len(empty.Weeks) != 3 || empty.DaysRecorded != 0 {
		t.Fatalf("empty history must still report 3 zero weeks, got %+v", empty)
	}
	for _, d := range []struct {
		day  string
		secs int
	}{{"2026-09-28", 600}, {"2026-09-29", 1800}, {"2026-09-21", 300}, {"2026-09-22", 0}} {
		if _, err := db.DB().Exec(`INSERT INTO pn_reading_days(uid,day,seconds,captured_at) VALUES(?,?,?,?)`, "u1", d.day, d.secs, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DB().Exec(`INSERT INTO pn_snapshots(uid,day,captured_at,streak_current,streak_max,my_rank,participants) VALUES('u1','2026-09-29','x',2,7,1,3)`); err != nil {
		t.Fatal(err)
	}
	var v rhythmView
	if err := rhythmAggregate(ctx, db.DB(), "u1", 3, now, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Weeks) != 3 {
		t.Fatalf("want exactly 3 weeks, got %d", len(v.Weeks))
	}
	this, prev, before := v.Weeks[2], v.Weeks[1], v.Weeks[0]
	if this.WeekStart != "2026-09-28" || this.Minutes != 40 || this.ActiveDays != 2 {
		t.Fatalf("this week wrong: %+v", this)
	}
	if prev.Minutes != 5 || prev.ActiveDays != 1 || before.Minutes != 0 {
		t.Fatalf("earlier weeks wrong: %+v %+v", prev, before)
	}
	if v.LongestStreak != 7 || len(v.RankTrend) != 1 || v.FirstDay != "2026-09-21" || v.DaysRecorded != 4 {
		t.Fatalf("summary wrong: %+v", v)
	}
}

func TestDigestAndVoicesFromStoredCards(t *testing.T) {
	ctx := context.Background()
	db, err := pnOpenStore(ctx, filepath.Join(t.TempDir(), "t.db"), "u1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cards := pnCardsFromItem(mustJSON(t, feedItemJSON))
	other := pnCardsFromItem(mustJSON(t, `{"article":{"id":1,"title":"Cooking","tags":["food"]},"conversation_starters":[{"id":9,"content":"yum","starter":{"user_id":"u2","name":"Stranger","is_friend":false}}]}`))
	if _, err := pnStoreCards(ctx, db.DB(), "u1", append(cards, other...), "forme", time.Now()); err != nil {
		t.Fatal(err)
	}
	// Re-storing the same cards must not duplicate them.
	if _, err := pnStoreCards(ctx, db.DB(), "u1", cards, "forme", time.Now()); err != nil {
		t.Fatal(err)
	}

	var d digestView
	if err := digestBuild(ctx, db.DB(), "u1", time.Now().Add(-time.Hour), false, 10, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.People) != 1 || d.People[0].Name != "Rohan Pai" || len(d.People[0].Thoughts) != 1 {
		t.Fatalf("friend-only digest wrong: %+v", d.People)
	}
	var all digestView
	if err := digestBuild(ctx, db.DB(), "u1", time.Now().Add(-time.Hour), true, 10, &all); err != nil || len(all.People) != 2 {
		t.Fatalf("--all digest wrong: %v %+v", err, all.People)
	}
	var none digestView
	if err := digestBuild(ctx, db.DB(), "u1", time.Now().Add(time.Hour), true, 10, &none); err != nil || len(none.People) != 0 || none.People == nil {
		t.Fatalf("future cutoff must give empty non-nil list: %v %+v", err, none.People)
	}

	var v voicesView
	if err := voicesBuild(ctx, db.DB(), "u1", "obesity", "professional", 10, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Voices) != 1 || v.Voices[0].Employer != "Sony" || v.Voices[0].Thoughts != 1 {
		t.Fatalf("voices wrong: %+v", v.Voices)
	}
	var miss voicesView
	if err := voicesBuild(ctx, db.DB(), "u1", "quantum", "", 10, &miss); err != nil || len(miss.Voices) != 0 {
		t.Fatalf("mismatching topic must return nothing: %v %+v", err, miss.Voices)
	}
	var noBadge voicesView
	if err := voicesBuild(ctx, db.DB(), "u1", "food", "any", 10, &noBadge); err != nil || len(noBadge.Voices) != 0 {
		t.Fatalf("badge filter must exclude uncredentialed people: %v %+v", err, noBadge.Voices)
	}
}

func TestOwedFromReplies(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	replies := []any{
		mustJSON(t, `{"user_id":"me","user_name":"Me","content":"thanks","created_at":"2026-09-30T08:00:00Z"}`),
		mustJSON(t, `{"user_id":"them","user_name":"Ann","content":"but why?","created_at":"2026-09-30T10:00:00Z"}`),
	}
	got, ok := owedFromReplies("7", "3", "Title", "my take", replies, "me", now)
	if !ok || got.LastReplyBy != "Ann" || got.WaitingHours != 2 {
		t.Fatalf("expected owed thread from Ann waiting 2h, got %+v ok=%v", got, ok)
	}
	replies = append(replies, mustJSON(t, `{"user_id":"me","user_name":"Me","content":"because","created_at":"2026-09-30T11:00:00Z"}`))
	if _, ok := owedFromReplies("7", "3", "Title", "my take", replies, "me", now); ok {
		t.Fatal("answered thread must not be owed")
	}
	if _, ok := owedFromReplies("7", "3", "Title", "x", nil, "me", now); ok {
		t.Fatal("thread without replies must not be owed")
	}
}

func TestQueueSelect(t *testing.T) {
	items := []queueItem{
		{Title: "long", URL: "u1", Minutes: 25},
		{Title: "short", URL: "u2", Minutes: 3},
		{Title: "discussed", URL: "u3", Minutes: 8, Discussed: true},
		{Title: "unknown", URL: "u4", Minutes: 0},
		{Title: "dup of short", URL: "u2", Minutes: 3},
	}
	got := queueSelect(items, 10, false, 10)
	if len(got) != 2 || got[0].Title != "discussed" || got[1].Title != "short" {
		t.Fatalf("fits=10 wrong: %+v", got)
	}
	if got := queueSelect(items, 10, true, 10); len(got) != 3 {
		t.Fatalf("include-unknown should add the unknown-length item: %+v", got)
	}
	if got := queueSelect(items, 0, false, 2); len(got) != 2 {
		t.Fatalf("limit not applied: %+v", got)
	}
	if got := queueSelect(nil, 5, false, 5); got == nil || len(got) != 0 {
		t.Fatal("empty input must give an empty non-nil slice")
	}
	parsed := queueItemsFrom([]any{mustJSON(t, `{"article":{"title":"T","url":"u","reading_time":{"minutes":5},"total_thoughts":2}}`)}, "read_later")
	if len(parsed) != 1 || parsed[0].Minutes != 5 || !parsed[0].Discussed {
		t.Fatalf("parse wrong: %+v", parsed)
	}
}

func TestResonanceRank(t *testing.T) {
	list := []any{
		mustJSON(t, `{"conversation_id":1,"content":"a","insights_count":3,"resonate_count":1,"created_at":"2026-09-29T10:00:00Z","article":{"title":"X","tags":["ai"]}}`),
		mustJSON(t, `{"conversation_id":2,"content":"b","star_count":1,"created_at":"2026-09-22T10:00:00Z","article":{"title":"Y","tags":["ai"]}}`),
		mustJSON(t, `{"conversation_id":3,"content":"c","created_at":"2026-09-22T10:00:00Z","article":{"title":"Z","category":"Health"}}`),
	}
	ts := resonanceFrom(list)
	top, groups := resonanceRank(ts, "tag", 2)
	if len(top) != 2 || top[0].ThoughtID != "1" || top[0].Score != 4 {
		t.Fatalf("top wrong: %+v", top)
	}
	if len(groups) != 2 || groups[0].Key != "ai" || groups[0].Score != 5 || groups[0].Thoughts != 2 {
		t.Fatalf("tag groups wrong: %+v", groups)
	}
	_, weeks := resonanceRank(ts, "week", 5)
	if len(weeks) != 2 {
		t.Fatalf("week groups wrong: %+v", weeks)
	}
	top, groups = resonanceRank(nil, "none", 5)
	if len(top) != 0 || len(groups) != 0 {
		t.Fatal("no thoughts must give no rows")
	}
}

func TestPnMapRejectsNonObjectBodies(t *testing.T) {
	for _, body := range []string{"", "null", "[1,2]", "<html>502</html>", `{"error":"boom"}`} {
		if _, err := pnMap(json.RawMessage(body)); err == nil {
			t.Fatalf("body %q must be an error, not an empty success", body)
		}
	}
	if m, err := pnMap(json.RawMessage(`{"success":true,"error":""}`)); err != nil || m == nil {
		t.Fatalf("valid body rejected: %v", err)
	}
}

func TestResonanceEmptyIsNonNilSlice(t *testing.T) {
	top, _ := resonanceRank(nil, "none", 5)
	if top == nil {
		t.Fatal("top must be [] (non-nil) so JSON is [] not null")
	}
	b, _ := json.Marshal(top)
	if string(b) != "[]" {
		t.Fatalf("got %s", b)
	}
}

func TestVoicesTopicWordBoundary(t *testing.T) {
	cases := []struct {
		needle, tags, title, text string
		want                      bool
	}{
		{"ai", "AI|robots", "x", "y", true},
		{"ai", "", "He said it was fine", "maintain training", false},
		{"ai", "", "Why AI hallucinates", "", true},
		{"venture capital", "", "", "a Venture Capital round", true},
		{"ai", "", "AI-powered", "", true},
	}
	for _, c := range cases {
		if got := voicesMatchTopic(c.needle, c.tags, c.title, c.text); got != c.want {
			t.Errorf("%+v: got %v want %v", c, got, c.want)
		}
	}
}

func TestStoredFriendFlagIsNotDowngraded(t *testing.T) {
	ctx := context.Background()
	db, err := pnOpenStore(ctx, filepath.Join(t.TempDir(), "t.db"), "u1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	friend := pnCardsFromItem(mustJSON(t, feedItemJSON))
	notFriend := pnCardsFromItem(mustJSON(t, strings.Replace(feedItemJSON, `"is_friend":true`, `"is_friend":false`, 1)))
	if _, err := pnStoreCards(ctx, db.DB(), "u1", friend, "friends", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := pnStoreCards(ctx, db.DB(), "u1", notFriend, "forme", time.Now()); err != nil {
		t.Fatal(err)
	}
	var d digestView
	if err := digestBuild(ctx, db.DB(), "u1", time.Now().Add(-time.Hour), false, 10, &d); err != nil || len(d.People) != 1 {
		t.Fatalf("friend card was downgraded by a later non-friend sighting: %v %+v", err, d.People)
	}
}

func TestCardsWithoutThoughtIDAreSkipped(t *testing.T) {
	item := mustJSON(t, `{"article":{"id":5,"title":"T"},"conversation_starters":[{"content":"a","starter":{"user_id":"u1","name":"A"}},{"content":"b","starter":{"user_id":"u2","name":"B"}}]}`)
	if got := pnCardsFromItem(item); len(got) != 0 {
		t.Fatalf("cards without ids would collide on one key, got %d", len(got))
	}
}

func TestQueueMergesDuplicates(t *testing.T) {
	items := []queueItem{
		{Title: "A", URL: "u", Source: "read_later", Minutes: 0},
		{Title: "A", URL: "u", Source: "bookmark", Minutes: 6, FriendThoughts: 3, Discussed: true},
	}
	got := queueSelect(items, 10, false, 10)
	if len(got) != 1 || !got[0].Discussed || got[0].Minutes != 6 {
		t.Fatalf("duplicate must keep the richer data: %+v", got)
	}
}

func TestOwedUnparseableOwnReplyAssumesAnswered(t *testing.T) {
	now := time.Now()
	replies := []any{
		mustJSON(t, `{"user_id":"them","user_name":"Ann","content":"?","created_at":"2026-09-30T10:00:00Z"}`),
		mustJSON(t, `{"user_id":"me","user_name":"Me","content":"answer","created_at":"not-a-date"}`),
	}
	if _, ok := owedFromReplies("1", "2", "T", "x", replies, "me", now); ok {
		t.Fatal("an own reply with an unreadable timestamp must not leave the thread flagged as owed")
	}
}

func TestRhythmLongestStreakSpansAllHistory(t *testing.T) {
	ctx := context.Background()
	db, err := pnOpenStore(ctx, filepath.Join(t.TempDir(), "t.db"), "u1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range []struct {
		day string
		max int
	}{{"2026-01-01", 30}, {"2026-09-29", 4}} {
		if _, err := db.DB().Exec(`INSERT INTO pn_snapshots(uid,day,captured_at,streak_current,streak_max) VALUES(?,?,?,?,?)`, "u1", s.day, "x", 1, s.max); err != nil {
			t.Fatal(err)
		}
	}
	// 90 seconds across two days must report 1 minute, not 0 (sum seconds, then divide).
	for _, d := range []string{"2026-09-28", "2026-09-29"} {
		if _, err := db.DB().Exec(`INSERT INTO pn_reading_days(uid,day,seconds,captured_at) VALUES(?,?,?,?)`, "u1", d, 45, "x"); err != nil {
			t.Fatal(err)
		}
	}
	var v rhythmView
	if err := rhythmAggregate(ctx, db.DB(), "u1", 1, time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), &v); err != nil {
		t.Fatal(err)
	}
	if v.LongestStreak != 30 {
		t.Fatalf("longest streak must span all history, got %d", v.LongestStreak)
	}
	if v.Weeks[0].Minutes != 1 {
		t.Fatalf("minutes must be floored once after summing seconds, got %d", v.Weeks[0].Minutes)
	}
}
