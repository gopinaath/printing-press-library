// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/client"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
	"github.com/spf13/cobra"
)

// owdNovelTestServer sandboxes the home directory, serves the handler, and
// points the CLI's base URL at it for the rest of the test.
func owdNovelTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv("ONEWORD_DOMAINS_BASE_URL", srv.URL)
	return srv
}

// owdNovelRun executes the root command and returns stdout, stderr, and the error.
func owdNovelRun(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// owdNovelRunJSON runs the command with --json and decodes stdout into v.
func owdNovelRunJSON(t *testing.T, v any, args ...string) (string, error) {
	t.Helper()
	out, errOut, err := owdNovelRun(t, append(args, "--json")...)
	if err != nil {
		return errOut, err
	}
	if uerr := json.Unmarshal([]byte(out), v); uerr != nil {
		t.Fatalf("stdout is not JSON (%v):\n%s\nstderr:\n%s", uerr, out, errOut)
	}
	return errOut, nil
}

// owdNovelTestStore opens the sandboxed store the CLI itself will open.
func owdNovelTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, _, err := owdOpenStore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// owdNovelJSONHandler answers every request with a per-route function.
func owdNovelJSONHandler(routes map[string]func(r *http.Request) (int, string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fn, ok := routes[r.URL.Path]
		if !ok {
			for prefix, f := range routes {
				if strings.HasSuffix(prefix, "/") && strings.HasPrefix(r.URL.Path, prefix) {
					fn, ok = f, true
					break
				}
			}
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		status, body := fn(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func TestOwdNovelSourceErr(t *testing.T) {
	// annotated returns a command named name under a root, carrying the
	// pp:data-source annotation owdSourceErr reads its strategy from.
	annotated := func(strategy string, path ...string) *cobra.Command {
		root := &cobra.Command{Use: "oneword-domains-pp-cli"}
		parent := root
		for _, name := range path {
			child := &cobra.Command{Use: name + " <arg>"}
			parent.AddCommand(child)
			parent = child
		}
		parent.Annotations = map[string]string{"pp:data-source": strategy}
		return parent
	}
	cases := []struct {
		strategy, source string
		wantCode         int
	}{
		{"live", "local", 2},
		{"live", "live", 0},
		{"live", "auto", 0},
		{"local", "live", 2},
		{"local", "local", 0},
		{"auto", "local", 0},
		{"auto", "live", 0},
	}
	for _, c := range cases {
		err := owdSourceErr(annotated(c.strategy, "x"), &rootFlags{dataSource: c.source})
		got := 0
		if err != nil {
			got = ExitCode(err)
		}
		if got != c.wantCode {
			t.Fatalf("%s/%s: exit %d want %d (err=%v)", c.strategy, c.source, got, c.wantCode, err)
		}
	}
	if err := owdSourceErr(annotated("local", "words", "mine"), &rootFlags{dataSource: "live"}); err == nil || err.Error() != "words mine reads the local store only; --data-source live is not supported" {
		t.Fatalf("the message names the command path without the root: %v", err)
	}
	// Every real command that calls owdSourceErr produces the same message
	// it did when the strategy and name were passed as literals.
	for _, c := range []struct{ path, strategy string }{
		{"check", "live"}, {"compare", "live"}, {"gpt generate", "live"}, {"brainstorm", "live"},
		{"listings watch", "live"}, {"domains intersect", "live"}, {"tlds inventory", "live"}, {"words mine", "local"},
	} {
		cmd, _, err := RootCmd().Find(strings.Fields(c.path))
		if err != nil {
			t.Fatal(err)
		}
		flip := map[string]string{"live": "local", "local": "live"}[c.strategy]
		what := map[string]string{"live": "the live API", "local": "the local store"}[c.strategy]
		want := c.path + " reads " + what + " only; --data-source " + flip + " is not supported"
		if err := owdSourceErr(cmd, &rootFlags{dataSource: flip}); err == nil || err.Error() != want {
			t.Fatalf("%s: got %v want %q", c.path, err, want)
		}
	}
	// Under --json the usage error is also the stdout envelope for machine callers.
	cmd := annotated("live", "check")
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := owdSourceErr(cmd, &rootFlags{dataSource: "local", asJSON: true})
	var env map[string]any
	if ExitCode(err) != 2 || json.Unmarshal(out.Bytes(), &env) != nil || env["code"] != float64(2) || !strings.Contains(fmt.Sprint(env["error"]), "--data-source local is not supported") {
		t.Fatalf("json envelope: err=%v out=%q", err, out.String())
	}
	out.Reset()
	if err := owdSourceErr(cmd, &rootFlags{dataSource: "local"}); ExitCode(err) != 2 || out.Len() != 0 {
		t.Fatalf("without --json nothing is written to stdout: err=%v out=%q", err, out.String())
	}
}

func TestOwdAPIErr(t *testing.T) {
	if owdAPIErr(nil, nil, nil) != nil {
		t.Fatal("nil passes through")
	}
	typed := notFoundErr(errors.New("x"))
	if owdAPIErr(nil, nil, typed) != typed {
		t.Fatal("typed errors pass through untouched")
	}
	session := owdAPIErr(nil, nil, &client.APIError{Method: "GET", Path: "/api/domains", StatusCode: 401})
	if ExitCode(session) != 4 || !strings.Contains(session.Error(), "auth login --chrome") {
		t.Fatalf("401 must become the session auth error: %v", session)
	}
	if ExitCode(owdAPIErr(nil, nil, &client.APIError{Method: "GET", Path: "/x", StatusCode: 404})) != 3 {
		t.Fatal("404 should classify as not found")
	}
	if ExitCode(owdAPIErr(nil, nil, errors.New("dial tcp: refused"))) != 5 {
		t.Fatal("plain errors are API errors")
	}
	// The unknown-word mapping belongs to owdCheckDomain (see
	// TestOwdCheckDomainUnknownWordIsNotFound); a raw 500 from another
	// /api/domains/ route is not an unknown word.
	count := owdAPIErr(nil, nil, &client.APIError{Method: "GET", Path: "/api/domains/count", StatusCode: 500})
	if ExitCode(count) == 3 || strings.Contains(count.Error(), "not in the One Word Domains dictionary") {
		t.Fatalf("a raw 500 from /api/domains/count must not read as an unknown word: %v", count)
	}
	// Under --json the classified error is also the stdout envelope; typed
	// input and plain runs write nothing.
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	err := owdAPIErr(cmd, &rootFlags{asJSON: true}, &client.APIError{Method: "GET", Path: "/api/domains", StatusCode: 403})
	var env map[string]any
	if ExitCode(err) != 4 || json.Unmarshal(out.Bytes(), &env) != nil || env["code"] != float64(4) || !strings.Contains(fmt.Sprint(env["error"]), "auth login --chrome") {
		t.Fatalf("json envelope: err=%v out=%q", err, out.String())
	}
	out.Reset()
	if err := owdAPIErr(cmd, &rootFlags{}, errors.New("boom")); ExitCode(err) != 5 || out.Len() != 0 {
		t.Fatalf("without --json nothing is written: err=%v out=%q", err, out.String())
	}
	if err := owdAPIErr(cmd, &rootFlags{asJSON: true}, typed); err != typed || out.Len() != 0 {
		t.Fatalf("typed errors are passed through without an envelope: err=%v out=%q", err, out.String())
	}
	errs := []cliutil.FanoutError{{Source: "a", Err: errors.New("boom")}, {Source: "b", Err: &client.APIError{Method: "GET", Path: "/x", StatusCode: 403}}}
	if err := owdFirstSessionErr(errs); ExitCode(err) != 4 {
		t.Fatalf("the first 401/403 among fan-out errors is the session error: %v", err)
	}
	if owdFirstSessionErr(errs[:1]) != nil || owdFirstSessionErr(nil) != nil {
		t.Fatal("no session error without a 401/403")
	}
}

func TestOwdTypedErr(t *testing.T) {
	if owdTypedErr(nil, nil, nil) != nil {
		t.Fatal("nil passes through")
	}
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	session := authErr(&owdSessionError{method: "GET", path: "/api/domains/smart.com"})
	err := owdTypedErr(cmd, &rootFlags{asJSON: true}, session)
	var env map[string]any
	if err != session || json.Unmarshal(out.Bytes(), &env) != nil || env["code"] != float64(4) || !strings.Contains(fmt.Sprint(env["error"]), "auth login --chrome") {
		t.Fatalf("typed errors are returned as is with the json envelope: err=%v out=%q", err, out.String())
	}
	out.Reset()
	// An untyped local-store failure is exit 1 and still gets an envelope.
	plain := errors.New("recording listings: disk full")
	env = nil
	if err := owdTypedErr(cmd, &rootFlags{asJSON: true}, plain); err != plain || json.Unmarshal(out.Bytes(), &env) != nil || env["code"] != float64(1) || env["error"] != "recording listings: disk full" {
		t.Fatalf("untyped errors: err=%v out=%q", err, out.String())
	}
	out.Reset()
	if err := owdTypedErr(cmd, &rootFlags{}, session); err != session || out.Len() != 0 {
		t.Fatalf("without --json nothing is written: err=%v out=%q", err, out.String())
	}
	if err := owdTypedErr(nil, &rootFlags{asJSON: true}, session); err != session {
		t.Fatalf("nil cmd: %v", err)
	}
}

func TestOwdNovelFailuresAndWarn(t *testing.T) {
	errs := []cliutil.FanoutError{{Source: "a.com", Err: errors.New("boom")}, {Source: "b.com"}}
	got := owdFailures(errs)
	if len(got) != 2 || got[0].Source != "a.com" || got[0].Error != "boom" || got[1].Error != "" {
		t.Fatalf("unexpected: %+v", got)
	}
	var buf bytes.Buffer
	owdWarnFailuresListed(&buf, nil, 3, "checks")
	owdWarnFailuresInline(&buf, nil, 3, "checks")
	if buf.Len() != 0 {
		t.Fatal("no warning without failures")
	}
	owdWarnFailuresListed(&buf, errs, 3, "checks")
	if !strings.Contains(buf.String(), "2 of 3 checks failed") || !strings.Contains(buf.String(), "fetch_failures") {
		t.Fatalf("warning must carry the denominator and point at fetch_failures: %q", buf.String())
	}
	buf.Reset()
	owdWarnFailuresInline(&buf, errs, 3, "checks")
	if !strings.Contains(buf.String(), "2 of 3 checks failed") || strings.Contains(buf.String(), "fetch_failures") || !strings.Contains(buf.String(), "warn: a.com: boom") {
		t.Fatalf("without a fetch_failures key each failure is listed inline: %q", buf.String())
	}
}

func TestOwdNovelScanStringAndRounding(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if owdScanString(ts) != "2026-01-02T03:04:05Z" || owdScanString([]byte("x")) != "x" || owdScanString(nil) != "" || owdScanString(7) != "7" {
		t.Fatal("scan string normalization failed")
	}
	if owdScanTime(ts) != ts || owdScanTime("2026-01-02T03:04:05Z") != ts || owdScanTime([]byte("2026-01-02 03:04:05")) != ts || !owdScanTime("garbage").IsZero() || !owdScanTime(nil).IsZero() {
		t.Fatal("scan time normalization failed")
	}
	if owdRound2(1.005) != 1.0 || owdRound2(2.346) != 2.35 || owdRound2(0.166) != 0.17 {
		t.Fatal("round2")
	}
	if owdRoundPct(93.548) != 93.5 || owdRoundPct(10.497) != 10.5 || owdRoundPct(100) != 100 || owdRoundPct(0) != 0 {
		t.Fatal("roundPct")
	}
}
