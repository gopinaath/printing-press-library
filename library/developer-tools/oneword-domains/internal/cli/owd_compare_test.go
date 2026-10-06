// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

func TestOwdCompareHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"compare", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("compare --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "compare", "smart.com smart.io oasis.ai"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("compare --help missing %q:\n%s", want, out.String())
		}
	}
}

func TestOwdCompareResultShape(t *testing.T) {
	out := owdCompareResult{Rows: make([]owdCheckRow, 0), FetchFailures: make([]owdFailure, 0)}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"rows":[],"cheapest_available":"","fetch_failures":[]}` {
		t.Fatalf("empty envelope must use [] not null: %s", b)
	}
	f, _ := json.Marshal(owdFailure{Source: "x.com", Error: "boom"})
	if string(f) != `{"source":"x.com","error":"boom"}` {
		t.Fatalf("fetch_failures elements share the {source, error} shape: %s", f)
	}
}

func TestOwdCompareLive(t *testing.T) {
	seen := owdCheckFixtureServer(t)
	var out owdCompareResult
	errOut, err := owdNovelRunJSON(t, &out, "compare", "smart.com", "Smart.AI", "smart.io", "zzqq.com", "smart.com")
	if err != nil {
		t.Fatal(err)
	}
	domains := make([]string, 0, len(out.Rows))
	for _, r := range out.Rows {
		domains = append(domains, r.Domain)
	}
	if strings.Join(domains, ",") != "smart.io,smart.ai,smart.com,zzqq.com" || out.CheapestAvailable != "smart.io" {
		t.Fatalf("rows: %v cheapest=%q", domains, out.CheapestAvailable)
	}
	if out.Rows[1].PopularityPct != 93.5 || out.Rows[3].Error != "not in dictionary" || len(out.Rows[3].Suggestions) != 1 {
		t.Fatalf("row detail: %+v", out.Rows)
	}
	if len(out.FetchFailures) != 1 || out.FetchFailures[0].Source != "io" || !strings.Contains(errOut, "1 of 8 fetches failed") || !strings.Contains(errOut, "fetch_failures") {
		t.Fatalf("the io detail failure is listed under fetch_failures: %+v stderr=%q", out.FetchFailures, errOut)
	}
	before := len(owdSeenDomainRequests(seen))
	for _, arg := range []string{"smart/../x.com", "smart?x.com", "smart..com", "smart"} {
		if _, _, err := owdNovelRun(t, "compare", arg, "--json"); ExitCode(err) != 2 || !strings.Contains(err.Error(), arg) {
			t.Fatalf("compare %q: want usage error naming it, got %v", arg, err)
		}
	}
	if _, _, err := owdNovelRun(t, "compare", "smart.xyz", "--json"); ExitCode(err) != 2 || !strings.Contains(err.Error(), "xyz") {
		t.Fatalf("unknown TLD must be a usage error: %v", err)
	}
	if len(owdSeenDomainRequests(seen)) != before {
		t.Fatalf("rejected arguments must not reach the availability route: %v", *seen)
	}
	for _, args := range [][]string{
		{"compare", "--data-source", "local", "smart.com", "--json"},
		{"compare", "--json"},
	} {
		if _, _, err := owdNovelRun(t, args...); ExitCode(err) != 2 {
			t.Fatalf("%v: want usage error, got %v", args, err)
		}
	}
}
