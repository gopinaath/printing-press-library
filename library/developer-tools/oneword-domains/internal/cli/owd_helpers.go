// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored helpers shared by every One Word Domains hand-built command.

package cli

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/client"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/config"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
	"github.com/spf13/cobra"
)

// owdTLDTotal is the popularity denominator the site uses ("registered in N of 93 TLDs").
const owdTLDTotal = 93

const owdDBName = "oneword-domains-pp-cli"

// Bounds shared by every hand-authored command.
const (
	// owdPageSize is the row count of one page from the site's list routes.
	owdPageSize = 100
	// owdDefaultConcurrency is the default fan-out width against the site.
	owdDefaultConcurrency = 4
	// owdDogfoodTLDs, owdDogfoodChecks and owdDogfoodPages cap TLD fan-outs,
	// live availability checks and paged scans inside the live dogfood matrix.
	owdDogfoodTLDs   = 5
	owdDogfoodChecks = 10
	owdDogfoodPages  = 2
	// owdMaxConcurrency bounds --concurrency so one batch cannot hammer the site.
	owdMaxConcurrency = 16
	// owdDefaultMaxChecks is how many word/TLD pairs one `check` or `recheck`
	// run looks up unless --max-checks says otherwise (owdDogfoodChecks in
	// the live matrix).
	owdDefaultMaxChecks = 200
	// owdTLDListComplete is the smallest row count at which a locally stored
	// TLD list counts as complete. Six of the 93 TLDs carry no min price (so
	// a daily "min" snapshot holds at most 87 rows), and the tolerance of five
	// keeps a list that lost a row or two from forcing a live refetch every run.
	owdTLDListComplete = owdTLDTotal - 5
	// owdMinLocalWords is the words-table size below which the local
	// dictionary is too thin to answer alone (brainstorm, hacks --tld, words mine).
	owdMinLocalWords = 1000
)

// owdSlugRe is the only shape one label of a word, TLD or domain may have
// before it is placed in a request path.
var owdSlugRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// owdTLD mirrors one row of GET /api/tlds.
type owdTLD struct {
	Slug        string `json:"slug"`
	Type        string `json:"type"`
	Structure   string `json:"structure"`
	Description string `json:"description,omitempty"`
	Top10m      int    `json:"top10m"`
	TotalReg    int    `json:"totalReg"`
	Views       int    `json:"views"`
	MinPrice    string `json:"minPrice"`
}

// owdRegistrarPrice is one registrar/price pair from the TLD detail route.
type owdRegistrarPrice struct {
	Name  string `json:"name"`
	Price string `json:"price"`
}

// owdCloneRegistrar returns a copy of p (nil in, nil out), so a row never
// aliases the detail it was built from.
func owdCloneRegistrar(p *owdRegistrarPrice) *owdRegistrarPrice {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// owdTLDDetail mirrors GET /api/tlds/{tld}.
type owdTLDDetail struct {
	Slug              string              `json:"slug"`
	Type              string              `json:"type"`
	Structure         string              `json:"structure"`
	Description       string              `json:"description"`
	Top10m            int                 `json:"top10m"`
	TotalReg          int                 `json:"totalReg"`
	Views             int                 `json:"views"`
	Registrars        []owdRegistrarPrice `json:"registrars"`
	CheapestRegistrar *owdRegistrarPrice  `json:"cheapestRegistrar"`
}

// owdDomainCheck mirrors GET /api/domains/{word}.{tld}.
type owdDomainCheck struct {
	Slug              string              `json:"slug"`
	Available         bool                `json:"available"`
	Premium           bool                `json:"premium"`
	Price             *string             `json:"price"`
	TldSlug           string              `json:"tldSlug"`
	Aftermarket       bool                `json:"aftermarket"`
	TldCount          int                 `json:"tldCount"`
	Registrars        []owdRegistrarPrice `json:"registrars"`
	CheapestRegistrar *owdRegistrarPrice  `json:"cheapestRegistrar"`
}

// owdListing mirrors one row of GET /api/listings.
type owdListing struct {
	Domain   string `json:"domain"`
	Type     string `json:"type"`
	UserID   string `json:"userId"`
	Price    string `json:"price"`
	BidCount int    `json:"bidCount"`
	EndDate  string `json:"endDate"`
}

// owdNewListing is a listing seen for the first time, in the snake_case shape
// the watch envelope uses (the site's userId is dropped).
type owdNewListing struct {
	Domain   string `json:"domain"`
	Type     string `json:"type"`
	Price    string `json:"price"`
	BidCount int    `json:"bid_count"`
	EndDate  string `json:"end_date"`
}

// owdGenerated is one object from the DomainsGPT stream.
type owdGenerated struct {
	Domain    string `json:"domain"`
	Available bool   `json:"available"`
}

// owdHack is a domain-hack candidate (word split so that its ending is a TLD).
type owdHack struct {
	Word   string `json:"word"`
	Stem   string `json:"stem"`
	TLD    string `json:"tld"`
	Domain string `json:"domain"`
}

// owdPair is one word/TLD pair to check.
type owdPair struct {
	Word   string
	TLD    string
	Domain string
}

// owdWordLine is one line of a word file: a bare word (checked on the
// caller's TLD set) or word.tld (pinned to that TLD).
type owdWordLine struct {
	Word string
	TLD  string
}

// owdNormTLD lowercases, trims and strips a leading dot from a TLD.
func owdNormTLD(s string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "."))
}

// owdSplitDomain splits "smart.com" into ("smart", "com").
func owdSplitDomain(d string) (string, string, error) {
	d = strings.ToLower(strings.TrimSpace(d))
	// Words never contain dots, so the TLD is everything after the first dot;
	// this keeps multi-label TLDs such as co.uk and com.au intact.
	i := strings.Index(d, ".")
	if i <= 0 || i == len(d)-1 {
		return "", "", fmt.Errorf("%q is not a word.tld domain", d)
	}
	return d[:i], d[i+1:], nil
}

// owdValidateLabels rejects a value whose dot-separated labels are not plain
// lowercase slugs (letters, digits, hyphens), before it can reach a request
// path. The error names the offending value and is a usage error (exit 2).
func owdValidateLabels(kind, v string, minLabels int) error {
	labels := strings.Split(v, ".")
	if len(labels) < minLabels {
		return usageErr(fmt.Errorf("%s %q is not a word.tld domain", kind, v))
	}
	for _, l := range labels {
		if !owdSlugRe.MatchString(l) {
			return usageErr(fmt.Errorf("%s %q may only contain lowercase letters, digits and hyphens", kind, v))
		}
	}
	return nil
}

// owdValidateWord accepts one bare dictionary word.
func owdValidateWord(w string) error {
	if !owdSlugRe.MatchString(w) {
		return usageErr(fmt.Errorf("word %q may only contain lowercase letters, digits and hyphens", w))
	}
	return nil
}

// owdValidateTLD accepts a TLD slug, including the two-label ones the site tracks (co.uk).
func owdValidateTLD(t string) error { return owdValidateLabels("TLD", t, 1) }

// owdValidateDomain accepts word.tld (or word.co.uk).
func owdValidateDomain(d string) error { return owdValidateLabels("domain", d, 2) }

// owdDedupe lowercases, trims and de-duplicates while preserving order.
func owdDedupe(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		it = strings.ToLower(strings.TrimSpace(it))
		if it == "" || seen[it] {
			continue
		}
		seen[it] = true
		out = append(out, it)
	}
	return out
}

// owdParseCSVList splits a comma-separated flag value into trimmed,
// lowercased, dot-stripped, de-duplicated items (first occurrence wins).
func owdParseCSVList(s string) []string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = owdNormTLD(parts[i])
	}
	return owdDedupe(parts)
}

// owdPopularityPct converts the site's tldCount (TLDs where the word is still free)
// into the "registered in N of 93 TLDs" percentage the site shows as dots.
func owdPopularityPct(tldCount int) float64 {
	if tldCount < 0 {
		tldCount = 0
	}
	if tldCount > owdTLDTotal {
		tldCount = owdTLDTotal
	}
	return float64(owdTLDTotal-tldCount) / float64(owdTLDTotal) * 100
}

// owdParseGPTStream decodes DomainsGPT's concatenated JSON objects
// (`{ "domain" : "a.com", "available" : true }{ ... }`) into a slice. A clean
// end of stream returns the names; a truncated trailing object returns the
// names parsed so far plus an error.
func owdParseGPTStream(b []byte) ([]owdGenerated, error) {
	out := make([]owdGenerated, 0)
	dec := json.NewDecoder(bytes.NewReader(b))
	for {
		var g owdGenerated
		err := dec.Decode(&g)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, fmt.Errorf("parsing DomainsGPT stream after %d names: %w", len(out), err)
		}
		if g.Domain != "" {
			out = append(out, g)
		}
	}
	return out, nil
}

// owdHacks lists the domain-hack candidates for one word against a TLD set.
func owdHacks(word string, tlds []string) []owdHack {
	word = strings.ToLower(strings.TrimSpace(word))
	out := make([]owdHack, 0)
	for _, t := range tlds {
		t = owdNormTLD(t)
		if t == "" || !strings.HasSuffix(word, t) || len(word) <= len(t) {
			continue
		}
		stem := word[:len(word)-len(t)]
		out = append(out, owdHack{Word: word, Stem: stem, TLD: t, Domain: stem + "." + t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TLD < out[j].TLD })
	return out
}

// owdEndsWithin reports whether an RFC3339 end date falls within the window from now.
func owdEndsWithin(endDate string, within time.Duration, now time.Time) bool {
	t, err := time.Parse(time.RFC3339, endDate)
	if err != nil {
		return false
	}
	return !t.Before(now) && t.Sub(now) <= within
}

// owdPriceFloat parses the site's string prices ("72.4", "1", "") into a
// float. Garbage, negative, NaN and infinite values are not prices.
func owdPriceFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0, false
	}
	return f, true
}

// owdPrice is a price parsed once for sorting (ok is false when unparseable).
type owdPrice struct {
	v  float64
	ok bool
}

// owdParsePrice parses s with owdPriceFloat into an owdPrice.
func owdParsePrice(s string) owdPrice {
	v, ok := owdPriceFloat(s)
	return owdPrice{v: v, ok: ok}
}

// owdKeyed pairs an item with its precomputed sort key.
type owdKeyed[T, K any] struct {
	v T
	k K
}

// owdSortKeyed stable-sorts items with less over keys computed once per item
// (decorate, sort, undecorate), so no comparator re-parses a field.
func owdSortKeyed[T, K any](items []T, key func(T) K, less func(a, b owdKeyed[T, K]) bool) {
	keyed := make([]owdKeyed[T, K], len(items))
	for i, it := range items {
		keyed[i] = owdKeyed[T, K]{v: it, k: key(it)}
	}
	sort.SliceStable(keyed, func(i, j int) bool { return less(keyed[i], keyed[j]) })
	for i := range keyed {
		items[i] = keyed[i].v
	}
}

// owdDogfoodCap lowers a scan/fetch bound when running inside the live
// dogfood matrix. Zero means "unlimited" to every caller, so it is capped too.
func owdDogfoodCap(n, capUnderDogfood int) int {
	if cliutil.IsDogfoodEnv() && (n == 0 || n > capUnderDogfood) {
		return capUnderDogfood
	}
	return n
}

// owdLoopCtx is the context for a multi-request loop. It is bounded by
// --timeout only when the user set one; the generated client already applies
// the per-request timeout, so an implicit whole-loop bound would cut long
// scans such as 'words mine --refresh' or 'check --tld all'.
func owdLoopCtx(cmd *cobra.Command, flags *rootFlags) (context.Context, context.CancelFunc) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if flags != nil && flags.timeoutExplicit {
		return boundCtx(ctx, flags)
	}
	return ctx, func() {}
}

// owdSessionError is the site's 401/403 for a route that needs the
// lifetime-pass session. It is the cause inside the typed auth error, so a
// fan-out can recognise a failure that owdCheckDomain already mapped.
type owdSessionError struct{ method, path string }

func (e *owdSessionError) Error() string {
	return fmt.Sprintf("%s %s needs a signed-in One Word Domains session (lifetime pass): sign in at https://oneword.domains in Chrome, then run 'oneword-domains-pp-cli auth login --chrome'", e.method, e.path)
}

// owdIsSessionError reports whether an error is the site's 401 or 403 for a
// missing lifetime-pass session and returns a typed auth error with the fix
// (an already-mapped session error is returned as is).
func owdIsSessionError(err error) (error, bool) {
	var session *owdSessionError
	if errors.As(err, &session) {
		return err, true
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
		return authErr(&owdSessionError{method: apiErr.Method, path: apiErr.Path}), true
	}
	return err, false
}

// owdFirstSessionErr returns the typed session error for the first 401/403
// among fan-out failures, so a missing lifetime pass fails once with the fix.
func owdFirstSessionErr(errs []cliutil.FanoutError) error {
	for _, e := range errs {
		if typed, ok := owdIsSessionError(e.Err); ok {
			return typed
		}
	}
	return nil
}

// owdIsUnknownWordError reports whether the availability route's HTTP 500
// (its response for words outside the dictionary) is what failed.
func owdIsUnknownWordError(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode >= 500 && strings.Contains(apiErr.Path, "/api/domains/")
}

// owdUnknownWordErr is the typed not-found error (exit 3) for a domain whose
// word is outside the dictionary (the site answers HTTP 500 for it).
func owdUnknownWordErr(domain string) error {
	word, _, err := owdSplitDomain(domain)
	if err != nil {
		word = domain
	}
	return notFoundErr(fmt.Errorf("%q is not in the One Word Domains dictionary (the site answers 500 for unknown words); try 'oneword-domains-pp-cli words list --prefix %s' or 'gpt generate' for invented names", domain, word))
}

// owdUnknownTLDErr is the usage error (exit 2) for TLDs the site does not track.
func owdUnknownTLDErr(tlds ...string) error {
	return usageErr(fmt.Errorf("unknown TLD(s) %s: One Word Domains tracks %d TLDs (run 'oneword-domains-pp-cli tlds list --sort-alpha')", strings.Join(tlds, ", "), owdTLDTotal))
}

// owdRequireKnownTLDs rejects the TLDs that the fetched list lacks.
func owdRequireKnownTLDs(tlds []string, known map[string]owdTLD) error {
	unknown := make([]string, 0)
	for _, t := range tlds {
		if _, ok := known[t]; !ok {
			unknown = append(unknown, t)
		}
	}
	if len(unknown) > 0 {
		return owdUnknownTLDErr(unknown...)
	}
	return nil
}

// owdMissingArgErr reports a missing positional argument: a JSON object on
// stdout under --json, and always a usage error (exit 2).
func owdMissingArgErr(cmd *cobra.Command, flags *rootFlags, usage string) error {
	path := cmd.CommandPath() + " " + usage
	if flags != nil && flags.asJSON {
		if perr := printJSONFiltered(cmd.OutOrStdout(), map[string]any{
			"error": "missing required argument",
			"usage": path,
		}, flags); perr != nil {
			return perr
		}
	}
	return usageErr(fmt.Errorf("missing required argument\nUsage: %s", path))
}

// owdParseWindowFlag parses a duration flag (7d, 48h, 1w) that must be positive.
func owdParseWindowFlag(name, value string) (time.Duration, error) {
	d, err := cliutil.ParseDurationLoose(value)
	if err != nil || d <= 0 {
		return 0, usageErr(fmt.Errorf("--%s must be a positive duration such as 7d, 48h or 1w", name))
	}
	return d, nil
}

// owdIndexTLDs indexes a TLD list by slug and returns the slugs in list order.
func owdIndexTLDs(tlds []owdTLD) (map[string]owdTLD, []string) {
	known := make(map[string]owdTLD, len(tlds))
	slugs := make([]string, 0, len(tlds))
	for _, t := range tlds {
		if t.Slug == "" {
			continue
		}
		known[t.Slug] = t
		slugs = append(slugs, t.Slug)
	}
	return known, slugs
}

// owdOpenStore opens the CLI's local SQLite store, creating it if needed. The
// owd_* snapshot tables are created by store.migrateExtras on every open.
func owdOpenStore(ctx context.Context) (*store.Store, string, error) {
	path := defaultDBPath(owdDBName)
	db, err := store.OpenWithContext(ctx, path)
	if err != nil {
		return nil, path, fmt.Errorf("opening local store %s: %w", path, err)
	}
	return db, path, nil
}

// owdOpenStoreOptional opens the local store for commands that only snapshot
// into it: a failure is a warning on w and the command runs without a store.
// The returned close function is always safe to defer.
func owdOpenStoreOptional(ctx context.Context, w io.Writer) (*store.Store, func()) {
	db, _, err := owdOpenStore(ctx)
	if err != nil {
		if w != nil {
			fmt.Fprintf(w, "warning: %v (results will not be snapshotted)\n", err)
		}
		return nil, func() {}
	}
	return db, func() { _ = db.Close() }
}

// owdNow returns the UTC timestamp used for snapshot rows.
func owdNow() time.Time { return time.Now().UTC() }

// owdScanTime normalizes a scanned DATETIME (TEXT, BLOB or a driver-decoded
// time) into UTC; unparseable values become the zero time.
func owdScanTime(v any) time.Time {
	if t, ok := v.(time.Time); ok {
		return t.UTC()
	}
	return cliutil.ParseStoredTime(owdScanString(v)).UTC()
}

// owdSnapshotErrs collects local snapshot write failures so a command prints
// one warning instead of one per row. Snapshot writers run sequentially on
// the command goroutine, never inside a fan-out.
type owdSnapshotErrs struct {
	n     int
	first error
}

func (s *owdSnapshotErrs) add(err error) {
	if err == nil {
		return
	}
	s.n++
	if s.first == nil {
		s.first = err
	}
}

func (s *owdSnapshotErrs) warn(w io.Writer) {
	if s == nil || s.n == 0 || w == nil {
		return
	}
	fmt.Fprintf(w, "warning: %d local snapshot write(s) failed, so 'recheck' and 'tlds drift' will not see this run: %v\n", s.n, s.first)
}

// owdFetchTLDs returns the TLD list from the local typed table when it is
// populated, otherwise live from GET /api/tlds (upserting into the store; a
// failed upsert is a warning on w, which may be nil).
func owdFetchTLDs(ctx context.Context, c *client.Client, db *store.Store, forceLive bool, w io.Writer) ([]owdTLD, string, error) {
	if db != nil && !forceLive {
		if out, ok := owdLocalTLDs(ctx, db); ok {
			return out, "local", nil
		}
	}
	if c == nil {
		return nil, "", fmt.Errorf("no local TLD data and no API client; run 'oneword-domains-pp-cli sync --resources tlds'")
	}
	data, err := c.Get(ctx, "/api/tlds", nil)
	if err != nil {
		return nil, "", err
	}
	// Decode the body once: the raw items are cached as-is, each decoded row is returned.
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, "", fmt.Errorf("decoding /api/tlds: %w", err)
	}
	var out []owdTLD
	if items != nil {
		out = make([]owdTLD, len(items))
	}
	for i, item := range items {
		if err := json.Unmarshal(item, &out[i]); err != nil {
			return nil, "", fmt.Errorf("decoding /api/tlds: %w", err)
		}
	}
	if db != nil {
		if _, _, err := db.UpsertBatch("tlds", items); err != nil && w != nil {
			fmt.Fprintf(w, "warning: caching tlds locally failed: %v\n", err)
		}
	}
	return out, "live", nil
}

// owdLocalTLDs reads the typed tlds table. ok is false when the query or the
// row iteration failed or the table holds fewer rows than a complete list, so
// the caller falls back to the live route.
func owdLocalTLDs(ctx context.Context, db *store.Store) ([]owdTLD, bool) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM tlds`)
	if err != nil {
		return nil, false
	}
	defer rows.Close()
	out := make([]owdTLD, 0, owdTLDTotal)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var t owdTLD
		if json.Unmarshal([]byte(raw), &t) == nil && t.Slug != "" {
			out = append(out, t)
		}
	}
	if rows.Err() != nil || len(out) < owdTLDListComplete {
		return nil, false
	}
	return out, true
}

// owdFetchTLDDetail returns GET /api/tlds/{tld}. It does not write the price
// snapshot; owdDetailsByTLD records it after the fan-out completes.
func owdFetchTLDDetail(ctx context.Context, c *client.Client, tld string) (*owdTLDDetail, error) {
	tld = owdNormTLD(tld)
	if err := owdValidateTLD(tld); err != nil {
		return nil, err
	}
	data, err := c.Get(ctx, "/api/tlds/"+cliutil.EscapePathParam(tld), nil)
	if err != nil {
		return nil, err
	}
	var d owdTLDDetail
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("decoding /api/tlds/%s: %w", tld, err)
	}
	if d.Slug == "" {
		return nil, owdUnknownTLDErr(tld)
	}
	return &d, nil
}

// owdDetailsByTLD fetches the TLD detail route for each TLD concurrently and
// then records the price snapshots, stamped at, on the calling goroutine.
// Each TLD whose snapshot write fails is counted in snap.
func owdDetailsByTLD(ctx context.Context, c *client.Client, db *store.Store, tlds []string, concurrency int, at time.Time, snap *owdSnapshotErrs) (map[string]*owdTLDDetail, []cliutil.FanoutError) {
	results, errs := cliutil.FanoutRun(ctx, tlds, func(t string) string { return t },
		func(ctx context.Context, t string) (*owdTLDDetail, error) { return owdFetchTLDDetail(ctx, c, t) },
		cliutil.WithConcurrency(concurrency))
	out := make(map[string]*owdTLDDetail, len(results))
	for _, r := range results {
		out[r.Source] = r.Value
		snap.add(owdRecordTLDPrices(db, r.Value, at))
	}
	return out, errs
}

// owdChecksByDomain runs the availability route for each domain concurrently
// (results keep the order of domains). A 401/403 is returned as the session
// error before anything is recorded; otherwise the successful checks are
// snapshotted, stamped at, with write failures counted in snap.
func owdChecksByDomain(ctx context.Context, c *client.Client, db *store.Store, domains []string, concurrency int, at time.Time, snap *owdSnapshotErrs) ([]cliutil.FanoutResult[*owdDomainCheck], []cliutil.FanoutError, error) {
	results, errs := cliutil.FanoutRun(ctx, domains, func(d string) string { return d },
		func(ctx context.Context, d string) (*owdDomainCheck, error) { return owdCheckDomain(ctx, c, d) },
		cliutil.WithConcurrency(concurrency))
	if err := owdFirstSessionErr(errs); err != nil {
		return nil, nil, err
	}
	owdRecordChecks(db, results, at, snap)
	return results, errs, nil
}

// owdInTx runs fn with query prepared once inside a single transaction and
// commits; an error from fn rolls the whole batch back. Snapshot writers run
// sequentially on the command goroutine, so no writes overlap.
func owdInTx(db *store.Store, query string, fn func(stmt *sql.Stmt) error) error {
	tx, err := db.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()
	if err := fn(stmt); err != nil {
		return err
	}
	return tx.Commit()
}

// owdPriceInsert appends one owd_tld_prices snapshot row.
const owdPriceInsert = `INSERT INTO owd_tld_prices (tld, registrar, price, snapshot_at) VALUES (?, ?, ?, ?)`

// owdRecordTLDPrices appends one snapshot row per registrar plus the cheapest
// ("min"), in one transaction.
func owdRecordTLDPrices(db *store.Store, d *owdTLDDetail, at time.Time) error {
	if db == nil || d == nil {
		return nil
	}
	ts := at.Format(time.RFC3339)
	return owdInTx(db, owdPriceInsert, func(stmt *sql.Stmt) error {
		for _, r := range d.Registrars {
			if p, ok := owdPriceFloat(r.Price); ok {
				if _, err := stmt.Exec(d.Slug, r.Name, p, ts); err != nil {
					return fmt.Errorf("recording %s price for .%s: %w", r.Name, d.Slug, err)
				}
			}
		}
		if d.CheapestRegistrar != nil {
			if p, ok := owdPriceFloat(d.CheapestRegistrar.Price); ok {
				if _, err := stmt.Exec(d.Slug, "min", p, ts); err != nil {
					return fmt.Errorf("recording min price for .%s: %w", d.Slug, err)
				}
			}
		}
		return nil
	})
}

// owdRecordMinPrices appends a "min" snapshot for every TLD in a list response,
// in one transaction, and returns how many rows were written (0 when the
// transaction failed and was rolled back).
func owdRecordMinPrices(db *store.Store, tlds []owdTLD, at time.Time) (int, error) {
	if db == nil {
		return 0, nil
	}
	ts := at.Format(time.RFC3339)
	n := 0
	err := owdInTx(db, owdPriceInsert, func(stmt *sql.Stmt) error {
		for _, t := range tlds {
			if p, ok := owdPriceFloat(t.MinPrice); ok {
				if _, err := stmt.Exec(t.Slug, "min", p, ts); err != nil {
					return fmt.Errorf("recording min price for .%s: %w", t.Slug, err)
				}
				n++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// owdCheckDomain calls GET /api/domains/{word}.{tld}. A 500 means the word is
// not in the dictionary; that becomes a typed not-found error (exit 3). Other
// failures come back raw: every caller is a fan-out whose owdFirstSessionErr
// maps a 401/403 to the session error. The domain is validated label by
// label before it is placed in the path.
func owdCheckDomain(ctx context.Context, c *client.Client, domain string) (*owdDomainCheck, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if err := owdValidateDomain(domain); err != nil {
		return nil, err
	}
	data, err := c.Get(ctx, "/api/domains/"+cliutil.EscapePathParam(domain), nil)
	if err != nil {
		if owdIsUnknownWordError(err) {
			return nil, owdUnknownWordErr(domain)
		}
		return nil, err
	}
	var d owdDomainCheck
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("decoding /api/domains/%s: %w", domain, err)
	}
	return &d, nil
}

// owdCheckInsert appends one owd_domain_checks availability snapshot row.
const owdCheckInsert = `INSERT INTO owd_domain_checks (word, tld, domain, available, premium, price, aftermarket, tld_count, checked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

// owdRecordChecks appends every successful availability result in one
// transaction, after the fan-out that produced them has completed. A row
// that fails is counted in snap and the rest are still written.
func owdRecordChecks(db *store.Store, checks []cliutil.FanoutResult[*owdDomainCheck], at time.Time, snap *owdSnapshotErrs) {
	if db == nil || len(checks) == 0 {
		return
	}
	ts := at.Format(time.RFC3339)
	snap.add(owdInTx(db, owdCheckInsert, func(stmt *sql.Stmt) error {
		for _, r := range checks {
			d := r.Value
			if d == nil {
				continue
			}
			word, tld, err := owdSplitDomain(d.Slug)
			if err != nil {
				continue
			}
			var price any
			if d.Price != nil {
				price = *d.Price
			}
			if _, err := stmt.Exec(word, tld, d.Slug, boolInt(d.Available), boolInt(d.Premium), price, boolInt(d.Aftermarket), d.TldCount, ts); err != nil {
				snap.add(fmt.Errorf("recording check for %s: %w", d.Slug, err))
			}
		}
		return nil
	}))
}

// owdIndexChecks indexes a fan-out's availability results by domain and its
// failures by domain (message only), for joining back onto the input pairs.
func owdIndexChecks(results []cliutil.FanoutResult[*owdDomainCheck], errs []cliutil.FanoutError) (map[string]*owdDomainCheck, map[string]string) {
	byDomain := make(map[string]*owdDomainCheck, len(results))
	for _, r := range results {
		byDomain[r.Source] = r.Value
	}
	failed := make(map[string]string, len(errs))
	for _, e := range errs {
		msg := ""
		if e.Err != nil {
			msg = e.Err.Error()
		}
		failed[e.Source] = msg
	}
	return byDomain, failed
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// owdLookupChunk bounds the IN (...) list of a batched store lookup so every
// query stays well under SQLite's bound-variable limit.
const owdLookupChunk = 500

// owdInArgs returns the "?,?,..." placeholder list and bound arguments for an
// IN (...) clause over vals.
func owdInArgs(vals []string) (string, []any) {
	args := make([]any, len(vals))
	for i, v := range vals {
		args[i] = v
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(vals)), ","), args
}

// owdLocalWords returns which of words (lowercased and trimmed) the local
// typed words table holds, in one query per owdLookupChunk words. Words are
// missing from the set when there is no store or a query fails.
func owdLocalWords(ctx context.Context, db *store.Store, words []string) map[string]bool {
	found := map[string]bool{}
	if db == nil {
		return found
	}
	for chunk := range slices.Chunk(owdDedupe(words), owdLookupChunk) {
		in, args := owdInArgs(chunk)
		rows, err := db.DB().QueryContext(ctx, `SELECT slug FROM words WHERE slug IN (`+in+`)`, args...)
		if err != nil {
			return found
		}
		for rows.Next() {
			var slug sql.NullString
			if rows.Scan(&slug) == nil && slug.Valid {
				found[slug.String] = true
			}
		}
		_ = rows.Close()
	}
	return found
}

// owdLiveWordLookup asks GET /api/words?query=<word> whether word (already
// lowercased and trimmed) is in the dictionary. It also returns the slugs the
// site answered with: they are the prefix suggestions for an unknown word.
func owdLiveWordLookup(ctx context.Context, c *client.Client, word string) (bool, []string, error) {
	data, err := c.Get(ctx, "/api/words", map[string]string{"query": word})
	if err != nil {
		return false, nil, err
	}
	var rows []struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return false, nil, fmt.Errorf("decoding /api/words: %w", err)
	}
	slugs := make([]string, 0, len(rows))
	has := false
	for _, r := range rows {
		slugs = append(slugs, r.Slug)
		if strings.EqualFold(r.Slug, word) {
			has = true
		}
	}
	return has, slugs, nil
}

// owdWordsTableCount returns the size of the local typed words table (0 when
// there is no store or the query fails).
func owdWordsTableCount(ctx context.Context, db *store.Store) int {
	if db == nil {
		return 0
	}
	var n int
	if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM words`).Scan(&n); err != nil {
		return 0
	}
	return n
}

// owdDictResult is what the dictionary pre-check learned about each word.
type owdDictResult struct {
	InDict map[string]bool
	Failed map[string]string
	Errs   []cliutil.FanoutError
	// slugs holds the /api/words answer for words the live lookup did not
	// find, so suggestions need no second request for the same query.
	slugs map[string][]string
}

// owdDictPrecheck is the dictionary pre-check shared by check, compare and
// hacks (the site answers HTTP 500 for words it does not know). Words are
// looked up in the local typed words table in one batch first; only the
// misses fan out to GET /api/words. A 401/403 from the site is returned as
// the session error.
func owdDictPrecheck(ctx context.Context, c *client.Client, db *store.Store, words []string, concurrency int) (owdDictResult, error) {
	res := owdDictResult{InDict: map[string]bool{}, Failed: map[string]string{}, slugs: map[string][]string{}}
	local := owdLocalWords(ctx, db, words)
	misses := make([]string, 0, len(words))
	for _, w := range words {
		norm := strings.ToLower(strings.TrimSpace(w))
		switch {
		case local[norm]:
			res.InDict[w] = true
		case norm == "" || c == nil:
			res.InDict[w] = false
		default:
			misses = append(misses, w)
		}
	}
	type lookup struct {
		has   bool
		slugs []string
	}
	results, errs := cliutil.FanoutRun(ctx, misses, func(w string) string { return w },
		func(ctx context.Context, w string) (lookup, error) {
			has, slugs, err := owdLiveWordLookup(ctx, c, strings.ToLower(strings.TrimSpace(w)))
			return lookup{has: has, slugs: slugs}, err
		},
		cliutil.WithConcurrency(concurrency))
	if err := owdFirstSessionErr(errs); err != nil {
		return res, err
	}
	for _, r := range results {
		res.InDict[r.Source] = r.Value.has
		if !r.Value.has {
			res.slugs[r.Source] = r.Value.slugs
		}
	}
	for _, e := range errs {
		res.Failed[e.Source] = e.Err.Error()
	}
	res.Errs = errs
	return res, nil
}

// suggest returns dictionary neighbours for an unknown word, reusing the
// /api/words answer the pre-check already fetched for it when there is one.
func (d owdDictResult) suggest(ctx context.Context, c *client.Client, word string, n int) []string {
	if slugs, ok := d.slugs[word]; ok {
		return owdSuggestFrom(ctx, c, word, slugs, n)
	}
	return owdSuggestFor(ctx, c, word, n)
}

// owdSuggestWords returns up to n dictionary words that start with prefix.
func owdSuggestWords(ctx context.Context, c *client.Client, prefix string, n int) []string {
	if c == nil || prefix == "" {
		return nil
	}
	data, err := c.Get(ctx, "/api/words", map[string]string{"query": strings.ToLower(prefix)})
	if err != nil {
		return nil
	}
	var rows []struct {
		Slug string `json:"slug"`
	}
	if json.Unmarshal(data, &rows) != nil {
		return nil
	}
	out := make([]string, 0, n)
	for _, r := range rows {
		if len(out) >= n {
			break
		}
		out = append(out, r.Slug)
	}
	return out
}

// owdSuggestFor returns dictionary neighbours for an unknown word: the word
// itself as a prefix first, then its first three letters.
func owdSuggestFor(ctx context.Context, c *client.Client, word string, n int) []string {
	return owdSuggestFrom(ctx, c, word, owdSuggestWords(ctx, c, word, n), n)
}

// owdSuggestFrom finishes owdSuggestFor given the slugs already fetched for
// the word itself as a prefix: the first n of them, else the first n words
// starting with the word's first three letters. Never nil.
func owdSuggestFrom(ctx context.Context, c *client.Client, word string, prefixSlugs []string, n int) []string {
	out := make([]string, 0, n)
	out = append(out, prefixSlugs[:min(n, len(prefixSlugs))]...)
	if len(out) == 0 && len(word) > 3 {
		if more := owdSuggestWords(ctx, c, word[:3], n); more != nil {
			out = more
		}
	}
	return out
}

// owdSplitKnownDomain splits "smart.co.uk" into ("smart", "co.uk") by
// preferring the longest TLD the site tracks; unknown suffixes fall back to
// the last-dot split so the caller can report them.
func owdSplitKnownDomain(domain string, slugs []string) (string, string, error) {
	d := strings.ToLower(strings.TrimSpace(domain))
	best := ""
	for _, s := range slugs {
		s = owdNormTLD(s)
		if s == "" || !strings.HasSuffix(d, "."+s) || len(d) <= len(s)+1 {
			continue
		}
		if len(s) > len(best) {
			best = s
		}
	}
	if best != "" {
		return d[:len(d)-len(best)-1], best, nil
	}
	return owdSplitDomain(d)
}

// owdTopTLDs returns the n most-viewed TLD slugs (ties broken alphabetically);
// n <= 0 returns every slug.
func owdTopTLDs(tlds []owdTLD, n int) []string {
	sorted := make([]owdTLD, len(tlds))
	copy(sorted, tlds)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Views != sorted[j].Views {
			return sorted[i].Views > sorted[j].Views
		}
		return sorted[i].Slug < sorted[j].Slug
	})
	out := make([]string, 0, len(sorted))
	for _, t := range sorted {
		if n > 0 && len(out) >= n {
			break
		}
		if t.Slug != "" {
			out = append(out, t.Slug)
		}
	}
	return out
}

// owdFetchListings pages GET /api/listings (owdPageSize per page) with the
// given filters until a short page or maxPages. capped is true when the scan
// stopped at maxPages with a full last page, so more listings may exist and
// the result must not be treated as the complete feed.
func owdFetchListings(ctx context.Context, c *client.Client, params map[string]string, maxPages int) ([]owdListing, int, bool, error) {
	all := make([]owdListing, 0, 128)
	pages := 0
	for page := 1; page <= maxPages; page++ {
		p := map[string]string{}
		for k, v := range params {
			if v != "" {
				p[k] = v
			}
		}
		p["page"] = strconv.Itoa(page)
		data, err := c.Get(ctx, "/api/listings", p)
		if err != nil {
			return all, pages, false, err
		}
		var rows []owdListing
		if err := json.Unmarshal(data, &rows); err != nil {
			return all, pages, false, fmt.Errorf("decoding /api/listings page %d: %w", page, err)
		}
		pages++
		all = append(all, rows...)
		if len(rows) < owdPageSize {
			return all, pages, false, nil
		}
	}
	return all, pages, true, nil
}

// owdFetchWordsPage returns one page of the dictionary directory (owdPageSize words).
func owdFetchWordsPage(ctx context.Context, c *client.Client, params map[string]string, page int) ([]json.RawMessage, error) {
	p := map[string]string{"page": strconv.Itoa(page)}
	for k, v := range params {
		if v != "" {
			p[k] = v
		}
	}
	data, err := c.Get(ctx, "/api/words", p)
	if err != nil {
		return nil, err
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("decoding /api/words page %d: %w", page, err)
	}
	return rows, nil
}

// owdDirectoryParams is the filter set the lifetime-pass directory routes
// (GET /api/domains and /api/domains/count) share: category, price, search
// and the length bounds. Empty values stay empty strings; the scanners drop
// them before the request.
func owdDirectoryParams(category, price, search string, minLen, maxLen int) map[string]string {
	params := map[string]string{"category": category, "price": price, "search": strings.TrimSpace(search)}
	if minLen > 0 {
		params["minLength"] = strconv.Itoa(minLen)
	}
	if maxLen > 0 {
		params["maxLength"] = strconv.Itoa(maxLen)
	}
	return params
}

// owdOpenWordFile opens --file, with "-" meaning the command's stdin.
func owdOpenWordFile(cmd *cobra.Command, path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(cmd.InOrStdin()), nil
	}
	f, err := os.Open(path) // #nosec G304 -- user-supplied word list.
	if err != nil {
		return nil, usageErr(fmt.Errorf("--file: %w", err))
	}
	return f, nil
}

// owdSafeEchoRe is the shape a rejected word-file line must already have
// before its content may appear in an error message.
var owdSafeEchoRe = regexp.MustCompile(`^[a-z0-9.-]{1,64}$`)

// owdFileWordRe is the word part of a --file line: a lowercase letter or
// digit followed by up to 31 letters, digits or hyphens. With
// owdMaxWordDigits it keeps keys, hashes and tokens that happen to be
// lowercase out of the request path and the error text.
var owdFileWordRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// owdMaxWordDigits is the most digits a --file word (or an echoed line) may carry.
const owdMaxWordDigits = 2

// owdCountDigits counts the ASCII digits in s.
func owdCountDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

// owdFileWordOK reports whether w is an acceptable --file word: owdFileWordRe,
// at most owdMaxWordDigits digits, and not hyphens only.
func owdFileWordOK(w string) bool {
	return owdFileWordRe.MatchString(w) && owdCountDigits(w) <= owdMaxWordDigits && strings.Trim(w, "-") != ""
}

// owdWordLineErr reports a rejected word-file line. --file accepts any
// readable path and the MCP server relays stderr, so the content is echoed
// only when it is already a plain slug with at most owdMaxWordDigits digits;
// anything else is named by line number and the rule, never verbatim.
func owdWordLineErr(line int, raw string) error {
	const rule = "is not a word or word.tld (lowercase letters, digits and hyphens, at most 32 characters and two digits, optionally .tld)"
	if owdSafeEchoRe.MatchString(raw) && owdCountDigits(raw) <= owdMaxWordDigits {
		return fmt.Errorf("line %d: %q %s", line, raw, rule)
	}
	return fmt.Errorf("line %d %s", line, rule)
}

// owdUnknownTLDLineErr reports a pinned TLD the tracked list lacks by line
// number only: the value is never echoed.
func owdUnknownTLDLineErr(line int) error {
	return fmt.Errorf("line %d: unknown TLD (One Word Domains tracks %d TLDs; run 'oneword-domains-pp-cli tlds list --sort-alpha')", line, owdTLDTotal)
}

// owdReadWordLines reads a word file shared by 'check --file' and 'recheck
// --file': one entry per line, blank lines and # comments skipped,
// de-duplicated. Lines are never lowercased: one with an uppercase letter is
// rejected as it is, so a key or token is never folded into a word. A bare
// word is checked on the caller's TLD set; word.tld pins that TLD (slugs lets
// "smart.co.uk" split on a tracked two-label TLD and, when non-empty, is the
// set a pinned TLD must belong to). Rejected lines go through owdWordLineErr
// or owdUnknownTLDLineErr.
func owdReadWordLines(r io.Reader, slugs []string) ([]owdWordLine, error) {
	out := make([]owdWordLine, 0)
	seen := map[string]bool{}
	known := map[string]bool{}
	for _, s := range slugs {
		if s = owdNormTLD(s); s != "" {
			known[s] = true
		}
	}
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		if strings.ToLower(raw) != raw || !owdSafeEchoRe.MatchString(raw) {
			return nil, owdWordLineErr(line, raw)
		}
		entry := owdWordLine{Word: raw}
		if strings.Contains(raw, ".") {
			w, t, err := owdSplitKnownDomain(raw, slugs)
			if err != nil {
				return nil, owdWordLineErr(line, raw)
			}
			entry = owdWordLine{Word: w, TLD: t}
		}
		if entry.TLD != "" && len(known) > 0 && !known[entry.TLD] {
			return nil, owdUnknownTLDLineErr(line)
		}
		if !owdFileWordOK(entry.Word) || (entry.TLD != "" && owdValidateTLD(entry.TLD) != nil) {
			return nil, owdWordLineErr(line, raw)
		}
		key := entry.Word + "." + entry.TLD
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, entry)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// owdWordPairs crosses word-file entries with the TLD set: pinned entries
// keep their TLD, bare words take every TLD in tlds. Pairs are de-duplicated
// by domain in first-seen order.
func owdWordPairs(lines []owdWordLine, tlds []string) []owdPair {
	out := make([]owdPair, 0, len(lines)*max(len(tlds), 1))
	seen := map[string]bool{}
	add := func(w, t string) {
		d := w + "." + t
		if seen[d] {
			return
		}
		seen[d] = true
		out = append(out, owdPair{Word: w, TLD: t, Domain: d})
	}
	for _, l := range lines {
		if l.TLD != "" {
			add(l.Word, l.TLD)
			continue
		}
		for _, t := range tlds {
			add(l.Word, t)
		}
	}
	return out
}

// owdPairWords returns the distinct words of pairs (lowercased, in order).
func owdPairWords(pairs []owdPair) []string {
	words := make([]string, 0, len(pairs))
	for _, p := range pairs {
		words = append(words, p.Word)
	}
	return owdDedupe(words)
}

// owdPairTLDs returns the distinct TLDs of a pair list in first-seen order.
func owdPairTLDs(pairs []owdPair) []string {
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.TLD)
	}
	return owdDedupe(out)
}

// owdPairDomains returns each pair's domain, in order.
func owdPairDomains(pairs []owdPair) []string {
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.Domain)
	}
	return out
}

// owdFilterGoneByTLD keeps only the vanished domains under the watched TLD,
// so watching one TLD does not report every other TLD's sightings as gone.
func owdFilterGoneByTLD(gone []string, tld string) []string {
	tld = owdNormTLD(tld)
	out := make([]string, 0, len(gone))
	for _, d := range gone {
		if tld == "" || strings.HasSuffix(strings.ToLower(d), "."+tld) {
			out = append(out, d)
		}
	}
	return out
}

// owdDedupeListings drops empty and repeated domains (the site can repeat a
// listing across pages), keeping the first occurrence in order.
func owdDedupeListings(listings []owdListing) []owdListing {
	seen := map[string]bool{}
	out := make([]owdListing, 0, len(listings))
	for _, l := range listings {
		if l.Domain == "" || seen[l.Domain] {
			continue
		}
		seen[l.Domain] = true
		out = append(out, l)
	}
	return out
}

// owdListingDiff is what one watch run found against the sighting table.
type owdListingDiff struct {
	New       []owdNewListing    `json:"new"`
	Changed   []owdListingChange `json:"changed"`
	Gone      []string           `json:"gone"`
	Unchanged int                `json:"unchanged"`
}

type owdListingChange struct {
	Domain       string `json:"domain"`
	Price        string `json:"price"`
	PrevPrice    string `json:"prev_price"`
	BidCount     int    `json:"bid_count"`
	PrevBidCount int    `json:"prev_bid_count"`
	EndDate      string `json:"end_date"`
}

// owdRecordListings updates the sighting table and returns the diff classes.
// Listings are de-duplicated by domain first. "Gone" is computed only among
// prior rows under tld (empty means every row) and only when the scan was
// complete: an unfetched page is not a vanished listing. Gone rows are
// deleted in the same transaction, so each disappearance is reported once
// and a listing that reappears counts as new again.
func owdRecordListings(ctx context.Context, db *store.Store, listings []owdListing, tld string, at time.Time, complete bool) (owdListingDiff, error) {
	diff := owdListingDiff{New: make([]owdNewListing, 0), Changed: make([]owdListingChange, 0), Gone: make([]string, 0)}
	if db == nil {
		return diff, errors.New("no local store")
	}
	ts := at.Format(time.RFC3339)
	// Drain the prior state first (SQLite single-connection rule), then write.
	prior := map[string]owdListingChange{}
	rows, err := db.DB().QueryContext(ctx, `SELECT domain, COALESCE(price,''), COALESCE(bid_count,0), COALESCE(end_date,'') FROM owd_listing_seen`)
	if err != nil {
		return diff, err
	}
	for rows.Next() {
		var ch owdListingChange
		if err := rows.Scan(&ch.Domain, &ch.Price, &ch.BidCount, &ch.EndDate); err != nil {
			_ = rows.Close()
			return diff, err
		}
		prior[ch.Domain] = ch
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return diff, err
	}
	_ = rows.Close()
	unique := owdDedupeListings(listings)
	seen := make(map[string]bool, len(unique))
	for _, l := range unique {
		seen[l.Domain] = true
	}
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return diff, err
	}
	defer tx.Rollback()
	var insertNew, updateChanged, touchSeen, deleteGone *sql.Stmt
	for _, st := range []struct {
		dst   **sql.Stmt
		query string
	}{
		{&insertNew, `INSERT INTO owd_listing_seen (domain, type, first_seen, last_seen, price, bid_count, end_date) VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(domain) DO UPDATE SET type=excluded.type, last_seen=excluded.last_seen, price=excluded.price, bid_count=excluded.bid_count, end_date=excluded.end_date`},
		{&updateChanged, `UPDATE owd_listing_seen SET last_seen=?, prev_price=price, prev_bid_count=bid_count, price=?, bid_count=?, end_date=?, type=?, changed_at=? WHERE domain=?`},
		{&touchSeen, `UPDATE owd_listing_seen SET last_seen=?, end_date=? WHERE domain=?`},
		{&deleteGone, `DELETE FROM owd_listing_seen WHERE domain=?`},
	} {
		stmt, err := tx.PrepareContext(ctx, st.query)
		if err != nil {
			return diff, err
		}
		defer stmt.Close()
		*st.dst = stmt
	}
	for _, l := range unique {
		p, had := prior[l.Domain]
		switch {
		case !had:
			diff.New = append(diff.New, owdNewListing{Domain: l.Domain, Type: l.Type, Price: l.Price, BidCount: l.BidCount, EndDate: l.EndDate})
			if _, err := insertNew.ExecContext(ctx, l.Domain, l.Type, ts, ts, l.Price, l.BidCount, l.EndDate); err != nil {
				return diff, err
			}
		case p.Price != l.Price || p.BidCount != l.BidCount:
			diff.Changed = append(diff.Changed, owdListingChange{Domain: l.Domain, Price: l.Price, PrevPrice: p.Price, BidCount: l.BidCount, PrevBidCount: p.BidCount, EndDate: l.EndDate})
			if _, err := updateChanged.ExecContext(ctx, ts, l.Price, l.BidCount, l.EndDate, l.Type, ts, l.Domain); err != nil {
				return diff, err
			}
		default:
			diff.Unchanged++
			if _, err := touchSeen.ExecContext(ctx, ts, l.EndDate, l.Domain); err != nil {
				return diff, err
			}
		}
	}
	if complete {
		candidates := make([]string, 0)
		for d := range prior {
			if !seen[d] {
				candidates = append(candidates, d)
			}
		}
		diff.Gone = owdFilterGoneByTLD(candidates, tld)
		sort.Strings(diff.Gone)
		for _, d := range diff.Gone {
			if _, err := deleteGone.ExecContext(ctx, d); err != nil {
				return diff, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return diff, err
	}
	return diff, nil
}

// owdFirstSeen returns every sighting's first_seen timestamp, keyed by domain.
func owdFirstSeen(ctx context.Context, db *store.Store) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	if db == nil {
		return out, nil
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT domain, first_seen FROM owd_listing_seen`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		var fs any
		if err := rows.Scan(&d, &fs); err != nil {
			return out, err
		}
		if t := owdScanTime(fs); !t.IsZero() {
			out[d] = t
		}
	}
	return out, rows.Err()
}

// owdRecentChecks returns up to n most recent snapshots per domain (newest
// first) with their checked_at times, in one window query per owdLookupChunk
// domains. Domains without a snapshot are absent from the maps.
func owdRecentChecks(ctx context.Context, db *store.Store, domains []string, n int) (map[string][]owdDomainCheck, map[string][]time.Time, error) {
	checks := map[string][]owdDomainCheck{}
	times := map[string][]time.Time{}
	if db == nil || n <= 0 {
		return checks, times, nil
	}
	seen := make(map[string]bool, len(domains))
	uniq := make([]string, 0, len(domains))
	for _, d := range domains {
		if !seen[d] {
			seen[d] = true
			uniq = append(uniq, d)
		}
	}
	scan := func(chunk []string) error {
		in, args := owdInArgs(chunk)
		rows, err := db.DB().QueryContext(ctx, `SELECT domain, available, premium, price, aftermarket, tld_count, checked_at FROM (
			SELECT *, ROW_NUMBER() OVER (PARTITION BY domain ORDER BY checked_at DESC, id DESC) AS rn
			FROM owd_domain_checks WHERE domain IN (`+in+`)
		) WHERE rn <= ? ORDER BY domain, rn`, append(args, n)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d owdDomainCheck
			var avail, prem, after int
			var price sql.NullString
			var at any
			if err := rows.Scan(&d.Slug, &avail, &prem, &price, &after, &d.TldCount, &at); err != nil {
				return err
			}
			d.Available, d.Premium, d.Aftermarket = avail == 1, prem == 1, after == 1
			if price.Valid {
				v := price.String
				d.Price = &v
			}
			_, d.TldSlug, _ = owdSplitDomain(d.Slug)
			checks[d.Slug] = append(checks[d.Slug], d)
			times[d.Slug] = append(times[d.Slug], owdScanTime(at))
		}
		return rows.Err()
	}
	for chunk := range slices.Chunk(uniq, owdLookupChunk) {
		if err := scan(chunk); err != nil {
			return nil, nil, err
		}
	}
	return checks, times, nil
}

// owdLatestChecks returns the most recent snapshot per domain for the domains
// that have one. A store read failure reads as "no snapshots".
func owdLatestChecks(ctx context.Context, db *store.Store, domains []string) map[string]*owdDomainCheck {
	out := map[string]*owdDomainCheck{}
	checks, _, err := owdRecentChecks(ctx, db, domains, 1)
	if err != nil {
		return out
	}
	for d, cs := range checks {
		out[d] = &cs[0]
	}
	return out
}

// owdCredentialDomain is the host the session cookie and the DomainsGPT
// partner token are bound to (internal/client's canonicalCredentialDomain).
const owdCredentialDomain = "oneword.domains"

// owdGenerateEndpoint is the DomainsGPT route on the canonical host.
// ONEWORD_DOMAINS_BASE_URL redirects it (tests serve the stream from a
// loopback listener); the session cookie and partner token ride the request
// only when owdCredentialAppliesToURL accepts the resolved URL.
const owdGenerateEndpoint = "https://" + owdCredentialDomain + "/api/gpt/generate"

// owdCredentialAppliesToURL duplicates the unexported credentialAppliesToURL
// and credentialBoundHost of internal/client/client.go: a credential is
// attached only to an https URL whose host is the bound domain (configDomain,
// else owdCredentialDomain) or a subdomain of it. The one fail-open, as
// there, is the verifier's live-HTTP mode against a loopback mock.
func owdCredentialAppliesToURL(rawURL, configDomain string) bool {
	if cliutil.IsVerifyEnv() && cliutil.IsVerifyLiveHTTPEnv() && owdIsLoopbackURL(rawURL) {
		return true
	}
	target, err := url.Parse(rawURL)
	if err != nil || target.Hostname() == "" || !strings.EqualFold(target.Scheme, "https") {
		return false
	}
	bound := strings.TrimSpace(configDomain)
	if bound == "" {
		bound = owdCredentialDomain
	}
	bound = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(bound, "."), "."))
	if bound == "" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	return host == bound || strings.HasSuffix(host, "."+bound)
}

// owdIsLoopbackURL mirrors internal/client's isVerifyMockURL: localhost or a
// loopback IP.
func owdIsLoopbackURL(rawURL string) bool {
	target, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(target.Hostname())
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// owdGenerateURL is the DomainsGPT URL for this process.
func owdGenerateURL() string {
	if base := strings.TrimRight(strings.TrimSpace(cliutil.EnvOverride("ONEWORD_DOMAINS_BASE_URL")), "/"); base != "" {
		return base + "/api/gpt/generate"
	}
	return owdGenerateEndpoint
}

// owdMinMaskedCookieLen is the shortest individual cookie value masked on
// its own. Shorter values are still masked as part of the whole Cookie
// header, just not one by one, so a three-character value cannot blank out
// unrelated text.
const owdMinMaskedCookieLen = 8

// owdMaskCredentials replaces every credential value the request carries
// (the Cookie header and each cookie value in it, the Authorization header
// and its Bearer token, and the query-escaped form of each) with "****", so
// an error body that echoes the request never prints one.
func owdMaskCredentials(s string, req *http.Request) string {
	if req == nil || s == "" {
		return s
	}
	secrets := make([]string, 0)
	if ck := strings.TrimSpace(req.Header.Get("Cookie")); ck != "" {
		secrets = append(secrets, ck)
		for _, part := range strings.Split(ck, ";") {
			if _, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok && len(strings.TrimSpace(v)) >= owdMinMaskedCookieLen {
				secrets = append(secrets, strings.TrimSpace(v))
			}
		}
	}
	if auth := strings.TrimSpace(req.Header.Get("Authorization")); auth != "" {
		secrets = append(secrets, auth)
		if tok := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")); tok != "" {
			secrets = append(secrets, tok)
		}
	}
	// Longest first, so a value that contains another is masked whole.
	sort.SliceStable(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, v := range secrets {
		s = strings.ReplaceAll(s, v, "****")
		if esc := url.QueryEscape(v); esc != v {
			s = strings.ReplaceAll(s, esc, "****")
		}
	}
	return s
}

// owdErrBody trims an error body for the terminal: the credentials req
// carried masked, at most 4 KiB, HTML entities decoded, control characters
// removed. Masking runs before the cut and again after decoding, so neither
// a truncated nor an entity-encoded credential survives.
func owdErrBody(raw []byte, req *http.Request) string {
	s := owdMaskCredentials(string(raw), req)
	if len(s) > 4096 {
		s = s[:4096]
	}
	s = strings.TrimSpace(cliutil.ScrubTerminal(cliutil.CleanText(s)))
	return owdMaskCredentials(s, req)
}

// owdGenerateRequest builds the DomainsGPT POST. The stored session cookie
// (when flags carry a config) and the partner Bearer token from
// ONEWORD_DOMAINS_GPT_TOKEN are attached only when owdCredentialAppliesToURL
// accepts the target, so a ONEWORD_DOMAINS_BASE_URL override naming another
// host, or plain http, never receives either credential.
func owdGenerateRequest(ctx context.Context, cfg *config.Config, body map[string]any) (*http.Request, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	target := owdGenerateURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", "https://"+owdCredentialDomain)
	req.Header.Set("Referer", "https://"+owdCredentialDomain+"/domains-gpt")
	if ua := cliutil.EnvOverride("ONEWORD_DOMAINS_USER_AGENT"); ua != "" {
		req.Header.Set("User-Agent", ua)
	} else {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36")
	}
	if cfg != nil && owdCredentialAppliesToURL(target, cfg.CredentialDomain) {
		if ck := strings.TrimSpace(cfg.CookieCredential()); ck != "" {
			req.Header.Set("Cookie", ck)
		}
	}
	if tok := strings.TrimSpace(os.Getenv("ONEWORD_DOMAINS_GPT_TOKEN")); tok != "" && owdCredentialAppliesToURL(target, "") {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return req, nil
}

// owdGPTConfig loads the CLI config once per command invocation for the
// DomainsGPT session cookie; nil (no cookie) when flags is nil or the config
// cannot be read.
func owdGPTConfig(flags *rootFlags) *config.Config {
	if flags == nil {
		return nil
	}
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return nil
	}
	return cfg
}

// owdGenerate calls DomainsGPT over raw HTTP (the generated client rejects the
// concatenated-JSON stream) and parses the result. Redirects are not
// followed, so a credential is never replayed to another host.
func owdGenerate(ctx context.Context, flags *rootFlags, cfg *config.Config, body map[string]any) ([]owdGenerated, []byte, error) {
	req, err := owdGenerateRequest(ctx, cfg, body)
	if err != nil {
		return nil, nil, err
	}
	timeout := 90 * time.Second
	if flags != nil && flags.timeout > 0 && flags.timeoutExplicit {
		timeout = flags.timeout
	}
	httpClient := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, apiErr(fmt.Errorf("DomainsGPT request failed: %w", err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, apiErr(fmt.Errorf("reading DomainsGPT stream: %w", err))
	}
	switch {
	case resp.StatusCode == 429:
		return nil, raw, rateLimitErr(fmt.Errorf("DomainsGPT usage limit reached (anonymous callers get 10 generations); run 'oneword-domains-pp-cli gpt usage', sign in with 'auth login --chrome' for the larger quota, or set ONEWORD_DOMAINS_GPT_TOKEN"))
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return nil, raw, authErr(fmt.Errorf("DomainsGPT returned HTTP %d; sign in with 'auth login --chrome' or set ONEWORD_DOMAINS_GPT_TOKEN", resp.StatusCode))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, raw, apiErr(fmt.Errorf("DomainsGPT returned HTTP %d: %s", resp.StatusCode, owdErrBody(raw, req)))
	}
	names, err := owdParseGPTStream(raw)
	if err != nil && len(names) == 0 {
		return nil, raw, apiErr(err)
	}
	return names, raw, err
}

// owdUsage returns DomainsGPT usage/quota, tolerating string or number
// values. ok is false when the response lacks either field.
func owdUsage(ctx context.Context, c *client.Client) (used, quota int, ok bool, err error) {
	data, err := c.Get(ctx, "/api/gpt/usage", nil)
	if err != nil {
		return 0, 0, false, err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return 0, 0, false, fmt.Errorf("decoding /api/gpt/usage: %w", err)
	}
	u, uok := cliutil.ExtractInt(probe, "usage")
	q, qok := cliutil.ExtractInt(probe, "quota")
	return int(u), int(q), uok && qok, nil
}

// owdHumanTable prints rows through the generated auto-table when a terminal wants one.
func owdHumanTable(cmd *cobra.Command, flags *rootFlags, rows any) error {
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	var items []map[string]any
	if json.Unmarshal(b, &items) == nil && len(items) > 0 {
		return printAutoTable(cmd.OutOrStdout(), items)
	}
	return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
}

// owdTLDDetailMissing reports whether a /api/tlds/{tld} body is the empty
// shell the site returns (HTTP 200) for a TLD it does not track: no registrar
// prices, no cheapest registrar and no example sites.
func owdTLDDetailMissing(data json.RawMessage) bool {
	var d struct {
		Registrars        []json.RawMessage `json:"registrars"`
		CheapestRegistrar json.RawMessage   `json:"cheapestRegistrar"`
		Sites             []json.RawMessage `json:"sites"`
	}
	if json.Unmarshal(data, &d) != nil {
		return false
	}
	cheapest := strings.TrimSpace(string(d.CheapestRegistrar))
	return len(d.Registrars) == 0 && len(d.Sites) == 0 && (cheapest == "" || cheapest == "null")
}
