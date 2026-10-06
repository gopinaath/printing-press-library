---
date: 2026-10-02
target_cli: game-goat-pp-cli
amend_run_id: amend-2026-10-02T2130
scope_tier: all
findings_count: 8
mode: direct
status: complete (awaiting PR-draft checkpoint)
---

# Amend plan — game-goat-pp-cli (tier A of A-then-B)

CLI_DIR = `$PUBLISH_REPO_DIR/library/media-and-entertainment/game-goat` (managed clone
publish-clone checkout), branch `feat/game-goat-steam-store` off upstream/main 5d0984cb6.

## Research findings that shaped scope (ask 1f)

- partner.steamgames.com documents the key-gated Web API. The only catalog enumerator there is
  `IStoreService/GetAppList` (key required — 403 keyless; no demo/tag/free filters).
  `ISteamApps/GetAppList` is deprecated. There is no "list demos" endpoint.
- Valve's store services `IStoreQueryService/Query`, `IStoreQueryService/SearchSuggestions`,
  `IStoreBrowseService/GetItems` and `IStoreService/GetTagList` answer keyless on api.steampowered.com,
  take `context.country_code` + `language`, and return typed items (`type` int, `is_free`, tags, release,
  platforms, best_purchase_option, related_items.demos/parent_appid). They are not on the partner docs site.
- Storefront `/api/storesearch` caps at 10 results and ignores paging. `SearchSuggestions` caps at 100 and ignores `start`.
  Only `Query` paginates (`metadata.total_matching_records`).
- Verified type ints: 0 game, 1 demo, 2 mod, 4 dlc, 6 software, 7 video, 10 hardware, 11 soundtrack.
  `include_bundles` returns 0 rows. Free-to-play and early access are attributes, not types.
- RAWG `/games/{id}/stores` carries the exact Steam store URL (2454 DOOM 2016 → 379720; 52884 DOOM 1993 → 2280).
- Decision: STEAM_API_KEY is **not** needed. The steam-web sibling CLI already covers the key-gated Web API.

## F1 — typed Steam store-catalog client (feature, ask 1a)
Files: internal/source/steam/{store.go (new), steam.go, config.go, README.md, store_test.go (new)}.
Behavior: Search (SearchSuggestions, ≤100, type filter), Browse (Query; paginated; type/free/tag/coming-soon/released),
Items (GetItems batch typed StoreItem), TagList (GetTagList). AppType enum + IsFree/EarlyAccess/ComingSoon flags,
platforms, tags, release date, publishers/developers, price, demo/parent links, header image URL.
Reuses adaptiveDoer, the rate limit and sentinel errors (+ErrAmbiguousApp).
Tests: type mapping, flag parsing, request encoding (input_json), response decoding, paging metadata.

## F2 — store-aware identity resolution (bug, ask 1b)
Files: internal/cli/ratings.go, internal/cli/steam_resolve.go (new), internal/source/steam/{steam.go,cli_bridge.go}.
Behavior: ratings takes the appid from the RAWG Steam store link first. The fallback is a title search that strips
"(YYYY)", keeps only games, and breaks ties by release year; it reports ambiguity rather than guessing.
meta.steam_resolved_by is recorded. Scope note: only ratings consumes Steam; similar/discover/prices/retention/sync
resolve through RAWG/ITAD and are unaffected. There is no versus command.
Tests: store-link appid parse table, suffix strip, DOOM (2016) selection, year tie-break, ambiguity, no-match.

## F3 — CLI surface (feature, ask 1c)
Files: internal/cli/steam.go (new) + test. `steam search|app|browse`, --agent JSON {meta,results}, sources_missing
for reviews, mcp:read-only annotations → cobratree mirrors 3 new MCP tools (65 → 68 runtime). The typed manifest
(30) is unchanged. agent-context/which pick them up.

## F4 — region/currency knob (feature, ask 1d)
--country (flag > STEAM_COUNTRY > ITAD_COUNTRY > US, same validator as prices) and --lang (default english).
The hardcoded cc=us&l=en in storesearch/appdetails is replaced by client config.

## F5 — display name "Game GOAT" (bug, ask 2)
.printing-press.json, manifest.json (display_name + MCP env description), auth.go Short, MCP server name, root.go
Short/Long, README/SKILL/AGENTS headings, agentcookie.toml. Delete .printing-press.json.pre-name-fix.
Gate: `grep -rn "Rawg Video Game[s]"` returns zero hits (the bracket keeps this doc out of its own match).

## F6 — document the API tradeoff (feature, ask 1f)
Help Long text, README + SKILL "Steam data sources" note, recipes, command reference, internal/source/steam/README.md.

## F7 — prune stale novel features (bug)
novel_features_built drops backlog audit / finishline / radar / moods list (commands no longer exist).

## F8 — listing polish (polish, ask 3)
catalog_display_name / catalog_description in flight-goat voice; named novel_features with command + one-liner.
search_terms are derived by tools/generate-registry from these fields.

## Risks / dependencies
- The store services are undocumented, so their shapes can drift. Mitigations: typed decode tests, wrapped errors, and a deferred_to_upstream note in the patch file.
- F2 depends on F1 (Search). F3 depends on F1/F4. F8 depends on F3 (feature names).
- Generated "DO NOT EDIT" files only get string edits; these are recorded in the patch files.
- Deferred: F9 demo view (tier B).

## Results

All eight findings implemented, plus the F9 deferral record. Gates and live
evidence: see `2026-10-02T2130-amend-acceptance.md` in this directory. Summary:
gofmt clean, go vet PASS, go test ./... PASS (two new offline test files),
publish validate PASS on every leg except `phase5` (marker already stale on main
from the release-ledger bump to internal/cli/version.go), verify-skill PASS,
MCP surface 65 -> 68 tools with nothing removed, and zero remaining hits for the
old display name.

Patch entries: `.printing-press-patches/steam-store-services-are-the-keyless-catalog.json`,
`.printing-press-patches/display-name-and-catalog-listing-are-explicit.json`, plus
CLI-global `_meta.json` (upstream_tracking 4023/662, deferred_to_upstream).
