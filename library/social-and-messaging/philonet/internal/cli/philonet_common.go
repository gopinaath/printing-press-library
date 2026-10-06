// Copyright 2026 Som Samantray and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written helpers shared by the Philonet novel commands (today, rhythm,
// digest, owed, voices, queue, resonance). Kept in its own file so regeneration
// preserves it.

package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/internal/client"
	"github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/internal/config"
	"github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/internal/store"
)

// ---- tolerant JSON accessors -------------------------------------------------

// pnMap decodes a JSON object body. Anything else (HTML error page, array,
// empty body) is an error so it is never mistaken for a successful empty result.
func pnMap(raw json.RawMessage) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unexpected response (not a JSON object): %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("unexpected empty response")
	}
	if e, ok := m["error"].(string); ok && e != "" && m["success"] != true {
		return nil, fmt.Errorf("API error: %s", e)
	}
	return m, nil
}

// pnDig walks nested maps by key. A missing or non-map step yields nil.
func pnDig(v any, path ...string) any {
	cur := v
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

func pnStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

func pnInt(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	}
	return 0
}

func pnBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func pnList(v any) []any {
	l, _ := v.([]any)
	return l
}

// pnFirstOf returns the first non-nil value.
func pnFirstOf(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

// pnFirst returns the first present, non-nil value among keys.
func pnFirst(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func pnTrunc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// ---- transport ---------------------------------------------------------------

func pnGet(ctx context.Context, c *client.Client, path string, params map[string]string) (map[string]any, error) {
	raw, err := c.Get(ctx, path, params)
	if err != nil {
		return nil, err
	}
	return pnMap(raw)
}

func pnPost(ctx context.Context, c *client.Client, path string, body any) (map[string]any, error) {
	raw, _, err := c.Post(ctx, path, body)
	if err != nil {
		return nil, err
	}
	return pnMap(raw)
}

// pnUserID reads the `uid` claim from the configured bearer token. The token is
// only decoded, never logged or stored.
func pnUserID(flags *rootFlags) (string, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return "", configErr(err)
	}
	return pnUIDFromAuthHeader(cfg.AuthHeader())
}

func pnUIDFromAuthHeader(h string) (string, error) {
	tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	parts := strings.Split(tok, ".")
	if len(parts) < 2 {
		return "", configErr(fmt.Errorf("no Philonet token configured; set PHILONET_TOKEN (see 'philonet-pp-cli doctor')"))
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return "", configErr(fmt.Errorf("token is not a JWT; copy accessToken from philonet.ai local storage"))
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", configErr(fmt.Errorf("token payload unreadable"))
	}
	uid := pnStr(claims["uid"])
	if uid == "" {
		return "", configErr(fmt.Errorf("token has no uid claim"))
	}
	return uid, nil
}

// ---- local store for history -------------------------------------------------

// Every history row carries the owning account's uid, so two accounts sharing
// one database file (for example through the generator's legacy unscoped
// data.db fallback) can never read each other's history.
const pnSchema = `
CREATE TABLE IF NOT EXISTS pn_feed_cards (
	uid TEXT NOT NULL,
	card_key TEXT NOT NULL,
	article_id TEXT, article_title TEXT, article_url TEXT, category TEXT, tags TEXT,
	reading_minutes INTEGER,
	thought_id TEXT, thought_text TEXT, insightful INTEGER, thought_created_at TEXT,
	starter_id TEXT, starter_name TEXT, starter_is_friend INTEGER,
	alma_mater TEXT, employer TEXT,
	source_feed TEXT, first_seen TEXT NOT NULL,
	PRIMARY KEY (uid, card_key)
);
CREATE INDEX IF NOT EXISTS pn_feed_cards_seen ON pn_feed_cards(uid, first_seen);
CREATE TABLE IF NOT EXISTS pn_reading_days (
	uid TEXT NOT NULL,
	day TEXT NOT NULL, seconds INTEGER NOT NULL, captured_at TEXT NOT NULL,
	PRIMARY KEY (uid, day)
);
CREATE TABLE IF NOT EXISTS pn_import_marks (
	uid TEXT NOT NULL PRIMARY KEY,
	imported_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pn_account_meta (
	uid TEXT NOT NULL PRIMARY KEY,
	timezone TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pn_snapshots (
	uid TEXT NOT NULL,
	day TEXT NOT NULL, captured_at TEXT NOT NULL,
	streak_current INTEGER, streak_max INTEGER, my_rank INTEGER, participants INTEGER,
	PRIMARY KEY (uid, day)
);`

// pnHistoryDBPath is the default history file for one account. It is keyed by
// the account id, not by the bearer token: tokens are short-lived and get
// re-copied from the browser, and the generator's default path would point a
// refreshed token at a brand-new empty database and orphan its history.
func pnHistoryDBPath(uid string) string {
	sum := sha256.Sum256([]byte(uid))
	name := "history-" + hex.EncodeToString(sum[:])[:12] + ".db"
	dir, err := cliutil.DataDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return name
		}
		dir = filepath.Join(home, ".local", "share", "philonet-pp-cli")
	}
	return filepath.Join(dir, name)
}

func pnOpenStore(ctx context.Context, dbPath, uid string) (*store.Store, error) {
	defaulted := dbPath == ""
	if defaulted {
		dbPath = pnHistoryDBPath(uid)
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := pnPrepareHistory(ctx, db.DB()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if defaulted {
		// Carry this account's history forward from the previous default
		// location (once). An explicit --db is the caller's choice: no import.
		// The import is optional, so a problem with the old file is a warning,
		// never a reason to refuse to open the new history.
		if err := pnImportPriorHistory(ctx, db.DB(), defaultDBPath("philonet-pp-cli"), dbPath, uid); err != nil {
			if ctx.Err() != nil {
				_ = db.Close()
				return nil, err
			}
			fmt.Fprintf(os.Stderr, "warning: could not import earlier history (continuing without it): %v\n", err)
		}
	}
	return db, nil
}

// pnPrepareHistory migrates legacy tables and creates the scoped schema in ONE
// transaction. The store opens SQLite with _txlock=immediate, so BeginTx takes
// the write lock up front; a second process opening the same database waits
// (busy_timeout) and then sees the finished migration instead of racing the
// check-then-rename and failing.
func pnPrepareHistory(ctx context.Context, db *sql.DB) error {
	return pnRetryBusy(ctx, func() error { return pnPrepareHistoryOnce(ctx, db) })
}

// pnRetryBusy runs fn, retrying a bounded number of times when another process
// holds the SQLite write lock. Cancellation is reported together with the last
// busy error.
func pnRetryBusy(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 1; attempt <= pnBusyAttempts; attempt++ {
		if err = fn(); err == nil || !pnIsBusy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), err)
		case <-time.After(time.Duration(attempt) * 75 * time.Millisecond):
		}
	}
	return err
}

// pnBusyAttempts bounds retries when another process holds the write lock for
// longer than SQLite's own busy timeout (a slow disk, a long sync).
const pnBusyAttempts = 12

func pnIsBusy(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "database is locked") || strings.Contains(m, "sqlite_busy") || strings.Contains(m, "database table is locked")
}

func pnPrepareHistoryOnce(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("preparing history tables: %w", err)
	}
	if err := pnMigrateUnscoped(ctx, tx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("migrating history tables: %w", err)
	}
	if _, err := tx.ExecContext(ctx, pnSchema); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("preparing history tables: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("preparing history tables: %w", err)
	}
	return nil
}

// pnQuerier is the part of *sql.DB and *sql.Tx the migration needs.
type pnQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// pnMigrateUnscoped sets aside history tables created before rows carried an
// account id. Those rows cannot be attributed to an account, so they are kept
// under a *_legacy_unscoped name (never read) rather than shown to anyone.
func pnMigrateUnscoped(ctx context.Context, db pnQuerier) error {
	// Table names below are constants plus an integer suffix; nothing is built
	// from input.
	for _, table := range []string{"pn_feed_cards", "pn_reading_days", "pn_snapshots", "pn_account_meta"} {
		exists, hasUID, err := pnTableShape(ctx, db, table)
		if err != nil {
			return err
		}
		if !exists || hasUID {
			continue
		}
		// Pick a legacy name that is free, so a second unscoped table (for
		// example re-created by an older build) never blocks the migration.
		target := table + "_legacy_unscoped"
		for n := 2; ; n++ {
			taken, _, err := pnTableShape(ctx, db, target)
			if err != nil {
				return err
			}
			if !taken {
				break
			}
			target = table + "_legacy_unscoped_" + strconv.Itoa(n)
		}
		if table == "pn_feed_cards" {
			if _, err := db.ExecContext(ctx, `DROP INDEX IF EXISTS pn_feed_cards_seen`); err != nil {
				return err
			}
		}
		if _, err := db.ExecContext(ctx, `ALTER TABLE `+table+` RENAME TO `+target); err != nil {
			return err
		}
	}
	return nil
}

// pnTableShape reports whether table exists and whether it has a uid column.
func pnTableShape(ctx context.Context, db pnQuerier, table string) (exists, hasUID bool, err error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, false, err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return false, false, err
		}
		exists = true
		if name == "uid" {
			hasUID = true
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, false, err
	}
	return exists, hasUID, rows.Close()
}

// pnRefreshMode turns the global --data-source flag and the command's own
// --no-refresh into one decision for store-backed commands:
//
//	auto  -> refresh from the API unless --no-refresh; refresh failures are warnings
//	local -> never contact the API
//	live  -> always refresh and fail rather than fall back to stored data
func pnRefreshMode(flags *rootFlags, noRefresh bool) (refresh, strict bool, err error) {
	switch flags.dataSource {
	case "local":
		return false, false, nil
	case "live":
		if noRefresh {
			return false, false, usageErr(fmt.Errorf("--no-refresh conflicts with --data-source live"))
		}
		return true, true, nil
	default:
		return !noRefresh, false, nil
	}
}

// pnRequireLive rejects --data-source local on commands that have no stored
// data to serve it from.
func pnRequireLive(flags *rootFlags) error {
	if err := validateDataSourceStrategy(flags, "live"); err != nil {
		return usageErr(err)
	}
	return nil
}

// ---- feed cards --------------------------------------------------------------

type pnCard struct {
	Key            string
	ArticleID      string
	ArticleTitle   string
	ArticleURL     string
	Category       string
	Tags           []string
	ReadingMinutes int64
	ThoughtID      string
	ThoughtText    string
	Insightful     int64
	ThoughtAt      string
	StarterID      string
	StarterName    string
	StarterFriend  bool
	AlmaMater      string
	Employer       string
}

// pnCardsFromItem flattens one feed item into one card per conversation starter.
func pnCardsFromItem(item map[string]any) []pnCard {
	art, _ := item["article"].(map[string]any)
	if art == nil {
		return nil
	}
	var tags []string
	for _, t := range pnList(art["tags"]) {
		if s := pnStr(t); s != "" {
			tags = append(tags, s)
		}
	}
	base := pnCard{
		ArticleID:      pnStr(art["id"]),
		ArticleTitle:   pnStr(pnFirst(art, "title", "headline")),
		ArticleURL:     pnStr(art["url"]),
		Category:       pnStr(art["category"]),
		Tags:           tags,
		ReadingMinutes: pnInt(pnDig(art, "reading_time", "minutes")),
	}
	var out []pnCard
	for _, raw := range pnList(item["conversation_starters"]) {
		s, _ := raw.(map[string]any)
		if s == nil {
			continue
		}
		st, _ := s["starter"].(map[string]any)
		c := base
		c.ThoughtID = pnStr(s["id"])
		c.ThoughtText = pnStr(pnFirst(s, "content", "quote_headline", "quote"))
		c.Insightful = pnInt(s["insightful_count"]) + pnInt(s["star_count"]) + pnInt(s["resonate_count"])
		c.ThoughtAt = pnStr(s["created_at"])
		if st != nil {
			c.StarterID = pnStr(st["user_id"])
			c.StarterName = pnStr(st["name"])
			c.StarterFriend = pnBool(st["is_friend"])
			c.AlmaMater = pnStr(pnDig(st, "verifications", "alma_mater", "entity_name"))
			c.Employer = pnStr(pnDig(st, "verifications", "professional", "entity_name"))
		}
		if c.ThoughtID == "" {
			continue // without a thought id the card cannot be told apart from its siblings
		}
		c.Key = c.ArticleID + ":" + c.ThoughtID
		out = append(out, c)
	}
	return out
}

func pnStoreCards(ctx context.Context, db *sql.DB, uid string, cards []pnCard, feed string, now time.Time) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO pn_feed_cards
		(uid,card_key,article_id,article_title,article_url,category,tags,reading_minutes,thought_id,thought_text,insightful,thought_created_at,starter_id,starter_name,starter_is_friend,alma_mater,employer,source_feed,first_seen)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(uid, card_key) DO UPDATE SET insightful=excluded.insightful,
		starter_is_friend=MAX(pn_feed_cards.starter_is_friend, excluded.starter_is_friend),
		article_title=excluded.article_title, thought_text=excluded.thought_text, starter_name=excluded.starter_name,
		alma_mater=excluded.alma_mater, employer=excluded.employer`)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	n := 0
	for _, c := range cards {
		fr := 0
		if c.StarterFriend {
			fr = 1
		}
		if _, err := stmt.ExecContext(ctx, uid, c.Key, c.ArticleID, c.ArticleTitle, c.ArticleURL, c.Category, strings.Join(c.Tags, "|"), c.ReadingMinutes,
			c.ThoughtID, c.ThoughtText, c.Insightful, c.ThoughtAt, c.StarterID, c.StarterName, fr, c.AlmaMater, c.Employer, feed, now.UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

// pnRefreshFeeds pulls up to maxPages pages of each feed and stores the cards.
// It returns cards stored and per-page failures (kept, never silently dropped).
// In strict mode it stops at the first failure instead of walking the remaining
// feeds (each failing page already costs the client's full retry budget).
func pnRefreshFeeds(ctx context.Context, c *client.Client, db *sql.DB, uid string, feeds []string, maxPages int, strict bool) (int, []string) {
	total := 0
	var failures []string
	for _, feed := range feeds {
		session := ""
		for page := 1; page <= maxPages; page++ {
			params := map[string]string{"filter": feed, "page": strconv.Itoa(page), "pageSize": "10"}
			if session != "" {
				params["sessionId"] = session
			}
			m, err := pnGet(ctx, c, "/v1/feed2/forme", params)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s page %d: %v", feed, page, err))
				if strict {
					return total, failures
				}
				break
			}
			if s := pnStr(pnDig(m, "meta", "session_id")); s != "" {
				session = s
			}
			var cards []pnCard
			for _, it := range pnList(m["items"]) {
				if im, ok := it.(map[string]any); ok {
					cards = append(cards, pnCardsFromItem(im)...)
				}
			}
			n, err := pnStoreCards(ctx, db, uid, cards, feed, time.Now())
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s page %d store: %v", feed, page, err))
				if strict {
					return total, failures
				}
				break
			}
			total += n
			if !pnBool(m["has_more"]) || len(pnList(m["items"])) == 0 {
				break
			}
		}
	}
	return total, failures
}

// ---- small shared views ------------------------------------------------------

type pnFailure struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

func pnWeekStart(t time.Time) time.Time {
	d := int(t.Weekday()+6) % 7 // Monday = 0
	y, m, day := t.Date()
	return time.Date(y, m, day-d, 0, 0, 0, 0, t.Location())
}

func pnSortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// pnStoredLocation returns the account time zone recorded by the last refresh,
// or time.Local when none is recorded or it cannot be loaded.
func pnStoredLocation(ctx context.Context, db *sql.DB, uid string) *time.Location {
	var tz string
	if err := db.QueryRowContext(ctx, `SELECT timezone FROM pn_account_meta WHERE uid = ?`, uid).Scan(&tz); err != nil || tz == "" {
		return time.Local
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.Local
}

// pnImportPriorHistory copies this account's rows from the previous default
// history file (the generator's token-keyed database) into the new per-account
// file, once. Only tables that carry a uid column in BOTH files are read, only
// the columns the two files share are copied (by name, never by position), and
// only rows whose uid matches. Rows that cannot be attributed to an account are
// never imported. A missing prior file writes no marker, so an import can still
// happen if the file turns up later; the marker is written after a successful
// import, or when the prior file is the new file itself.
func pnImportPriorHistory(ctx context.Context, db *sql.DB, priorPath, newPath, uid string) error {
	var marked int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pn_import_marks WHERE uid = ?`, uid).Scan(&marked); err != nil {
		return fmt.Errorf("checking history import: %w", err)
	}
	if marked > 0 {
		return nil
	}
	if a, aerr := filepath.Abs(priorPath); aerr == nil {
		if b, berr := filepath.Abs(newPath); berr == nil && a == b {
			return pnMarkImported(ctx, db, uid)
		}
	}
	if _, err := os.Stat(priorPath); err != nil {
		return nil // nothing to import (yet)
	}
	return pnRetryBusy(ctx, func() error { return pnImportOnce(ctx, db, priorPath, uid) })
}

func pnMarkImported(ctx context.Context, db *sql.DB, uid string) error {
	_, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO pn_import_marks(uid, imported_at) VALUES(?, ?)`, uid, time.Now().UTC().Format(time.RFC3339))
	return err
}

// pnHistoryKeys are the columns that must exist on both sides for a table to be
// copied: the owning account and the row key.
var pnHistoryKeys = map[string][]string{
	"pn_feed_cards":   {"uid", "card_key"},
	"pn_reading_days": {"uid", "day"},
	"pn_snapshots":    {"uid", "day"},
	"pn_account_meta": {"uid"},
}

var pnSafeIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func pnImportOnce(ctx context.Context, db *sql.DB, priorPath, uid string) (retErr error) {
	conn, err := db.Conn(ctx) // ATTACH is per-connection
	if err != nil {
		return err
	}
	attached := false
	defer func() {
		if attached {
			if _, derr := conn.ExecContext(context.Background(), `DETACH DATABASE prior`); derr != nil {
				// Never hand a still-attached connection back to the pool.
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
		}
		_ = conn.Close()
	}()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS prior`, priorPath); err != nil {
		return fmt.Errorf("opening prior history: %w", err)
	}
	attached = true
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, table := range []string{"pn_feed_cards", "pn_reading_days", "pn_snapshots", "pn_account_meta"} {
		cols, err := pnSharedColumns(ctx, tx, table)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if cols == nil {
			continue // not attributable to an account, or nothing shared
		}
		list := strings.Join(cols, ", ")
		// Table names are constants and column names were validated against
		// pnSafeIdent and read from the schema, never from input.
		stmt := `INSERT OR IGNORE INTO main.` + table + ` (` + list + `) SELECT ` + list + ` FROM prior.` + table + ` WHERE uid = ?`
		if _, err := tx.ExecContext(ctx, stmt, uid); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("importing prior %s: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO main.pn_import_marks(uid, imported_at) VALUES(?, ?)`, uid, time.Now().UTC().Format(time.RFC3339)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// pnSharedColumns returns the columns present in both main.table and
// prior.table, in main's order, or nil when the prior table is missing or lacks
// the key columns (an older unscoped build, or an unrelated schema).
func pnSharedColumns(ctx context.Context, q pnQuerier, table string) ([]string, error) {
	read := func(query string) (map[string]bool, []string, error) {
		rows, err := q.QueryContext(ctx, query, table)
		if err != nil {
			return nil, nil, err
		}
		set := map[string]bool{}
		var order []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				_ = rows.Close()
				return nil, nil, err
			}
			set[name] = true
			order = append(order, name)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, nil, err
		}
		return set, order, rows.Close()
	}
	prior, _, err := read(`SELECT name FROM pragma_table_info(?, 'prior')`)
	if err != nil {
		return nil, err
	}
	_, mainOrder, err := read(`SELECT name FROM pragma_table_info(?, 'main')`)
	if err != nil {
		return nil, err
	}
	for _, k := range pnHistoryKeys[table] {
		if !prior[k] {
			return nil, nil
		}
	}
	var shared []string
	for _, c := range mainOrder {
		if prior[c] && pnSafeIdent.MatchString(c) {
			shared = append(shared, c)
		}
	}
	for _, k := range pnHistoryKeys[table] {
		found := false
		for _, c := range shared {
			if c == k {
				found = true
			}
		}
		if !found {
			return nil, nil
		}
	}
	return shared, nil
}
