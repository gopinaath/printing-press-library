// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

func TestNovelDomainsIntersectHelpWires(t *testing.T) {
	testenv.Isolate(t)
	out, _, err := owdNovelRun(t, "domains", "intersect", "--help")
	if err != nil {
		t.Fatalf("domains intersect --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "intersect", "use 'words mine' instead", "use 'tlds inventory' instead", "--taken-on", "--max-pages"} {
		if !strings.Contains(out, want) {
			t.Fatalf("domains intersect --help missing %q in output:\n%s", want, out)
		}
	}
	bare, _, err := owdNovelRun(t, "domains", "intersect")
	if err != nil || !strings.Contains(bare, "Usage:") {
		t.Fatalf("bare invocation prints help: err=%v out=%q", err, bare)
	}
}

func TestNovelDomainsIntersectUsage(t *testing.T) {
	testenv.Isolate(t)
	cases := [][]string{
		{"domains", "intersect", "--json"},
		{"domains", "intersect", "--tld", "com", "--json"},
		{"domains", "intersect", "--tld", "com,ai", "--data-source", "local", "--json"},
		{"domains", "intersect", "--tld", "com,ai", "--taken-on", "ai", "--json"},
		{"domains", "intersect", "--tld", "ai", "--taken-on", "com", "--json"},
		{"domains", "intersect", "--tld", "com,ai", "--price", "cheap", "--json"},
		{"domains", "intersect", "--tld", "com,ai", "--max-pages", "0", "--json"},
	}
	for _, args := range cases {
		if _, _, err := owdNovelRun(t, args...); ExitCode(err) != 2 {
			t.Fatalf("%v: want usage error, got %v", args, err)
		}
	}
	var env dryRunResult
	if _, err := owdNovelRunJSON(t, &env, "domains", "intersect", "--tld", "com,ai", "--max-pages", "1", "--dry-run"); err != nil || !env.DryRun || env.Action != "domains intersect" {
		t.Fatalf("dry-run: %+v err=%v", env, err)
	}
}

func owdIntersectPage(words []string, tld string) string {
	rows := make([]string, 0, len(words))
	for i, w := range words {
		price := "null"
		if i%2 == 1 {
			price = `"12"`
		}
		rows = append(rows, fmt.Sprintf(`{"slug":"%s.%s","tldCount":%d,"premium":%v,"price":%s}`, w, tld, 10+i, i%3 == 0, price))
	}
	return "[" + strings.Join(rows, ",") + "]"
}

func owdIntersectServer(t *testing.T, status int) *[]string {
	t.Helper()
	seen := &[]string{}
	big := func(prefix string) []string {
		out := make([]string, 0, 100)
		for i := 0; i < 100; i++ {
			out = append(out, fmt.Sprintf("%s%03d", prefix, i))
		}
		return out
	}
	sets := map[string][]string{
		"com": {"alpha", "beta", "gamma"},
		"ai":  {"beta", "gamma", "delta"},
		"co":  {"gamma"},
		"x":   big("x"),
		"y":   big("y"),
	}
	owdNovelTestServer(t, owdNovelJSONHandler(map[string]func(r *http.Request) (int, string){
		"/api/domains": func(r *http.Request) (int, string) {
			*seen = append(*seen, r.URL.RawQuery)
			if status != 200 {
				return status, "Unauthorized"
			}
			tld := r.URL.Query().Get("tld")
			if r.URL.Query().Get("page") != "1" {
				return 200, `[]`
			}
			return 200, owdIntersectPage(sets[tld], tld)
		},
	}))
	return seen
}

func TestNovelDomainsIntersectLive(t *testing.T) {
	seen := owdIntersectServer(t, 200)
	var out owdIntersectOutput
	if _, err := owdNovelRunJSON(t, &out, "domains", "intersect", "--tld", "com,ai", "--category", "positive", "--min-length", "4"); err != nil {
		t.Fatal(err)
	}
	if len(out.Words) != 2 || out.Words[0].Word != "beta" || out.Words[1].Word != "gamma" {
		t.Fatalf("intersection: %+v", out.Words)
	}
	if out.ScannedPages != 2 || out.PerTLDCounts["com"] != 3 || out.PerTLDCounts["ai"] != 3 || out.Note != "" {
		t.Fatalf("accounting: %+v", out)
	}
	beta := out.Words[0]
	if len(beta.TLDs) != 2 || beta.TLDs["com"].Price == nil || *beta.TLDs["com"].Price != "12" || !beta.TLDs["ai"].Premium || beta.TldCount != 11 {
		t.Fatalf("per-TLD detail: %+v", beta)
	}
	if len(*seen) == 0 || !strings.Contains((*seen)[0], "category=positive") || !strings.Contains((*seen)[0], "minLength=4") || !strings.Contains((*seen)[0], "tld=com") {
		t.Fatalf("filters must reach the API: %v", *seen)
	}

	if _, err := owdNovelRunJSON(t, &out, "domains", "intersect", "--tld", "com,ai", "--taken-on", "co", "--max-pages", "5"); err != nil {
		t.Fatal(err)
	}
	if len(out.Words) != 1 || out.Words[0].Word != "beta" || len(out.TakenOn) != 1 {
		t.Fatalf("taken-on subtraction: %+v", out)
	}
	if _, err := owdNovelRunJSON(t, &out, "domains", "intersect", "--tld", "ai", "--taken-on", "com", "--max-pages", "5"); err != nil {
		t.Fatal(err)
	}
	if len(out.Words) != 1 || out.Words[0].Word != "delta" {
		t.Fatalf("single tld with taken-on: %+v", out)
	}
	if _, err := owdNovelRunJSON(t, &out, "domains", "intersect", "--tld", "com,ai", "--limit", "1"); err != nil || len(out.Words) != 1 {
		t.Fatalf("limit: %+v err=%v", out, err)
	}
	// Full pages under the cap with no overlap must say so.
	if _, err := owdNovelRunJSON(t, &out, "domains", "intersect", "--tld", "x,y", "--max-pages", "1"); err != nil {
		t.Fatal(err)
	}
	if len(out.Words) != 0 || out.Words == nil || !strings.Contains(out.Note, "page cap reached") || out.ScannedPages != 2 {
		t.Fatalf("capped scan note: %+v", out)
	}
}

func TestNovelDomainsIntersectNeedsSession(t *testing.T) {
	owdIntersectServer(t, 401)
	_, _, err := owdNovelRun(t, "domains", "intersect", "--tld", "com,ai", "--max-pages", "1", "--json")
	if ExitCode(err) != 4 || !strings.Contains(err.Error(), "auth login --chrome") {
		t.Fatalf("401 must surface as the session auth error (exit 4): %v", err)
	}
}

func TestOwdIntersectWords(t *testing.T) {
	e := func(n int) owdIntersectEntry { return owdIntersectEntry{TldCount: n} }
	sets := map[string]map[string]owdIntersectEntry{
		"com": {"a": e(1), "b": e(2), "c": e(3)},
		"ai":  {"b": e(2), "c": e(3), "d": e(4)},
		"co":  {"c": e(3)},
	}
	got := owdIntersectWords(sets, []string{"com", "ai"}, nil)
	if len(got) != 2 || got[0].Word != "b" || got[1].Word != "c" || got[1].TldCount != 3 || len(got[1].TLDs) != 2 {
		t.Fatalf("intersect: %+v", got)
	}
	got = owdIntersectWords(sets, []string{"com", "ai"}, []string{"co"})
	if len(got) != 1 || got[0].Word != "b" {
		t.Fatalf("taken-on: %+v", got)
	}
	if got := owdIntersectWords(sets, nil, nil); len(got) != 0 || got == nil {
		t.Fatal("no free TLDs yields an empty, non-nil slice")
	}
	if got := owdIntersectWords(sets, []string{"com", "missing"}, nil); len(got) != 0 {
		t.Fatalf("unknown TLD set intersects to nothing: %+v", got)
	}
}

func TestOwdIntersectParseRows(t *testing.T) {
	arr, err := owdIntersectParseRows(json.RawMessage(`[{"slug":"a.com"},{"slug":"b.com"}]`))
	if err != nil || len(arr) != 2 {
		t.Fatalf("array: %v %d", err, len(arr))
	}
	env, err := owdIntersectParseRows(json.RawMessage(`{"domains":[{"slug":"a.com"}],"total":1}`))
	if err != nil || len(env) != 1 {
		t.Fatalf("envelope: %v %d", err, len(env))
	}
	if _, err := owdIntersectParseRows(json.RawMessage(`"nope"`)); err == nil {
		t.Fatal("scalar bodies are errors")
	}
}
