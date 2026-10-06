---
date: 2026-10-02
target_cli: game-goat-pp-cli
amend_run_id: amend-2026-10-02T2130
scope_tier: all
status: pass
---

# Amend acceptance proof - game-goat-pp-cli (Steam as a first-class store source)

Branch `feat/game-goat-steam-store` off upstream/main `5d0984cb6`, CLI dir
`library/media-and-entertainment/game-goat`.

## Gates

| Check | Command | Result |
|---|---|---|
| gofmt | `gofmt -l internal/ cmd/` | clean (no output) |
| vet | `go vet ./...` | PASS |
| tests | `go test ./...` | PASS, every package (2 new offline test files) |
| build | `go build ./...` | PASS |
| consolidated | `cli-printing-press publish validate --dir <cli> --json` | PASS on manifest, go mod tidy, module path, govulncheck, go vet, go build, --help, --version, verify-skill, patches, manuscripts. `phase5` FAILS on marker staleness - see below. |
| skill docs | `cli-printing-press verify-skill --dir <cli>` | PASS (flag-names, flag-commands, positional-args, shell-var-quotes, unknown-command, canonical-sections) |
| MCP surface | `tools/list` against the built MCP binary | 68 tools (was 65): `steam_app`, `steam_browse`, `steam_search` added, nothing removed |
| agent surface | `agent-context` / `which` | `steam` listed; `which "search the steam store"` -> `steam search` (score 9) |
| naming gate | `grep -rn "Rawg Video Game[s]"` | 0 hits |

## New offline tests

`internal/source/steam/store_test.go` (httptest, no network): app-type taxonomy
(service ints + flag parsing, including the bundle rejection), request encoding
(`context.country_code`/`language`, `type_filters`, `start`/`count`,
`price_filters.only_free_items`, `tagids_must_match`), response decoding (price,
platforms, tags, demo links, early access, header image), tag-dictionary caching
and its non-fatal failure path, and `ResolveAppIDWithHint` selection
(9 subtests: remake suffix, explicit year, year tie-break, one-year drift,
ambiguity, no exact match).

`internal/cli/steam_resolve_test.go`: RAWG store-link appid parsing
(9 table cases), `steamYearFromRelease`, `steamAppIDForRAWGGame` against a stub
RAWG server (5 subtests incl. degradation), `resolveSteamCountry` precedence
(5 subtests), `resolveSteamLanguage`, and the typed-error exit-code mapping.

## Live evidence (keyless)

- `steam search doom --limit 3 --agent` -> typed records with tags, price, release date, header image.
- `steam browse --type demo --free --limit 2 --agent` -> `total: 39782`, `next_page: 2` (the enumeration gap is closed).
- `steam app 379720 --country DE --agent` -> price `3,99 EUR` (region knob reaches both hosts).
- `steam app "DOOM (2016)" --agent` -> appid `379720`, `resolved_by: title` (the reported failure is fixed).
- `ratings "DOOM" --year 2016 --agent` -> `steam_resolved_by: rawg-store-link`, appid 379720.
- `steam browse --type game --tag Roguelike` -> `total: 14867`; a typo returns close matches instead of widening the search.
- `steam browse --type bundle` -> rejected with the no-keyless-bundle-filter explanation.

## phase5 marker note

`publish validate` fails the `phase5` leg with
`phase5 marker source fingerprint does not match the current CLI source`.
That marker was already stale on `main` before this amend: it records
`internal/cli/version.go`, which was last changed by the release-ledger bot
commit `10efe2eda` (chore(releases): update CLI release ledger [skip ci]) -
not by this branch. The marker is a print-time attestation and an amend does not
re-stamp it; the per-file hashes recorded in it no longer match main either.

## Deferred

F9 (thin demo view: "free demos on Steam right now" filterable by title, tag and
release, plus demo-availability annotation on existing game output) -
`2026-10-02T2130-amend-game-goat-pp-cli-deferred.md`.
