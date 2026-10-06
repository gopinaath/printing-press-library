// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

func TestNovelListingsRankHelpWires(t *testing.T) {
	testenv.Isolate(t)
	out, _, err := owdNovelRun(t, "listings", "rank", "--help")
	if err != nil {
		t.Fatalf("listings rank --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "rank", "use 'listings watch' instead", "--ending", "--max-bids", "--max-checks", "--max-pages"} {
		if !strings.Contains(out, want) {
			t.Fatalf("listings rank --help missing %q in output:\n%s", want, out)
		}
	}
}

func owdRankFixtureServer(t *testing.T, now time.Time) {
	t.Helper()
	end := func(h float64) string {
		return now.Add(time.Duration(h * float64(time.Hour))).Format("2006-01-02T15:04:05.000Z")
	}
	listings := fmt.Sprintf(`[{"domain":"ants.co","type":"auction","userId":"u","price":"350","bidCount":0,"endDate":%q},{"domain":"bee.co","type":"auction","userId":"u","price":"20","bidCount":4,"endDate":%q},{"domain":"cat.co","type":"auction","userId":"u","price":"100","bidCount":1,"endDate":%q},{"domain":"dog.io","type":"auction","userId":"u","price":"5","bidCount":0,"endDate":%q}]`, end(10), end(2), end(50), end(1))
	counts := map[string]int{"ants.co": 80, "bee.co": 10, "cat.co": 40, "dog.io": 90}
	owdNovelTestServer(t, owdNovelJSONHandler(map[string]func(r *http.Request) (int, string){
		"/api/listings": func(r *http.Request) (int, string) {
			if r.URL.Query().Get("page") != "1" {
				return 200, `[]`
			}
			return 200, listings
		},
		"/api/tlds": func(r *http.Request) (int, string) {
			return 200, `[{"slug":"co","type":"ccTld","structure":"normal","top10m":1,"totalReg":2,"views":3,"minPrice":"10"},{"slug":"io","type":"ccTld","structure":"normal","top10m":1,"totalReg":2,"views":3,"minPrice":"30"}]`
		},
		"/api/domains/": func(r *http.Request) (int, string) {
			d := strings.TrimPrefix(r.URL.Path, "/api/domains/")
			n, ok := counts[d]
			if !ok {
				return 404, `{}`
			}
			return 200, fmt.Sprintf(`{"slug":%q,"available":false,"premium":false,"price":null,"tldSlug":"co","aftermarket":true,"tldCount":%d}`, d, n)
		},
	}))
}

func owdRankDomains(rows []owdRankRow) string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Domain)
	}
	return strings.Join(out, ",")
}

func TestNovelListingsRankScoresAndSorts(t *testing.T) {
	now := time.Now().UTC()
	owdRankFixtureServer(t, now)

	var out owdRankOutput
	if _, err := owdNovelRunJSON(t, &out, "listings", "rank", "--tld", "co"); err != nil {
		t.Fatal(err)
	}
	if out.ScannedListings != 4 || out.Matched != 3 || out.Checked != 3 || len(out.FetchFailures) != 0 {
		t.Fatalf("accounting: %+v", out)
	}
	if got := owdRankDomains(out.Rows); got != "bee.co,cat.co,ants.co" {
		t.Fatalf("popularity order: %s", got)
	}
	bee := out.Rows[0]
	if bee.TakenOf93 == nil || *bee.TakenOf93 != 83 || bee.PriceX == nil || *bee.PriceX != 2 || bee.BidsPerDay != 4 || !bee.Checked || bee.MinPrice != "10" || bee.PopularityPct == nil {
		t.Fatalf("bee scoring: %+v", bee)
	}
	if bee.HoursLeft == nil || *bee.HoursLeft < 1.5 || *bee.HoursLeft > 2.5 {
		t.Fatalf("hours_left: %+v", bee.HoursLeft)
	}

	// Snapshots are now stored, so the next runs need no domain calls.
	if _, err := owdNovelRunJSON(t, &out, "listings", "rank", "--tld", "co", "--sort", "ending"); err != nil {
		t.Fatal(err)
	}
	if got := owdRankDomains(out.Rows); got != "bee.co,ants.co,cat.co" {
		t.Fatalf("ending order: %s", got)
	}
	for i := 1; i < len(out.Rows); i++ {
		if *out.Rows[i-1].HoursLeft > *out.Rows[i].HoursLeft {
			t.Fatalf("hours_left must ascend: %+v", out.Rows)
		}
	}
	if _, err := owdNovelRunJSON(t, &out, "listings", "rank", "--tld", "co", "--sort", "price-x"); err != nil || owdRankDomains(out.Rows) != "bee.co,cat.co,ants.co" {
		t.Fatalf("price-x order: %s err=%v", owdRankDomains(out.Rows), err)
	}
	if _, err := owdNovelRunJSON(t, &out, "listings", "rank", "--tld", "co", "--sort", "bids-per-day", "--max-bids", "1"); err != nil || owdRankDomains(out.Rows) != "cat.co,ants.co" {
		t.Fatalf("max-bids + bids-per-day: %s err=%v", owdRankDomains(out.Rows), err)
	}
	if _, err := owdNovelRunJSON(t, &out, "listings", "rank", "--tld", "co", "--ending", "24h", "--sort", "ending"); err != nil || owdRankDomains(out.Rows) != "bee.co,ants.co" {
		t.Fatalf("ending window: %s err=%v", owdRankDomains(out.Rows), err)
	}
	if _, err := owdNovelRunJSON(t, &out, "listings", "rank", "--tld", "co", "--limit", "1"); err != nil || len(out.Rows) != 1 || out.Rows[0].Domain != "bee.co" {
		t.Fatalf("limit: %+v err=%v", out.Rows, err)
	}
	if _, err := owdNovelRunJSON(t, &out, "listings", "rank", "--tld", "io"); err != nil || owdRankDomains(out.Rows) != "dog.io" || out.Rows[0].PriceX == nil || *out.Rows[0].PriceX != 0.17 {
		t.Fatalf("io listing: %+v err=%v", out.Rows, err)
	}
	if _, _, err := owdNovelRun(t, "listings", "rank", "--sort", "weird", "--json"); ExitCode(err) != 2 {
		t.Fatalf("bad sort must be a usage error: %v", err)
	}
	var env dryRunResult
	if _, err := owdNovelRunJSON(t, &env, "listings", "rank", "--tld", "co", "--dry-run"); err != nil || !env.DryRun || env.Action != "listings rank" {
		t.Fatalf("dry-run: %+v err=%v", env, err)
	}
}

func TestNovelListingsRankMaxChecksNote(t *testing.T) {
	now := time.Now().UTC()
	owdRankFixtureServer(t, now)
	var out owdRankOutput
	if _, err := owdNovelRunJSON(t, &out, "listings", "rank", "--tld", "co", "--max-checks", "1", "--sort", "ending"); err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 1 || out.Rows[0].Domain != "bee.co" || out.Matched != 3 || !strings.Contains(out.Note, "--max-checks") {
		t.Fatalf("cap must keep the soonest-ending listing and explain itself: %+v", out)
	}
}

func TestNovelListingsRankLocalSource(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now().UTC()
	db := owdNovelTestStore(t)
	end := now.Add(3 * time.Hour).Format(time.RFC3339)
	items := []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`{"domain":"ants.co","type":"auction","userId":"u","price":"350","bidCount":2,"endDate":%q}`, end)),
		json.RawMessage(fmt.Sprintf(`{"domain":"bee.co","type":"auction","userId":"u","price":"20","bidCount":0,"endDate":%q}`, end)),
	}
	if _, _, err := db.UpsertBatch("listings", items); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO owd_domain_checks (word, tld, domain, available, premium, price, aftermarket, tld_count, checked_at) VALUES ('ants','co','ants.co',0,0,NULL,1,80,?)`, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	var out owdRankOutput
	if _, err := owdNovelRunJSON(t, &out, "listings", "rank", "--tld", "co", "--data-source", "local"); err != nil {
		t.Fatal(err)
	}
	if out.ScannedListings != 2 || out.Checked != 1 || len(out.Rows) != 2 || out.Rows[0].Domain != "ants.co" || out.Rows[0].TakenOf93 == nil || *out.Rows[0].TakenOf93 != 13 || out.Rows[1].TakenOf93 != nil || out.Rows[1].Checked {
		t.Fatalf("local ranking: %+v", out)
	}
}

func TestOwdRankHelpers(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	in := []owdListing{
		{Domain: "a.co", Price: "10", BidCount: 3, EndDate: "2026-09-24T14:00:00.000Z"},
		{Domain: "b.io", Price: "10", BidCount: 0, EndDate: "2026-09-25T14:00:00.000Z"},
		{Domain: "c.co", Price: "x", BidCount: 1, EndDate: "garbage"},
	}
	if got := owdRankFilter(in, "co", 0, nil, now); len(got) != 2 {
		t.Fatalf("tld filter: %+v", got)
	}
	if got := owdRankFilter(in, "", 3*time.Hour, nil, now); len(got) != 1 || got[0].Domain != "a.co" {
		t.Fatalf("ending filter: %+v", got)
	}
	one := 1
	if got := owdRankFilter(in, "", 0, &one, now); len(got) != 2 || got[0].Domain != "b.io" {
		t.Fatalf("max-bids filter: %+v", got)
	}
	if h := owdRankHoursLeft("2026-09-24T14:00:00.000Z", now); h == nil || *h != 2 {
		t.Fatalf("hours left: %v", h)
	}
	if owdRankHoursLeft("garbage", now) != nil {
		t.Fatal("unparseable end date is null")
	}
	threeDays, oneHour := now.Add(-72*time.Hour), now.Add(-time.Hour)
	if owdRankBidsPerDay(6, &threeDays, now) != 2 || owdRankBidsPerDay(6, &oneHour, now) != 6 || owdRankBidsPerDay(6, nil, now) != 6 {
		t.Fatal("bids per day floors the window at one day")
	}
	hundred, twoDays := 100, now.Add(-48*time.Hour)
	row := owdRankScore(in[0], &hundred, map[string]owdTLD{"co": {Slug: "co", MinPrice: "5"}}, &twoDays, now)
	if row.TakenOf93 == nil || *row.TakenOf93 != 0 || row.PriceX == nil || *row.PriceX != 2 || row.BidsPerDay != 1.5 || row.TLD != "co" {
		t.Fatalf("score clamps and ratios: %+v", row)
	}
	row = owdRankScore(in[2], nil, nil, nil, now)
	if row.TakenOf93 != nil || row.PriceX != nil || row.HoursLeft != nil || row.PopularityPct != nil || row.Checked {
		t.Fatalf("unknown values stay null: %+v", row)
	}

	taken := func(n int) *int { return &n }
	rows := []owdRankRow{
		{Domain: "b", TakenOf93: taken(50), PriceX: owdFloatPtr(3), BidsPerDay: 1, HoursLeft: owdFloatPtr(5)},
		{Domain: "a", TakenOf93: taken(50), PriceX: owdFloatPtr(1), BidsPerDay: 0, HoursLeft: owdFloatPtr(9)},
		{Domain: "c", BidsPerDay: 7, HoursLeft: nil},
		{Domain: "d", TakenOf93: taken(90), BidsPerDay: 2, HoursLeft: owdFloatPtr(1)},
	}
	cases := map[string]string{"popularity": "d,a,b,c", "price-x": "a,b,c,d", "bids-per-day": "c,d,b,a", "ending": "d,b,a,c"}
	for key, want := range cases {
		r := append([]owdRankRow{}, rows...)
		owdRankSort(r, key)
		if got := owdRankDomains(r); got != want {
			t.Fatalf("sort %s: got %s want %s", key, got, want)
		}
	}
	pre := append([]owdListing{}, in...)
	owdRankPreorder(pre, "ending", now)
	if pre[0].Domain != "a.co" || pre[2].Domain != "c.co" {
		t.Fatalf("preorder ending: %+v", pre)
	}
	owdRankPreorder(pre, "bids-per-day", now)
	if pre[0].Domain != "a.co" || pre[1].Domain != "c.co" {
		t.Fatalf("preorder bids: %+v", pre)
	}
}
