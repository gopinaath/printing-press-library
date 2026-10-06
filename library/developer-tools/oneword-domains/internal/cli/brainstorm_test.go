// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

func TestNovelBrainstormHelpWires(t *testing.T) {
	testenv.Isolate(t)
	out, _, err := owdNovelRun(t, "brainstorm", "--help")
	if err != nil {
		t.Fatalf("brainstorm --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "brainstorm", "use 'gpt generate' instead", "--include-taken", "--sort", "--exclude"} {
		if !strings.Contains(out, want) {
			t.Fatalf("brainstorm --help missing %q in output:\n%s", want, out)
		}
	}
}

func TestNovelBrainstormDryRunAndUsage(t *testing.T) {
	testenv.Isolate(t)
	var env dryRunResult
	if _, err := owdNovelRunJSON(t, &env, "brainstorm", "--type", "brandable", "--tld", "ai", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if !env.DryRun || env.Action != "brainstorm" {
		t.Fatalf("unexpected dry-run envelope: %+v", env)
	}
	cases := [][]string{
		{"brainstorm", "--data-source", "local", "--json"},
		{"brainstorm", "--type", "weird", "--json"},
		{"brainstorm", "--sort", "weird", "--json"},
		{"brainstorm", "--min-length", "9", "--max-length", "3", "--json"},
		{"brainstorm", "--word", "open", "--position", "middle", "--json"},
		{"brainstorm", "--tld", ",", "--json"},
	}
	for _, args := range cases {
		_, _, err := owdNovelRun(t, args...)
		if ExitCode(err) != 2 {
			t.Fatalf("%v: want usage error (exit 2), got %v", args, err)
		}
	}
}

func TestOwdBrainstormFilter(t *testing.T) {
	in := []owdGenerated{
		{Domain: "Alpha.ai", Available: true},
		{Domain: "beta.ai", Available: false},
		{Domain: "alpha.ai", Available: true},
		{Domain: "gamma.ai", Available: true},
		{Domain: "", Available: true},
	}
	got := owdBrainstormFilter(in, false, []string{"gamma.ai"})
	if len(got) != 1 || got[0].Domain != "alpha.ai" {
		t.Fatalf("available-only filter: %+v", got)
	}
	got = owdBrainstormFilter(in, true, nil)
	if len(got) != 3 || got[1].Domain != "beta.ai" || got[1].Available {
		t.Fatalf("include-taken keeps taken names once: %+v", got)
	}
}

func TestOwdBrainstormMinPrice(t *testing.T) {
	cases := []struct {
		name string
		d    *owdTLDDetail
		list string
		want string
	}{
		{"cheapest wins", &owdTLDDetail{CheapestRegistrar: &owdRegistrarPrice{Name: "porkbun", Price: "72.4"}, Registrars: []owdRegistrarPrice{{Name: "godaddy", Price: "209.98"}}}, "1", "72.4"},
		{"lowest registrar when cheapest missing", &owdTLDDetail{Registrars: []owdRegistrarPrice{{Name: "a", Price: "9.5"}, {Name: "b", Price: "3.25"}, {Name: "c", Price: ""}}}, "1", "3.25"},
		{"list fallback", &owdTLDDetail{}, "12", "12"},
		{"nil detail uses list", nil, "8", "8"},
		{"nothing", nil, "", ""},
	}
	for _, c := range cases {
		if got := owdBrainstormMinPrice(c.d, c.list); got != c.want {
			t.Fatalf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestOwdBrainstormSort(t *testing.T) {
	rows := func() []owdBrainstormRow {
		return []owdBrainstormRow{
			{Domain: "zeta.com", Name: "zeta", MinPrice: "10"},
			{Domain: "ab.ai", Name: "ab", MinPrice: "72.4"},
			{Domain: "mid.io", Name: "mid", MinPrice: ""},
			{Domain: "beta.com", Name: "beta", MinPrice: "10"},
		}
	}
	cases := []struct {
		key  string
		want []string
	}{
		{"price", []string{"beta", "zeta", "ab", "mid"}},
		{"length", []string{"ab", "mid", "beta", "zeta"}},
		{"name", []string{"ab", "beta", "mid", "zeta"}},
	}
	for _, c := range cases {
		r := rows()
		owdBrainstormSort(r, c.key)
		got := make([]string, 0, len(r))
		for _, x := range r {
			got = append(got, x.Name)
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Fatalf("sort %s: got %v want %v", c.key, got, c.want)
		}
	}
}

func TestOwdBrainstormBatchID(t *testing.T) {
	at := time.Date(2026, 9, 24, 6, 14, 50, 0, time.UTC)
	id := owdBrainstormBatchID(at)
	if !strings.HasPrefix(id, "2026-09-24T06:14:50Z-") || len(id) != len("2026-09-24T06:14:50Z-")+8 {
		t.Fatalf("unexpected batch id %q", id)
	}
	if owdBrainstormBatchID(at) == id {
		t.Fatal("batch ids must carry a random suffix")
	}
}

func TestOwdBrainstormSaveAndRealWord(t *testing.T) {
	testenv.Isolate(t)
	ctx := context.Background()
	db := owdNovelTestStore(t)
	defer db.Close()
	if _, _, err := db.UpsertBatch("words", []json.RawMessage{json.RawMessage(`{"slug":"smart","categories":[{"categorySlug":"positive"}]}`)}); err != nil {
		t.Fatal(err)
	}
	yes := true
	rows := []owdBrainstormRow{
		{Domain: "smartly.ai", TLD: "ai", Name: "smartly", Available: true, RealWord: &yes, MinPrice: "72.4", CheapestRegistrar: &owdRegistrarPrice{Name: "porkbun", Price: "72.4"}, Batch: "b1"},
		{Domain: "zorbix.ai", TLD: "ai", Name: "zorbix", Available: true, MinPrice: "72.4", Batch: "b1"},
	}
	if err := owdBrainstormSave(ctx, db, rows, owdBrainstormMeta{Type: "brandable", Context: "ctx"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM owd_generations WHERE batch = 'b1' AND type = 'brandable'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("saved rows = %d err=%v", n, err)
	}
	var realWord, cheap any
	if err := db.DB().QueryRow(`SELECT real_word, cheapest_registrar FROM owd_generations WHERE domain = 'zorbix.ai'`).Scan(&realWord, &cheap); err != nil || realWord != nil || cheap != nil {
		t.Fatalf("unknown real_word and missing registrar must be NULL: %v %v %v", realWord, cheap, err)
	}
	names := []string{"smart", "zorbix"}
	if got := owdNewRealWords(ctx, nil, db, names, true, 0).lookup(ctx, "smart"); got == nil || !*got {
		t.Fatal("local dictionary hit must be true")
	}
	if got := owdNewRealWords(ctx, nil, db, names, true, 0).lookup(ctx, "zorbix"); got == nil || *got {
		t.Fatal("miss against a big local dictionary must be false")
	}
	if got := owdNewRealWords(ctx, nil, db, names, false, 0).lookup(ctx, "zorbix"); got != nil {
		t.Fatal("miss against a small dictionary with no live budget must be unknown")
	}
}

func TestNovelBrainstormLive(t *testing.T) {
	generations := 0
	owdNovelTestServer(t, owdNovelJSONHandler(map[string]func(r *http.Request) (int, string){
		"/api/tlds": func(r *http.Request) (int, string) {
			return 200, `[{"slug":"com","type":"gTld","structure":"normal","top10m":1,"totalReg":2,"views":3,"minPrice":"9.5"},{"slug":"ai","type":"ccTld","structure":"normal","top10m":1,"totalReg":2,"views":3,"minPrice":"72.4"}]`
		},
		"/api/gpt/usage": func(r *http.Request) (int, string) { return 200, `{"ok":true}` },
		"/api/gpt/generate": func(r *http.Request) (int, string) {
			generations++
			return 200, `{ "domain" : "openlumix.ai", "available" : true }{ "domain" : "openvance.ai", "available" : false }{ "domain" : "smart.ai", "available" : true }{ "domain" : "smart.ai", "available" : true }`
		},
		"/api/tlds/": func(r *http.Request) (int, string) {
			return 200, `{"slug":"ai","type":"ccTld","registrars":[{"name":"porkbun","price":"72.4"},{"name":"godaddy","price":"209.98"}],"cheapestRegistrar":{"name":"porkbun","price":"72.4"}}`
		},
		"/api/words": func(r *http.Request) (int, string) {
			if r.URL.Query().Get("query") == "smart" {
				return 200, `[{"slug":"smart"}]`
			}
			return 200, `[]`
		},
	}))
	var rows []owdBrainstormRow
	errOut, err := owdNovelRunJSON(t, &rows, "brainstorm", "--type", "brandable", "--tld", "ai", "--min-length", "5", "--max-length", "10")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Name != "openlumix" || rows[1].Name != "smart" || generations != 1 {
		t.Fatalf("available names, de-duplicated, sorted by price then name: %+v gen=%d", rows, generations)
	}
	if rows[0].RealWord == nil || *rows[0].RealWord || rows[1].RealWord == nil || !*rows[1].RealWord {
		t.Fatalf("real-word flag: %+v", rows)
	}
	if rows[0].MinPrice != "72.4" || rows[0].CheapestRegistrar == nil || rows[0].CheapestRegistrar.Name != "porkbun" || rows[0].Batch == "" || rows[0].TLD != "ai" {
		t.Fatalf("pricing: %+v", rows[0])
	}
	if strings.Contains(errOut, "quota is") {
		t.Fatalf("a usage response without the fields must not warn about the quota: %q", errOut)
	}
	db := owdNovelTestStore(t)
	var saved int
	_ = db.DB().QueryRow(`SELECT COUNT(*) FROM owd_generations WHERE type = 'brandable'`).Scan(&saved)
	_ = db.Close()
	if saved != 2 {
		t.Fatalf("batch must be saved locally: %d", saved)
	}
	if _, err := owdNovelRunJSON(t, &rows, "brainstorm", "--tld", "ai", "--include-taken", "--sort", "length", "--limit", "2"); err != nil || len(rows) != 2 || rows[0].Name != "smart" || generations != 2 {
		t.Fatalf("include-taken + sort + limit: %+v err=%v gen=%d", rows, err, generations)
	}
	if _, _, err := owdNovelRun(t, "brainstorm", "--tld", "ai,xyz", "--json"); ExitCode(err) != 2 || !strings.Contains(err.Error(), "xyz") || generations != 2 {
		t.Fatalf("unknown TLDs are rejected before any generation: err=%v gen=%d", err, generations)
	}
}
