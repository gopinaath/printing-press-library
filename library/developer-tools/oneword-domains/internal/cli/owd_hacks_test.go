// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

func TestOwdHacksHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"hacks", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("hacks --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "hacks", "--tld", "--limit", "--refresh", "--max-pages", "hacks --tld art --limit 50"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("hacks --help missing %q:\n%s", want, out.String())
		}
	}
}

func TestOwdSuffixWords(t *testing.T) {
	cases := []struct {
		name  string
		words []string
		tld   string
		want  string
	}{
		{"suffix only", []string{"apart", "art", "smart", "artist", "Heart", "smart"}, "art", "apart,heart,smart"},
		{"dotted tld", []string{"family", "fly", "ly"}, ".ly", "family,fly"},
		{"empty tld", []string{"smart"}, "", ""},
		{"none", []string{"smart"}, "io", ""},
	}
	for _, c := range cases {
		got := strings.Join(owdSuffixWords(c.words, c.tld), ",")
		if got != c.want {
			t.Fatalf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestOwdHackWordRows(t *testing.T) {
	rows := owdHackWordRows([]string{"heart", "smart", "start"}, "art", 2)
	if len(rows) != 2 || rows[0].Domain != "he.art" || rows[0].Stem != "he" || rows[1].Word != "smart" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	if all := owdHackWordRows([]string{"heart", "smart"}, "art", 0); len(all) != 2 {
		t.Fatalf("limit 0 should keep all rows: %+v", all)
	}
	b, _ := json.Marshal(rows[0])
	if strings.Contains(string(b), `"checked"`) {
		t.Fatalf("the always-false checked field is gone: %s", b)
	}
}

func TestOwdSlugsFromRaw(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"slug":"smart","categories":[]}`),
		json.RawMessage(`{"categories":[]}`),
		json.RawMessage(`not json`),
		json.RawMessage(`{"slug":"heart"}`),
	}
	got := owdSlugsFromRaw(raw)
	if strings.Join(got, ",") != "smart,heart" {
		t.Fatalf("got %v", got)
	}
}

func TestOwdHackRowNullability(t *testing.T) {
	b, err := json.Marshal(owdHackRow{owdHack: owdHack{Word: "smart", Stem: "sm", TLD: "art", Domain: "sm.art"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), `{"word":"smart","stem":"sm","tld":"art","domain":"sm.art",`) {
		t.Fatalf("embedded hack fields lead the row: %s", b)
	}
	for _, want := range []string{`"checkable":false`, `"available":null`, `"premium":null`, `"price":null`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("row missing %s: %s", want, b)
		}
	}
	if strings.Contains(string(b), `"error"`) {
		t.Fatalf("empty error must be omitted: %s", b)
	}
}

func TestOwdHacksUsage(t *testing.T) {
	testenv.Isolate(t)
	for _, args := range [][]string{
		{"hacks", "smart", "oasis", "--json"},
		{"hacks", "sm/art", "--json"},
		{"hacks", "smart.com", "--json"},
		{"hacks", "--tld", "a/rt", "--json"},
		{"hacks", "smart", "--limit", "-1", "--json"},
		{"hacks", "smart", "--max-pages", "0", "--json"},
	} {
		if _, _, err := owdNovelRun(t, args...); ExitCode(err) != 2 {
			t.Fatalf("%v: want usage error, got %v", args, err)
		}
	}
}
