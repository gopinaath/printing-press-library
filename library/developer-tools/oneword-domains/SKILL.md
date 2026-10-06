---
name: pp-oneword-domains
description: "Check a word across every TLD with registrar prices, watch the aftermarket, and generate names with DomainsGPT, all from the terminal with a local store the site itself does not offer. Trigger phrases: `is smart.com available`, `cheapest TLD for a word`, `compare .ai vs .io prices`, `cheapest one-word domain auctions`, `brainstorm domain names with DomainsGPT`, `use oneword-domains`, `run oneword-domains`."
author: "Victor Wibisono"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - oneword-domains-pp-cli
    install:
      - kind: go
        bins: [oneword-domains-pp-cli]
        module: github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/cmd/oneword-domains-pp-cli
---

# One Word Domains — Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `oneword-domains-pp-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install oneword-domains --cli-only
   ```
2. Verify: `oneword-domains-pp-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.6 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/cmd/oneword-domains-pp-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

One Word Domains tracks 27,627 dictionary words across 93 TLDs with prices from six registrars, an aftermarket feed, and the DomainsGPT name generator, but publishes no API. This CLI replays its JSON routes so agents and scripts can compare TLDs, check availability, mine the dictionary, browse auctions, and run DomainsGPT, then keeps everything in SQLite for price drift, auction watches, and offline search. No other tool targeted oneword.domains as of the September 2026 ecosystem search.

## When to Use This CLI

Reach for this CLI when a task is about naming something and buying the domain: comparing TLDs by price and popularity, checking whether a dictionary word is free on a given extension, brainstorming brandable names with live availability, or scanning one-word aftermarket auctions. It is the only programmatic access to One Word Domains data, and its local store answers questions the site cannot, like price drift over time and which auctions appeared since yesterday.

## Anti-triggers

Do not use this CLI for:
- Do not use it to register or buy a domain; it links to registrars but never checks out
- Do not use it for WHOIS, RDAP, DNS, or certificate lookups on arbitrary domains; use domain-goat or whois for that
- Do not use it to check availability of invented, non-dictionary names except through gpt generate; the check route only knows the 27,627 indexed words
- Do not use it to manage DNS or hosting for domains you already own

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Naming pipeline
- **`brainstorm`** — Generate DomainsGPT names, keep only the available ones with price, cheapest registrar, and a dictionary flag, and save the batch locally; one call per TLD, quota 10 anonymous.

  _Pick this when an agent needs priced, available, deduplicated name candidates in one call instead of generate-then-check loops._

  ```bash
  oneword-domains-pp-cli brainstorm --type brandable --context "a terminal tool for domain search" --tld ai,com --json
  ```
- **`recheck`** — Re-check every word and TLD you looked up before and report what changed: taken, freed, premium, price, or popularity.

  _Use it to refresh a shortlist before buying instead of re-running every lookup by hand._

  ```bash
  oneword-domains-pp-cli recheck --since 7d --json
  ```
- **`check`** — Check one word, or a file of words, across the most-viewed TLDs or all 93, with availability, premium flag, popularity, and the cheapest registrar price per TLD.

  _Use it whenever an agent needs is-it-free-and-what-does-it-cost for a dictionary word._

  ```bash
  oneword-domains-pp-cli check smart --tld com,io,ai --json
  ```
- **`compare`** — Compare several full domains in one table: available, premium, price, popularity, and cheapest registrar, with the cheapest available one called out.

  _Pick it to decide between a few candidate domains in a single call._

  ```bash
  oneword-domains-pp-cli compare smart.com smart.io oasis.ai --json
  ```

### Aftermarket intelligence
- **`listings rank`** — Rank aftermarket auctions by how contested the word is across TLDs, price relative to fresh registration, and bid pace.

  _Reach for it when scanning auctions for underpriced words instead of reading through 455 listings by hand._

  ```bash
  oneword-domains-pp-cli listings rank --tld co --sort popularity --max-checks 25 --agent --select rows.domain,rows.price,rows.price_x,rows.taken_of_93,rows.end_date
  ```
- **`listings watch`** — Diff the aftermarket feed against the last run: new listings, price or bid changes, listings that disappeared, and auctions ending within a window.

  _Run it daily to learn what changed on the aftermarket without re-reading every listing._

  ```bash
  oneword-domains-pp-cli listings watch --tld co --json
  ```

### Lifetime-pass database
- **`domains intersect`** — Find dictionary words that are free on every TLD you list, or free on one and taken on another; needs the lifetime-pass session from auth login --chrome or auth login --cookies-file.

  _Use it when a brand needs the same word on several extensions and the user has a lifetime pass._

  ```bash
  oneword-domains-pp-cli domains intersect --tld com,ai --category positive --min-length 4 --max-length 6 --csv
  ```

### Local dictionary
- **`words mine`** — Filter the 27,627-word dictionary by several categories at once, length, prefix, or glob, offline after a one-time words mine --refresh.

  _Pick it to build a candidate pool before checking availability, without burning API calls._

  ```bash
  oneword-domains-pp-cli words mine --category adjectives,positive --min-len 4 --max-len 6 --json
  ```
- **`hacks`** — Find domain hacks: split a word so its ending is one of the 93 TLDs (sm.art), or list dictionary words ending in a TLD.

  _Use it for brandable hacks like he.art or ch.art that no availability search surfaces._

  ```bash
  oneword-domains-pp-cli hacks --tld art --limit 20 --json
  ```

### TLD intelligence
- **`tlds inventory`** — Compare how many available one-word domains each TLD still has for a category and length filter, next to its cheapest registration price (public route).

  _Use it to choose an extension by remaining supply and cost before mining words._

  ```bash
  oneword-domains-pp-cli tlds inventory --category positive --min-len 4 --max-len 6 --sort count --json
  ```
- **`tlds drift`** — Record registrar price snapshots for every TLD and report which prices moved since an earlier snapshot.

  _Use it to see whether a TLD got cheaper or dearer before buying._

  ```bash
  oneword-domains-pp-cli tlds drift --since 30d --registrars
  ```

### DomainsGPT
- **`gpt generate`** — Run DomainsGPT and get the streamed names back as real JSON, optionally only the available ones.

  _Use it for raw name generation; use brainstorm when you also want prices and dictionary flags._

  ```bash
  oneword-domains-pp-cli gpt generate --type portmanteau --word open --position prefix --tld com --available-only --json
  ```

## Discovery Signals

This CLI was generated with browser-observed traffic context.
- Capture coverage: 55 API entries from 56 total network entries
- Protocols: rest_json (75% confidence), browser_rendered (70% confidence), html_scrape (55% confidence)
- Auth signals: chrome_mcp_session
- Generation hints: requires_js_rendering
- Candidate command ideas: create_generate — Derived from observed POST /gpt/generate traffic.; create_view — Derived from observed POST /api/tlds/ai/view traffic.; list_ai — Derived from observed GET /api/tlds/ai traffic.; list_ai.json — Derived from observed GET /_next/data/qzfSin9peIEieIakzu-ki/tlds/ai.json traffic.; list_api_key — Derived from observed GET /api/auth/api-key traffic.; list_com — Derived from observed GET /api/tlds/com traffic.; list_count — Derived from observed GET /api/domains/count traffic.; list_domains — Derived from observed GET /api/domains traffic.
- Caveats: error_status_cluster: Endpoint cluster only observed error HTTP statuses.

## Command Reference

**domains** — Availability checks for word plus TLD and the lifetime-pass domain database; search needs auth login --chrome

- `oneword-domains-pp-cli domains check` — Check availability, premium flag, price, aftermarket status, and popularity (tldCount) for a dictionary word on one TLD
- `oneword-domains-pp-cli domains count` — Count available one-word domains for a TLD with the same filters as search; returns a bare integer; normally public
- `oneword-domains-pp-cli domains saves` — List the domains saved to your One Word Domains account; requires auth login --chrome
- `oneword-domains-pp-cli domains search` — Search the 1,380,000-plus available one-word domain database for a TLD with text, category, price, length, sort

**gpt** — DomainsGPT: AI-generated brandable names with live availability

- `oneword-domains-pp-cli gpt saves` — List DomainsGPT names saved to your account; requires auth login --chrome
- `oneword-domains-pp-cli gpt usage` — Show DomainsGPT usage and quota for the current identity; anonymous callers get 10, signed-in accounts more

**listings** — Aftermarket domain auctions and buy-now listings with price, bids, and end date

- `oneword-domains-pp-cli listings count` — Count aftermarket listings matching the filters; returns a bare integer
- `oneword-domains-pp-cli listings filters` — List the TLDs that currently have aftermarket listings, with counts
- `oneword-domains-pp-cli listings list` — List aftermarket listings, 100 per page, filtered by TLD, registrar, and word length, sorted by domain, price, bids

**tlds** — Top-level domains: registrations, top-10M presence, views, per-registrar prices, and the cheapest registrar

- `oneword-domains-pp-cli tlds get` — Get one TLD with its description, registrar price list, cheapest registrar, and example sites
- `oneword-domains-pp-cli tlds list` — List all 93 TLDs with registration counts, top-10M site counts, page views, and minimum registration price

**words** — The dictionary behind One Word Domains, 27,627 words tagged with categories

- `oneword-domains-pp-cli words count` — Count dictionary words matching the directory filters; returns a bare integer
- `oneword-domains-pp-cli words get` — Get one dictionary word with its category tags
- `oneword-domains-pp-cli words list` — List dictionary words; with --prefix up to 10 autocomplete matches


## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. Set `ONEWORD_DOMAINS_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths:

- `oneword-domains-pp-cli domains`
- `oneword-domains-pp-cli domains search`
- `oneword-domains-pp-cli gpt`
- `oneword-domains-pp-cli listings`
- `oneword-domains-pp-cli listings list`
- `oneword-domains-pp-cli tlds`
- `oneword-domains-pp-cli tlds get`
- `oneword-domains-pp-cli tlds list`
- `oneword-domains-pp-cli words`
- `oneword-domains-pp-cli words get`
- `oneword-domains-pp-cli words list`

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
oneword-domains-pp-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match — fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Cheapest way to buy a word

```bash
oneword-domains-pp-cli check smart --tld com,io,ai,co,app --available-only --json --select domain,available,cheapest_registrar.name,cheapest_registrar.price
```

Fans the word across the listed TLDs, keeps only free ones, and narrows the JSON to the price you would pay.

### Underpriced short .co auctions ending soon

```bash
oneword-domains-pp-cli listings rank --tld co --sort popularity --max-checks 25 --agent --select rows.domain,rows.price,rows.price_x,rows.taken_of_93,rows.hours_left
```

Compact agent envelope ranking .co auctions by how many of the 93 TLDs already took the word, with price relative to fresh registration; add --ending 48h once the site's feed carries live end dates again.

### Brainstorm with DomainsGPT and keep only free names

```bash
oneword-domains-pp-cli brainstorm --type brandable --context "a terminal tool for domain search" --tld ai --json
```

Streams about 20 names, keeps the available ones, prices them, flags dictionary words, and saves the batch locally.

### Which TLD got cheaper this month

```bash
oneword-domains-pp-cli tlds drift --since 30d --registrars
```

Records a per-registrar price snapshot and diffs it against the snapshot from 30 days earlier; the first run only records, so movers appear once two dated snapshots exist.

### Mine positive tech words that are free on .ai and .com

```bash
oneword-domains-pp-cli domains intersect --tld com,ai --category positive --min-length 4 --max-length 6 --csv
```

Lifetime-pass database query across two TLDs at once, exported as CSV for a spreadsheet.

## Auth Setup

Every headline command works without an account. The lifetime-pass database (domains search, domains intersect), saved domains, and saved DomainsGPT names use your browser session: sign in to oneword.domains in Chrome (email magic link), then run oneword-domains-pp-cli auth login --chrome, which needs one cookie extractor installed (pycookiecheat, cookies, or cookie-scoop-cli) and reads Google Chrome profiles. If you signed in from Brave or another Chromium browser, export the cookies to a file and run oneword-domains-pp-cli auth login --cookies-file <path> instead. There is no API key for the site; DomainsGPT partner tokens, when you have one, go in ONEWORD_DOMAINS_GPT_TOKEN.

Run `oneword-domains-pp-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json` — one JSON document on stdout (sync progress events go to stderr)
- `--compact` — keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain` — tabular rows (collection envelopes unwrap to the row array)
- `--quiet` — one identity value per row, no envelope

- **Pipeable** — JSON on stdout, errors on stderr
- **Filterable** — `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  oneword-domains-pp-cli domains search --tld ai --agent --select slug,tldCount,premium
  ```
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — sync/search commands can use the local SQLite store when available
- **Non-interactive** — never prompts, every input is a flag
- **Read-only** — do not use this CLI for create, update, delete, publish, comment, upvote, invite, order, send, or other mutating requests

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set — piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `ONEWORD_DOMAINS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `ONEWORD_DOMAINS_CONFIG_DIR`, `ONEWORD_DOMAINS_DATA_DIR`, `ONEWORD_DOMAINS_STATE_DIR`, `ONEWORD_DOMAINS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `ONEWORD_DOMAINS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `oneword-domains-pp-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "oneword-domains": {
        "command": "oneword-domains-pp-mcp",
        "env": {
          "ONEWORD_DOMAINS_HOME": "/srv/oneword-domains"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `ONEWORD_DOMAINS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `ONEWORD_DOMAINS_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data — the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
oneword-domains-pp-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "oneword-domains-pp-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `oneword-domains-pp-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `oneword-domains-pp-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities — direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `oneword-domains-pp-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall — argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
oneword-domains-pp-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
oneword-domains-pp-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
oneword-domains-pp-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know — a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback — fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
oneword-domains-pp-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`oneword-domains-pp-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `ONEWORD_DOMAINS_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
oneword-domains-pp-cli feedback "the --since flag is inclusive but docs say exclusive"
oneword-domains-pp-cli feedback --stdin < notes.txt
oneword-domains-pp-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `ONEWORD_DOMAINS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ONEWORD_DOMAINS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
oneword-domains-pp-cli profile save briefing --json
oneword-domains-pp-cli --profile briefing domains search --tld ai
oneword-domains-pp-cli profile list --json
oneword-domains-pp-cli profile show briefing
oneword-domains-pp-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `oneword-domains-pp-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/cmd/oneword-domains-pp-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add oneword-domains-pp-mcp -- oneword-domains-pp-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which oneword-domains-pp-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   oneword-domains-pp-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `oneword-domains-pp-cli <command> --help`.
