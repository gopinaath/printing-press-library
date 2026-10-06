// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/client"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/config"
	"github.com/spf13/cobra"
)

// owdTestClient returns a generated client pointed at srv with caching off.
func owdTestClient(t *testing.T, srv *httptest.Server) *client.Client {
	t.Helper()
	testenv.Isolate(t)
	c := client.New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.NoCache = true
	return c
}

// owdFailIfHit is an httptest server that fails the test on any request.
func owdFailIfHit(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP request: %s %s", r.Method, r.URL.String())
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOwdSplitDomain(t *testing.T) {
	cases := []struct {
		in, word, tld string
		wantErr       bool
	}{
		{"smart.com", "smart", "com", false},
		{" Oasis.AI ", "oasis", "ai", false},
		{"smart", "", "", true},
		{".com", "", "", true},
		{"smart.", "", "", true},
	}
	for _, c := range cases {
		w, tld, err := owdSplitDomain(c.in)
		if (err != nil) != c.wantErr {
			t.Fatalf("%q: err=%v wantErr=%v", c.in, err, c.wantErr)
		}
		if w != c.word || tld != c.tld {
			t.Fatalf("%q: got %q/%q want %q/%q", c.in, w, tld, c.word, c.tld)
		}
	}
}

func TestOwdValidateInputs(t *testing.T) {
	for _, w := range []string{"smart", "sm-art", "x1"} {
		if err := owdValidateWord(w); err != nil {
			t.Fatalf("word %q must pass: %v", w, err)
		}
	}
	for _, w := range []string{"", "Smart", "sm art", "smart.com", "sm/art", "sm?art", "sm#art", "..", "sm..art"} {
		if err := owdValidateWord(w); ExitCode(err) != 2 || !strings.Contains(err.Error(), fmt.Sprintf("%q", w)) {
			t.Fatalf("word %q must be a usage error naming it: %v", w, err)
		}
	}
	for _, tld := range []string{"com", "co.uk", "com.au", "x-y"} {
		if err := owdValidateTLD(tld); err != nil {
			t.Fatalf("tld %q must pass: %v", tld, err)
		}
	}
	for _, tld := range []string{"", ".", "..", "co/uk", "co?x", "co#x", "co..uk", ".com", "COM"} {
		if err := owdValidateTLD(tld); ExitCode(err) != 2 {
			t.Fatalf("tld %q must be a usage error: %v", tld, err)
		}
	}
	for _, d := range []string{"smart.com", "smart.co.uk"} {
		if err := owdValidateDomain(d); err != nil {
			t.Fatalf("domain %q must pass: %v", d, err)
		}
	}
	for _, d := range []string{"smart", "smart/x.com", "smart?x.com", "smart#x.com", "smart/../x.com", "smart..com", "..", "sm art.com"} {
		if err := owdValidateDomain(d); ExitCode(err) != 2 || !strings.Contains(err.Error(), d) {
			t.Fatalf("domain %q must be a usage error naming it: %v", d, err)
		}
	}
}

func TestOwdCheckDomainRejectsUnsafeInputBeforeAnyRequest(t *testing.T) {
	c := owdTestClient(t, owdFailIfHit(t))
	for _, d := range []string{"smart/x.com", "smart?x.com", "smart#x.com", "smart/../x.com", "..", "smart..com", "smart", "Smart.COM/", "sm art.com"} {
		_, err := owdCheckDomain(context.Background(), c, d)
		if ExitCode(err) != 2 || !strings.Contains(err.Error(), strings.ToLower(strings.TrimSpace(d))) {
			t.Fatalf("%q: want usage error naming the value, got %v", d, err)
		}
	}
	for _, tld := range []string{"co/uk", "co?x", "co#x", "..", "co..uk", ""} {
		_, err := owdFetchTLDDetail(context.Background(), c, tld)
		if ExitCode(err) != 2 {
			t.Fatalf("tld %q: want usage error, got %v", tld, err)
		}
	}
}

func TestOwdCheckDomainUnknownWordIsNotFound(t *testing.T) {
	paths := make([]string, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/domains/zzqq.com":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal Server Error"))
		case "/api/domains/locked.com":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		case "/api/domains/smart.co.uk":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"slug":"smart.co.uk","available":true,"premium":false,"price":null,"tldSlug":"co.uk","aftermarket":false,"tldCount":4}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := owdTestClient(t, srv)
	// The generated client retries 5xx with backoff; the dogfood switch turns
	// that off (it still hits the network) so the 500 surfaces at once.
	t.Setenv(cliutil.DogfoodEnvVar, "1")
	_, err := owdCheckDomain(context.Background(), c, "zzqq.com")
	if ExitCode(err) != 3 || !strings.Contains(err.Error(), `"zzqq.com" is not in the One Word Domains dictionary`) {
		t.Fatalf("500 must become the not-found error (exit 3): %v", err)
	}
	// A 401 comes back raw from the route helper; the fan-out maps it to the
	// session error once and records nothing.
	var snap owdSnapshotErrs
	res, errs, err := owdChecksByDomain(context.Background(), c, nil, []string{"smart.co.uk", "locked.com"}, 2, time.Now(), &snap)
	if ExitCode(err) != 4 || !strings.Contains(err.Error(), "GET /api/domains/locked.com needs a signed-in") || res != nil || errs != nil {
		t.Fatalf("401 through the fan-out must be the session error: %v", err)
	}
	d, err := owdCheckDomain(context.Background(), c, " Smart.CO.UK ")
	if err != nil || d == nil || !d.Available || d.TldCount != 4 {
		t.Fatalf("two-label TLDs must reach the site: %+v err=%v", d, err)
	}
	if len(paths) == 0 || paths[len(paths)-1] != "/api/domains/smart.co.uk" {
		t.Fatalf("path must be the escaped lowercase domain: %v", paths)
	}
}

func TestOwdFetchListingsStopsOnShortPage(t *testing.T) {
	pages := make([]string, 0)
	rows := func(n int, prefix string) string {
		out := make([]string, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, fmt.Sprintf(`{"domain":"%s%03d.co","type":"auction","userId":"u","price":"1","bidCount":0,"endDate":"2026-06-02T00:00:00.000Z"}`, prefix, i))
		}
		return "[" + strings.Join(out, ",") + "]"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "1":
			_, _ = w.Write([]byte(rows(owdPageSize, "a")))
		case "2":
			_, _ = w.Write([]byte(rows(3, "b")))
		default:
			t.Errorf("page %s must not be requested after a short page", page)
			_, _ = w.Write([]byte(rows(owdPageSize, "c")))
		}
	}))
	t.Cleanup(srv.Close)
	c := owdTestClient(t, srv)
	all, n, capped, err := owdFetchListings(context.Background(), c, map[string]string{"tld": "co", "empty": ""}, 10)
	if err != nil || n != 2 || capped || len(all) != owdPageSize+3 || strings.Join(pages, ",") != "1,2" {
		t.Fatalf("short page must stop the scan and is not capped: n=%d capped=%v rows=%d pages=%v err=%v", n, capped, len(all), pages, err)
	}
	pages = pages[:0]
	all, n, capped, err = owdFetchListings(context.Background(), c, map[string]string{"tld": "co"}, 1)
	if err != nil || n != 1 || !capped || len(all) != owdPageSize || strings.Join(pages, ",") != "1" {
		t.Fatalf("max-pages must cap a full page and say so: n=%d capped=%v rows=%d pages=%v err=%v", n, capped, len(all), pages, err)
	}
	pages = pages[:0]
	all, n, capped, err = owdFetchListings(context.Background(), c, map[string]string{"tld": "co"}, 2)
	if err != nil || n != 2 || capped || len(all) != owdPageSize+3 {
		t.Fatalf("a short page exactly at the cap is complete: n=%d capped=%v rows=%d err=%v", n, capped, len(all), err)
	}
}

func TestOwdSuggestForPrefixFallback(t *testing.T) {
	queries := make([]string, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		queries = append(queries, q)
		w.Header().Set("Content-Type", "application/json")
		if q == "sma" {
			_, _ = w.Write([]byte(`[{"slug":"smart"},{"slug":"small"},{"slug":"smash"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(srv.Close)
	c := owdTestClient(t, srv)
	got := owdSuggestFor(context.Background(), c, "smartly", 2)
	if strings.Join(got, ",") != "smart,small" || strings.Join(queries, ",") != "smartly,sma" {
		t.Fatalf("prefix fallback: got %v queries %v", got, queries)
	}
	queries = queries[:0]
	got = owdSuggestFor(context.Background(), c, "zzq", 5)
	if got == nil || len(got) != 0 || strings.Join(queries, ",") != "zzq" {
		t.Fatalf("short words get no fallback and an empty non-nil slice: got %v queries %v", got, queries)
	}
}

func TestOwdDictPrecheckBatchesLocalAndReusesSlugs(t *testing.T) {
	var mu sync.Mutex
	queries := make([]string, 0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		mu.Lock()
		queries = append(queries, q)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch q {
		case "open":
			_, _ = w.Write([]byte(`[{"slug":"open"}]`))
		case "smartx":
			_, _ = w.Write([]byte(`[{"slug":"smartxy"},{"slug":"smartxz"}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)
	c := owdTestClient(t, srv)
	ctx := context.Background()
	db := owdNovelTestStore(t)
	defer db.Close()
	// More local words than one IN (...) chunk holds.
	local := make([]json.RawMessage, 0, owdLookupChunk+2)
	words := make([]string, 0, owdLookupChunk+4)
	for i := 0; i < owdLookupChunk+2; i++ {
		w := fmt.Sprintf("word%d", i)
		local = append(local, json.RawMessage(fmt.Sprintf(`{"slug":%q}`, w)))
		words = append(words, w)
	}
	if _, _, err := db.UpsertBatch("words", local); err != nil {
		t.Fatal(err)
	}
	if got := owdLocalWords(ctx, db, words); len(got) != len(words) {
		t.Fatalf("chunked local lookup found %d of %d", len(got), len(words))
	}
	words = append(words, "open", "smartx")
	res, err := owdDictPrecheck(ctx, c, db, words, 4)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(queries)
	if strings.Join(queries, ",") != "open,smartx" {
		t.Fatalf("only local misses may go live: %v", queries)
	}
	if !res.InDict["word0"] || !res.InDict["open"] || res.InDict["smartx"] || len(res.Failed) != 0 {
		t.Fatalf("pre-check result: %+v", res)
	}
	queries = queries[:0]
	if got := res.suggest(ctx, c, "smartx", 1); strings.Join(got, ",") != "smartxy" || len(queries) != 0 {
		t.Fatalf("suggestions must reuse the pre-check answer: %v queries %v", got, queries)
	}
	if got := res.suggest(ctx, c, "zzq", 5); got == nil || len(got) != 0 || strings.Join(queries, ",") != "zzq" {
		t.Fatalf("a word the pre-check never fetched falls back to a live query: %v %v", got, queries)
	}
}

func TestOwdRealWordsMemoizesLiveLookups(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"slug":"` + r.URL.Query().Get("query") + `"}]`))
	}))
	t.Cleanup(srv.Close)
	c := owdTestClient(t, srv)
	ctx := context.Background()
	rw := owdNewRealWords(ctx, c, nil, []string{"lumix", "lumix"}, false, 1)
	for i := 0; i < 2; i++ {
		if got := rw.lookup(ctx, "Lumix"); got == nil || !*got {
			t.Fatalf("lookup %d: %v", i, got)
		}
	}
	if hits != 1 || rw.budget != 0 {
		t.Fatalf("a repeated name must cost one live lookup: hits=%d budget=%d", hits, rw.budget)
	}
	if got := rw.lookup(ctx, "other"); got != nil || hits != 1 {
		t.Fatalf("an exhausted budget leaves the flag unknown: %v hits=%d", got, hits)
	}
}

func TestOwdRecentChecksBatch(t *testing.T) {
	testenv.Isolate(t)
	ctx := context.Background()
	db := owdNovelTestStore(t)
	defer db.Close()
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	price := "499"
	for i, dc := range []owdDomainCheck{
		{Slug: "smart.com", TldCount: 5},
		{Slug: "smart.com", TldCount: 6, Available: true, Price: &price},
		{Slug: "smart.com", TldCount: 7}, // same checked_at as the row above: the later id wins
		{Slug: "open.ai", TldCount: 2, Aftermarket: true},
	} {
		at := t0.Add(time.Duration(min(i, 1)) * time.Hour)
		var snap owdSnapshotErrs
		owdRecordChecks(db, []cliutil.FanoutResult[*owdDomainCheck]{{Source: dc.Slug, Value: &dc}}, at, &snap)
		if snap.n != 0 {
			t.Fatalf("recording check %d failed", i)
		}
	}
	checks, times, err := owdRecentChecks(ctx, db, []string{"smart.com", "open.ai", "smart.com", "none.io"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	sc := checks["smart.com"]
	if len(sc) != 2 || sc[0].TldCount != 7 || sc[1].TldCount != 6 || !sc[1].Available || sc[1].Price == nil || *sc[1].Price != "499" || sc[0].TldSlug != "com" {
		t.Fatalf("smart.com newest-first, capped at 2: %+v", sc)
	}
	if !times["smart.com"][0].Equal(t0.Add(time.Hour)) || len(checks["open.ai"]) != 1 || !checks["open.ai"][0].Aftermarket {
		t.Fatalf("times/open.ai: %v %+v", times, checks["open.ai"])
	}
	if _, ok := checks["none.io"]; ok {
		t.Fatal("a domain without snapshots must be absent")
	}
	latest := owdLatestChecks(ctx, db, []string{"smart.com", "none.io"})
	if len(latest) != 1 || latest["smart.com"].TldCount != 7 {
		t.Fatalf("latest: %+v", latest)
	}
}

func TestOwdRecordListingsLifecycle(t *testing.T) {
	testenv.Isolate(t)
	db := owdNovelTestStore(t)
	defer db.Close()
	ctx := context.Background()
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	l := func(d, price string, bids int) owdListing {
		return owdListing{Domain: d, Type: "auction", UserID: "u", Price: price, BidCount: bids, EndDate: "2026-06-02T00:00:00.000Z"}
	}
	count := func() int {
		var n int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM owd_listing_seen`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// New, with a duplicate page row counted once.
	diff, err := owdRecordListings(ctx, db, []owdListing{l("ants.co", "10", 0), l("bee.co", "20", 1), l("ants.co", "10", 0), l("dog.io", "5", 0)}, "co", at, true)
	if err != nil || len(diff.New) != 3 || diff.Unchanged != 0 || len(diff.Changed) != 0 || len(diff.Gone) != 0 || count() != 3 {
		t.Fatalf("first run: %+v err=%v rows=%d", diff, err, count())
	}
	if diff.New[0].Domain != "ants.co" || diff.New[0].Type != "auction" || diff.New[0].Price != "10" || diff.New[0].EndDate == "" {
		t.Fatalf("new row shape: %+v", diff.New[0])
	}
	b, _ := json.Marshal(diff.New[0])
	if string(b) != `{"domain":"ants.co","type":"auction","price":"10","bid_count":0,"end_date":"2026-06-02T00:00:00.000Z"}` {
		t.Fatalf("new rows must be snake_case without userId: %s", b)
	}

	// Unchanged.
	diff, err = owdRecordListings(ctx, db, []owdListing{l("ants.co", "10", 0), l("bee.co", "20", 1), l("dog.io", "5", 0)}, "co", at.Add(time.Hour), true)
	if err != nil || len(diff.New) != 0 || diff.Unchanged != 3 || len(diff.Changed) != 0 || len(diff.Gone) != 0 {
		t.Fatalf("second run: %+v err=%v", diff, err)
	}

	// Changed.
	diff, err = owdRecordListings(ctx, db, []owdListing{l("ants.co", "10", 0), l("bee.co", "25", 2), l("dog.io", "5", 0)}, "co", at.Add(2*time.Hour), true)
	if err != nil || len(diff.Changed) != 1 || diff.Unchanged != 2 || len(diff.Gone) != 0 {
		t.Fatalf("third run: %+v err=%v", diff, err)
	}
	if ch := diff.Changed[0]; ch.Domain != "bee.co" || ch.Price != "25" || ch.PrevPrice != "20" || ch.BidCount != 2 || ch.PrevBidCount != 1 {
		t.Fatalf("change row: %+v", ch)
	}

	// Gone under the watched TLD only: ants.co vanished; dog.io is not under .co.
	diff, err = owdRecordListings(ctx, db, []owdListing{l("bee.co", "25", 2)}, "co", at.Add(3*time.Hour), true)
	if err != nil || strings.Join(diff.Gone, ",") != "ants.co" || diff.Unchanged != 1 || count() != 2 {
		t.Fatalf("gone run: %+v err=%v rows=%d", diff, err, count())
	}
	// Reported once: the next run under .co sees nothing gone.
	diff, err = owdRecordListings(ctx, db, []owdListing{l("bee.co", "25", 2)}, "co", at.Add(4*time.Hour), true)
	if err != nil || len(diff.Gone) != 0 || diff.Unchanged != 1 {
		t.Fatalf("gone must not repeat: %+v err=%v", diff, err)
	}
	// Reappear: new again.
	diff, err = owdRecordListings(ctx, db, []owdListing{l("ants.co", "12", 0), l("bee.co", "25", 2)}, "co", at.Add(5*time.Hour), true)
	if err != nil || len(diff.New) != 1 || diff.New[0].Domain != "ants.co" || len(diff.Gone) != 0 || count() != 3 {
		t.Fatalf("reappear run: %+v err=%v rows=%d", diff, err, count())
	}
	// A partial scan records what it fetched but never computes gone.
	diff, err = owdRecordListings(ctx, db, []owdListing{l("bee.co", "25", 2)}, "", at.Add(6*time.Hour), false)
	if err != nil || len(diff.Gone) != 0 || diff.Unchanged != 1 || count() != 3 {
		t.Fatalf("partial run: %+v err=%v rows=%d", diff, err, count())
	}
	// A complete scan with no TLD filter reports every vanished row.
	diff, err = owdRecordListings(ctx, db, []owdListing{l("bee.co", "25", 2)}, "", at.Add(7*time.Hour), true)
	if err != nil || strings.Join(diff.Gone, ",") != "ants.co,dog.io" || count() != 1 {
		t.Fatalf("all-tld gone: %+v err=%v rows=%d", diff, err, count())
	}
}

func TestOwdReadWordLinesAndPairs(t *testing.T) {
	lines, err := owdReadWordLines(strings.NewReader("smart\n\n# comment\noasis\nsmart\nopen.com\nzen.co.uk\nopen.com\n"), []string{"com", "co.uk"})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(lines))
	for _, l := range lines {
		got = append(got, l.Word+"|"+l.TLD)
	}
	if strings.Join(got, ",") != "smart|,oasis|,open|com,zen|co.uk" {
		t.Fatalf("lines: %v", got)
	}
	pairs := owdWordPairs(lines, []string{"com", "ai"})
	domains := make([]string, 0, len(pairs))
	for _, p := range pairs {
		domains = append(domains, p.Domain)
	}
	if strings.Join(domains, ",") != "smart.com,smart.ai,oasis.com,oasis.ai,open.com,zen.co.uk" || pairs[5].Word != "zen" || pairs[5].TLD != "co.uk" {
		t.Fatalf("pairs: %v", domains)
	}
	for _, bad := range []string{"sm art\n", "smart/x\n", ".com\n", "smart..com\n", "sm?art.com\n"} {
		if _, err := owdReadWordLines(strings.NewReader(bad), nil); err == nil || !strings.Contains(err.Error(), "line 1") {
			t.Fatalf("%q must be rejected with its line number: %v", bad, err)
		}
	}
	if empty, err := owdReadWordLines(strings.NewReader("# only\n\n"), nil); err != nil || len(empty) != 0 {
		t.Fatalf("comments only: %v %v", empty, err)
	}
	// Lines that are not plain slugs are named by number only, never echoed.
	for _, secret := range []string{"access_token = \"dummy=abc\"\n", "smart\x1b[31m\n", "Bearer sk-live-000\n", "smart/x\n"} {
		_, err := owdReadWordLines(strings.NewReader(secret), nil)
		if err == nil || !strings.Contains(err.Error(), "line 1 is not a word or word.tld") || strings.Contains(err.Error(), "dummy") || strings.Contains(err.Error(), "sk-live") || strings.Contains(err.Error(), "\x1b") || strings.Contains(err.Error(), "smart") {
			t.Fatalf("%q: a rejected line must be named by number only, got %v", secret, err)
		}
	}
	if _, err := owdReadWordLines(strings.NewReader("ok\nsmart..com\n"), nil); err == nil || !strings.Contains(err.Error(), `line 2: "smart..com" is not a word or word.tld`) {
		t.Fatalf("a line that is already a plain slug may be echoed: %v", err)
	}
	// Uppercase is rejected before any lowercasing, and the word part must be
	// a short slug with at most two digits: keys, hashes and tokens never
	// become words, and none of them is echoed.
	for _, bad := range []string{"AKIAIOSFODNN7EXAMPLE", "5f4dcc3b5aa765d61d8327deb882cf99", "hvs.CAESIJd3Rz9oQm5tail", "Smart", "Open.com", "abc123", "smart.COM"} {
		_, err := owdReadWordLines(strings.NewReader(bad+"\n"), []string{"com"})
		if err == nil || !strings.Contains(err.Error(), "line 1") || strings.Contains(err.Error(), bad) {
			t.Fatalf("%q must be rejected by line number without being echoed: %v", bad, err)
		}
	}
	for _, bad := range []string{"-", "---", "-smart", strings.Repeat("a", 33)} {
		if _, err := owdReadWordLines(strings.NewReader(bad+"\n"), []string{"com"}); err == nil || !strings.Contains(err.Error(), "line 1") {
			t.Fatalf("%q must be rejected with its line number: %v", bad, err)
		}
	}
	lines, err = owdReadWordLines(strings.NewReader("3d\nsmart\nsmart.com\nsm-art\na1b2\n"+strings.Repeat("a", 32)+"\n"), []string{"com"})
	if err != nil || len(lines) != 6 || lines[0].Word != "3d" || lines[2].Word != "smart" || lines[2].TLD != "com" || lines[3].Word != "sm-art" || lines[4].Word != "a1b2" {
		t.Fatalf("plain words, up to two digits and 32 characters, and a pinned tracked TLD are accepted: %+v err=%v", lines, err)
	}
	// A pinned TLD outside the tracked list is reported by line number only.
	if _, err := owdReadWordLines(strings.NewReader("smart\nsmart.xyz\n"), []string{"com", "ai"}); err == nil || !strings.Contains(err.Error(), "line 2: unknown TLD") || strings.Contains(err.Error(), "xyz") {
		t.Fatalf("unknown pinned TLD: %v", err)
	}
	// Without a tracked list (recheck under --data-source local) any TLD pins.
	if lines, err := owdReadWordLines(strings.NewReader("smart.xyz\n"), nil); err != nil || len(lines) != 1 || lines[0].TLD != "xyz" {
		t.Fatalf("no tracked list skips the TLD check: %+v err=%v", lines, err)
	}
}

func TestOwdDirectoryParams(t *testing.T) {
	p := owdDirectoryParams("positive", "available", " art ", 4, 6)
	if len(p) != 5 || p["category"] != "positive" || p["price"] != "available" || p["search"] != "art" || p["minLength"] != "4" || p["maxLength"] != "6" {
		t.Fatalf("params: %v", p)
	}
	p = owdDirectoryParams("", "", "", 0, 0)
	if _, has := p["minLength"]; has || len(p) != 3 || p["category"] != "" {
		t.Fatalf("zero bounds are omitted and empty filters stay empty for the scanners to drop: %v", p)
	}
}

func TestOwdSplitKnownDomain(t *testing.T) {
	slugs := []string{"com", "co", "uk", "co.uk", "ai"}
	cases := []struct {
		in, word, tld string
		wantErr       bool
	}{
		{"smart.com", "smart", "com", false},
		{"smart.co.uk", "smart", "co.uk", false},
		{"Smart.CO", "smart", "co", false},
		{"smart.xyz", "smart", "xyz", false}, // unknown suffix falls back to last-dot split
		{"smart", "", "", true},
		{".com", "", "", true},
	}
	for _, c := range cases {
		w, tld, err := owdSplitKnownDomain(c.in, slugs)
		if (err != nil) != c.wantErr {
			t.Fatalf("%q: err=%v wantErr=%v", c.in, err, c.wantErr)
		}
		if w != c.word || tld != c.tld {
			t.Fatalf("%q: got %q/%q want %q/%q", c.in, w, tld, c.word, c.tld)
		}
	}
}

func TestOwdTopTLDs(t *testing.T) {
	tlds := []owdTLD{{Slug: "io", Views: 10}, {Slug: "com", Views: 50}, {Slug: "ai", Views: 50}, {Slug: "co", Views: 1}}
	got := owdTopTLDs(tlds, 3)
	if strings.Join(got, ",") != "ai,com,io" {
		t.Fatalf("top 3 = %v", got)
	}
	if all := owdTopTLDs(tlds, 0); len(all) != 4 {
		t.Fatalf("n=0 should return all, got %v", all)
	}
}

func TestOwdDogfoodCapTreatsZeroAsUnlimited(t *testing.T) {
	t.Setenv(cliutil.DogfoodEnvVar, "")
	if owdDogfoodCap(0, 5) != 0 || owdDogfoodCap(50, 5) != 50 || owdDogfoodCap(3, 5) != 3 {
		t.Fatal("outside the dogfood matrix the bound is untouched")
	}
	t.Setenv(cliutil.DogfoodEnvVar, "1")
	if owdDogfoodCap(0, 5) != 5 || owdDogfoodCap(50, 5) != 5 || owdDogfoodCap(3, 5) != 3 {
		t.Fatal("inside the dogfood matrix zero (unlimited) and larger bounds are capped")
	}
}

func TestOwdSessionErrorSurvivesMapping(t *testing.T) {
	raw := &client.APIError{Method: "GET", Path: "/api/domains/smart.com", StatusCode: 401}
	mapped, ok := owdIsSessionError(raw)
	if !ok || ExitCode(mapped) != 4 || !strings.Contains(mapped.Error(), "GET /api/domains/smart.com needs a signed-in") || !strings.Contains(mapped.Error(), "auth login --chrome") {
		t.Fatalf("401 maps to the session error: %v %v", mapped, ok)
	}
	if again, ok := owdIsSessionError(mapped); !ok || again != mapped {
		t.Fatalf("an already-mapped session error is recognised as is: %v %v", again, ok)
	}
	errs := []cliutil.FanoutError{{Source: "a.com", Err: errors.New("boom")}, {Source: "b.com", Err: mapped}}
	if err := owdFirstSessionErr(errs); err != mapped {
		t.Fatalf("a fan-out whose worker already mapped the 401 still fails once: %v", err)
	}
	if _, ok := owdIsSessionError(notFoundErr(errors.New("x"))); ok {
		t.Fatal("other typed errors are not session errors")
	}
}

func TestOwdCredentialAppliesToURL(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(cliutil.VerifyEnvVar, "")
	t.Setenv(cliutil.VerifyLiveHTTPEnvVar, "")
	cases := []struct {
		url, domain string
		want        bool
	}{
		{"https://oneword.domains/api/gpt/generate", "", true},
		{"https://ONEWORD.domains./x", "", true},
		{"https://api.oneword.domains/x", "", true},
		{"http://oneword.domains/x", "", false},
		{"https://oneword.domains.evil.example/x", "", false},
		{"https://evil.example/oneword.domains", "", false},
		{"http://127.0.0.1:8080/api/gpt/generate", "", false},
		{"https://localhost/x", "", false},
		{"https://example.com/x", ".example.com", true},
		{"https://sub.example.com/x", "example.com", true},
		{"https://oneword.domains/x", "example.com", false},
		{"", "", false},
		{"://bad", "", false},
	}
	for _, c := range cases {
		if got := owdCredentialAppliesToURL(c.url, c.domain); got != c.want {
			t.Fatalf("%q (bound %q): got %v want %v", c.url, c.domain, got, c.want)
		}
	}
	// The verifier's live-HTTP mode may hit a loopback mock, as in the generated client.
	t.Setenv(cliutil.VerifyEnvVar, "1")
	t.Setenv(cliutil.VerifyLiveHTTPEnvVar, "1")
	if !owdCredentialAppliesToURL("http://127.0.0.1:1/x", "") || !owdCredentialAppliesToURL("http://localhost:1/x", "") || owdCredentialAppliesToURL("http://evil.example/x", "") {
		t.Fatal("verify live-HTTP mode fails open for loopback mocks only")
	}
}

func TestOwdGenerateRequestBindsCredentialsToCanonicalHost(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(cliutil.VerifyEnvVar, "")
	var mu sync.Mutex
	got := http.Header{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r.Header.Clone()
		mu.Unlock()
		_, _ = w.Write([]byte(`{ "domain" : "openlumix.com", "available" : true }`))
	}))
	t.Cleanup(srv.Close)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("access_token = \"session=dummy-cookie\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := &rootFlags{configPath: cfgPath}
	t.Setenv("ONEWORD_DOMAINS_GPT_TOKEN", "dummy-token")
	t.Setenv("ONEWORD_DOMAINS_BASE_URL", srv.URL)
	names, _, err := owdGenerate(context.Background(), flags, owdGPTConfig(flags), map[string]any{"type": "random"})
	if err != nil || len(names) != 1 {
		t.Fatalf("generate against the loopback listener: names=%v err=%v", names, err)
	}
	mu.Lock()
	cookie, auth := got.Get("Cookie"), got.Get("Authorization")
	mu.Unlock()
	if cookie != "" || auth != "" {
		t.Fatalf("an http loopback override must receive no credential headers: cookie=%q auth=%q", cookie, auth)
	}
	// The same request built for the canonical https host carries both.
	t.Setenv("ONEWORD_DOMAINS_BASE_URL", "")
	req, err := owdGenerateRequest(context.Background(), owdGPTConfig(flags), map[string]any{"type": "random"})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.String() != owdGenerateEndpoint || req.Header.Get("Cookie") != "session=dummy-cookie" || req.Header.Get("Authorization") != "Bearer dummy-token" {
		t.Fatalf("the canonical host must receive the session cookie and partner token: url=%s cookie=%q auth=%q", req.URL, req.Header.Get("Cookie"), req.Header.Get("Authorization"))
	}
	if req.Method != http.MethodPost || req.Header.Get("Origin") != "https://oneword.domains" {
		t.Fatalf("request shape: %s %s origin=%q", req.Method, req.URL, req.Header.Get("Origin"))
	}
	// An https override naming a look-alike host is not the canonical host either.
	t.Setenv("ONEWORD_DOMAINS_BASE_URL", "https://oneword.domains.evil.example")
	req, err = owdGenerateRequest(context.Background(), owdGPTConfig(flags), map[string]any{"type": "random"})
	if err != nil || req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
		t.Fatalf("a look-alike host must receive nothing: err=%v cookie=%q auth=%q", err, req.Header.Get("Cookie"), req.Header.Get("Authorization"))
	}
	// A nil config never attaches a cookie, but the token still binds to the host.
	t.Setenv("ONEWORD_DOMAINS_BASE_URL", "")
	req, err = owdGenerateRequest(context.Background(), nil, map[string]any{"type": "random"})
	if err != nil || req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "Bearer dummy-token" {
		t.Fatalf("nil flags: err=%v cookie=%q auth=%q", err, req.Header.Get("Cookie"), req.Header.Get("Authorization"))
	}
}

func TestOwdParseGPTStream(t *testing.T) {
	raw := []byte(`{ "domain" : "openlumix.com", "available" : true }{ "domain" : "openvance.com", "available" : false }{ "domain" : "openspire.com", "available" : true }`)
	got, err := owdParseGPTStream(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Domain != "openlumix.com" || !got[0].Available || got[1].Available {
		t.Fatalf("unexpected parse: %+v", got)
	}
	empty, err := owdParseGPTStream([]byte("   "))
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty stream: %v %+v", err, empty)
	}
	if _, err := owdParseGPTStream([]byte(`{"domain":"a.com","available":true}{"domain":`)); err == nil {
		t.Fatal("truncated stream should error")
	}
}

func TestOwdHacks(t *testing.T) {
	got := owdHacks("smart", []string{"art", "com", "rt", "smart"})
	if len(got) != 2 || got[0].Domain != "sm.art" || got[0].Stem != "sm" || got[1].Domain != "sma.rt" {
		t.Fatalf("unexpected hacks: %+v", got)
	}
	if len(owdHacks("ai", []string{"ai"})) != 0 {
		t.Fatal("word equal to tld must not produce a hack")
	}
}

func TestOwdPopularityPct(t *testing.T) {
	if p := owdPopularityPct(93); p != 0 {
		t.Fatalf("all free should be 0%%, got %v", p)
	}
	if p := owdPopularityPct(0); p != 100 {
		t.Fatalf("none free should be 100%%, got %v", p)
	}
	if p := owdPopularityPct(6); math.Abs(p-93.548) > 0.01 {
		t.Fatalf("tldCount 6 should be ~93.55%%, got %v", p)
	}
}

func TestOwdEndsWithin(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if !owdEndsWithin("2026-06-01T12:00:00.000Z", 24*time.Hour, now) {
		t.Fatal("12h ahead should be within 24h")
	}
	if owdEndsWithin("2026-06-03T00:00:00.000Z", 24*time.Hour, now) {
		t.Fatal("48h ahead should not be within 24h")
	}
	if owdEndsWithin("2026-05-31T00:00:00.000Z", 24*time.Hour, now) {
		t.Fatal("past dates are never within the window")
	}
	if owdEndsWithin("garbage", time.Hour, now) {
		t.Fatal("unparseable dates are never within the window")
	}
}

func TestOwdParseCSVListAndDedupe(t *testing.T) {
	if got := owdParseCSVList(" .COM, ai ,, io , com, .AI"); strings.Join(got, ",") != "com,ai,io" {
		t.Fatalf("csv list must normalize and de-duplicate: %v", got)
	}
	if got := owdParseCSVList(","); len(got) != 0 {
		t.Fatalf("empty items drop out: %v", got)
	}
	if got := owdDedupe([]string{" Com", "ai", "", "com", "AI ", "io"}); strings.Join(got, ",") != "com,ai,io" {
		t.Fatalf("dedupe: %v", got)
	}
	if got := owdDedupe(nil); got == nil || len(got) != 0 {
		t.Fatal("nil input yields an empty, non-nil slice")
	}
	if owdNormTLD(" .COM ") != "com" || owdNormTLD("") != "" {
		t.Fatal("owdNormTLD")
	}
}

func TestOwdPriceFloat(t *testing.T) {
	for in, want := range map[string]float64{"72.4": 72.4, " 9.13 ": 9.13, "0": 0, "1": 1} {
		if v, ok := owdPriceFloat(in); !ok || v != want {
			t.Fatalf("%q should parse to %v, got %v/%v", in, want, v, ok)
		}
	}
	for _, in := range []string{"", "   ", "abc", "$12", "12,5", "-5", "-0.01", "NaN", "Inf", "-Inf", "1e999", "7 USD"} {
		if v, ok := owdPriceFloat(in); ok {
			t.Fatalf("%q must not parse as a price, got %v", in, v)
		}
	}
}

func TestOwdUsageOK(t *testing.T) {
	body := `{"usage":"3","quota":10}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := owdTestClient(t, srv)
	used, quota, ok, err := owdUsage(context.Background(), c)
	if err != nil || !ok || used != 3 || quota != 10 {
		t.Fatalf("usage: %d/%d ok=%v err=%v", used, quota, ok, err)
	}
	body = `{"remaining":7}`
	if _, _, ok, err := owdUsage(context.Background(), c); err != nil || ok {
		t.Fatalf("missing fields must report ok=false: ok=%v err=%v", ok, err)
	}
}

func TestOwdLoopCtx(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	ctx, cancel := owdLoopCtx(cmd, &rootFlags{timeout: time.Minute})
	defer cancel()
	if _, has := ctx.Deadline(); has {
		t.Fatal("an implicit --timeout must not bound the loop")
	}
	ctx, cancel = owdLoopCtx(cmd, &rootFlags{timeout: time.Minute, timeoutExplicit: true})
	defer cancel()
	if _, has := ctx.Deadline(); !has {
		t.Fatal("an explicit --timeout must bound the loop")
	}
	ctx, cancel = owdLoopCtx(&cobra.Command{}, nil)
	defer cancel()
	if ctx == nil {
		t.Fatal("a command without a context still yields one")
	}
}

func TestOwdIndexTLDsAndWindowFlag(t *testing.T) {
	known, slugs := owdIndexTLDs([]owdTLD{{Slug: "com"}, {Slug: ""}, {Slug: "ai", MinPrice: "72.4"}})
	if len(known) != 2 || strings.Join(slugs, ",") != "com,ai" || known["ai"].MinPrice != "72.4" {
		t.Fatalf("index: %v %v", known, slugs)
	}
	if err := owdRequireKnownTLDs([]string{"com", "xyz", "abc"}, known); ExitCode(err) != 2 || !strings.Contains(err.Error(), "xyz, abc") {
		t.Fatalf("unknown TLDs must be a usage error listing them: %v", err)
	}
	if err := owdRequireKnownTLDs([]string{"com", "ai"}, known); err != nil {
		t.Fatal(err)
	}
	if d, err := owdParseWindowFlag("since", "7d"); err != nil || d != 7*24*time.Hour {
		t.Fatalf("7d: %v %v", d, err)
	}
	for _, bad := range []string{"soon", "-1h", "0s", ""} {
		if _, err := owdParseWindowFlag("since", bad); ExitCode(err) != 2 || !strings.Contains(err.Error(), "--since") {
			t.Fatalf("%q must be a usage error naming the flag: %v", bad, err)
		}
	}
}

func TestOwdGenerateMasksCredentialsInErrorBody(t *testing.T) {
	testenv.Isolate(t)
	// The verifier's live-HTTP mode is the one case a loopback listener
	// receives the credentials, so their echo can be observed.
	t.Setenv(cliutil.VerifyEnvVar, "1")
	t.Setenv(cliutil.VerifyLiveHTTPEnvVar, "1")
	var mu sync.Mutex
	gotCookie, gotAuth := "", ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotCookie, gotAuth = r.Header.Get("Cookie"), r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("cookie=" + r.Header.Get("Cookie") + " auth=" + r.Header.Get("Authorization") + " esc=" + url.QueryEscape(r.Header.Get("Cookie")) + " tok=" + strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") + " boom"))
	}))
	t.Cleanup(srv.Close)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("access_token = \"session=dummy-cookie-value\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONEWORD_DOMAINS_GPT_TOKEN", "dummy-partner-token")
	t.Setenv("ONEWORD_DOMAINS_BASE_URL", srv.URL)
	_, _, err := owdGenerate(context.Background(), &rootFlags{configPath: cfgPath}, owdGPTConfig(&rootFlags{configPath: cfgPath}), map[string]any{"type": "random"})
	if err == nil || ExitCode(err) != 5 || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("5xx must be an API error: %v", err)
	}
	mu.Lock()
	cookie, auth := gotCookie, gotAuth
	mu.Unlock()
	if cookie != "session=dummy-cookie-value" || auth != "Bearer dummy-partner-token" {
		t.Fatalf("the loopback exception must have attached both credentials: cookie=%q auth=%q", cookie, auth)
	}
	msg := err.Error()
	for _, secret := range []string{"dummy-cookie-value", "dummy-partner-token", url.QueryEscape("session=dummy-cookie-value")} {
		if strings.Contains(msg, secret) {
			t.Fatalf("credential %q leaked into the error: %s", secret, msg)
		}
	}
	if !strings.Contains(msg, "****") || !strings.Contains(msg, "boom") {
		t.Fatalf("the body must survive with the credentials masked: %s", msg)
	}
	// Without a request the scrubber only trims and bounds the body.
	if got := owdErrBody([]byte("  plain  "), nil); got != "plain" {
		t.Fatalf("nil request: %q", got)
	}
}

func TestOwdGenerateNoRedirectAndBoundedErrorBody(t *testing.T) {
	testenv.Isolate(t)
	hits := make([]string, 0)
	mode := "redirect"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		switch {
		case r.URL.Path == "/elsewhere":
			t.Errorf("redirect must not be followed")
		case mode == "redirect":
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		default:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(strings.Repeat("x", 10000) + "\x1b[31mred\x1b[0m\ttab"))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ONEWORD_DOMAINS_BASE_URL", srv.URL)
	if owdGenerateURL() != srv.URL+"/api/gpt/generate" {
		t.Fatalf("base URL override: %s", owdGenerateURL())
	}
	_, _, err := owdGenerate(context.Background(), nil, nil, map[string]any{"type": "random"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") || strings.Join(hits, ",") != "/api/gpt/generate" {
		t.Fatalf("redirects must surface as the last response: err=%v hits=%v", err, hits)
	}
	mode = "error"
	_, _, err = owdGenerate(context.Background(), nil, nil, map[string]any{"type": "random"})
	if err == nil || ExitCode(err) != 5 || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("5xx must be an API error: %v", err)
	}
	if len(err.Error()) > 4096+100 || strings.Contains(err.Error(), "\x1b") || strings.Contains(err.Error(), "\t") {
		t.Fatalf("error body must be capped and scrubbed (len=%d)", len(err.Error()))
	}
	t.Setenv("ONEWORD_DOMAINS_BASE_URL", "")
	if owdGenerateURL() != owdGenerateEndpoint {
		t.Fatalf("no override falls back to the site: %s", owdGenerateURL())
	}
}

func TestOwdSplitDomainMultiLabelTLD(t *testing.T) {
	for in, want := range map[string][2]string{"smart.co.uk": {"smart", "co.uk"}, "open.com.au": {"open", "com.au"}, "sm.art": {"sm", "art"}, "Smart.COM": {"smart", "com"}} {
		w, tld, err := owdSplitDomain(in)
		if err != nil || w != want[0] || tld != want[1] {
			t.Fatalf("owdSplitDomain(%q) = %q, %q, %v; want %q, %q", in, w, tld, err, want[0], want[1])
		}
	}
	for _, bad := range []string{"smart", ".com", "smart."} {
		if _, _, err := owdSplitDomain(bad); err == nil {
			t.Fatalf("owdSplitDomain(%q) accepted an invalid domain", bad)
		}
	}
}
