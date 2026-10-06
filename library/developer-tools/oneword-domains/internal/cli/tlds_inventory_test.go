// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

func TestNovelTldsInventoryHelpWires(t *testing.T) {
	testenv.Isolate(t)
	out, _, err := owdNovelRun(t, "tlds", "inventory", "--help")
	if err != nil {
		t.Fatalf("tlds inventory --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "inventory", "use 'domains intersect' instead", "use 'tlds list' or 'tlds get' instead", "--max-price", "--max-checks", "--type"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tlds inventory --help missing %q in output:\n%s", want, out)
		}
	}
}

const owdInventoryTLDFixture = `[{"slug":"ai","type":"ccTld","structure":"normal","top10m":30,"totalReg":1000,"views":1,"minPrice":"72.4"},{"slug":"com","type":"gTld","structure":"normal","top10m":900,"totalReg":5000,"views":2,"minPrice":"9.5"},{"slug":"io","type":"ccTld","structure":"normal","top10m":100,"totalReg":800,"views":3,"minPrice":"30"}]`

func owdInventoryServer(t *testing.T, counts map[string]string, countStatus int) *[]string {
	t.Helper()
	seen := &[]string{}
	owdNovelTestServer(t, owdNovelJSONHandler(map[string]func(r *http.Request) (int, string){
		"/api/tlds": func(r *http.Request) (int, string) { return 200, owdInventoryTLDFixture },
		"/api/domains/count": func(r *http.Request) (int, string) {
			*seen = append(*seen, r.URL.RawQuery)
			if countStatus != 200 {
				return countStatus, "Unauthorized"
			}
			body, ok := counts[r.URL.Query().Get("tld")]
			if !ok {
				return 404, `{"error":"no such tld"}`
			}
			return 200, body
		},
	}))
	return seen
}

func owdInventoryTLDs(rows []owdInventoryRow) string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.TLD)
	}
	return strings.Join(out, ",")
}

func TestNovelTldsInventoryCounts(t *testing.T) {
	seen := owdInventoryServer(t, map[string]string{"ai": "120", "com": `"45"`, "io": "300"}, 200)
	var out owdInventoryOutput
	if _, err := owdNovelRunJSON(t, &out, "tlds", "inventory", "--category", "positive", "--min-len", "4"); err != nil {
		t.Fatal(err)
	}
	if out.Matched != 3 || out.Checked != 3 || len(out.FetchFailures) != 0 || out.Note != "" {
		t.Fatalf("accounting: %+v", out)
	}
	if got := owdInventoryTLDs(out.Rows); got != "io,ai,com" {
		t.Fatalf("count order: %s", got)
	}
	if out.Rows[2].AvailableCount != 45 || out.Rows[2].MinPrice != "9.5" || out.Rows[2].Type != "gTld" || out.Rows[2].Top10m != 900 || out.Rows[2].TotalReg != 5000 {
		t.Fatalf("row detail: %+v", out.Rows[2])
	}
	if len(*seen) != 3 || !strings.Contains((*seen)[0], "category=positive") || !strings.Contains((*seen)[0], "minLength=4") {
		t.Fatalf("filters must reach the count route: %v", *seen)
	}
	if _, err := owdNovelRunJSON(t, &out, "tlds", "inventory", "--sort", "price"); err != nil || owdInventoryTLDs(out.Rows) != "com,io,ai" {
		t.Fatalf("price order: %s err=%v", owdInventoryTLDs(out.Rows), err)
	}
	if _, err := owdNovelRunJSON(t, &out, "tlds", "inventory", "--sort", "top10m", "--limit", "2"); err != nil || owdInventoryTLDs(out.Rows) != "com,io" {
		t.Fatalf("top10m order + limit: %s err=%v", owdInventoryTLDs(out.Rows), err)
	}
	if _, err := owdNovelRunJSON(t, &out, "tlds", "inventory", "--type", "cctld"); err != nil || owdInventoryTLDs(out.Rows) != "io,ai" || out.Matched != 2 {
		t.Fatalf("type filter: %s err=%v", owdInventoryTLDs(out.Rows), err)
	}
	if _, err := owdNovelRunJSON(t, &out, "tlds", "inventory", "--max-price", "20"); err != nil || owdInventoryTLDs(out.Rows) != "com" {
		t.Fatalf("max-price filter: %s err=%v", owdInventoryTLDs(out.Rows), err)
	}
	if _, err := owdNovelRunJSON(t, &out, "tlds", "inventory", "--max-checks", "1"); err != nil || len(out.Rows) != 1 || out.Matched != 3 || !strings.Contains(out.Note, "--max-checks") {
		t.Fatalf("max-checks cap: %+v err=%v", out, err)
	}
	if _, err := owdNovelRunJSON(t, &out, "tlds", "inventory", "--max-price", "1"); err != nil || len(out.Rows) != 0 || out.Rows == nil || out.Note == "" {
		t.Fatalf("no TLD matched prints empty rows with a note: %+v err=%v", out, err)
	}
	for _, args := range [][]string{
		{"tlds", "inventory", "--data-source", "local", "--json"},
		{"tlds", "inventory", "--type", "weird", "--json"},
		{"tlds", "inventory", "--sort", "weird", "--json"},
		{"tlds", "inventory", "--price", "cheap", "--json"},
	} {
		if _, _, err := owdNovelRun(t, args...); ExitCode(err) != 2 {
			t.Fatalf("%v: want usage error, got %v", args, err)
		}
	}
	var env dryRunResult
	if _, err := owdNovelRunJSON(t, &env, "tlds", "inventory", "--category", "positive", "--dry-run"); err != nil || !env.DryRun || env.Action != "tlds inventory" {
		t.Fatalf("dry-run: %+v err=%v", env, err)
	}
}

func TestNovelTldsInventoryPartialFailure(t *testing.T) {
	owdInventoryServer(t, map[string]string{"ai": "120", "com": "45"}, 200)
	var out owdInventoryOutput
	errOut, err := owdNovelRunJSON(t, &out, "tlds", "inventory")
	if err != nil {
		t.Fatal(err)
	}
	if out.Checked != 2 || len(out.FetchFailures) != 1 || out.FetchFailures[0].Source != "io" || owdInventoryTLDs(out.Rows) != "ai,com" {
		t.Fatalf("partial failure: %+v", out)
	}
	if !strings.Contains(errOut, "1 of 3 TLD counts failed") {
		t.Fatalf("stderr must carry the denominator: %q", errOut)
	}
}

func TestNovelTldsInventoryNeedsSession(t *testing.T) {
	seen := owdInventoryServer(t, nil, 401)
	_, _, err := owdNovelRun(t, "tlds", "inventory", "--category", "positive", "--max-checks", "3", "--json")
	if ExitCode(err) != 4 || !strings.Contains(err.Error(), "auth login --chrome") {
		t.Fatalf("401 must surface as the session auth error (exit 4): %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("the probe must stop the fan-out after one failing call, saw %d", len(*seen))
	}
}

func TestOwdInventoryHelpers(t *testing.T) {
	for raw, want := range map[string]int{"123": 123, ` "45" `: 45, "0": 0, `{"count":7}`: 7} {
		if got, err := owdInventoryParseCount([]byte(raw)); err != nil || got != want {
			t.Fatalf("parse %q: %d %v", raw, got, err)
		}
	}
	if _, err := owdInventoryParseCount([]byte(`{"error":"x"}`)); err == nil {
		t.Fatal("non-numeric bodies are errors")
	}
	if cr := owdInventoryCheapest(`{"cheapestRegistrar":{"name":"porkbun","price":"72.4"},"godaddy":"1"}`); cr == nil || cr.Name != "porkbun" {
		t.Fatalf("cheapestRegistrar wins: %+v", cr)
	}
	if cr := owdInventoryCheapest(`{"namecheap":"9.98","godaddy":"11.99","porkbun":null,"registrars":[{"name":"gandi","price":"15"}]}`); cr == nil || cr.Name != "namecheap" || cr.Price != "9.98" {
		t.Fatalf("lowest registrar field: %+v", cr)
	}
	if owdInventoryCheapest(`{"slug":"ai"}`) != nil || owdInventoryCheapest("junk") != nil {
		t.Fatal("no prices yields nil")
	}
	tlds := []owdTLD{{Slug: "ai", Type: "ccTld", MinPrice: "72.4"}, {Slug: "com", Type: "gTld", MinPrice: "9.5"}, {Slug: "x", Type: "gTld", MinPrice: ""}}
	if got := owdInventoryFilter(tlds, "gtld", nil); len(got) != 2 {
		t.Fatalf("type filter: %+v", got)
	}
	twenty := 20.0
	if got := owdInventoryFilter(tlds, "", &twenty); len(got) != 1 || got[0].Slug != "com" {
		t.Fatalf("max-price filter drops unknown prices: %+v", got)
	}
	rows := []owdInventoryRow{{TLD: "b", AvailableCount: 5, MinPrice: "10", Top10m: 1}, {TLD: "a", AvailableCount: 5, MinPrice: "2", Top10m: 3}, {TLD: "c", AvailableCount: 9, MinPrice: "", Top10m: 2}}
	for key, want := range map[string]string{"count": "c,a,b", "price": "a,b,c", "top10m": "a,c,b"} {
		r := append([]owdInventoryRow{}, rows...)
		owdInventorySort(r, key)
		if got := owdInventoryTLDs(r); got != want {
			t.Fatalf("sort %s: got %s want %s", key, got, want)
		}
	}
}
