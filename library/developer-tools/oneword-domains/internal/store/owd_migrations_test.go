// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestOWDSchemaExtras asserts that the One Word Domains snapshot tables and
// indexes registered in migrateExtras exist after a plain open, and that
// re-opening the same store (re-running the migrations) is idempotent.
func TestOWDSchemaExtras(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "owd.db")
	s, err := OpenWithContext(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithContext(ctx, path)
	if err != nil {
		t.Fatalf("re-open must be idempotent: %v", err)
	}
	defer s.Close()
	for _, obj := range []struct{ typ, name string }{
		{"table", "owd_domain_checks"},
		{"table", "owd_tld_prices"},
		{"table", "owd_listing_seen"},
		{"table", "owd_generations"},
		{"index", "owd_domain_checks_domain_idx"},
		{"index", "owd_tld_prices_idx"},
		{"index", "owd_generations_batch_idx"},
		{"index", "owd_words_slug_idx"},
	} {
		var n int
		if err := s.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type=? AND name=?`, obj.typ, obj.name).Scan(&n); err != nil || n != 1 {
			t.Fatalf("%s %s missing (n=%d err=%v)", obj.typ, obj.name, n, err)
		}
	}
	if _, err := s.DB().Exec(`INSERT INTO owd_domain_checks (word, tld, domain, available, premium, aftermarket, tld_count, checked_at) VALUES ('smart','com','smart.com',0,0,0,6,'2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
}
