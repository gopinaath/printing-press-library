// Copyright 2026 Victor Wibisono and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored root-level `hacks`: domain hacks (sm.art, sma.rt) for one
// word with live availability for stems the dictionary knows, or every
// dictionary word that ends in a given TLD (from the local words table,
// paged from the directory when the table is thin).
// pp:data-source auto

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/client"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/internal/store"
	"github.com/spf13/cobra"
)

// owdHackRow is one domain-hack candidate for a word with its live status.
type owdHackRow struct {
	owdHack
	Checkable bool    `json:"checkable"`
	Available *bool   `json:"available"`
	Premium   *bool   `json:"premium"`
	Price     *string `json:"price"`
	Error     string  `json:"error,omitempty"`
}

// owdHackWordRow is one dictionary word that ends in the requested TLD.
type owdHackWordRow struct {
	Word   string `json:"word"`
	Stem   string `json:"stem"`
	TLD    string `json:"tld"`
	Domain string `json:"domain"`
}

// owdHacksRun is the resolved state both `hacks` modes share.
type owdHacksRun struct {
	flags     *rootFlags
	c         *client.Client // nil under --data-source local
	db        *store.Store   // nil when the store could not be opened (live mode only)
	tld       string
	limit     int
	maxPages  int
	local     bool
	forceLive bool
}

// owdSuffixWords keeps the words that end in tld with a non-empty stem,
// de-duplicated and ranked so --limit keeps the most useful hacks: short
// multi-letter stems first (sm.art, ch.art), single-letter stems last because
// registries usually reserve one-character names, alphabetical within a tie.
func owdSuffixWords(words []string, tld string) []string {
	tld = owdNormTLD(tld)
	out := make([]string, 0)
	if tld == "" {
		return out
	}
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if len(w) <= len(tld) || !strings.HasSuffix(w, tld) {
			continue
		}
		out = append(out, w)
	}
	out = owdDedupe(out)
	sort.Slice(out, func(i, j int) bool {
		a, b := len(out[i])-len(tld), len(out[j])-len(tld)
		if (a == 1) != (b == 1) {
			return b == 1
		}
		if a != b {
			return a < b
		}
		return out[i] < out[j]
	})
	return out
}

// owdHackWordRows turns suffix matches into rows (stem.tld).
func owdHackWordRows(words []string, tld string, limit int) []owdHackWordRow {
	rows := make([]owdHackWordRow, 0, len(words))
	for _, w := range words {
		if limit > 0 && len(rows) >= limit {
			break
		}
		stem := w[:len(w)-len(tld)]
		rows = append(rows, owdHackWordRow{Word: w, Stem: stem, TLD: tld, Domain: stem + "." + tld})
	}
	return rows
}

// owdSlugsFromRaw extracts the slug of each raw /api/words row.
func owdSlugsFromRaw(items []json.RawMessage) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		var r struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal(it, &r) == nil && r.Slug != "" {
			out = append(out, r.Slug)
		}
	}
	return out
}

// owdLocalSuffixWords reads dictionary words ending in tld from the typed words table.
func owdLocalSuffixWords(ctx context.Context, db *store.Store, tld string) ([]string, error) {
	out := make([]string, 0)
	if db == nil {
		return out, nil
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT slug FROM words WHERE slug LIKE ? AND length(slug) > ?`, "%"+tld, len(tld))
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			_ = rows.Close()
			return out, err
		}
		out = append(out, s)
	}
	_ = rows.Close()
	return out, rows.Err()
}

// owdHacksTLDMode lists the dictionary words that end in the TLD: the live
// directory is paged first when the local words table is thin (or on
// --refresh), then the local table is read and the union is filtered.
func owdHacksTLDMode(ctx context.Context, cmd *cobra.Command, run owdHacksRun) error {
	count := owdWordsTableCount(ctx, run.db)
	fetch := !run.local && (run.forceLive || count < owdMinLocalWords)
	words := make([]string, 0)
	if fetch {
		pages := owdDogfoodCap(run.maxPages, owdDogfoodPages)
		for page := 1; page <= pages; page++ {
			items, err := owdFetchWordsPage(ctx, run.c, map[string]string{"contains": run.tld}, page)
			if err != nil {
				if page == 1 {
					return owdAPIErr(cmd, run.flags, err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: stopped at page %d: %v\n", page, err)
				break
			}
			if run.db != nil && len(items) > 0 {
				if _, _, err := run.db.UpsertBatch("words", items); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: caching words locally failed: %v\n", err)
				}
			}
			words = append(words, owdSlugsFromRaw(items)...)
			if len(items) < owdPageSize {
				break
			}
		}
	} else {
		maybeEmitSyncHints(cmd, run.db, "words", run.flags.maxAge)
	}
	localWords, err := owdLocalSuffixWords(ctx, run.db, run.tld)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: reading local words failed: %v\n", err)
	}
	words = append(words, localWords...)
	rows := owdHackWordRows(owdSuffixWords(words, run.tld), run.tld, run.limit)
	if !wantsHumanTable(cmd.OutOrStdout(), run.flags) {
		return printJSONFiltered(cmd.OutOrStdout(), rows, run.flags)
	}
	if len(rows) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No dictionary words ending in .%s found (try --refresh --max-pages 20).\n", run.tld)
		return nil
	}
	return owdHumanTable(cmd, run.flags, rows)
}

// owdHacksWordMode splits the word at every TLD ending and checks the stems
// the dictionary knows: live, or from the latest local snapshot under
// --data-source local.
func owdHacksWordMode(ctx context.Context, cmd *cobra.Command, run owdHacksRun, word string, slugs []string) error {
	candidateTLDs := slugs
	if run.tld != "" {
		candidateTLDs = []string{run.tld}
	}
	cands := owdHacks(word, candidateTLDs)
	rows := make([]owdHackRow, 0, len(cands))
	if len(cands) == 0 {
		if !wantsHumanTable(cmd.OutOrStdout(), run.flags) {
			return printJSONFiltered(cmd.OutOrStdout(), rows, run.flags)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "No TLD ends %q (try 'hacks --tld <tld>' to search words by ending).\n", word)
		return nil
	}
	stems := make([]string, 0, len(cands))
	for _, h := range cands {
		stems = append(stems, h.Stem)
	}
	stems = owdDedupe(stems)
	dict, err := owdDictPrecheck(ctx, run.c, run.db, stems, owdDefaultConcurrency)
	if err != nil {
		return owdTypedErr(cmd, run.flags, err)
	}
	checkable := make([]owdHack, 0, len(cands))
	for _, h := range cands {
		if dict.InDict[h.Stem] {
			checkable = append(checkable, h)
		}
	}
	checkByDomain := map[string]*owdDomainCheck{}
	failedByDomain := map[string]string{}
	var checkErrs []cliutil.FanoutError
	var snap owdSnapshotErrs
	if run.local {
		domains := make([]string, 0, len(checkable))
		for _, h := range checkable {
			domains = append(domains, h.Domain)
		}
		latest := owdLatestChecks(ctx, run.db, domains)
		for _, h := range checkable {
			if dc, ok := latest[h.Domain]; ok {
				checkByDomain[h.Domain] = dc
			} else {
				failedByDomain[h.Domain] = "no local snapshot (run without --data-source local)"
			}
		}
	} else {
		domains := make([]string, 0, len(checkable))
		for _, h := range checkable {
			domains = append(domains, h.Domain)
		}
		checks, errs, err := owdChecksByDomain(ctx, run.c, run.db, domains, owdDefaultConcurrency, owdNow(), &snap)
		if err != nil {
			return owdTypedErr(cmd, run.flags, err)
		}
		checkErrs = errs
		checkByDomain, failedByDomain = owdIndexChecks(checks, checkErrs)
	}
	for _, h := range cands {
		row := owdHackRow{owdHack: h, Checkable: dict.InDict[h.Stem]}
		if dc, ok := checkByDomain[h.Domain]; ok {
			avail, prem := dc.Available, dc.Premium
			row.Available, row.Premium, row.Price = &avail, &prem, dc.Price
		} else if msg, failed := failedByDomain[h.Domain]; failed {
			row.Error = msg
		}
		rows = append(rows, row)
	}
	if run.limit > 0 && len(rows) > run.limit {
		rows = rows[:run.limit]
	}
	allErrs := slices.Concat(dict.Errs, checkErrs)
	owdWarnFailuresInline(cmd.ErrOrStderr(), allErrs, len(stems)+len(checkable), "fetches")
	snap.warn(cmd.ErrOrStderr())
	if !wantsHumanTable(cmd.OutOrStdout(), run.flags) {
		return printJSONFiltered(cmd.OutOrStdout(), rows, run.flags)
	}
	return owdHumanTable(cmd, run.flags, rows)
}

func newNovelHacksCmd(flags *rootFlags) *cobra.Command {
	var tld string
	var limit, maxPages int
	var refresh bool
	cmd := &cobra.Command{
		Use:   "hacks [<word>]",
		Short: "Find domain hacks: a word split so its ending is a TLD (sm.art), or every dictionary word ending in a TLD",
		Long: strings.TrimSpace(`
With a word, list every split whose ending is one of the 93 TLDs (smart ->
sm.art, sma.rt). Stems the dictionary knows are checked live for availability;
other stems are listed with checkable=false because the site can only price
dictionary words. With --tld and no word, list dictionary words that end in
that TLD (--tld art -> smart, heart, chart) from the local words table, paging
the live directory first when the table holds fewer than 1000 words. Results
are ranked shortest multi-letter stem first, with single-letter stems (b.art)
last, so --limit keeps the strongest hacks.`),
		Example: strings.Trim(`
  oneword-domains-pp-cli hacks smart
  oneword-domains-pp-cli hacks smart --json
  oneword-domains-pp-cli hacks --tld art --limit 50
  oneword-domains-pp-cli hacks --tld ly --refresh --max-pages 20 --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "auto",
			"pp:happy-args":  "word=smart",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "hacks")
			}
			if len(args) > 1 {
				return usageErr(fmt.Errorf("hacks takes one word"))
			}
			word := ""
			if len(args) == 1 {
				word = strings.ToLower(strings.TrimSpace(args[0]))
			}
			tld = owdNormTLD(tld)
			if word == "" && tld == "" {
				return owdMissingArgErr(cmd, flags, "<word> | --tld <tld>")
			}
			if word != "" {
				if err := owdValidateWord(word); err != nil {
					return err
				}
			}
			if tld != "" {
				if err := owdValidateTLD(tld); err != nil {
					return err
				}
			}
			if limit < 0 {
				return usageErr(fmt.Errorf("--limit must be zero or positive"))
			}
			if maxPages < 1 {
				return usageErr(fmt.Errorf("--max-pages must be at least 1"))
			}
			run := owdHacksRun{
				flags: flags, tld: tld, limit: limit, maxPages: maxPages,
				local:     flags.dataSource == "local",
				forceLive: refresh || flags.dataSource == "live",
			}
			ctx, cancel := owdLoopCtx(cmd, flags)
			defer cancel()
			if run.local {
				db, _, err := owdOpenStore(ctx)
				if err != nil {
					return err
				}
				defer db.Close()
				run.db = db
			} else {
				var err error
				if run.c, err = flags.newClient(); err != nil {
					return err
				}
				db, closeDB := owdOpenStoreOptional(ctx, cmd.ErrOrStderr())
				defer closeDB()
				run.db = db
			}
			tlds, _, err := owdFetchTLDs(ctx, run.c, run.db, run.forceLive, cmd.ErrOrStderr())
			if err != nil {
				if run.local {
					hintIfUnsynced(cmd, run.db, "tlds")
					return usageErr(fmt.Errorf("--data-source local needs synced TLDs: %w", err))
				}
				return owdAPIErr(cmd, flags, err)
			}
			known, slugs := owdIndexTLDs(tlds)
			if tld != "" {
				if err := owdRequireKnownTLDs([]string{tld}, known); err != nil {
					return err
				}
			}
			if word == "" {
				return owdHacksTLDMode(ctx, cmd, run)
			}
			return owdHacksWordMode(ctx, cmd, run, word, slugs)
		},
	}
	cmd.Flags().StringVar(&tld, "tld", "", "TLD ending to search for (with no word: list dictionary words ending in it)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows to print (0 for all)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Page the live dictionary again even when the local words table is populated")
	cmd.Flags().IntVar(&maxPages, "max-pages", 10, "Dictionary pages (100 words each) to fetch in --tld mode")
	return cmd
}
