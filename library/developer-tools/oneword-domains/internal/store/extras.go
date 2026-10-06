// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"fmt"
)

// migrateExtras runs after the generated store migrations and before the
// schema-version stamp. It is the canonical place for novel-feature auxiliary
// tables that need to live in the local store.
//
// Edit this file when adding tables for novel commands. Keep migrations
// idempotent with CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS so
// every store open can safely re-run them.
func (s *Store) migrateExtras(ctx context.Context, conn *sql.Conn) error {
	migrations := []string{
		// One Word Domains snapshot tables used by check, recheck, tlds drift,
		// listings watch/rank, and brainstorm.
		`CREATE TABLE IF NOT EXISTS owd_domain_checks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			word TEXT NOT NULL,
			tld TEXT NOT NULL,
			domain TEXT NOT NULL,
			available INTEGER NOT NULL,
			premium INTEGER NOT NULL,
			price TEXT,
			aftermarket INTEGER NOT NULL,
			tld_count INTEGER NOT NULL,
			checked_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS owd_domain_checks_domain_idx ON owd_domain_checks(domain, checked_at)`,
		`CREATE TABLE IF NOT EXISTS owd_tld_prices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tld TEXT NOT NULL,
			registrar TEXT NOT NULL,
			price REAL,
			snapshot_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS owd_tld_prices_idx ON owd_tld_prices(tld, registrar, snapshot_at)`,
		`CREATE TABLE IF NOT EXISTS owd_listing_seen (
			domain TEXT PRIMARY KEY,
			type TEXT,
			first_seen DATETIME NOT NULL,
			last_seen DATETIME NOT NULL,
			price TEXT,
			bid_count INTEGER,
			end_date TEXT,
			prev_price TEXT,
			prev_bid_count INTEGER,
			changed_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS owd_generations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME NOT NULL,
			batch TEXT NOT NULL,
			type TEXT,
			context TEXT,
			word TEXT,
			position TEXT,
			tld TEXT NOT NULL,
			domain TEXT NOT NULL,
			available INTEGER NOT NULL,
			real_word INTEGER,
			min_price TEXT,
			cheapest_registrar TEXT,
			cheapest_price TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS owd_generations_batch_idx ON owd_generations(batch, created_at)`,
		// words is created by the generated store migrations; slug lookups
		// (dictionary pre-checks, brainstorm real-word tests) need an index.
		`CREATE INDEX IF NOT EXISTS owd_words_slug_idx ON words(slug)`,
	}
	for _, m := range migrations {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("extra migration failed: %w", err)
		}
	}
	return nil
}
