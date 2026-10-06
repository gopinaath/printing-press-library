# internal/source/steam — keyless Steam storefront client

Sibling source-client module for game-goat-pp-cli's multi-source commands
(`ratings`, `versus`). Steam enrichment is **keyless**: no credential, no auth
header, no `os.Getenv` — the three endpoints this client calls are public
storefront surfaces, live-probed during research:

| Endpoint | Purpose | Response shape |
|---|---|---|
| `GET /api/storesearch/?term=<title>&cc=us&l=en` | title → appid | `{"total":n,"items":[{"type":"app","name":…,"id":<appid>}]}` |
| `GET /appreviews/<appid>?json=1&num_per_page=0&language=all&purchase_type=all` | review rollup | `{"success":1,"query_summary":{"review_score_desc":…,"score":…}}` |
| `GET /api/appdetails?appids=<appid>&cc=us&l=en` | price (optional) | `{"<appid>":{"success":true,"data":{…}}}` — decoded via `json.RawMessage` to tolerate the dynamic top-level key |

## API surface

- `steam.go` — typed client: `ResolveAppID`, `ReviewSummary`, `AppDetails`,
  plus sentinel errors `ErrAppNotFound` / `ErrReviewsUnavailable` so callers
  degrade on typed errors instead of string matching.
- `cli_bridge.go` — `SteamReviewForTitle(ctx, title) (*SteamReview, error)`:
  the thin wrapper `internal/cli` commands call; resolves the title, fetches
  the review summary, and opportunistically attaches price (a price failure
  degrades to `Price: nil`, never fails the review).
- `config.go` — `Config{RateLimit}` only. Keyless by design: no credential
  fields. Defaults: 3 req/sec sustained, burst 5.
- `cross_domain.go` — `adaptiveDoer`: the shared generic policy (token-bucket
  burst + `cliutil.AdaptiveLimiter` pacing + 429 backoff) that a future
  sibling source can reuse instead of re-implementing it.

## Rate-limit and retry policy

- Pacing: `cliutil.AdaptiveLimiter` at 3 req/sec with a 5-request burst
  allowance; the adaptive limiter halves the rate on 429 and ramps after
  successes.
- Retries: at most **one** retry per HTTP call, only on 429/5xx, never on
  other 4xx. A 429 backoff honors `Retry-After` (default 2s) capped at 10s.
- Exhausted 429 retries surface as `*cliutil.RateLimitError` —
  empty-on-throttle is indistinguishable from "no data exists", so the
  typed error keeps downstream commands honest.
- HTTP client: `net/http` with a 10s timeout, User-Agent
  `game-goat-pp-cli/0.1.0 (+printing-press)`, context-aware via `boundCtx`
  at every command call site.

## Why there is no `mcp_server.go`

Phase 11's MCP exposure contract says the MCP binary mirrors the Cobra tree
at startup (`internal/mcp/cobratree/`), and command annotations
(`mcp:read-only`) control tool hints — a per-module MCP server file is not
  part of the sibling source-client layout. Every command that consumes this
module (`ratings`, `versus`) is a top-level Cobra command and is therefore
exposed on the MCP surface automatically. No `mcp_server.go` stub is needed
here.

## Tests

`steam_test.go` covers storesearch parsing (incl. empty → `ErrAppNotFound`),
appreviews parsing (incl. `success:false` → `ErrReviewsUnavailable`), the
429 backoff-then-retry path (attempt-counted, real sleeps kept
milliseconds), 5xx retry-once, non-retryable 4xx, appdetails dynamic-key
parsing, and the bridge end-to-end including price degradation.

## Store services (catalog enumeration)

Catalog enumeration does not use the storefront host. It uses four keyless
services on `https://api.steampowered.com`, with the payload URL-encoded in an
`input_json` query parameter and the locale in `context{language,country_code}`:

| Service | Purpose | Notes |
|---------|---------|-------|
| `IStoreQueryService/SearchSuggestions/v1/` | plural text search | up to 100 results; an offset is ignored, so text search has no second page |
| `IStoreQueryService/Query/v1/` | filtered, paginated enumeration | `metadata.total_matching_records` drives paging; type / free / tag / coming-soon / released filters |
| `IStoreBrowseService/GetItems/v1/` | batch appid to full record | one `store_items` entry per appid, including `related_items.demos` |
| `IStoreService/GetTagList/v1/` | tagid to tag name | fetched once per process and cached on the client |

These four are not listed in the partner documentation at
https://partner.steamgames.com/doc/api, whose only catalog enumerator
(`IStoreService/GetAppList`) needs a Steam Web API key and cannot filter by demo,
tag, or price. No Steam API key is used anywhere in this client.

## Region and locale

Both hosts take the region from the client: `Config.Country` (default `US`) and
`Config.Language` (default `english`), reaching the storefront as `cc`/`l` and the
services as `context.country_code`/`context.language`. The CLI resolves the country
as `--country`, then `STEAM_COUNTRY`, then `ITAD_COUNTRY`, then `US`, so one setting
localises both the Steam store and the IsThereAnyDeal price path.

## Catalog types and attributes

`AppType` maps the service `type` integer: 0 game, 1 demo, 2 mod, 4 dlc, 6 software,
7 video, 10 hardware, 11 soundtrack, anything else `other`. Free to play and early
access are attributes rather than types (Steam models early access as a tag),
so they live on `StoreItem.IsFree` and `StoreItem.EarlyAccess` plus the `--free` filter.
Bundles cannot be enumerated: no keyless store service exposes a bundle filter,
so `--type bundle` is rejected with that explanation from `typeFilters`.

## Identity resolution

`ResolveAppIDWithHint(ctx, title, year)` strips a trailing `(YYYY)`, searches games
only, and breaks name collisions by release year: an exact year first, then a
single candidate within a year. Anything else returns `ErrAmbiguousApp` with the
candidate list instead of guessing. The CLI prefers a stronger source when it
has one: the Steam store link RAWG records for the game, which is exact by
construction.

## Catalog tests

`store_test.go` covers this layer offline against `httptest`: the taxonomy and flag
parsing, request encoding of `context`, `type_filters`, `start`/`count`, `tagids` and
`only_free_items`, response decoding, the tag-name merge, and
`ResolveAppIDWithHint` selection (year tie-break, ambiguity, no match).
