# Browser-Sniff Discovery Report: oneword-domains

Target: https://oneword.domains/tlds (user chose "the website itself" in Phase 0). Capture date: 2026-09-24 (UTC 2026-09-23T23:46Z to 23:56Z).

## 1. User Goal Flow
- Goal: "Check a word across TLDs and find the cheapest way to buy it", then browse aftermarket listings and generate names with DomainsGPT.
- Steps completed (14 interaction rounds):
  1. Home `/` loaded; session check via `/api/auth/session` (signed in, lifetime-pass account).
  2. Client-side navigation to `/tlds` → `/_next/data/<build>/tlds.json`; toggled sort → `GET /api/tlds?sortAlpha=true`.
  3. Client-side navigation to `/tlds/ai` → `/_next/data/<build>/tlds/ai.json?tld=ai` (pageProps: key, tldData, domains[100], count 2225, categoryFilters[17], premiumFilters[1]); page fired `POST /api/tlds/ai/view` (view counter) and `GET /api/domains/saves`.
  4. Filter changes via the Next.js router (search=sm, minLength=3&maxLength=5, sort=priceDesc, page=2, category=adjectives, price=available, sort=popularityDesc). The page's SWR hooks did not refetch on shallow query changes (`revalidateOnMount:false` + fallbackData), so the gated routes were fired directly from page context with the session: 11 `GET /api/domains?...` variants and their `GET /api/domains/count?...` twins, all 200.
  5. Client-side navigation to `/aftermarket` and `/aftermarket/godaddy` → `/_next/data/<build>/aftermarket(.json|/godaddy.json)`; sort/tld/length filters via router.
  6. Client-side navigation to `/domains-gpt` → `GET /api/tlds/com`, `GET /api/gpt/usage` (session: usage 5 / quota 2250); one `POST /api/gpt/generate` (brandable, .ai, 6-10 chars) streamed 20 `{domain, available}` objects in ~15 s.
  7. `/dashboard` and `/dashboard/domains-gpt` → `GET /api/auth/api-key` returned 400 "This action with HTTP GET is not supported by NextAuth.js" (route shadowed by the NextAuth catch-all; the API-key feature appears broken).
- Steps skipped: saving a domain (`POST /api/domains/{domain}/save`) and DomainsGPT saves (write actions, not needed for a read-first CLI); Stripe checkout; admin routes (`/admin/*`, present in the bundle but not visited).
- Secondary flows: aftermarket bargains, DomainsGPT generation, dictionary directory (`/api/words` via direct HTTP).
- Coverage: 7 of 7 planned steps.

## 2. Pages & Interactions
1. `https://oneword.domains/` — navigate (full load); installed fetch/XHR body interceptor with write-time redaction of apiKey/token/secret/password/email/userId values.
2. `/tlds` — clicked nav link (SPA); clicked sort toggle.
3. `/tlds/ai` — clicked TLD link (SPA); router.replace for search, minLength/maxLength, sort, page, category, price.
4. `/aftermarket` — clicked link; router.replace sort=priceDesc, tld=co, minLength=3&maxLength=4.
5. `/aftermarket/godaddy` — clicked registrar link.
6. `/domains-gpt` — clicked link; clicked "Generate".
7. `/dashboard`, `/dashboard/domains-gpt` — router.push.
8. Direct page-context fetches (session): the `/api/domains` matrix above, `/api/gpt/saves`, `/api/domains/saves`, `/api/logs`, `/api/gpt/usage`, `/api/listings/filters?registrar=godaddy`, `/api/words?query=sm`, `POST /api/gpt/generate`.

## 3. Browser-Sniff Configuration
- Backend: Claude chrome-MCP (Chrome extension) in a fresh capture tab of the user's logged-in Chrome. browser-use 0.13.8 was present but its daemon needs Chrome remote-debugging approval, which the user was not asked to grant; agent-browser 0.35.2 present but not needed.
- Pacing: page-context fetches spaced 500-600 ms; no 429s; effective ~1.5 req/s. DomainsGPT generate takes 10-15 s per call.
- Proxy pattern: not detected (plain REST JSON on `/api/*`; no GraphQL, no batchexecute).
- Export path: chrome-MCP tool output redacts base64 and query strings, so the session capture was exported as percent-encoded JSON in 32 FNV-1a-verified chunks (28,028 chars) and merged with 26 fresh unauthenticated curl captures by `assemble_capture.py` into `browser-sniff-capture.json` (56 entries). List bodies from the session were trimmed to the first 5 rows (`truncated_items` records the original length); public bodies are complete.
- `probe-reachability https://oneword.domains/api/tlds` → `mode: standard_http` (stdlib 200 in 377 ms, surf-chrome 200 in 349 ms).

## 4. Endpoints Discovered
| Method | Path | Status | Content-Type | Auth |
|---|---|---|---|---|
| GET | /api/tlds (`sortAlpha`) | 200 | application/json | public |
| GET | /api/tlds/{tld} | 200 (unknown slug → 200 with nulls) | application/json | public |
| POST | /api/tlds/{tld}/view | 200 | application/json | public (write: increments views; excluded from CLI) |
| GET | /api/words (`query` \| `contains`, `category`, `page`) | 200 | application/json | public |
| GET | /api/words/count (`contains`, `category`) | 200 | application/json (bare int) | public |
| GET | /api/words/{word} | 200 | application/json | public |
| GET | /api/domains/{word}.{tld} | 200 (unknown word → 500 HTML) | application/json | public |
| GET | /api/domains (`tld`, `search`, `category`, `price`, `minLength`, `maxLength`, `sort`, `page`) | 200 with session; 401 without | application/json | auth-required (lifetime pass) |
| GET | /api/domains/count (same filters) | 200 | application/json (bare int) | public |
| GET | /api/domains/saves | 200 with session; 401 without | application/json | auth-required |
| GET | /api/listings (`tld`, `registrar`, `minLength`, `maxLength`, `sort`, `page`) | 200 | application/json | public |
| GET | /api/listings/count (same filters) | 200 | application/json (bare int) | public |
| GET | /api/listings/filters (`registrar`) | 200 | application/json | public |
| GET | /api/gpt/usage | 200 | text/plain (JSON body) | public (anonymous quota 10; session quota 2250) |
| POST | /api/gpt/generate and https://api.oneword.domains/gpt/generate | 200 | (no content-type; concatenated JSON objects) | public up to quota; optional Bearer token |
| GET | /api/gpt/saves | 200 with session; 401 without | application/json | auth-required |
| GET | /api/logs | 200 | text/plain (JSON `[]`) | public |
| GET | /api/auth/api-key | 400 | text/plain | broken (NextAuth catch-all) |
| GET | /_next/data/{buildId}/tlds/{tld}.json | 200 | application/json | public; build-id-bound, excluded from the spec |
| GET | /api/og/tld?tld= | 200 | image/png | public (share card; excluded) |

Sort vocabulary (from the bundles): listings `domain|price|bids|ending` + `Asc|Desc`; domains `domain|popularity|price` + `Asc|Desc`. Category slugs (17): adjectives, adverbs, battleships, collections, french, gods, names, nouns, other, places, positive, scientific, spanish, species, stars, tech, verbs. Price filter: `available`. Registrar slugs: godaddy, namecheap, ionos, porkbun, gandi, oneohone, google.

## 5. Traffic Analysis
- Protocols: `rest_json` (0.75). No GraphQL, no RPC envelope.
- Auth signals: none detected by the analyzer (session is an HttpOnly NextAuth cookie the page never exposes; no Authorization header on any same-origin call). Bearer token documented for DomainsGPT only.
- Parameter evidence: query keys read from the page bundles' router-query builders (`search`, `minLength`, `maxLength`, `page`, `sort`, `category`, `price`, `tld`, `registrar`, `contains`, `query`, `sortAlpha`) and confirmed by live 200s.
- Protection signals: none (Vercel, no challenge headers, robots disallows /api for crawlers only).
- Generation hints: analyzer emitted `requires_js_rendering` because of the 500 HTML error page for an unknown domain; this is an error-shape, not a runtime requirement. Runtime is plain HTTP.
- Warnings: `error_status_cluster` for `/api/auth/api-key` (400 only) — drop from the spec.

## 6. Coverage Analysis
- Exercised: TLDs (list, detail, view), words (directory, autocomplete, count, detail), domain availability check, domain database search + count (pass-gated), saved domains, aftermarket listings (+count, +filters), DomainsGPT (usage, generate, saves).
- Missed: write actions (save/unsave domain, save GPT results, listing creation for sellers, Stripe checkout), admin routes, `/api/site*` (site thumbnails; 500 without params). The brief's workflows are all covered.

## 7. Response Samples
- `/api/tlds` item: `{"slug":"ai","type":"ccTld","structure":"normal","description":"...","top10m":30230,"totalReg":1466201,"views":55088,"minPrice":"72.4"}`
- `/api/tlds/{tld}`: `{"slug":"com","type":"gTld","structure":"normal","top10m":4406170,"totalReg":328881307,"description":"...","namecheap":null,...,"registrars":[{"name":"godaddy","price":"12.99"},...],"cheapestRegistrar":{"name":"ionos","price":"1"},"sites":[]}` (unknown slug: `{"godaddy":null,...,"registrars":[],"cheapestRegistrar":null,"sites":[]}`)
- `/api/words?query=sm`: `[{"slug":"smack"},{"slug":"small"},...]` (max 10); directory mode: `[{"slug":"4k","categories":[{"categorySlug":"tech"}]},...]` (100/page)
- `/api/words/{word}`: `{"slug":"smart","categories":[{"wordSlug":"smart","categorySlug":"adjectives","updatedAt":"2022-07-15T06:29:07.919Z"},...]}`
- `/api/domains/{domain}`: `{"slug":"smart.com","available":false,"premium":false,"price":null,"word":{"tldCount":6},"tldSlug":"com","aftermarket":false,"tldCount":6,"registrars":[],"cheapestRegistrar":null}`
- `/api/domains?tld=ai` (session): `[{"slug":"abaft.ai","tldCount":83,"premium":false,"price":null},...]`; count: `2225`
- `/api/listings`: `[{"domain":"aby.tech","type":"auction","userId":"cld...","price":"1","bidCount":0,"endDate":"2026-05-22T18:09:00.000Z"},...]`; filters: `[{"tld":"us","_count":22},...]`
- `/api/gpt/usage`: anonymous `{"usage":"0","quota":"10"}` (strings); session `{"usage":5,"quota":2250}` (numbers)
- `POST /gpt/generate`: `{ "domain" : "openlumix.com", "available" : true }{ "domain" : "openvance.com", "available" : false }...` (no newlines, no wrapper; ~20 objects; 945-973 bytes)
- Error: `/api/domains/zzqqxx.com` → 500 Next.js HTML error page; gated routes without session → 401 `Unauthorized` (text/plain).

## 8. Rate Limiting Events
None. 0 × 429 across ~85 requests. DomainsGPT anonymous quota is 10 per identity (usage endpoint); the docs state 1000 req/min per token.

## 9. Authentication Context
- Authenticated session used: yes, via the user's Chrome (chrome-MCP), lifetime-pass account. Transfer method: none needed (extension runs inside the logged-in profile).
- Auth-only endpoints: `/api/domains` (list), `/api/domains/saves`, `/api/gpt/saves`, plus the larger DomainsGPT quota on `/api/gpt/usage`.
- Scheme: cookie session (NextAuth, HttpOnly, default cookie name `__Secure-next-auth.session-token` on https); no Authorization header is constructed by the SPA. Cookie replay outside the browser was NOT validated here because cookie values were never read; the generated CLI's `auth login --chrome` (Chrome cookie-store import) is the validation point in Phase 5 dogfood. If it fails, the gated commands keep a clear "lifetime pass session required" error.
- Session state, cookie values, the account email, and the user id were excluded from every artifact (write-time redaction; leak check: 0 hits for session-token, Bearer, Cookie, set-cookie).

## 10. Bundle Extraction
- Bundles analyzed: `/_next/static/chunks/pages/*.js` and shared chunks for build `qzfSin9peIEieIakzu-ki` (35 files).
- API base: same origin (`/api/*`); DomainsGPT docs use `https://api.oneword.domains` which serves the same app.
- Bundle-only endpoints (not exercised): `POST/DELETE /api/domains/{domain}/save`, `POST /api/domains/saves` (sync local saves), `POST /api/gpt/save`, `POST /api/gpt/saves`, `POST /api/auth/api-key` (reset), `POST /api/auth/upsert-user`, `POST /api/stripe/checkout-session`, `/api/site`, `/api/site/approve`, `/api/site/reorder`, `/api/site/thumbnail`, `/api/og/docs`, `/api/og/tld`, admin pages.
- Config: Mixpanel + GA analytics only; word popularity = registered in (93 − tldCount) TLDs; affiliate hub `partners.dub.co/owd`; "Buy Lifetime Pass" modal opens on any 401 from `/api/domains`.
