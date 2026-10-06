// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func ptrBool(b bool) *bool    { return &b }
func ptrStr(s string) *string { return &s }

func TestOwdRowCost(t *testing.T) {
	cases := []struct {
		name string
		row  owdCheckRow
		want float64
		ok   bool
	}{
		{"premium price wins", owdCheckRow{Price: ptrStr("2500"), CheapestRegistrar: &owdRegistrarPrice{Name: "porkbun", Price: "9.13"}, MinPrice: "8"}, 2500, true},
		{"cheapest registrar", owdCheckRow{CheapestRegistrar: &owdRegistrarPrice{Name: "porkbun", Price: "9.13"}, MinPrice: "8"}, 9.13, true},
		{"min price fallback", owdCheckRow{MinPrice: "72.4"}, 72.4, true},
		{"nothing known", owdCheckRow{}, 0, false},
	}
	for _, c := range cases {
		got, ok := owdRowCost(c.row)
		if ok != c.ok || got != c.want {
			t.Fatalf("%s: got %v/%v want %v/%v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestOwdSortCheckRows(t *testing.T) {
	rows := []owdCheckRow{
		{Domain: "smart.com", Available: ptrBool(false)},
		{Domain: "zed.io", Available: ptrBool(true), MinPrice: "30"},
		{Domain: "bad", Error: "not in dictionary"},
		{Domain: "smart.art", Available: ptrBool(true), CheapestRegistrar: &owdRegistrarPrice{Name: "namecheap", Price: "1.98"}},
		{Domain: "smart.ai", Available: ptrBool(true), MinPrice: "72.4"},
	}
	owdSortCheckRows(rows)
	got := make([]string, 0, len(rows))
	for _, r := range rows {
		got = append(got, r.Domain)
	}
	if strings.Join(got, ",") != "smart.art,zed.io,smart.ai,smart.com,bad" {
		t.Fatalf("order = %v", got)
	}
	if owdCheapestAvailable(rows) != "smart.art" {
		t.Fatalf("cheapest = %q", owdCheapestAvailable(rows))
	}
	if owdCheapestAvailable([]owdCheckRow{{Domain: "x.com", Available: ptrBool(false)}}) != "" {
		t.Fatal("no available rows should yield empty")
	}
}

func TestOwdBuildCheckRow(t *testing.T) {
	dc := &owdDomainCheck{Slug: "smart.com", Available: false, TldCount: 6}
	detail := &owdTLDDetail{Slug: "com", CheapestRegistrar: &owdRegistrarPrice{Name: "porkbun", Price: "9.13"}}
	row := owdBuildCheckRow(dc, "smart", "com", "8.5", detail)
	if row.Available == nil || *row.Available || row.PopularityPct != 93.5 || row.MinPrice != "8.5" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.CheapestRegistrar == nil || row.CheapestRegistrar.Name != "porkbun" {
		t.Fatalf("cheapest registrar not joined: %+v", row)
	}
	bare := owdBuildCheckRow(nil, "smart", "io", "", nil)
	if bare.Available != nil || bare.Domain != "smart.io" || bare.CheapestRegistrar != nil {
		t.Fatalf("nil check should leave availability null: %+v", bare)
	}
}

func TestOwdPrintCheckRowsTable(t *testing.T) {
	taken, free := false, true
	price := "72.4"
	rows := []owdCheckRow{
		{Domain: "smart.com", Available: &taken, TldCount: 6, PopularityPct: 93.5, MinPrice: "1"},
		{Domain: "oasis.ai", Available: &free, Premium: true, Price: &price, TldCount: 40, PopularityPct: 57.0, MinPrice: "72.4", CheapestRegistrar: &owdRegistrarPrice{Name: "porkbun", Price: "72.4"}},
		{Domain: "zzqq.com", Error: "not in dictionary"},
		{Domain: "evil.com\x1b[2J", Available: &free, Price: ptrStr("1\t2"), MinPrice: "3\n4", CheapestRegistrar: &owdRegistrarPrice{Name: "reg\x07", Price: "5"}, Error: "note\x1b[31m"},
	}
	var buf bytes.Buffer
	owdPrintCheckRowsTable(&buf, rows)
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 5 {
		t.Fatalf("want header + 4 rows, got %d lines:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "DOMAIN") || !strings.Contains(lines[0], "AVAILABLE") {
		t.Fatalf("header missing fixed columns: %q", lines[0])
	}
	if !strings.Contains(lines[1], "taken") || !strings.Contains(lines[1], "93.5%") {
		t.Fatalf("taken row must show taken and popularity: %q", lines[1])
	}
	if !strings.Contains(lines[2], "yes") || !strings.Contains(lines[2], "porkbun 72.4") {
		t.Fatalf("available row must show yes and registrar: %q", lines[2])
	}
	if !strings.Contains(lines[3], "?") || !strings.Contains(lines[3], "not in dictionary") {
		t.Fatalf("error row must show ? and the note: %q", lines[3])
	}
	// Server-controlled strings are scrubbed: no escapes, bells, tabs or
	// newlines survive into the terminal.
	if strings.ContainsAny(out, "\x1b\x07") || strings.Count(out, "\n") != 5 || !strings.Contains(lines[4], "evil.com[2J") || !strings.Contains(lines[4], "1 2") || !strings.Contains(lines[4], "3 4") || !strings.Contains(lines[4], "reg 5") {
		t.Fatalf("cells must be scrubbed:\n%q", out)
	}
}
