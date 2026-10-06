# One Word Domains CLI Brief

## API Identity
- Domain: https://oneword.domains (Next.js on Vercel; `api.oneword.domains` serves the same app). Built by Steven Tey, launched May 2020 (#2 Product Hunt, HN front page), v2.0 Dec 2022 with a $149 lifetime pass. Sibling product DomainsGPT (Mar 2023, 15.3M names generated) at `POST https://api.oneword.domains/gpt/generate`.
- Users: founders and indie hackers naming a product; domain investors browsing resale inventory; brand/naming agencies; AI agents that need "is `word.tld` available and what does it cost".
- Data profile: 93 TLDs with registration counts, top-10M site counts, page views, per-registrar pricing (GoDaddy, Namecheap, IONOS, Porkbun, Gandi, 101domain) and cheapest registrar; a 27,627-word dictionary tagged with categories (adjectives, nouns, verbs, positive, tech, spanish, scientific, ...); ~1.38M available one-word domains (login + lifetime pass only); 455 live aftermarket listings (auctions with price, bid count, end date; GoDaddy only today); DomainsGPT name generation with live availability.
- Official docs: `/docs` (Nextra). Only DomainsGPT is documented (`/docs/domains-gpt/api`, `/docs/domains-gpt/getting-started`). The "One Word Domains API" docs page exists but is empty (WIP). The docs say the DomainsGPT API is "beta testing with select partners" and rate limited to 1000 req/min per token, yet the endpoint answers unauthenticated requests up to an anonymous quota.
- No OpenAPI spec anywhere. No CLI, MCP server, npm, or PyPI wrapper exists for this site (GitHub, npm, PyPI, and code search all empty; the unaffiliated `UniqueDomains` GitHub org publishes static CSV/JSON one-word lists, not an API client). This will be the first.

## Reachability Risk
- Low. Every probe from a stdlib HTTP client returned 200 with `server: Vercel`, no challenge headers, no `x-vercel-mitigated`, no Cloudflare. `robots.txt` disallows `/api` for crawlers but the routes answer normally. Response headers carry no rate-limit fields. DomainsGPT generate took 10.8 s for one batch of ~20 names.
- Auth wall (not a bot wall): `GET /api/domains?...`, `/api/domains/count?...`, `/api/domains/saves`, `/api/gpt/saves`, `/api/auth/api-key` return `401` (body `Unauthorized`) without a NextAuth session cookie. The `/tlds/{tld}` page opens the "Buy Lifetime Pass" modal on that 401, so the domain database is pass-gated, not just login-gated. Login is NextAuth email magic link only (`/api/auth/providers` → `email`), so the CLI cannot mint a session itself; cookie import from Chrome is the only way in.
- Quota: `GET /api/gpt/usage` → `{"usage":"0","quota":"10"}` for anonymous callers (identity source unconfirmed, likely IP or cookie). The site's UI shows a 429 toast "You've reached your usage limit. Enter an API key to continue." Signed-in users get an API key at `/dashboard/domains-gpt` (`GET/POST /api/auth/api-key`), sent as `Authorization: Bearer <TOKEN>`.
- Probe-safe endpoint used: `GET /api/tlds` (27.7 KB, 93 rows, no params).
- Not found: no GitHub issues, Reddit, or HN reports of blocking, 403s, or downtime. Site counter grew from 1.26M (2022) to 1,381,826 (now), TLD pages are dated to the current year, aftermarket listings end in the future, so it is actively maintained.

## Discovered Surface (verbatim probes, all unauthenticated unless marked)
| Method | Path | Params | Returns |
|---|---|---|---|
| GET | `/api/tlds` | `sortAlpha=true` (default sort is by popularity/views) | `[{slug,type(ccTld/gTld),structure(normal/...),description,top10m,totalReg,views,minPrice}]` × 93 |
| GET | `/api/tlds/{tld}` | slug is case-insensitive; unknown TLD returns 200 with nulls and empty `registrars` (treat as not-found) | `{slug,type,structure,top10m,totalReg,description,namecheap,godaddy,ionos,porkbun,gandi,oneohone,registrars:[{name,price}],cheapestRegistrar:{name,price},sites:[...]}` |
| GET | `/api/words` | `query=<prefix>` (autocomplete, max 10, `{slug}` only); or directory mode: `contains=<substr>`, `category=<slug>`, `page=N` (100/page, alphabetical, `{slug,categories:[{categorySlug}]}`) | array |
| GET | `/api/words/count` | same filters as directory mode (`contains`, `category`) | bare integer (27627 total; `category=tech` → 392; `contains=art` → 208) |
| GET | `/api/words/{word}` | | `{slug,categories:[{wordSlug,categorySlug,updatedAt}]}` |
| GET | `/api/domains/{word}.{tld}` | word must exist in the dictionary; unknown word → 500, missing tld → 500 | `{slug,available,premium,price,word:{tldCount},tldSlug,aftermarket,tldCount,registrars:[],cheapestRegistrar}` (`tldCount` = number of TLDs where the word is still available; popularity score = (N-tldCount)/N) |
| GET | `/api/listings` | `tld=`, `registrar=` (currently only `godaddy`), `minLength=`, `maxLength=`, `page=N` (100/page), `sort=` one of `domainAsc|domainDesc|priceAsc|priceDesc|bidsAsc|bidsDesc|endingAsc|endingDesc` (default `domainAsc`; unknown params are ignored, not rejected) | `[{domain,type(auction),userId,price,bidCount,endDate}]` |
| GET | `/api/listings/count` | same filters | bare integer (455) |
| GET | `/api/listings/filters` | `registrar=` | `[{tld,_count}]` × 30 |
| GET | `/api/gpt/usage` | | `{usage:"0",quota:"10"}` (strings) |
| POST | `https://api.oneword.domains/gpt/generate` (also `/api/gpt/generate` on the main host) | JSON body `{type,context,minLength,maxLength,word,position,tld,domains[]}`; `type` ∈ portmanteau, combination, brandable, nonenglish, alternate, random; `position` ∈ prefix, suffix, anywhere; optional `Authorization: Bearer <TOKEN>` | streamed body of concatenated JSON objects with spaces, no newlines: `{ "domain" : "openlumix.com", "available" : true }{ "domain" : ... }` (~20 per call); 429 when over quota |
| GET | `/api/og/tld?tld=ai` | | PNG share card |
| GET | `/api/logs` | | `[]` public activity feed (empty at probe time) |
| GET (auth) | `/api/domains`, `/api/domains/count` | `tld=`, `search=`, `category=`, `price=`, `minLength=`, `maxLength=`, `sort=`, `page=` (from the `/tlds/[tld]` page bundle) | 401 without lifetime-pass session |
| POST/DELETE (auth) | `/api/domains/{domain}/save`, `/api/domains/saves`, `/api/gpt/save`, `/api/gpt/saves` | | saved-domain lists |
| GET/POST (auth) | `/api/auth/api-key` | | DomainsGPT token issue/reset |

SSR page data (`__NEXT_DATA__`, replayable HTML extraction) adds: `/` → `allTlds`, `popularWords` (300 words with categories), `newUsers`; `/tlds/{tld}` → `tldData`, `listings` (aftermarket for that TLD), `count`, `alternatives` (similar TLDs with example `sites` and thumbnails); `/aftermarket[/{registrar}]` → `listings`, `count`, `registrarFilters`, `tldFilters`; `/domains-gpt` → `nameTypes` with usage counts, `generations` total.

Registrar link convention (from the page bundle): "Bid/Buy on GoDaddy" deep links plus the `partners.dub.co/owd` affiliate hub. Registrar names in data are lowercase slugs: `godaddy`, `namecheap`, `ionos`, `porkbun`, `gandi`, `oneohone` (101domain).

## Top Workflows
1. **Check a word across TLDs and find the cheapest way to buy it.** `owd check smart` → matrix of `smart.{tld}` availability from `/api/domains/{word}.{tld}` joined with `/api/tlds/{tld}` registrar prices, sorted by price. This is the site's headline flow and the HN thread's most-asked need ("is it a real word, how much, where cheapest").
2. **Compare TLDs.** `owd tlds --sort minPrice` / `--type ccTld` / `owd tld ai` → registrations, top-10M presence, views, per-registrar prices, cheapest registrar, alternatives. Investors and founders use `/tlds` exactly this way.
3. **Hunt aftermarket bargains.** `owd listings --tld co --sort priceAsc --max-length 4`, `owd listings --ending 24h`, `owd listings --bids`, with a local watch that flags new listings, price changes, and auctions ending soon.
4. **Brainstorm with DomainsGPT and keep only what is free.** `owd generate --type portmanteau --word open --position prefix --tld com --context "..."` streams `{domain, available}` as NDJSON, `--available-only`, then pipes into `owd check` for pricing. Usage/quota surfaced up front so the anonymous 10-call cap never surprises.
5. **Mine the dictionary.** `owd words --category tech --contains art`, `owd words --prefix sm`, `owd word smart` → categories, popularity (registered in N of 93 TLDs), and a one-shot `owd words --category positive --min-len 4 --max-len 6 --check ai` pipeline that checks each word against a TLD.

## Table Stakes
- From Instant Domain Search: as-you-type results, bulk check of many names, CSV export, MCP server for agents.
- From Namecheap Beast Mode: bulk seed terms with transforms (prefix/suffix, drop vowels), TLD comparison in one view.
- From the site itself: TLD directory, per-TLD registrar price compare with cheapest highlighted, category filters, min/max length, sort by price/bids/ending, saved domains.
- From the domain-goat peer CLI in the library: JSON/agent output, offline store, watchlists, price history.
- Pain points to fix: stale availability (surface `updatedAt` and re-check on demand), too few filters (length, category, price, TLD type), few registrars (show all six and the cheapest), no bulk export (CSV/JSON everywhere), no API (this CLI is the API).

## Data Layer
- Primary entities: `tld` (93 rows, upserted per sync with a `snapshot_at`), `tld_price` (registrar, price, snapshot_at → price history and drift), `word` (27,627 rows, categories many-to-many), `domain_check` (word, tld, available, premium, price, tldCount, checked_at), `listing` (domain, type, price, bidCount, endDate, registrar, first_seen, last_seen, price history), `generation` (DomainsGPT runs and results), `watch` (rules: tld/word/listing with thresholds).
- Sync cursor: none exposed; use full-snapshot upserts (TLDs and listings are small: 93 rows and 455 rows), page through `/api/words?page=N` (277 pages) for the dictionary, and `first_seen`/`last_seen` timestamps for listings to detect new and gone.
- FTS/search: FTS5 over word slugs plus category tags, listing domains, and generated names; offline `owd tlds` and `owd words` after one sync.

## Codebase Intelligence
- Source: not open source; findings from the production Next.js bundle (`/_next/static/chunks/pages/*.js`, build `qzfSin9peIEieIakzu-ki`).
- Auth: NextAuth (email magic link), session cookie; DomainsGPT token via `Authorization: Bearer`. Env var plan: `ONEWORD_DOMAINS_TOKEN` for DomainsGPT (optional), `auth login --chrome` cookie import for pass-holders.
- Data model: Prisma-style (`_count` in filter rows, `updatedAt` on join rows, cuid `userId`). Word popularity = registered TLD count out of 93 (`n.ZT` constant in the bundle).
- Rate limiting: anonymous DomainsGPT quota 10 (usage endpoint), documented 1000 rpm per token; no limits observed on the data routes.
- Architecture: SWR on the client with `dedupingInterval` 60 s (listings/domains) and 3600 s (tlds); SSR pages carry the same data as the JSON routes, so HTML extraction is a fallback, not the primary transport.

## Product Thesis
- Name: `oneword-domains` (binary `oneword-domains-pp-cli`, short alias `owd`).
- Why it should exist: One Word Domains has the best curated one-word inventory and registrar price data on the web, and zero programmatic access. This CLI turns its JSON routes into an agent-native tool: check a word across 93 TLDs with prices in one command, watch the aftermarket, brainstorm with DomainsGPT and filter to what is actually free, and keep a local SQLite store that gives price history, listing alerts, and offline search that the site itself does not offer. First and only client for this data.

## Build Priorities
1. `tlds` / `tld <slug>` with registrar compare, cheapest, alternatives, `--sort`, `--type`, plus sync into SQLite with price snapshots and `tld price-history` / `tld drift`.
2. `check <word> [--tld ...]` matrix (word × TLDs) joined with pricing; `--available-only`, `--cheapest`, CSV/JSON; bulk `check --file words.txt`.
3. `listings` with every server filter and sort, `--ending <dur>`, `--min-bids`, `listings watch` with new/changed/ending-soon diffs, `listings filters`.
4. `generate` (DomainsGPT) with robust stream parsing into NDJSON, `--available-only`, `usage`, optional Bearer token, and a `brainstorm` pipeline that generates then prices.
5. `words` directory: `--prefix`, `--contains`, `--category`, `--min-len/--max-len`, `word <slug>`, `categories`, full dictionary sync with FTS.
6. Agent-native: `--json` everywhere, MCP server exposing check/tlds/listings/generate/words, `auth login --chrome` for pass-holders to unlock `domains search` (gated routes emitted but degraded gracefully to a clear "lifetime pass required" error).
