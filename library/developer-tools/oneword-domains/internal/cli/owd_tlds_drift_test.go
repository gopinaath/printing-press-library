// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil/testenv"
)

func TestOwdTldsDriftHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"tlds", "drift", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("tlds drift --help error = %v", err)
	}
	for _, want := range []string{"Usage:", "drift", "--since", "--snapshot", "--registrars", "tlds drift --since 7d --json"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("tlds drift --help missing %q:\n%s", want, out.String())
		}
	}
}

func TestOwdDriftRows(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 1, d, 12, 0, 0, 0, time.UTC) }
	samples := []owdPriceSample{
		{"ai", "min", 72.4, day(1)},
		{"ai", "min", 72.4, day(10)},
		{"ai", "min", 80.0, day(31)}, // +10.5% vs day 10
		{"com", "min", 9.0, day(1)},
		{"com", "min", 9.0, day(31)}, // unchanged
		{"art", "namecheap", 1.98, day(1)},
		{"art", "namecheap", 0.99, day(31)}, // -50%
		{"io", "min", 30.0, day(31)},        // only one sample
		{"dev", "min", 12.0, day(20)},
		{"dev", "min", 15.0, day(31)}, // then-sample is after the cutoff: skipped
	}
	cutoff := day(31).Add(-15 * 24 * time.Hour) // day 16
	rows := owdDriftRows(samples, cutoff)
	if len(rows) != 2 {
		t.Fatalf("want 2 drift rows, got %+v", rows)
	}
	if rows[0].TLD != "art" || rows[0].Registrar != "namecheap" || rows[0].DeltaPct != -50 || rows[0].Delta != -0.99 {
		t.Fatalf("largest |delta_pct| first: %+v", rows[0])
	}
	if rows[1].TLD != "ai" || rows[1].PriceThen != 72.4 || rows[1].PriceNow != 80 || rows[1].DeltaPct != 10.5 || rows[1].ThenAt != "2026-01-10T12:00:00Z" {
		t.Fatalf("ai row: %+v", rows[1])
	}
	if got := owdDriftRows([]owdPriceSample{{"x", "min", 0, day(1)}, {"x", "min", 5, day(31)}}, cutoff); len(got) != 1 || got[0].DeltaPct != 0 {
		t.Fatalf("zero then-price must not divide by zero: %+v", got)
	}
	if got := owdDriftRows(nil, cutoff); got == nil || len(got) != 0 {
		t.Fatal("no samples must yield an empty, non-nil slice")
	}
}

func TestOwdSnapshotDates(t *testing.T) {
	s := []owdPriceSample{
		{"ai", "min", 1, time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)},
		{"ai", "min", 1, time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC)},
		{"ai", "min", 1, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
	}
	if n := owdSnapshotDates(s); n != 2 {
		t.Fatalf("want 2 days, got %d", n)
	}
	if n := owdSnapshotDates(nil); n != 0 {
		t.Fatalf("want 0 days, got %d", n)
	}
}

// owdDriftTLDFixture is a 93-TLD list where six TLDs carry no min price, as
// the live list does.
func owdDriftTLDFixture() string {
	rows := make([]string, 0, owdTLDTotal)
	for i := 0; i < owdTLDTotal; i++ {
		price := fmt.Sprintf(`"%d.5"`, 5+i)
		if i%16 == 15 { // 15, 31, 47, 63, 79 and one more below
			price = "null"
		}
		if i == 92 {
			price = `""`
		}
		rows = append(rows, fmt.Sprintf(`{"slug":"t%02d","type":"gTld","structure":"normal","top10m":1,"totalReg":2,"views":%d,"minPrice":%s}`, i, owdTLDTotal-i, price))
	}
	return "[" + strings.Join(rows, ",") + "]"
}

func TestNovelTldsDriftSnapshotsOncePerDay(t *testing.T) {
	listFetches := 0
	owdNovelTestServer(t, owdNovelJSONHandler(map[string]func(r *http.Request) (int, string){
		"/api/tlds": func(r *http.Request) (int, string) { listFetches++; return 200, owdDriftTLDFixture() },
		"/api/tlds/": func(r *http.Request) (int, string) {
			tld := strings.TrimPrefix(r.URL.Path, "/api/tlds/")
			return 200, fmt.Sprintf(`{"slug":%q,"registrars":[{"name":"porkbun","price":"3"}],"cheapestRegistrar":{"name":"porkbun","price":"3"}}`, tld)
		},
	}))
	minRowsToday := func() (rows, tlds int) {
		db := owdNovelTestStore(t)
		defer db.Close()
		_ = db.DB().QueryRow(`SELECT COUNT(*), COUNT(DISTINCT tld) FROM owd_tld_prices WHERE registrar='min'`).Scan(&rows, &tlds)
		return rows, tlds
	}
	var out []owdDriftRow
	errOut, err := owdNovelRunJSON(t, &out, "tlds", "drift")
	if err != nil || !strings.Contains(errOut, "snapshot: recorded min prices for 87 TLDs") || len(out) != 0 {
		t.Fatalf("first run must snapshot the 87 priced TLDs: err=%v stderr=%q out=%v", err, errOut, out)
	}
	if rows, tlds := minRowsToday(); rows != 87 || tlds != 87 {
		t.Fatalf("min rows after first run: %d rows / %d TLDs", rows, tlds)
	}
	errOut, err = owdNovelRunJSON(t, &out, "tlds", "drift", "--no-cache")
	if err != nil || strings.Contains(errOut, "snapshot:") || !strings.Contains(errOut, "only one snapshot recorded") {
		t.Fatalf("a second run the same day must not snapshot again: err=%v stderr=%q", err, errOut)
	}
	if rows, _ := minRowsToday(); rows != 87 {
		t.Fatalf("min rows must not grow on the second run: %d", rows)
	}
	// Registrar rows written by check/compare do not count as the daily snapshot.
	db := owdNovelTestStore(t)
	if _, err := db.DB().Exec(`DELETE FROM owd_tld_prices WHERE registrar='min'`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 90; i++ {
		if _, err := db.DB().Exec(`INSERT INTO owd_tld_prices (tld, registrar, price, snapshot_at) VALUES (?, 'porkbun', 3, ?)`, fmt.Sprintf("t%02d", i), owdNow().Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()
	errOut, err = owdNovelRunJSON(t, &out, "tlds", "drift", "--no-cache")
	if err != nil || !strings.Contains(errOut, "snapshot: recorded min prices for 87 TLDs") {
		t.Fatalf("registrar rows alone must not suppress the daily snapshot: err=%v stderr=%q", err, errOut)
	}
	// --snapshot forces one; --data-source local never fetches. The dogfood
	// cap keeps the --registrars fan-out to owdDogfoodTLDs detail fetches.
	t.Setenv(cliutil.DogfoodEnvVar, "1")
	errOut, err = owdNovelRunJSON(t, &out, "tlds", "drift", "--snapshot", "--registrars", "--no-cache")
	if err != nil || !strings.Contains(errOut, "snapshot: recorded min prices for 87 TLDs") || !strings.Contains(errOut, fmt.Sprintf("recorded registrar prices for %d TLDs", owdDogfoodTLDs)) {
		t.Fatalf("--snapshot --registrars: err=%v stderr=%q", err, errOut)
	}
	// --registrars records the per-registrar rows even though today's min
	// snapshot already exists (re-recording the min rows for the day is fine).
	registrarRows := func() int {
		db := owdNovelTestStore(t)
		defer db.Close()
		var n int
		_ = db.DB().QueryRow(`SELECT COUNT(*) FROM owd_tld_prices WHERE registrar='porkbun'`).Scan(&n)
		return n
	}
	regBefore := registrarRows()
	errOut, err = owdNovelRunJSON(t, &out, "tlds", "drift", "--registrars", "--no-cache")
	if err != nil || !strings.Contains(errOut, fmt.Sprintf("recorded registrar prices for %d TLDs", owdDogfoodTLDs)) || registrarRows() != regBefore+owdDogfoodTLDs {
		t.Fatalf("--registrars must record registrar rows although today's snapshot exists: err=%v stderr=%q rows=%d->%d", err, errOut, regBefore, registrarRows())
	}
	fetchesBefore := listFetches
	errOut, err = owdNovelRunJSON(t, &out, "tlds", "drift", "--snapshot", "--data-source", "local")
	if err != nil || strings.Contains(errOut, "snapshot:") || !strings.Contains(errOut, "reading recorded prices only") || listFetches != fetchesBefore {
		t.Fatalf("local mode must not fetch: err=%v stderr=%q fetches=%d", err, errOut, listFetches-fetchesBefore)
	}
	for _, args := range [][]string{
		{"tlds", "drift", "--since", "soon", "--json"},
		{"tlds", "drift", "extra", "--json"},
	} {
		if _, _, err := owdNovelRun(t, args...); ExitCode(err) != 2 {
			t.Fatalf("%v: want usage error, got %v", args, err)
		}
	}
}
