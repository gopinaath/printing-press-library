// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestOwdGPTBody(t *testing.T) {
	b := owdGPTBody(owdGPTRequest{Type: "portmanteau", MinLen: 7, MaxLen: 12, Word: "Open", Position: "prefix", TLD: ".COM", Exclude: []string{"openlumix.com"}})
	if b["tld"] != "com" || b["word"] != "open" || b["position"] != "prefix" {
		t.Fatalf("unexpected body: %+v", b)
	}
	if _, has := b["context"]; has {
		t.Fatal("empty context must be omitted")
	}
	if ex, ok := b["domains"].([]string); !ok || len(ex) != 1 {
		t.Fatalf("exclude list missing: %+v", b)
	}
	b2 := owdGPTBody(owdGPTRequest{Type: "random", Context: "ctx", MinLen: 6, MaxLen: 10, Position: "prefix", TLD: "ai"})
	if _, has := b2["word"]; has {
		t.Fatal("word must be omitted when empty")
	}
	if b2["context"] != "ctx" {
		t.Fatal("context should pass through")
	}
}

// owdGPTStreamServer serves the DomainsGPT stream and records the last body.
func owdGPTStreamServer(t *testing.T, stream string) (*map[string]any, *int) {
	t.Helper()
	body := &map[string]any{}
	status := new(int)
	*status = 200
	owdNovelTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/gpt/generate" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var got map[string]any
		_ = json.NewDecoder(r.Body).Decode(&got)
		*body = got
		w.WriteHeader(*status)
		_, _ = w.Write([]byte(stream))
	}))
	return body, status
}

func TestOwdGptGenerateLive(t *testing.T) {
	body, status := owdGPTStreamServer(t, `{ "domain" : "openlumix.com", "available" : true }{ "domain" : "openvance.com", "available" : false }`)
	var rows []owdGenerated
	if _, err := owdNovelRunJSON(t, &rows, "gpt", "generate", "--type", "portmanteau", "--word", "Open", "--position", "prefix", "--tld", ".COM", "--exclude", "a.com,A.com,b.com"); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Domain != "openlumix.com" || !rows[0].Available || rows[1].Available {
		t.Fatalf("parsed stream: %+v", rows)
	}
	if (*body)["tld"] != "com" || (*body)["word"] != "open" || (*body)["type"] != "portmanteau" {
		t.Fatalf("request body: %+v", *body)
	}
	if ex, ok := (*body)["domains"].([]any); !ok || len(ex) != 2 {
		t.Fatalf("exclude list is de-duplicated: %+v", (*body)["domains"])
	}
	if _, err := owdNovelRunJSON(t, &rows, "gpt", "generate", "--available-only"); err != nil || len(rows) != 1 {
		t.Fatalf("available-only: %+v err=%v", rows, err)
	}
	out, _, err := owdNovelRun(t, "gpt", "generate", "--raw")
	if err != nil || !strings.HasPrefix(out, `{ "domain" : "openlumix.com"`) {
		t.Fatalf("raw stream: err=%v out=%q", err, out)
	}
	*status = 429
	if _, _, err := owdNovelRun(t, "gpt", "generate", "--json"); ExitCode(err) != 7 {
		t.Fatalf("429 must be the rate-limit exit: %v", err)
	}
	*status = 401
	if _, _, err := owdNovelRun(t, "gpt", "generate", "--json"); ExitCode(err) != 4 {
		t.Fatalf("401 must be the auth exit: %v", err)
	}
	for _, args := range [][]string{
		{"gpt", "generate", "--data-source", "local", "--json"},
		{"gpt", "generate", "--type", "weird", "--json"},
		{"gpt", "generate", "--word", "open", "--position", "middle", "--json"},
		{"gpt", "generate", "--min-length", "9", "--max-length", "3", "--json"},
	} {
		if _, _, err := owdNovelRun(t, args...); ExitCode(err) != 2 {
			t.Fatalf("%v: want usage error, got %v", args, err)
		}
	}
}
