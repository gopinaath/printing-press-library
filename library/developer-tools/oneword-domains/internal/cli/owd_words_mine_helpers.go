// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Pure helpers for words mine: the SQLite query builder over the typed words
// table and the dictionary refresh loop.

package cli

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/client"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
)

// owdMineCategories mirrors the category slugs the site's directory accepts.
var owdMineCategories = []string{"adjectives", "adverbs", "battleships", "collections", "french", "gods", "names", "nouns", "other", "places", "positive", "scientific", "spanish", "species", "stars", "tech", "verbs"}

// owdMineOpts are the filters words mine turns into SQL.
type owdMineOpts struct {
	Categories []string
	Any        bool
	MinLen     int
	MaxLen     int
	Prefix     string
	Glob       string
	Limit      int
}

// owdMineRow is one dictionary word with its categories.
type owdMineRow struct {
	Word       string   `json:"word"`
	Categories []string `json:"categories"`
	Length     int      `json:"length"`
}

// owdMineEscapeLike escapes LIKE wildcards so a prefix matches literally.
func owdMineEscapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// owdMineQuery builds the words query. Categories AND together through
// GROUP BY/HAVING on the distinct matched slugs; --any relaxes that to OR.
func owdMineQuery(o owdMineOpts) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0, 8)
	sb.WriteString(`SELECT w.slug, w.data FROM words w`)
	if len(o.Categories) > 0 {
		sb.WriteString(` JOIN json_each(json_extract(w.data, '$.categories')) je ON json_extract(je.value, '$.categorySlug') IN (`)
		for i, c := range o.Categories {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("?")
			args = append(args, c)
		}
		sb.WriteString(")")
	}
	where := []string{`w.slug IS NOT NULL`, `w.slug <> ''`}
	if o.MinLen > 0 {
		where = append(where, `length(w.slug) >= ?`)
		args = append(args, o.MinLen)
	}
	if o.MaxLen > 0 {
		where = append(where, `length(w.slug) <= ?`)
		args = append(args, o.MaxLen)
	}
	if o.Prefix != "" {
		where = append(where, `w.slug LIKE ? ESCAPE '\'`)
		args = append(args, owdMineEscapeLike(strings.ToLower(o.Prefix))+"%")
	}
	if o.Glob != "" {
		where = append(where, `w.slug GLOB ?`)
		args = append(args, strings.ToLower(o.Glob))
	}
	sb.WriteString(" WHERE " + strings.Join(where, " AND "))
	if len(o.Categories) > 0 {
		sb.WriteString(` GROUP BY w.slug`)
		if !o.Any {
			sb.WriteString(` HAVING COUNT(DISTINCT json_extract(je.value, '$.categorySlug')) = ?`)
			args = append(args, len(o.Categories))
		}
	}
	sb.WriteString(` ORDER BY w.slug`)
	if o.Limit > 0 {
		sb.WriteString(` LIMIT ?`)
		args = append(args, o.Limit)
	}
	return sb.String(), args
}

// owdMineCategoriesOf extracts the sorted category slugs from a stored words row.
func owdMineCategoriesOf(data string) []string {
	var row struct {
		Categories []struct {
			CategorySlug string `json:"categorySlug"`
		} `json:"categories"`
	}
	out := make([]string, 0)
	if json.Unmarshal([]byte(data), &row) != nil {
		return out
	}
	for _, c := range row.Categories {
		if c.CategorySlug != "" {
			out = append(out, c.CategorySlug)
		}
	}
	sort.Strings(out)
	return owdDedupe(out)
}

// owdMineRun executes the query and drains every row before returning.
func owdMineRun(ctx context.Context, db *store.Store, query string, args []any) ([]owdMineRow, error) {
	rows, err := db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]owdMineRow, 0)
	for rows.Next() {
		var slug string
		var data any
		if err := rows.Scan(&slug, &data); err != nil {
			return nil, err
		}
		out = append(out, owdMineRow{Word: slug, Categories: owdMineCategoriesOf(owdScanString(data)), Length: len(slug)})
	}
	return out, rows.Err()
}

// owdMineRefresh pages the whole dictionary into the typed words table until a
// short page or the page cap. complete reports whether the short page was reached.
func owdMineRefresh(ctx context.Context, c *client.Client, db *store.Store, maxPages int) (stored, pages int, complete bool, err error) {
	for page := 1; page <= maxPages; page++ {
		items, err := owdFetchWordsPage(ctx, c, nil, page)
		if err != nil {
			return stored, pages, false, err
		}
		pages++
		if len(items) > 0 {
			n, _, err := db.UpsertBatch("words", items)
			stored += n
			if err != nil {
				return stored, pages, false, err
			}
		}
		if len(items) < owdPageSize {
			return stored, pages, true, nil
		}
	}
	return stored, pages, false, nil
}
