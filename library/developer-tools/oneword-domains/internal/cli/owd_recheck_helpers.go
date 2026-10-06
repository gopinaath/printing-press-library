// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Pure helpers for the recheck command: history selection and the snapshot
// diff. Word files go through the reader shared with 'check --file'.

package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
)

// owdRecheckChange is one field that differs between two snapshots.
type owdRecheckChange struct {
	Domain    string `json:"domain"`
	Field     string `json:"field"`
	Before    any    `json:"before"`
	After     any    `json:"after"`
	CheckedAt string `json:"checked_at"`
}

// owdRecheckResult is the command's output object.
type owdRecheckResult struct {
	Checked       int                `json:"checked"`
	Changed       []owdRecheckChange `json:"changed"`
	Unchanged     int                `json:"unchanged"`
	Baseline      int                `json:"baseline"`
	FetchFailures []owdFailure       `json:"fetch_failures"`
	Note          string             `json:"note,omitempty"`
}

func owdRecheckNewResult() owdRecheckResult {
	return owdRecheckResult{Changed: make([]owdRecheckChange, 0), FetchFailures: make([]owdFailure, 0)}
}

// owdRecheckPriceValue turns the nullable price into a JSON-friendly value.
func owdRecheckPriceValue(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// owdRecheckDiff compares the fields the site can flip between two snapshots.
func owdRecheckDiff(domain string, before, after *owdDomainCheck, at time.Time) []owdRecheckChange {
	out := make([]owdRecheckChange, 0)
	if before == nil || after == nil {
		return out
	}
	ts := at.UTC().Format(time.RFC3339)
	if before.Available != after.Available {
		out = append(out, owdRecheckChange{Domain: domain, Field: "available", Before: before.Available, After: after.Available, CheckedAt: ts})
	}
	if before.Premium != after.Premium {
		out = append(out, owdRecheckChange{Domain: domain, Field: "premium", Before: before.Premium, After: after.Premium, CheckedAt: ts})
	}
	bp, ap := owdRecheckPriceValue(before.Price), owdRecheckPriceValue(after.Price)
	if bp != ap {
		out = append(out, owdRecheckChange{Domain: domain, Field: "price", Before: bp, After: ap, CheckedAt: ts})
	}
	if before.TldCount != after.TldCount {
		out = append(out, owdRecheckChange{Domain: domain, Field: "tld_count", Before: before.TldCount, After: after.TldCount, CheckedAt: ts})
	}
	return out
}

// owdRecheckHistory lists the distinct domains ever checked, optionally only
// those first checked within the window and restricted to some TLDs. Rows are
// fully drained before returning so follow-up queries are safe.
func owdRecheckHistory(ctx context.Context, db *store.Store, window time.Duration, now time.Time, tlds []string) ([]owdPair, error) {
	want := map[string]bool{}
	for _, t := range tlds {
		want[t] = true
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT word, tld, domain, MIN(checked_at) FROM owd_domain_checks GROUP BY domain ORDER BY domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]owdPair, 0)
	for rows.Next() {
		var p owdPair
		var firstRaw any
		if err := rows.Scan(&p.Word, &p.TLD, &p.Domain, &firstRaw); err != nil {
			return nil, err
		}
		if len(want) > 0 && !want[p.TLD] {
			continue
		}
		if first := owdScanTime(firstRaw); window > 0 && (first.IsZero() || now.Sub(first) > window) {
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// record counts one diffed domain: unchanged, or its field changes appended.
func (r *owdRecheckResult) record(changes []owdRecheckChange) {
	if len(changes) == 0 {
		r.Unchanged++
		return
	}
	r.Changed = append(r.Changed, changes...)
}

// owdRecheckLocal diffs the last two stored snapshots per domain without any network call.
func owdRecheckLocal(ctx context.Context, db *store.Store, pairs []owdPair) (owdRecheckResult, error) {
	out := owdRecheckNewResult()
	checksByDomain, timesByDomain, err := owdRecentChecks(ctx, db, owdPairDomains(pairs), 2)
	if err != nil {
		return out, fmt.Errorf("reading snapshots: %w", err)
	}
	for _, p := range pairs {
		checks, times := checksByDomain[p.Domain], timesByDomain[p.Domain]
		if len(checks) == 0 {
			continue
		}
		out.Checked++
		if len(checks) == 1 {
			out.Baseline++
			continue
		}
		out.record(owdRecheckDiff(p.Domain, &checks[1], &checks[0], times[0]))
	}
	sort.SliceStable(out.Changed, func(i, j int) bool {
		if out.Changed[i].Domain != out.Changed[j].Domain {
			return out.Changed[i].Domain < out.Changed[j].Domain
		}
		return out.Changed[i].Field < out.Changed[j].Field
	})
	return out, nil
}
