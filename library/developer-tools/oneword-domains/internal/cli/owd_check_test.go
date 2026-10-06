// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

// owdCheckFixtureServer serves the routes check/compare need: two TLDs, a
// dictionary with smart and open, availability for smart.* and open.*, a 500
// for every other word, and the TLD detail route. It returns the request log.
func owdCheckFixtureServer(t *testing.T) *[]string {
	t.Helper()
	return owdCheckFixture(t, nil)
}

// owdCheckFixture is the fixture behind owdCheckFixtureServer with a settable
// status for the availability route: pointing domainStatus at 401 turns it
// into a site without a lifetime-pass session.
func owdCheckFixture(t *testing.T, domainStatus *int) *[]string {
	t.Helper()
	var mu sync.Mutex
	seen := &[]string{}
	record := func(r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		*seen = append(*seen, r.URL.Path)
	}
	tlds := `[{"slug":"com","type":"gTld","structure":"normal","top10m":900,"totalReg":5000,"views":10,"minPrice":"9.5"},{"slug":"ai","type":"ccTld","structure":"normal","top10m":30,"totalReg":1000,"views":5,"minPrice":"72.4"},{"slug":"io","type":"ccTld","structure":"normal","top10m":100,"totalReg":800,"views":3,"minPrice":"30"}]`
	owdNovelTestServer(t, owdNovelJSONHandler(map[string]func(r *http.Request) (int, string){
		"/api/tlds": func(r *http.Request) (int, string) { record(r); return 200, tlds },
		"/api/words": func(r *http.Request) (int, string) {
			record(r)
			switch r.URL.Query().Get("query") {
			case "smart":
				return 200, `[{"slug":"smart"},{"slug":"smartly"}]`
			case "open":
				return 200, `[{"slug":"open"}]`
			case "zzq":
				return 200, `[{"slug":"zzz"}]`
			}
			return 200, `[]`
		},
		"/api/domains/": func(r *http.Request) (int, string) {
			record(r)
			if domainStatus != nil && *domainStatus != 0 {
				return *domainStatus, `{"error":"unauthorized"}`
			}
			d := strings.TrimPrefix(r.URL.Path, "/api/domains/")
			switch d {
			case "smart.com":
				return 200, `{"slug":"smart.com","available":false,"premium":false,"price":null,"tldSlug":"com","aftermarket":false,"tldCount":6}`
			case "smart.ai":
				return 200, `{"slug":"smart.ai","available":true,"premium":true,"price":"499","tldSlug":"ai","aftermarket":false,"tldCount":6}`
			case "smart.io":
				return 200, `{"slug":"smart.io","available":true,"premium":false,"price":null,"tldSlug":"io","aftermarket":false,"tldCount":6}`
			case "open.com", "open.ai", "open.io":
				return 200, fmt.Sprintf(`{"slug":%q,"available":false,"premium":false,"price":null,"tldSlug":"x","aftermarket":true,"tldCount":2}`, d)
			}
			return 500, `Internal Server Error`
		},
		"/api/tlds/": func(r *http.Request) (int, string) {
			record(r)
			tld := strings.TrimPrefix(r.URL.Path, "/api/tlds/")
			if tld == "io" {
				return 404, `{"error":"no"}`
			}
			return 200, fmt.Sprintf(`{"slug":%q,"type":"gTld","registrars":[{"name":"porkbun","price":"9.13"},{"name":"godaddy","price":"12"}],"cheapestRegistrar":{"name":"porkbun","price":"9.13"}}`, tld)
		},
	}))
	return seen
}

func owdSeenDomainRequests(seen *[]string) []string {
	out := make([]string, 0)
	for _, p := range *seen {
		if strings.HasPrefix(p, "/api/domains/") || strings.HasPrefix(p, "/api/tlds/") {
			out = append(out, p)
		}
	}
	return out
}

func TestOwdCheckHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"check", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("check --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "check", "--tld", "--file", "--available-only", "--max-price", "--concurrency", "--max-checks", "check smart --tld com,io,ai --json"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("check --help missing %q:\n%s", want, out.String())
		}
	}
}

func TestOwdCheckRejectsUnsafeArgBeforeAnyRequest(t *testing.T) {
	owdNovelTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP request: %s %s", r.Method, r.URL.String())
		http.NotFound(w, r)
	}))
	for _, arg := range []string{"smart/../x", "smart?x", "smart#x", "smart..x", "sm/art", "smart/x.com"} {
		_, _, err := owdNovelRun(t, "check", arg, "--tld", "com", "--json")
		if ExitCode(err) != 2 || !strings.Contains(err.Error(), arg) {
			t.Fatalf("check %q: want usage error naming it, got %v", arg, err)
		}
	}
	for _, args := range [][]string{
		{"check", "--data-source", "local", "smart", "--json"},
		{"check", "smart", "oasis", "--json"},
		{"check", "--tld", "com", "--json"},
		{"check", "smart", "--max-price", "-1", "--json"},
		{"check", "smart", "--max-checks", "-1", "--json"},
	} {
		if _, _, err := owdNovelRun(t, args...); ExitCode(err) != 2 {
			t.Fatalf("%v: want usage error, got %v", args, err)
		}
	}
	var env dryRunResult
	if _, err := owdNovelRunJSON(t, &env, "check", "smart", "--dry-run"); err != nil || !env.DryRun || env.Action != "check" {
		t.Fatalf("dry-run: %+v err=%v", env, err)
	}
}

func TestOwdCheckLive(t *testing.T) {
	seen := owdCheckFixtureServer(t)
	var rows []owdCheckRow
	errOut, err := owdNovelRunJSON(t, &rows, "check", "smart", "--tld", ".COM,ai,com")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Domain != "smart.ai" || rows[1].Domain != "smart.com" {
		t.Fatalf("available first, one row per TLD: %+v", rows)
	}
	if rows[0].PopularityPct != 93.5 || rows[0].Available == nil || !*rows[0].Available || !rows[0].Premium || rows[0].Price == nil || *rows[0].Price != "499" || rows[0].MinPrice != "72.4" {
		t.Fatalf("row detail: %+v", rows[0])
	}
	if rows[0].CheapestRegistrar == nil || rows[0].CheapestRegistrar.Name != "porkbun" || rows[1].CheapestRegistrar == nil {
		t.Fatalf("cheapest registrar must be joined: %+v", rows)
	}
	if strings.Contains(errOut, "warning") {
		t.Fatalf("clean run must not warn: %q", errOut)
	}
	db := owdNovelTestStore(t)
	var checks, prices int
	_ = db.DB().QueryRow(`SELECT COUNT(*) FROM owd_domain_checks`).Scan(&checks)
	_ = db.DB().QueryRow(`SELECT COUNT(DISTINCT tld) FROM owd_tld_prices WHERE registrar <> 'min'`).Scan(&prices)
	_ = db.Close()
	if checks != 2 || prices != 2 {
		t.Fatalf("snapshots must be recorded after the fan-out: checks=%d priced TLDs=%d", checks, prices)
	}

	// A TLD whose detail route fails still yields a row, with a warning.
	errOut, err = owdNovelRunJSON(t, &rows, "check", "smart", "--tld", "io", "--available-only")
	if err != nil || len(rows) != 1 || rows[0].Domain != "smart.io" || rows[0].CheapestRegistrar != nil || !strings.Contains(errOut, "1 of 3 fetches failed") {
		t.Fatalf("detail failure: %+v err=%v stderr=%q", rows, err, errOut)
	}
	// Unknown TLD and unknown word.
	if _, _, err := owdNovelRun(t, "check", "smart", "--tld", "com,xyz", "--json"); ExitCode(err) != 2 || !strings.Contains(err.Error(), "xyz") {
		t.Fatalf("unknown TLD must be a usage error: %v", err)
	}
	before := len(owdSeenDomainRequests(seen))
	out, _, err := owdNovelRun(t, "check", "zzqq", "--tld", "com", "--json")
	var notFound map[string]any
	if ExitCode(err) != 3 || json.Unmarshal([]byte(out), &notFound) != nil || notFound["code"] != float64(3) || notFound["word"] != "zzqq" || !strings.Contains(out, "zzz") {
		t.Fatalf("unknown word must be exit 3 with suggestions on stdout: err=%v out=%s", err, out)
	}
	if len(owdSeenDomainRequests(seen)) != before {
		t.Fatalf("the dictionary pre-check must stop unknown words before the availability route: %v", *seen)
	}
}

func TestOwdCheckFileAndStdin(t *testing.T) {
	owdCheckFixtureServer(t)
	path := filepath.Join(t.TempDir(), "words.txt")
	if err := os.WriteFile(path, []byte("smart\n# comment\n\nopen.ai\nsmart\nzzqq\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var rows []owdCheckRow
	errOut, err := owdNovelRunJSON(t, &rows, "check", "--file", path, "--tld", "com")
	if err != nil {
		t.Fatal(err)
	}
	domains := make([]string, 0, len(rows))
	for _, r := range rows {
		domains = append(domains, r.Domain)
	}
	// smart is bare (uses --tld), open.ai is pinned, zzqq is unknown.
	if strings.Join(domains, ",") != "open.ai,smart.com," || rows[2].Word != "zzqq" || rows[2].Error != "not in dictionary" {
		t.Fatalf("file rows: %v %+v", domains, rows)
	}
	if !strings.Contains(errOut, "1 word(s) not in the dictionary") || strings.Contains(errOut, "zzqq") {
		t.Fatalf("unknown --file words are counted on stderr, never echoed: %q", errOut)
	}
	if err := os.WriteFile(path, []byte("smart.xyz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := owdNovelRun(t, "check", "--file", path, "--json"); ExitCode(err) != 2 || !strings.Contains(err.Error(), "line 1: unknown TLD") || strings.Contains(err.Error(), "xyz") {
		t.Fatalf("a pinned unknown TLD is a usage error named by line, never echoed: %v", err)
	}
	// Uppercase is rejected as it is: a key is never lowercased into a word.
	if err := os.WriteFile(path, []byte("smart\nAKIAIOSFODNN7EXAMPLE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := owdNovelRun(t, "check", "--file", path, "--tld", "com", "--json"); ExitCode(err) != 2 || !strings.Contains(err.Error(), "line 2 is not a word") || strings.Contains(strings.ToLower(err.Error()), "akia") {
		t.Fatalf("an uppercase line must be rejected without being echoed: %v", err)
	}
	// A line that is not a plain slug is reported by number only: --file can
	// name any readable file and the MCP server relays the error.
	if err := os.WriteFile(path, []byte("access_token = \"dummy=abc\"\nsmart\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := owdNovelRun(t, "check", "--file", path, "--tld", "com", "--json"); ExitCode(err) != 2 || !strings.Contains(err.Error(), "line 1 is not a word") || strings.Contains(err.Error(), "dummy") || strings.Contains(err.Error(), "access_token") {
		t.Fatalf("a rejected line must not be echoed: %v", err)
	}
	if _, _, err := owdNovelRun(t, "check", "--file", filepath.Join(t.TempDir(), "missing.txt"), "--json"); ExitCode(err) != 2 {
		t.Fatalf("missing file must be a usage error: %v", err)
	}

	cmd := RootCmd()
	cmd.SetArgs([]string{"check", "--file", "-", "--tld", "ai", "--json"})
	cmd.SetIn(strings.NewReader("smart\n"))
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	if err := cmd.Execute(); err != nil || !strings.Contains(out.String(), `"smart.ai"`) {
		t.Fatalf("stdin word list: err=%v out=%s stderr=%s", err, out.String(), errBuf.String())
	}
}

func TestOwdCheckSessionErrorEnvelope(t *testing.T) {
	status := http.StatusUnauthorized
	seen := owdCheckFixture(t, &status)
	// The typed session error raised by the fan-out (not by owdAPIErr) still
	// reaches stdout as the JSON envelope under --json.
	out, _, err := owdNovelRun(t, "check", "smart", "--tld", "com,io", "--json")
	var env map[string]any
	if ExitCode(err) != 4 || json.Unmarshal([]byte(out), &env) != nil || env["code"] != float64(4) || !strings.Contains(fmt.Sprint(env["error"]), "auth login --chrome") {
		t.Fatalf("check: want exit 4 with a json envelope, got err=%v out=%q", err, out)
	}
	if len(owdSeenDomainRequests(seen)) == 0 {
		t.Fatalf("the availability route must have been hit: %v", *seen)
	}
	env = nil
	out, _, err = owdNovelRun(t, "compare", "smart.com", "smart.io", "--json")
	if ExitCode(err) != 4 || json.Unmarshal([]byte(out), &env) != nil || env["code"] != float64(4) {
		t.Fatalf("compare: want exit 4 with a json envelope, got err=%v out=%q", err, out)
	}
	// Without --json nothing is written to stdout.
	out, _, err = owdNovelRun(t, "check", "smart", "--tld", "com,io")
	if ExitCode(err) != 4 || out != "" {
		t.Fatalf("without --json stdout stays empty: err=%v out=%q", err, out)
	}
}

func TestOwdCheckMaxChecksCapsPairs(t *testing.T) {
	seen := owdCheckFixtureServer(t)
	path := filepath.Join(t.TempDir(), "words.txt")
	if err := os.WriteFile(path, []byte("smart\nopen\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var rows []owdCheckRow
	errOut, err := owdNovelRunJSON(t, &rows, "check", "--file", path, "--tld", "com,ai", "--max-checks", "3", "--concurrency", "99")
	if err != nil {
		t.Fatal(err)
	}
	domains := make([]string, 0, len(rows))
	for _, r := range rows {
		domains = append(domains, r.Domain)
	}
	sort.Strings(domains)
	if strings.Join(domains, ",") != "open.com,smart.ai,smart.com" || !strings.Contains(errOut, "only the first 3 are checked") {
		t.Fatalf("the cap applies to word x TLD pairs after expansion: %v stderr=%q", domains, errOut)
	}
	n := 0
	for _, p := range *seen {
		if strings.HasPrefix(p, "/api/domains/") {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("only the capped pairs may reach the availability route, saw %d: %v", n, *seen)
	}
	if _, err := owdNovelRunJSON(t, &rows, "check", "--file", path, "--tld", "com,ai", "--max-checks", "0"); err != nil || len(rows) != 4 {
		t.Fatalf("--max-checks 0 is unlimited outside the dogfood matrix: %d rows err=%v", len(rows), err)
	}
}
