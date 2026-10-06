package cli

import (
	"fmt"
	"testing"
)

// Hand-authored, beside the generated tests in this package.
//
// counter.top makes three promises that callers depend on, and only one of
// them was guarded before this file existed. The counter is backed by a Go
// map, whose iteration order is randomised by the runtime, so the ordering
// rules are the ONLY thing that makes the output of every intel command
// reproducible for the same input.
//
// The three promises:
//  1. n == 0 returns EVERY entry, not zero entries. sponsors.go relies on
//     this for the class distribution beside a ranked sponsor list.
//  2. n greater than the number of entries does not truncate and does not
//     panic.
//  3. Ties on Count are broken by Label, ascending.
//
// Mutation-verified against the tests that existed before this file:
// flipping `n > 0` to `n >= 0` and flipping the Label comparison both left
// the package green. Flipping the Count comparison did fail three existing
// tests, which is why this file does not restate that rule beyond the
// ordering assertions it already needs.

// topTestCounter builds a counter through the production constructor and the
// production add method, so the empty-key skip in add stays part of the
// fixture rather than being bypassed by a struct literal.
func topTestCounter(t *testing.T, labels map[string]int) *counter {
	t.Helper()
	c := newCounter()
	for label, times := range labels {
		for i := 0; i < times; i++ {
			c.add(label)
		}
	}
	return c
}

func topLabels(entries []rankedEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Label)
	}
	return out
}

func topJoined(entries []rankedEntry) string {
	return fmt.Sprintf("%v", topLabels(entries))
}

// TestCounterTopLimitBoundaries pins what n means at its edges. The n == 0
// case is the one with a live caller (sponsors.go passes 0 for the sponsor
// class distribution) and the one no test covered.
func TestCounterTopLimitBoundaries(t *testing.T) {
	// Five distinct labels, all with different counts so ordering is
	// unambiguous and only the truncation behaviour is under test.
	fixture := map[string]int{
		"Industry":   5,
		"NIH":        4,
		"Other":      3,
		"Academic":   2,
		"Unassigned": 1,
	}
	const distinct = 5

	cases := []struct {
		name     string
		n        int
		wantLen  int
		wantHead string
	}{
		{name: "zero returns everything", n: 0, wantLen: distinct, wantHead: "Industry"},
		{name: "one returns the single highest", n: 1, wantLen: 1, wantHead: "Industry"},
		{name: "exactly the number of entries", n: distinct, wantLen: distinct, wantHead: "Industry"},
		{name: "more than the number of entries", n: distinct + 1, wantLen: distinct, wantHead: "Industry"},
		{name: "negative behaves like zero", n: -1, wantLen: distinct, wantHead: "Industry"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := topTestCounter(t, fixture).top(tc.n)
			if len(got) != tc.wantLen {
				t.Errorf("top(%d) returned %d entries %s, want %d",
					tc.n, len(got), topJoined(got), tc.wantLen)
			}
			if len(got) > 0 && got[0].Label != tc.wantHead {
				t.Errorf("top(%d) head = %q, want %q", tc.n, got[0].Label, tc.wantHead)
			}
		})
	}
}

// TestCounterTopEmptyCounter covers the degenerate input the boundary table
// cannot express: no entries at all. The assertion is on the number of
// entries only. Whether the result is a nil slice or an allocated empty one
// is deliberately NOT pinned: the field it feeds carries omitempty, so both
// forms disappear from the JSON alike and no caller can tell them apart.
func TestCounterTopEmptyCounter(t *testing.T) {
	for _, n := range []int{0, 1, 8} {
		got := newCounter().top(n)
		if len(got) != 0 {
			t.Errorf("empty counter top(%d) = %s, want no entries", n, topJoined(got))
		}
	}
}

// TestCounterTopBreaksTiesAlphabetically is the rule a mutation proved to be
// unguarded. Every label here shares the same count, so Count contributes
// nothing to the ordering and the alphabetical rule alone decides it.
func TestCounterTopBreaksTiesAlphabetically(t *testing.T) {
	// Inserted in an order unrelated to the expected one, so a top that
	// returned insertion order rather than sorted order would differ.
	fixture := map[string]int{
		"Zeta":  2,
		"Alpha": 2,
		"Mu":    2,
		"Beta":  2,
	}
	want := []string{"Alpha", "Beta", "Mu", "Zeta"}

	got := topLabels(topTestCounter(t, fixture).top(0))
	if len(got) != len(want) {
		t.Fatalf("top(0) returned %d entries %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tie order = %v, want %v", got, want)
		}
	}
}

// TestCounterTopTieOrderIsStableAcrossRuns is the map-randomisation guard.
// A single run can pass by luck when the runtime happens to walk the map in
// the expected order, so the same fixture is rebuilt and ranked repeatedly
// and every run must agree with the first.
func TestCounterTopTieOrderIsStableAcrossRuns(t *testing.T) {
	// A mix of tied and untied counts: the truncation boundary at n = 3
	// falls INSIDE the tied group, so which of the tied labels survives is
	// itself decided by the alphabetical rule.
	fixture := map[string]int{
		"Delta":   3,
		"Bravo":   2,
		"Charlie": 2,
		"Alpha":   2,
		"Echo":    1,
	}

	const runs = 20
	first := topJoined(topTestCounter(t, fixture).top(3))
	for i := 1; i < runs; i++ {
		got := topJoined(topTestCounter(t, fixture).top(3))
		if got != first {
			t.Fatalf("run %d produced %s, run 0 produced %s: ranking is not deterministic", i, got, first)
		}
	}

	// Naming the expected value as well, so a change that is stable but
	// wrong cannot pass by simply being consistently wrong.
	const want = "[Delta Alpha Bravo]"
	if first != want {
		t.Errorf("top(3) = %s, want %s", first, want)
	}
}

// TestCounterTopIgnoresBlankKeys keeps the add-side contract visible from the
// ranking side: a blank or whitespace-only key never becomes a ranked entry,
// so a sample with missing sponsor or country data does not grow an empty
// label in the distribution a caller prints.
func TestCounterTopIgnoresBlankKeys(t *testing.T) {
	c := newCounter()
	c.add("Industry")
	c.add("")
	c.add("   ")
	c.add("Industry")

	ranked := c.top(0)
	if len(ranked) != 1 {
		t.Fatalf("top(0) = %s, want only Industry", topJoined(ranked))
	}
	if ranked[0].Label != "Industry" || ranked[0].Count != 2 {
		t.Errorf("entry = %+v, want Industry x2", ranked[0])
	}
	if c.total() != 2 {
		t.Errorf("total = %d, want 2: blank keys must not be recorded", c.total())
	}
}
