// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

func TestNovelRecheckHelpWires(t *testing.T) {
	testenv.Isolate(t)
	out, _, err := owdNovelRun(t, "recheck", "--help")
	if err != nil {
		t.Fatalf("recheck --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "recheck", "use 'listings watch' instead", "--since", "--file", "--max-checks"} {
		if !strings.Contains(out, want) {
			t.Fatalf("recheck --help missing %q in output:\n%s", want, out)
		}
	}
}

func TestNovelRecheckEmptyHistory(t *testing.T) {
	testenv.Isolate(t)
	var res owdRecheckResult
	errOut, err := owdNovelRunJSON(t, &res, "recheck")
	if err != nil {
		t.Fatal(err)
	}
	if res.Checked != 0 || len(res.Changed) != 0 || res.Unchanged != 0 || res.Changed == nil {
		t.Fatalf("empty history must be a zero object with an empty array: %+v", res)
	}
	if !strings.Contains(errOut, "no prior checks; run 'oneword-domains-pp-cli check <word>' first") {
		t.Fatalf("missing hint on stderr: %q", errOut)
	}
	var env dryRunResult
	if _, err := owdNovelRunJSON(t, &env, "recheck", "--since", "7d", "--dry-run"); err != nil || !env.DryRun || env.Action != "recheck" {
		t.Fatalf("dry-run envelope: %+v err=%v", env, err)
	}
	if _, _, err := owdNovelRun(t, "recheck", "--since", "soon", "--json"); ExitCode(err) != 2 {
		t.Fatalf("bad --since must be a usage error: %v", err)
	}
}

func owdRecheckSeed(t *testing.T, domain string, available int, price any, tldCount int, at time.Time) {
	t.Helper()
	db := owdNovelTestStore(t)
	defer db.Close()
	word, tld, _ := owdSplitDomain(domain)
	if _, err := db.DB().Exec(`INSERT INTO owd_domain_checks (word, tld, domain, available, premium, price, aftermarket, tld_count, checked_at) VALUES (?, ?, ?, ?, 0, ?, 0, ?, ?)`,
		word, tld, domain, available, price, tldCount, at.UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
}

func owdRecheckServer(t *testing.T, answers map[string]string) {
	t.Helper()
	owdNovelTestServer(t, owdNovelJSONHandler(map[string]func(r *http.Request) (int, string){
		"/api/tlds": func(r *http.Request) (int, string) {
			return 200, `[{"slug":"com","type":"gTld","structure":"normal","top10m":1,"totalReg":2,"views":3,"minPrice":"9.5"},{"slug":"ai","type":"ccTld","structure":"normal","top10m":1,"totalReg":2,"views":3,"minPrice":"72.4"},{"slug":"io","type":"ccTld","structure":"normal","top10m":1,"totalReg":2,"views":3,"minPrice":"30"}]`
		},
		"/api/domains/": func(r *http.Request) (int, string) {
			d := strings.TrimPrefix(r.URL.Path, "/api/domains/")
			if body, ok := answers[d]; ok {
				return 200, body
			}
			return 404, `{"error":"unknown"}`
		},
	}))
}

func TestNovelRecheckDetectsChanges(t *testing.T) {
	owdRecheckServer(t, map[string]string{
		"smart.com": `{"slug":"smart.com","available":true,"premium":false,"price":"12","tldSlug":"com","aftermarket":false,"tldCount":5}`,
		"open.com":  `{"slug":"open.com","available":false,"premium":false,"price":null,"tldSlug":"com","aftermarket":false,"tldCount":2}`,
	})
	old := time.Now().Add(-48 * time.Hour)
	owdRecheckSeed(t, "smart.com", 0, nil, 6, old)
	owdRecheckSeed(t, "open.com", 0, nil, 2, old)

	var res owdRecheckResult
	if _, err := owdNovelRunJSON(t, &res, "recheck"); err != nil {
		t.Fatal(err)
	}
	if res.Checked != 2 || res.Unchanged != 1 || res.Baseline != 0 || len(res.FetchFailures) != 0 {
		t.Fatalf("unexpected counts: %+v", res)
	}
	fields := map[string]owdRecheckChange{}
	for _, c := range res.Changed {
		if c.Domain != "smart.com" {
			t.Fatalf("only smart.com changed: %+v", c)
		}
		fields[c.Field] = c
	}
	if len(fields) != 3 || fields["available"].After != true || fields["price"].After != "12" || fields["tld_count"].Before != float64(6) {
		t.Fatalf("expected available/price/tld_count changes: %+v", res.Changed)
	}

	// The local diff compares the two stored snapshots without the network.
	var local owdRecheckResult
	if _, err := owdNovelRunJSON(t, &local, "recheck", "--data-source", "local"); err != nil {
		t.Fatal(err)
	}
	if local.Checked != 2 || len(local.Changed) != 3 || local.Unchanged != 1 {
		t.Fatalf("local diff mismatch: %+v", local)
	}

	// Another live run against the same answers: nothing changes.
	var again owdRecheckResult
	if _, err := owdNovelRunJSON(t, &again, "recheck", "--no-cache"); err != nil {
		t.Fatal(err)
	}
	if again.Checked != 2 || len(again.Changed) != 0 || again.Unchanged != 2 {
		t.Fatalf("second run must be unchanged: %+v", again)
	}

	// --since narrows to pairs first checked inside the window.
	var since owdRecheckResult
	errOut, err := owdNovelRunJSON(t, &since, "recheck", "--since", "1h")
	if err != nil || since.Checked != 0 || !strings.Contains(errOut, "no prior checks match the filter") {
		t.Fatalf("since filter: %+v err=%v stderr=%q", since, err, errOut)
	}
	// --tld restricts the history; --limit and --max-checks cap it.
	var capped owdRecheckResult
	if _, err := owdNovelRunJSON(t, &capped, "recheck", "--tld", "com", "--max-checks", "1"); err != nil || capped.Checked != 1 || !strings.Contains(capped.Note, "--max-checks") {
		t.Fatalf("max-checks cap: %+v err=%v", capped, err)
	}
	if _, err := owdNovelRunJSON(t, &capped, "recheck", "--tld", "io"); err != nil || capped.Checked != 0 {
		t.Fatalf("tld filter: %+v err=%v", capped, err)
	}
}

func TestNovelRecheckFileAndFailures(t *testing.T) {
	owdRecheckServer(t, map[string]string{
		"smart.com": `{"slug":"smart.com","available":false,"premium":false,"price":null,"tldSlug":"com","aftermarket":false,"tldCount":6}`,
		"smart.ai":  `{"slug":"smart.ai","available":true,"premium":true,"price":"499","tldSlug":"ai","aftermarket":false,"tldCount":6}`,
		"open.com":  `{"slug":"open.com","available":false,"premium":false,"price":null,"tldSlug":"com","aftermarket":false,"tldCount":2}`,
	})
	path := filepath.Join(t.TempDir(), "words.txt")
	// smart and open are bare (crossed with --tld); open.com pins its TLD and
	// de-duplicates against open x com.
	if err := os.WriteFile(path, []byte("smart\n# comment\n\nopen.com\nsmart\nopen\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var res owdRecheckResult
	errOut, err := owdNovelRunJSON(t, &res, "recheck", "--file", path, "--tld", "com,ai")
	if err != nil {
		t.Fatal(err)
	}
	if res.Checked != 3 || res.Baseline != 3 || len(res.FetchFailures) != 1 || res.FetchFailures[0].Source != "open.ai" {
		t.Fatalf("file run: %+v", res)
	}
	if !strings.Contains(errOut, "1 of 4 re-checks failed") {
		t.Fatalf("stderr must carry the failure denominator: %q", errOut)
	}
	if _, _, err := owdNovelRun(t, "recheck", "--file", filepath.Join(t.TempDir(), "missing.txt"), "--json"); ExitCode(err) != 2 {
		t.Fatalf("missing file must be a usage error: %v", err)
	}
	if _, _, err := owdNovelRun(t, "recheck", "--file", path, "--tld", "com,xyz", "--json"); ExitCode(err) != 2 || !strings.Contains(err.Error(), "xyz") {
		t.Fatalf("--tld is validated against the tracked TLDs: %v", err)
	}
	if err := os.WriteFile(path, []byte("smart.xyz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := owdNovelRun(t, "recheck", "--file", path, "--json"); ExitCode(err) != 2 || !strings.Contains(err.Error(), "line 1: unknown TLD") || strings.Contains(err.Error(), "xyz") {
		t.Fatalf("pinned TLDs are validated too, by line number only: %v", err)
	}
	if err := os.WriteFile(path, []byte("sm/art\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := owdNovelRun(t, "recheck", "--file", path, "--json"); ExitCode(err) != 2 {
		t.Fatalf("unsafe words are rejected before any check: %v", err)
	}
}

func TestOwdRecheckDiff(t *testing.T) {
	at := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	p12 := "12"
	cases := []struct {
		name          string
		before, after owdDomainCheck
		want          []string
	}{
		{"same", owdDomainCheck{TldCount: 6}, owdDomainCheck{TldCount: 6}, nil},
		{"freed", owdDomainCheck{}, owdDomainCheck{Available: true}, []string{"available"}},
		{"premium and price", owdDomainCheck{}, owdDomainCheck{Premium: true, Price: &p12}, []string{"premium", "price"}},
		{"popularity", owdDomainCheck{TldCount: 6}, owdDomainCheck{TldCount: 4}, []string{"tld_count"}},
	}
	for _, c := range cases {
		got := owdRecheckDiff("x.com", &c.before, &c.after, at)
		fields := make([]string, 0, len(got))
		for _, g := range got {
			if g.Domain != "x.com" || g.CheckedAt != "2026-09-24T00:00:00Z" {
				t.Fatalf("%s: bad row %+v", c.name, g)
			}
			fields = append(fields, g.Field)
		}
		if strings.Join(fields, ",") != strings.Join(c.want, ",") {
			t.Fatalf("%s: got %v want %v", c.name, fields, c.want)
		}
	}
	if got := owdRecheckDiff("x.com", nil, &owdDomainCheck{}, at); len(got) != 0 || got == nil {
		t.Fatal("nil prior yields an empty, non-nil slice")
	}
}

func TestOwdRecheckWordFileSharesTheCheckReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.txt")
	if err := os.WriteFile(path, []byte(" smart.com \n#skip\n\nopen\nsmart\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lines, err := owdReadWordLines(f, []string{"com", "ai"})
	if err != nil || len(lines) != 3 || lines[0].Word != "smart" || lines[0].TLD != "com" || lines[1].Word != "open" || lines[2].TLD != "" {
		t.Fatalf("lines=%+v err=%v", lines, err)
	}
	pairs := owdWordPairs(lines, []string{"com", "ai"})
	if len(pairs) != 4 || pairs[0].Domain != "smart.com" || pairs[1].Domain != "open.com" || pairs[2].Domain != "open.ai" || pairs[3].Domain != "smart.ai" || pairs[3].Word != "smart" {
		t.Fatalf("pairs=%+v", pairs)
	}
}

func TestOwdRecheckHistoryAndLocal(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now().UTC()
	owdRecheckSeed(t, "a.com", 0, nil, 6, now.Add(-72*time.Hour))
	owdRecheckSeed(t, "a.com", 1, "9", 5, now.Add(-time.Hour))
	owdRecheckSeed(t, "b.io", 1, nil, 3, now.Add(-time.Hour))
	db := owdNovelTestStore(t)
	defer db.Close()
	ctx := context.Background()
	all, err := owdRecheckHistory(ctx, db, 0, now, nil)
	if err != nil || len(all) != 2 || all[0].Domain != "a.com" || all[0].Word != "a" || all[0].TLD != "com" {
		t.Fatalf("history=%+v err=%v", all, err)
	}
	recent, err := owdRecheckHistory(ctx, db, 24*time.Hour, now, nil)
	if err != nil || len(recent) != 1 || recent[0].Domain != "b.io" {
		t.Fatalf("since window keys off the first check: %+v err=%v", recent, err)
	}
	onlyIO, err := owdRecheckHistory(ctx, db, 0, now, []string{"io"})
	if err != nil || len(onlyIO) != 1 {
		t.Fatalf("tld filter: %+v err=%v", onlyIO, err)
	}
	res, err := owdRecheckLocal(ctx, db, all)
	if err != nil || res.Checked != 2 || res.Baseline != 1 || len(res.Changed) != 3 {
		t.Fatalf("local diff: %+v err=%v", res, err)
	}
	if strings.Join([]string{res.Changed[0].Field, res.Changed[1].Field, res.Changed[2].Field}, " ") != "available price tld_count" {
		t.Fatalf("changes sorted by field: %+v", res.Changed)
	}
}
