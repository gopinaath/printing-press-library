// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

func TestNovelWordsMineHelpWires(t *testing.T) {
	testenv.Isolate(t)
	out, _, err := owdNovelRun(t, "words", "mine", "--help")
	if err != nil {
		t.Fatalf("words mine --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "mine", "use 'domains intersect' (lifetime pass)", "--any", "--glob", "--refresh", "--max-pages"} {
		if !strings.Contains(out, want) {
			t.Fatalf("words mine --help missing %q in output:\n%s", want, out)
		}
	}
}

func owdMineSeed(t *testing.T) {
	t.Helper()
	db := owdNovelTestStore(t)
	defer db.Close()
	fixtures := map[string][]string{
		"smart":  {"positive", "adjectives"},
		"cloud":  {"tech", "nouns"},
		"sync":   {"tech", "verbs"},
		"studio": {"nouns"},
		"solo":   {"adjectives"},
		"server": {"tech", "nouns"},
		"so_o":   {"other"},
	}
	items := make([]json.RawMessage, 0, len(fixtures))
	for slug, cats := range fixtures {
		parts := make([]string, 0, len(cats))
		for _, c := range cats {
			parts = append(parts, fmt.Sprintf(`{"categorySlug":%q}`, c))
		}
		items = append(items, json.RawMessage(fmt.Sprintf(`{"slug":%q,"categories":[%s]}`, slug, strings.Join(parts, ","))))
	}
	if _, _, err := db.UpsertBatch("words", items); err != nil {
		t.Fatal(err)
	}
}

func owdMineWords(rows []owdMineRow) string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Word)
	}
	return strings.Join(out, ",")
}

func TestNovelWordsMineFilters(t *testing.T) {
	testenv.Isolate(t)
	owdMineSeed(t)
	var rows []owdMineRow
	errOut, err := owdNovelRunJSON(t, &rows, "words", "mine", "--category", "tech")
	if err != nil || owdMineWords(rows) != "cloud,server,sync" {
		t.Fatalf("single category: %s err=%v", owdMineWords(rows), err)
	}
	if !strings.Contains(errOut, "--refresh") {
		t.Fatalf("small dictionary must hint at --refresh: %q", errOut)
	}
	for _, r := range rows {
		if !slices.Contains(r.Categories, "tech") || r.Length != len(r.Word) {
			t.Fatalf("row detail: %+v", r)
		}
	}
	if _, err := owdNovelRunJSON(t, &rows, "words", "mine", "--category", "tech,nouns"); err != nil || owdMineWords(rows) != "cloud,server" {
		t.Fatalf("AND categories: %s err=%v", owdMineWords(rows), err)
	}
	if _, err := owdNovelRunJSON(t, &rows, "words", "mine", "--category", "tech,nouns", "--any"); err != nil || owdMineWords(rows) != "cloud,server,studio,sync" {
		t.Fatalf("OR categories: %s err=%v", owdMineWords(rows), err)
	}
	if _, err := owdNovelRunJSON(t, &rows, "words", "mine", "--glob", "s*o"); err != nil || owdMineWords(rows) != "so_o,solo,studio" {
		t.Fatalf("glob: %s err=%v", owdMineWords(rows), err)
	}
	if _, err := owdNovelRunJSON(t, &rows, "words", "mine", "--prefix", "s", "--max-len", "4"); err != nil || owdMineWords(rows) != "so_o,solo,sync" {
		t.Fatalf("prefix + max-len: %s err=%v", owdMineWords(rows), err)
	}
	if _, err := owdNovelRunJSON(t, &rows, "words", "mine", "--prefix", "so_"); err != nil || owdMineWords(rows) != "so_o" {
		t.Fatalf("prefix escapes LIKE wildcards: %s err=%v", owdMineWords(rows), err)
	}
	if _, err := owdNovelRunJSON(t, &rows, "words", "mine", "--min-len", "6"); err != nil || owdMineWords(rows) != "server,studio" {
		t.Fatalf("min-len: %s err=%v", owdMineWords(rows), err)
	}
	if _, err := owdNovelRunJSON(t, &rows, "words", "mine", "--limit", "1"); err != nil || len(rows) != 1 {
		t.Fatalf("limit: %s err=%v", owdMineWords(rows), err)
	}
	if _, err := owdNovelRunJSON(t, &rows, "words", "mine", "--category", "gods"); err != nil || len(rows) != 0 || rows == nil {
		t.Fatalf("no match prints []: %v err=%v", rows, err)
	}
	for _, args := range [][]string{
		{"words", "mine", "--data-source", "live", "--json"},
		{"words", "mine", "--category", "nope", "--json"},
		{"words", "mine", "--category", "tech", "--any", "--json"},
		{"words", "mine", "--min-len", "5", "--max-len", "2", "--json"},
	} {
		if _, _, err := owdNovelRun(t, args...); ExitCode(err) != 2 {
			t.Fatalf("%v: want usage error, got %v", args, err)
		}
	}
	var env dryRunResult
	if _, err := owdNovelRunJSON(t, &env, "words", "mine", "--category", "tech", "--dry-run"); err != nil || !env.DryRun || env.Action != "words mine" {
		t.Fatalf("dry-run: %+v err=%v", env, err)
	}
}

func TestNovelWordsMineRefresh(t *testing.T) {
	pages := 0
	owdNovelTestServer(t, owdNovelJSONHandler(map[string]func(r *http.Request) (int, string){
		"/api/words": func(r *http.Request) (int, string) {
			pages++
			if r.URL.Query().Get("page") != "1" {
				return 200, `[]`
			}
			return 200, `[{"slug":"aurora","categories":[{"categorySlug":"nouns"},{"categorySlug":"positive"}]},{"slug":"zen","categories":[{"categorySlug":"positive"}]}]`
		},
	}))
	var rows []owdMineRow
	errOut, err := owdNovelRunJSON(t, &rows, "words", "mine", "--refresh", "--max-pages", "2", "--category", "positive")
	if err != nil || owdMineWords(rows) != "aurora,zen" || pages != 1 {
		t.Fatalf("refresh: %s pages=%d err=%v stderr=%q", owdMineWords(rows), pages, err, errOut)
	}
	if !strings.Contains(errOut, "refreshed the whole dictionary") {
		t.Fatalf("short page completes the refresh: %q", errOut)
	}
	db := owdNovelTestStore(t)
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM words`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("words table grew to %d (err=%v)", n, err)
	}
	if _, synced, _, err := db.GetSyncState("words"); err != nil || synced.IsZero() {
		t.Fatalf("a complete refresh stamps sync_state: %v %v", synced, err)
	}
}

func TestOwdMineQuery(t *testing.T) {
	q, args := owdMineQuery(owdMineOpts{Categories: []string{"tech", "nouns"}, MinLen: 4, MaxLen: 6, Prefix: "s%", Glob: "s*o", Limit: 5})
	for _, want := range []string{"json_each(json_extract(w.data, '$.categories'))", "IN (?, ?)", "length(w.slug) >= ?", "length(w.slug) <= ?", `LIKE ? ESCAPE '\'`, "GLOB ?", "GROUP BY w.slug HAVING COUNT(DISTINCT", "ORDER BY w.slug LIMIT ?"} {
		if !strings.Contains(q, want) {
			t.Fatalf("query missing %q:\n%s", want, q)
		}
	}
	if fmt.Sprint(args) != fmt.Sprint([]any{"tech", "nouns", 4, 6, `s\%%`, "s*o", 2, 5}) {
		t.Fatalf("args: %v", args)
	}
	q, args = owdMineQuery(owdMineOpts{Categories: []string{"tech", "nouns"}, Any: true})
	if strings.Contains(q, "HAVING") || !strings.Contains(q, "GROUP BY w.slug") || len(args) != 2 {
		t.Fatalf("any query: %s %v", q, args)
	}
	q, args = owdMineQuery(owdMineOpts{})
	if strings.Contains(q, "JOIN") || strings.Contains(q, "GROUP BY") || strings.Contains(q, "LIMIT") || len(args) != 0 {
		t.Fatalf("plain query: %s %v", q, args)
	}
	if owdMineEscapeLike(`a_b%c\`) != `a\_b\%c\\` {
		t.Fatalf("escape: %q", owdMineEscapeLike(`a_b%c\`))
	}
	cats := owdMineCategoriesOf(`{"slug":"x","categories":[{"categorySlug":"tech"},{"categorySlug":"adjectives"},{"categorySlug":"tech"}]}`)
	if strings.Join(cats, ",") != "adjectives,tech" {
		t.Fatalf("categories: %v", cats)
	}
	if got := owdMineCategoriesOf("junk"); len(got) != 0 || got == nil {
		t.Fatal("bad data yields an empty, non-nil slice")
	}
}
