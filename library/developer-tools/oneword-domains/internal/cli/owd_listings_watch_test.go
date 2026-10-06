// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

func TestOwdListingsWatchHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"listings", "watch", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("listings watch --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "watch", "--tld", "--ending-within", "--max-pages", "listings watch --tld co --json"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("listings watch --help missing %q:\n%s", want, out.String())
		}
	}
}

func TestOwdEndingSoonRows(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	listings := []owdListing{
		{Domain: "late.co", Price: "10", EndDate: "2026-06-03T00:00:00.000Z"},
		{Domain: "soon.co", Price: "20", BidCount: 2, EndDate: "2026-06-01T18:00:00.000Z"},
		{Domain: "sooner.co", Price: "30", EndDate: "2026-06-01T06:00:00.000Z"},
		{Domain: "past.co", Price: "40", EndDate: "2026-05-31T06:00:00.000Z"},
		{Domain: "bad.co", Price: "50", EndDate: "never"},
	}
	got := owdEndingSoonRows(listings, 24*time.Hour, now)
	if len(got) != 2 || got[0].Domain != "sooner.co" || got[1].Domain != "soon.co" || got[1].BidCount != 2 {
		t.Fatalf("unexpected ending soon: %+v", got)
	}
	if empty := owdEndingSoonRows(nil, time.Hour, now); empty == nil || len(empty) != 0 {
		t.Fatal("no listings must yield an empty, non-nil slice")
	}
}

func TestOwdFilterGoneByTLD(t *testing.T) {
	gone := []string{"ants.co", "zebra.io", "Apple.CO"}
	if got := owdFilterGoneByTLD(gone, "co"); len(got) != 2 || got[0] != "ants.co" || got[1] != "Apple.CO" {
		t.Fatalf("tld filter: %v", got)
	}
	if got := owdFilterGoneByTLD(gone, ""); len(got) != 3 {
		t.Fatalf("no tld keeps all: %v", got)
	}
	if got := owdFilterGoneByTLD(nil, "co"); got == nil || len(got) != 0 {
		t.Fatal("nil gone must yield an empty, non-nil slice")
	}
}

func TestOwdWatchResultShape(t *testing.T) {
	out := owdWatchResult{CheckedAt: "2026-06-01T00:00:00Z", New: make([]owdNewListing, 0), Changed: make([]owdListingChange, 0), Gone: make([]string, 0), EndingSoon: make([]owdEndingSoon, 0)}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"checked_at"`, `"total":0`, `"new":[]`, `"changed":[]`, `"gone":[]`, `"ending_soon":[]`, `"unchanged":0`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("envelope missing %s: %s", want, b)
		}
	}
	row, _ := json.Marshal(owdNewListing{Domain: "a.co", Type: "auction", Price: "1", BidCount: 2, EndDate: "x"})
	var keys map[string]any
	_ = json.Unmarshal(row, &keys)
	if len(keys) != 5 || keys["bid_count"] == nil || keys["end_date"] == nil || keys["userId"] != nil || keys["bidCount"] != nil {
		t.Fatalf("new rows must be snake_case without userId: %s", row)
	}
}

func TestNovelListingsWatchLive(t *testing.T) {
	end := time.Now().UTC().Add(3 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	row := func(d, price string, bids int) string {
		return fmt.Sprintf(`{"domain":%q,"type":"auction","userId":"u","price":%q,"bidCount":%d,"endDate":%q}`, d, price, bids, end)
	}
	page1 := "[" + row("ants.co", "10", 0) + "," + row("bee.co", "20", 1) + "," + row("ants.co", "10", 0) + "]"
	page2Status := 200
	owdNovelTestServer(t, owdNovelJSONHandler(map[string]func(r *http.Request) (int, string){
		"/api/listings": func(r *http.Request) (int, string) {
			if r.URL.Query().Get("tld") != "co" {
				return 200, `[]`
			}
			switch r.URL.Query().Get("page") {
			case "1":
				return 200, page1
			case "2":
				return page2Status, `[]`
			}
			return 200, `[]`
		},
	}))
	var out owdWatchResult
	errOut, err := owdNovelRunJSON(t, &out, "listings", "watch", "--tld", "co", "--max-pages", "2")
	if err != nil || out.Total != 2 || len(out.New) != 2 || out.Unchanged != 0 || len(out.Gone) != 0 || len(out.EndingSoon) != 2 {
		t.Fatalf("first run: %+v err=%v", out, err)
	}
	if out.New[0].Domain != "ants.co" || out.New[0].BidCount != 0 || !strings.Contains(errOut, "first run") {
		t.Fatalf("new rows: %+v stderr=%q", out.New, errOut)
	}
	if _, err := owdNovelRunJSON(t, &out, "listings", "watch", "--tld", "co", "--max-pages", "2"); err != nil || len(out.New) != 0 || out.Unchanged != 2 || len(out.Gone) != 0 {
		t.Fatalf("second run: %+v err=%v", out, err)
	}
	page1 = "[" + row("bee.co", "25", 3) + "]"
	if _, err := owdNovelRunJSON(t, &out, "listings", "watch", "--tld", "co", "--max-pages", "2"); err != nil || len(out.Changed) != 1 || out.Changed[0].PrevPrice != "20" || strings.Join(out.Gone, ",") != "ants.co" {
		t.Fatalf("changed + gone: %+v err=%v", out, err)
	}
	if _, err := owdNovelRunJSON(t, &out, "listings", "watch", "--tld", "co", "--max-pages", "2"); err != nil || len(out.Gone) != 0 || out.Unchanged != 1 {
		t.Fatalf("gone must be reported once: %+v err=%v", out, err)
	}
	page1 = "[" + row("ants.co", "11", 0) + "," + row("bee.co", "25", 3) + "]"
	if _, err := owdNovelRunJSON(t, &out, "listings", "watch", "--tld", "co", "--max-pages", "2"); err != nil || len(out.New) != 1 || out.New[0].Domain != "ants.co" || len(out.Gone) != 0 {
		t.Fatalf("reappear: %+v err=%v", out, err)
	}
	// A scan that fails after page 1 records the page but never computes gone.
	full := make([]string, 0, owdPageSize)
	for i := 0; i < owdPageSize; i++ {
		full = append(full, row(fmt.Sprintf("w%03d.co", i), "1", 0))
	}
	page1 = "[" + strings.Join(full, ",") + "]"
	page2Status = 404
	errOut, err = owdNovelRunJSON(t, &out, "listings", "watch", "--tld", "co", "--max-pages", "2")
	if err != nil || out.Total != owdPageSize || len(out.New) != owdPageSize || len(out.Gone) != 0 || !strings.Contains(errOut, "not computed from a partial scan") {
		t.Fatalf("partial scan: total=%d new=%d gone=%v err=%v stderr=%q", out.Total, len(out.New), out.Gone, err, errOut)
	}
	for _, args := range [][]string{
		{"listings", "watch", "--data-source", "local", "--json"},
		{"listings", "watch", "--ending-within", "soon", "--json"},
		{"listings", "watch", "--max-pages", "0", "--json"},
		{"listings", "watch", "extra", "--json"},
	} {
		if _, _, err := owdNovelRun(t, args...); ExitCode(err) != 2 {
			t.Fatalf("%v: want usage error, got %v", args, err)
		}
	}
}

func TestNovelListingsWatchPageCapIsNotComplete(t *testing.T) {
	end := time.Now().UTC().Add(3 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	page := func(n int, prefix string) string {
		rows := make([]string, 0, n)
		for i := 0; i < n; i++ {
			rows = append(rows, fmt.Sprintf(`{"domain":"%s%03d.co","type":"auction","userId":"u","price":"1","bidCount":0,"endDate":%q}`, prefix, i, end))
		}
		return "[" + strings.Join(rows, ",") + "]"
	}
	owdNovelTestServer(t, owdNovelJSONHandler(map[string]func(r *http.Request) (int, string){
		"/api/listings": func(r *http.Request) (int, string) {
			switch r.URL.Query().Get("page") {
			case "1":
				return 200, page(owdPageSize, "a")
			case "2":
				return 200, page(3, "b")
			}
			return 200, `[]`
		},
	}))
	seenRows := func() int {
		db := owdNovelTestStore(t)
		defer db.Close()
		var n int
		_ = db.DB().QueryRow(`SELECT COUNT(*) FROM owd_listing_seen`).Scan(&n)
		return n
	}
	var out owdWatchResult
	if _, err := owdNovelRunJSON(t, &out, "listings", "watch", "--max-pages", "5"); err != nil || out.Total != owdPageSize+3 || len(out.New) != owdPageSize+3 {
		t.Fatalf("full scan: total=%d new=%d err=%v", out.Total, len(out.New), err)
	}
	// A page-capped scan that ended on a full page is not complete: nothing
	// is reported gone, nothing is deleted, and stderr says why.
	errOut, err := owdNovelRunJSON(t, &out, "listings", "watch", "--max-pages", "1")
	if err != nil || out.Total != owdPageSize || len(out.Gone) != 0 || len(out.New) != 0 || out.Unchanged != owdPageSize || seenRows() != owdPageSize+3 || !strings.Contains(errOut, "--max-pages cap") || !strings.Contains(errOut, "not computed from a partial scan") {
		t.Fatalf("capped scan: total=%d new=%d gone=%v unchanged=%d rows=%d err=%v stderr=%q", out.Total, len(out.New), out.Gone, out.Unchanged, seenRows(), err, errOut)
	}
	if _, err := owdNovelRunJSON(t, &out, "listings", "watch", "--max-pages", "5"); err != nil || len(out.New) != 0 || len(out.Gone) != 0 || out.Unchanged != owdPageSize+3 {
		t.Fatalf("the next full scan sees nothing new or gone: new=%d gone=%v unchanged=%d err=%v", len(out.New), out.Gone, out.Unchanged, err)
	}
}
