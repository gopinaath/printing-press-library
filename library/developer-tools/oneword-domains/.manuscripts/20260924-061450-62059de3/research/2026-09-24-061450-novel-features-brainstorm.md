# Novel-features brainstorm (Phase 1.5c.5 subagent output, verbatim)

Inputs: brief `2026-09-24-061450-feat-oneword-domains-pp-cli-brief.md`; rubric `references/absorb-scoring.md`; prior research `none` (first print); absorbed table rows 1-20 as of spawn (rows 21-27 were added from the ecosystem catalogue afterwards and did not overlap any survivor).

## Customer model

### Priya — solo founder in naming week
**Today (without this CLI):** Tabs open: `oneword.domains/tlds/ai` with the category dropdown set to *tech*, the DomainsGPT page, Instant Domain Search, and a Namecheap cart. A Notion table of ~40 candidate names with one column per TLD she fills in by pasting each name into the site's search box. She cannot answer "which of the names I marked free on Tuesday are still free" without redoing every lookup, and the DomainsGPT names she liked are gone once the tab closes.
**Weekly ritual:** Sunday-night shortlist refresh (brief Top Workflows 1, 4, 5): re-check every shortlisted word on .com/.ai/.io, fold in the week's DomainsGPT output, cull what got taken, and note the cheapest registrar for the survivors.
**Frustration:** Availability rots. The brief's pain-points row names it directly ("stale availability, re-check on demand"): what she wrote down mid-week is wrong by Sunday, and nothing tells her *what changed*.

### Marco — evening domain flipper
**Today (without this CLI):** `oneword.domains/aftermarket` sorted by ending soonest, a GoDaddy auction tab per listing, and a spreadsheet of bids placed. He re-sorts by `priceAsc` and `maxLength=4` every evening (Top Workflow 3). He cannot answer "is this `.co` auction at $120 cheap relative to how contested the word is" without opening the word's own page to read "registered in N of 93 TLDs" and the TLD page to read the fresh-registration price.
**Weekly ritual:** Nightly scan of auctions ending in the next 48 hours under his max bid; weekly, 3-5 bids on words that are taken almost everywhere else but still cheap on the auctioned TLD.
**Frustration:** Ranking 455 listings by demand means 455 word-page visits plus 30 TLD-page visits. The site sorts only by domain, price, bids, or ending; it never joins popularity or registration cost onto the auction list.

### Dana — naming strategist at a brand agency, lifetime-pass holder
**Today (without this CLI):** Two or three client naming projects in parallel. Tabs: `oneword.domains/tlds/com` and `/tlds/ai`, each with `category=positive` and length 4-6 set (the pass-gated database, Top Workflows 2 and 5). She exports each TLD table by hand into a spreadsheet and VLOOKUPs to find words free on both TLDs. She cannot answer "which positive 4-6 letter words are free on .com *and* .ai" or "which TLD still has the most inventory under my filter" without visiting 93 pages one at a time.
**Weekly ritual:** Per project, per week: build a 50-word candidate pool from category combinations, pick a TLD, present a name × TLD × price matrix.
**Frustration:** The database is one TLD per page and one category per filter. Cross-TLD intersection and multi-category combinations do not exist on the site.

### Tom — indie hacker whose Claude Code agent names side projects
**Today (without this CLI):** Ships a side project most weeks; naming is the Friday step, delegated to an agent. He has a hand-rolled curl script for `POST /gpt/generate` that breaks on the concatenated-JSON stream, then the agent checks names one by one. He cannot get "10 free names on .com, with price and whether it is a real word" in one tool call, and the anonymous quota of 10 runs out mid-session with no warning.
**Weekly ritual:** Friday: agent generates, filters to available, prices, and hands back a ranked list; Tom registers one.
**Frustration:** Generate → filter → price is three tools and one quota surprise; the agent needs a single priced, available, persisted result.

## Candidates (pre-cut)

| # | Feature | Command | Description | Persona | Source | Inline kill/keep verdict |
|---|---------|---------|-------------|---------|--------|--------------------------|
| 1 | Brainstorm pipeline | `brainstorm --type portmanteau --word open --position prefix --tld com,ai --context "..."` | One DomainsGPT call per TLD (quota-checked via `gpt usage` first), keep `available:true`, join real-word flag (local `word` table) and registration price + cheapest registrar (local `tld`), persist to a local `generation` table, sort by `--sort length|price|name`. | Priya, Tom | (a), (b) | KEEP |
| 2 | Generation history | `gpt history --available --type portmanteau --contains lum` | Search the local `generation` table for names produced by earlier runs. | Priya | (c) | SOFT KILL: framework `search`/`sql` cover retrieval once #1 persists rows |
| 3 | Shortlist recheck with diff | `recheck [--since 7d] [--file words.txt --tld com,ai] [--all]` | Re-run `GET /api/domains/{word}.{tld}` for every previously checked pair, append a snapshot, print rows whose `available`/`premium`/`price`/`tldCount` changed; `--data-source local` diffs the last two snapshots offline. | Priya | (a) | KEEP |
| 4 | Auction ranking by demand | `listings rank --tld co --ending 48h --max-bids 3 --sort popularity|price-x|bids-per-day|ending` | Join local `listing` rows with local `tld` (minPrice, top10m) and the word's `tldCount`: `taken_of_93 = 93 - tldCount`, `price_x = price / minPrice`, `bids_per_day = bidCount / days since first_seen`. | Marco | (a), (c), (f) | KEEP with descope: mechanical columns only; `--max-checks 100` cap |
| 5 | Bid velocity | `listings heat` | Rank auctions by bid growth between the last two syncs. | Marco | (c) | SOFT KILL: a sort key of #4 |
| 6 | Listing timeline | `listings history <domain>` | Price/bid time series for one listing. | Marco | (c) | SOFT KILL: two-line `sql` query |
| 7 | Cross-TLD intersection | `domains intersect --tld com,ai [--taken-on co] --category positive --min-length 4 --max-length 6 [--max-pages 20]` | Page pass-gated `GET /api/domains?tld=X&...` per TLD, intersect word sets, optionally subtract words still free on `--taken-on` TLDs. | Dana | (a), (b) | KEEP, gated: 401 maps to "lifetime pass required; run `auth login --chrome`" |
| 8 | Dictionary mining, multi-category | `words mine --category positive,tech [--any] --min-len 4 --max-len 6 [--prefix s] [--glob 's*o']` | Local `word` JOIN `word_category` with AND/OR semantics, length bounds, prefix, GLOB. | Dana, Priya | (c), (b) | KEEP |
| 9 | Dictionary hunt with live check | `words hunt --category positive --max-len 6 --tld ai --available-only` | #8 plus a live check per word on one TLD. | Priya | (a) | SOFT KILL: `domains search` for pass-holders; `words mine | check --file -` otherwise |
| 10 | Inventory per TLD | `tlds inventory [--category positive --min-len 4 --max-len 6 --max-price 20] [--type gTld] --sort count|price|top10m` | `GET /api/domains/count?tld=X&...` for every TLD, joined with local `tld` prices. | Dana | (c), (b) | KEEP, auth-aware |
| 11 | Registrar scorecard | `tlds registrars` | Per registrar, count of TLDs where cheapest, mean delta. | Dana | (c) | SOFT KILL: monthly at best; `tlds drift` covers change |
| 12 | TLD aftermarket premium | `tlds market` | Auction price / min registration price per TLD. | Marco | (c) | SOFT KILL: folded into #4 `price_x` |
| 13 | Similar TLDs | `tlds alternatives <tld>` | `alternatives` from SSR data. | Priya | (b) | KILL: thin wrapper |
| 14 | Popular words | `words popular` | 300 `popularWords` from home SSR. | Priya | (b) | KILL: static wrapper |
| 15 | Seed transforms | `variants smart --prefix get,try --suffix ly --drop-vowels` | Beast-Mode style variants. | Tom | table stakes | KILL: non-dictionary words return 500 from the check route |
| 16 | Share card | `tlds card ai --out ai.png` | Wrapper over `/api/og/tld`. | none | (b) | KILL |
| 17 | Did-you-mean on 500 | `words nearest smrt` | Nearest dictionary slugs by edit distance. | Tom | (f) | REFRAME then KILL as a row: error-path behaviour of `check` |
| 18 | Budget planner | `plan --file words.txt --budget 50` | Cheapest combinations under a budget. | Priya | (a) | KILL: cumulative-sum sort over `check --file` |
| 19 | Recently re-tagged words | `words recent` | Fresh `updatedAt` on category rows. | none | (f) | KILL: no persona |
| 20 | Name-style usage | `gpt styles` | `nameTypes` usage counts from SSR. | Tom | (f) | KILL: social-proof wrapper |

## Survivors and kills

### Survivors

| # | Feature | Command | Score | Buildability | How It Works | Evidence | Long Description |
|---|---------|---------|-------|--------------|--------------|----------|------------------|
| 1 | Brainstorm pipeline (generate → available → price → real-word → persist) | `brainstorm --type portmanteau --word open --position prefix --tld com,ai --context "..."` | 9/10 (fit 3, pain 3, feas 2, research 1) — Priya, Tom | hand-code | Calls `GET /api/gpt/usage` to confirm quota, then `POST /gpt/generate` once per requested TLD, keeps `available:true`, joins each name against the local `word` table (real-word flag) and local `tld` row (minPrice, cheapestRegistrar; live `GET /api/tlds/{tld}` fallback), and appends rows to a local `generation` table with no external dependencies. | Brief Top Workflow 4; Build Priority 4 names a `brainstorm` pipeline; HN ask "is it a real word, how much, where cheapest"; anonymous quota 10 in Reachability. | Use this command to generate DomainsGPT names and keep only the available ones with registration price, cheapest registrar, and a real-word flag, saved locally. Do NOT use this command for one raw DomainsGPT call with the unfiltered stream; use 'gpt generate' instead. Do NOT use it to check words you already have; use 'check' instead. |
| 2 | Shortlist recheck with change diff | `recheck [--since 7d] [--file words.txt --tld com,ai] [--all]` | 8/10 (fit 3, pain 3, feas 1, research 1) — Priya | hand-code | Reads distinct (word, tld) pairs from the local `domain_check` table, re-calls `GET /api/domains/{word}.{tld}` for each, appends a snapshot row, and diffs `available`/`premium`/`price`/`tldCount` against the prior snapshot; `--data-source local` diffs the last two snapshots offline. | Brief Pain points: "stale availability (surface updatedAt and re-check on demand)"; Top Workflow 1; data layer defines `domain_check ... checked_at`. | Use this command to re-run every word/TLD pair you previously checked and report what changed (availability, premium, price, popularity). Do NOT use this command for a first check of new words; use 'check' instead. Do NOT use it for aftermarket auction changes; use 'listings watch' instead. |
| 3 | Auction ranking by word demand, price ratio, bid pace | `listings rank --tld co --ending 48h --max-bids 3 --sort popularity|price-x|bids-per-day|ending` | 8/10 (fit 3, pain 2, feas 2, research 1) — Marco | hand-code | Joins local `listing` rows (price, bidCount, endDate, first_seen from sync) with the local `tld` row (minPrice, top10m) and the word's `tldCount` from cached or live `GET /api/domains/{word}.{tld}`, computing `price_x = price/minPrice`, `taken_of_93 = 93 - tldCount`, `bids_per_day = bidCount / days since first_seen`; `--max-checks 100` caps live lookups. | Top Workflow 3; Codebase Intelligence `n.ZT = 93` popularity constant; data layer `listing.first_seen/last_seen`; site sorts listings by eight server keys only. | Use this command to rank live auctions by word demand (TLDs taken of 93), price versus fresh registration cost, and bid pace. Do NOT use this command to see which listings are new, changed, or ending since the last run; use 'listings watch' instead. Do NOT use it for the raw server-sorted list; use 'listings list' instead. |
| 4 | Cross-TLD intersection over the pass-gated database | `domains intersect --tld com,ai [--taken-on co] --category positive --min-length 4 --max-length 6 [--max-pages 20]` | 8/10 (fit 3, pain 2, feas 2, research 1) — Dana | hand-code | Pages `GET /api/domains?tld=X&category=..&minLength=..&maxLength=..&page=N` (session cookie) for each listed TLD, builds per-TLD word sets, and emits the intersection (minus words still free on any `--taken-on` TLD) with each TLD's price/premium per word; 401 maps to "lifetime pass required". | Table Stakes: Beast Mode "TLD comparison in one view"; Reachability: `/api/domains` is per-TLD and pass-gated; Top Workflows 2 and 5. | Use this command to find dictionary words available on every listed TLD at once, or free on one TLD but taken on another (lifetime pass). Do NOT use this command to filter the dictionary by category or length without availability; use 'words mine' instead. Do NOT use it to verify a word list you already have; use 'check --file' instead. Do NOT use it to count inventory per TLD; use 'tlds inventory' instead. |
| 5 | Multi-category dictionary mining | `words mine --category positive,tech [--any] --min-len 4 --max-len 6 [--prefix s] [--glob 's*o']` | 7/10 (fit 2, pain 2, feas 2, research 1) — Dana, Priya | hand-code | Runs one SQLite query over local `word` JOIN `word_category` with `GROUP BY slug HAVING COUNT(DISTINCT category_slug) = N` (AND) or `>= 1` (OR), `length(slug)` bounds, prefix and GLOB filters, fully offline after `sync --resources words`. | Pain points "too few filters"; `/api/words` takes one `category` and no length params; Build Priority 5. | Use this command to filter the local dictionary by several categories at once, length, prefix, or glob pattern. Do NOT use this command to find which words are available on a TLD; use 'domains intersect' (lifetime pass) or pipe its output into 'check --file' instead. |
| 6 | Available inventory per TLD for a filter set | `tlds inventory [--category positive --min-len 4 --max-len 6 --max-price 20] [--type gTld] --sort count|price|top10m` | 7/10 (fit 2, pain 2, feas 2, research 1) — Dana | hand-code | Calls `GET /api/domains/count?tld=X&category=..&minLength=..&maxLength=..` for every TLD in the local `tld` table (live `GET /api/tlds` when unsynced) and joins minPrice, cheapestRegistrar, top10m, totalReg into one sorted table; 401 maps to "lifetime pass required". | Top Workflow 2; `/api/domains/count` filters from the `/tlds/[tld]` bundle; the count route is public. | Use this command to compare how many available one-word domains each TLD still has for a category/length/price filter, with the TLD's cheapest registration price. Do NOT use this command to list the words themselves; use 'domains intersect' instead. Do NOT use it for registrations, top-10M presence, views, or registrar prices alone; use 'tlds list' or 'tlds get' instead. |

### Killed candidates

| Feature | Kill reason | Closest surviving sibling |
|---------|-------------|---------------------------|
| `gpt history` | Framework `search`/`sql` retrieve persisted `generation` rows; a dedicated list is a thin local wrapper. | `brainstorm` |
| `listings heat` | Bid velocity is one sort key of `listings rank`; `listings watch` reports changed bid counts. | `listings rank` |
| `listings history <domain>` | Single-listing time series is a two-line `sql` query, not a weekly ritual. | `listings rank` |
| `words hunt --tld` | Pass-holders use one `domains search` call; others pipe `words mine` into `check --file -`. | `domains intersect` |
| `tlds registrars` | Prices move slowly; `tlds drift` (absorbed) tracks change; monthly at best. | `tlds inventory` |
| `tlds market` | Only 30 TLDs carry listings from one registrar; folded into `listings rank` as `price_x`. | `listings rank` |
| `tlds alternatives` | Thin wrapper over one SSR blob, used once per project. | `tlds inventory` |
| `words popular` | Static wrapper over 300 SSR words; popularity arrives via `check` as `tldCount`. | `words mine` |
| `variants` | The check route returns 500 for non-dictionary strings, so variants cannot be verified; DomainsGPT `--word/--position` is the in-spec path. | `brainstorm` |
| `tlds card` | PNG wrapper with no persona ritual. | `tlds inventory` |
| `words nearest` | Error-path behaviour for `check`, not a command. | `words mine` |
| `plan` | Cumulative-sum sort over `check --file` output. | `recheck` |
| `words recent` | Curation feed with no persona. | `words mine` |
| `gpt styles` | Social-proof wrapper; no weekly use. | `brainstorm` |
