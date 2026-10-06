Manifest transcendence rows: 6 planned, 6 built. Phase 3 completion gate passed (help-walk 15/15, dogfood novel_features_check planned=6 found=6).

# Build log: oneword-domains-pp-cli

Absorbed hand-built rows (5): check (cross-TLD + --file), tlds drift, listings watch, hacks, compare. Behaviour rows: check --max-price, check popularity percent, listings list --ending-within.
Transcendence rows (6): brainstorm, recheck, listings rank, domains intersect, words mine, tlds inventory.

## Slice 1 (foundation) — done
- `internal/store/owd_migrations.go`: `EnsureOWDSchema` creates owd_domain_checks, owd_tld_prices, owd_listing_seen, owd_generations (idempotent, lazily called by `owdOpenStore`). Test: `owd_migrations_test.go`.
- `internal/cli/owd_helpers.go` (+ tests): TLD/detail/domain-check/listings/words fetchers, snapshot writers, listing diff, popularity math, domain hacks splitter, DomainsGPT stream parser, session/unknown-word error mapping, dogfood caps.
- `internal/cli/owd_gpt_generate.go`: hand-built `gpt generate` replaces the generated endpoint (the generated JSON client rejects the concatenated-object stream). Spec change: `gpt.generate` removed from `oneword-domains-spec.yaml`; manifest row 11 now `oneword-domains-pp-cli gpt generate`. Uses raw HTTP against the same-origin route so the stored session cookie gets the signed-in quota; `ONEWORD_DOMAINS_GPT_TOKEN` adds a partner Bearer token.
- Spec change: `domains.count` lost `no_auth: true` (the route is session-gated; the earlier 200 was an edge-cached response). Consequence: `tlds inventory` needs the lifetime-pass session.
- Decision: generated sync stays single-page for words/listings (cache auto-refresh must stay cheap); `words mine` and `hacks` load the full dictionary themselves (`--refresh`), `listings watch/rank` page /api/listings themselves.
- Manifest row 25 (`--ending-within`) is implemented as `listings rank --ending <dur>` and `listings watch --ending-within <dur>` rather than a flag on the generated `listings list` (generated files are not hand-edited).

## Slice 2 (absorbed hand-built) — done
Files: `internal/cli/owd_check.go`, `owd_compare.go`, `owd_hacks.go`, `owd_tlds_drift.go`, `owd_listings_watch.go` (+ tests, 29 TestOwd* pass).
Live assertions passed: `check smart --tld com,io,ai` (3 rows, smart.com available=false tld_count=6 popularity 93.5%, registrar prices joined); `check zzqqxx` exits 3 without any 500 retry (dictionary pre-check); `--select` narrows; `compare` wrapper with cheapest_available and fetch_failures; `hacks smart` → sm.art (checkable=false, stem not a word); `hacks --tld art --limit 5` → apart, bart, ...; `tlds drift --snapshot` records 87 min prices (6 TLDs have empty minPrice) and reports [] until a second day; `listings watch --tld co` first run 91 new, second run 91 unchanged; every command dry-run/--help/--json/negative-path OK.
Judgement calls: check emits a `{code:3,...,suggestions}` envelope for a single unknown word; per-row `error` instead of a wrapper for check; watch `new` rows keep the API's camelCase, `ending_soon` is snake_case; hacks unions live-paged suffix matches with local words.

## Slice 3 (transcendence) — done
Scaffolds implemented: brainstorm (live), recheck (auto), listings rank (auto), domains intersect (live), words mine (local), tlds inventory (live); helpers in owd_novel_helpers.go, owd_<name>_helpers.go; real tests with httptest fixtures cover the pass-gated happy paths and the 401 -> exit 4 path.
Live assertions passed: brainstorm (13 available .ai names priced and saved to owd_generations); recheck (4 pairs re-checked, no changes; empty-history hint); listings rank --tld co (91 scanned, 5 scored, taken_of_93/price_x/bids_per_day, --sort ending ascending); words mine (AND/OR/glob, --refresh grows the words table); domains intersect and tlds inventory return the typed session error (exit 4) without a session.
Deviations: intersect accepts one --tld when --taken-on is given; rank rows contain only scored listings (matched count + note for the rest); recheck adds baseline/note and disables the HTTP cache; brainstorm real_word is null once the live lookup budget is spent.

## Priority 1 review gate
tlds get / words list / listings filters: --help examples present, --dry-run prints the request, --json parses (object/array). Generated bare-integer count routes decode cleanly; the usage route decodes string-or-number.

## Completion gate
- help-walk: brainstorm, recheck, listings rank, domains intersect, words mine, tlds inventory, check, compare, hacks, tlds drift, listings watch, gpt generate, tlds list, sync, agent-context -> all resolve with "<path> [flags]".
- dogfood --research-dir: novel_features_check planned=6 found=6, verdict WARN with two template-shape warnings (4 generator dead helpers; config write/read field regex mismatch) -> retro candidates, not CLI bugs.
- Test presence: internal/cli and internal/store hand-authored helpers have table-driven tests.

## Deferred / known
- Session import: auth login --chrome found no oneword.domains cookie in any Google Chrome profile; investigating other Chromium stores. Pass-gated commands (domains search/count/saves/intersect, tlds inventory, gpt saves) are verified with fixtures and the typed auth error only, pending a real session.
- Generator limitations found: JSON client rejects DomainsGPT stream (hand-built generate); syncer treats every resource as single-page (no page-number pagination declared); edge cache made a gated route look public during discovery.
