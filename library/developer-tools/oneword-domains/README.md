# One Word Domains CLI

**Check a word across every TLD with registrar prices, watch the aftermarket, and generate names with DomainsGPT, all from the terminal with a local store the site itself does not offer.**

One Word Domains tracks 27,627 dictionary words across 93 TLDs with prices from six registrars, an aftermarket feed, and the DomainsGPT name generator, but publishes no API. This CLI replays its JSON routes so agents and scripts can compare TLDs, check availability, mine the dictionary, browse auctions, and run DomainsGPT, then keeps everything in SQLite for price drift, auction watches, and offline search. No other tool targeted oneword.domains as of the September 2026 ecosystem search.

Learn more at [One Word Domains](https://oneword.domains).

Created by [@devacto](https://github.com/devacto) (Victor Wibisono).

## Install

The recommended path installs both the `oneword-domains-pp-cli` binary and the `pp-oneword-domains` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install oneword-domains
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install oneword-domains --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install oneword-domains --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install oneword-domains --agent claude-code
npx -y @mvanhorn/printing-press-library install oneword-domains --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.6 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/cmd/oneword-domains-pp-cli@latest
```

This installs the CLI only — no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/oneword-domains-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install oneword-domains --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-oneword-domains --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-oneword-domains --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install oneword-domains --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

The bundle reuses your local browser session — set it up first if you haven't:

```bash
oneword-domains-pp-cli auth login --chrome
```

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/oneword-domains-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/developer-tools/oneword-domains/cmd/oneword-domains-pp-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "oneword-domains": {
      "command": "oneword-domains-pp-mcp"
    }
  }
}
```

</details>

## Authentication

Every headline command works without an account. The lifetime-pass database (domains search, domains intersect), saved domains, and saved DomainsGPT names use your browser session: sign in to oneword.domains in Chrome (email magic link), then run oneword-domains-pp-cli auth login --chrome, which needs one cookie extractor installed (pycookiecheat, cookies, or cookie-scoop-cli) and reads Google Chrome profiles. If you signed in from Brave or another Chromium browser, export the cookies to a file and run oneword-domains-pp-cli auth login --cookies-file <path> instead. There is no API key for the site; DomainsGPT partner tokens, when you have one, go in ONEWORD_DOMAINS_GPT_TOKEN.

## Quick Start

```bash
# Confirms the binary, config, and reachability of oneword.domains without touching the network
oneword-domains-pp-cli doctor --dry-run

# All 93 TLDs with registrations, popularity, and minimum registration price
oneword-domains-pp-cli tlds list --sort-alpha --json

# Per-registrar prices, the cheapest registrar, and example sites for .ai
oneword-domains-pp-cli tlds get ai

# Availability, premium flag, and how many of the 93 TLDs still have this word free
oneword-domains-pp-cli domains check smart.com

# Cheapest short .co listings on the aftermarket
oneword-domains-pp-cli listings list --tld co --sort priceAsc --max-length 5

# About 20 AI-generated names with live availability, parsed from the DomainsGPT stream into JSON
oneword-domains-pp-cli gpt generate --type portmanteau --word open --position prefix --tld com

```

## Unique Features

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

## Usage

Run `oneword-domains-pp-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `ONEWORD_DOMAINS_CONFIG_DIR`, `ONEWORD_DOMAINS_DATA_DIR`, `ONEWORD_DOMAINS_STATE_DIR`, or `ONEWORD_DOMAINS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `ONEWORD_DOMAINS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export ONEWORD_DOMAINS_HOME=/srv/oneword-domains
oneword-domains-pp-cli doctor
```

Under `ONEWORD_DOMAINS_HOME=/srv/oneword-domains`, the four dirs resolve to `/srv/oneword-domains/config`, `/srv/oneword-domains/data`, `/srv/oneword-domains/state`, and `/srv/oneword-domains/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `ONEWORD_DOMAINS_DATA_DIR` overrides an explicit `--home` for that kind. Use `ONEWORD_DOMAINS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `ONEWORD_DOMAINS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `oneword-domains-pp-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### domains

Availability checks for word plus TLD and the lifetime-pass domain database; search needs auth login --chrome

- **`oneword-domains-pp-cli domains check`** - Check availability, premium flag, price, aftermarket status, and popularity (tldCount) for a dictionary word on one TLD, such as smart.com; words outside the dictionary return HTTP 500
- **`oneword-domains-pp-cli domains count`** - Count available one-word domains for a TLD with the same filters as search; returns a bare integer; normally public, but the site sometimes answers 401 to anonymous callers, in which case auth login --chrome supplies the session
- **`oneword-domains-pp-cli domains saves`** - List the domains saved to your One Word Domains account; requires auth login --chrome; row shape unverified because only an empty list was observed
- **`oneword-domains-pp-cli domains search`** - Search the 1,380,000-plus available one-word domain database for a TLD with text, category, price, length, sort, and page filters. Requires a logged-in lifetime-pass session (auth login --chrome); returns 401 otherwise

### gpt

DomainsGPT: AI-generated brandable names with live availability

- **`oneword-domains-pp-cli gpt saves`** - List DomainsGPT names saved to your account; requires auth login --chrome; row shape unverified because only an empty list was observed
- **`oneword-domains-pp-cli gpt usage`** - Show DomainsGPT usage and quota for the current identity; anonymous callers get 10, signed-in accounts more

### listings

Aftermarket domain auctions and buy-now listings with price, bids, and end date

- **`oneword-domains-pp-cli listings count`** - Count aftermarket listings matching the filters; returns a bare integer
- **`oneword-domains-pp-cli listings filters`** - List the TLDs that currently have aftermarket listings, with counts
- **`oneword-domains-pp-cli listings list`** - List aftermarket listings, 100 per page, filtered by TLD, registrar, and word length, sorted by domain, price, bids, or ending time

### tlds

Top-level domains: registrations, top-10M presence, views, per-registrar prices, and the cheapest registrar

- **`oneword-domains-pp-cli tlds get`** - Get one TLD with its description, registrar price list, cheapest registrar, and example sites; unknown TLDs return an empty record
- **`oneword-domains-pp-cli tlds list`** - List all 93 TLDs with registration counts, top-10M site counts, page views, and minimum registration price, ordered by popularity unless --sort-alpha is set

### words

The dictionary behind One Word Domains, 27,627 words tagged with categories

- **`oneword-domains-pp-cli words count`** - Count dictionary words matching the directory filters; returns a bare integer
- **`oneword-domains-pp-cli words get`** - Get one dictionary word with its category tags
- **`oneword-domains-pp-cli words list`** - List dictionary words; with --prefix up to 10 autocomplete matches, otherwise a category or substring directory of 100 words per page in alphabetical order with categories


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`oneword-domains-pp-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`oneword-domains-pp-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`oneword-domains-pp-cli learnings list`** - Inspect taught rows
- **`oneword-domains-pp-cli learnings forget <query>`** - Undo a teach
- **`oneword-domains-pp-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`oneword-domains-pp-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`oneword-domains-pp-cli teach-pattern`** - Install a query/resource template up front
- **`oneword-domains-pp-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `ONEWORD_DOMAINS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `oneword-domains-pp-cli` opens the database, older binaries refuse it with a version error — upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
oneword-domains-pp-cli domains search --tld ai

# JSON for scripting and agents
oneword-domains-pp-cli domains search --tld ai --json
# Filter to specific fields
oneword-domains-pp-cli domains search --tld ai --json --select slug,tldCount,premium

# Dry run — show the request without sending
oneword-domains-pp-cli domains search --tld ai --dry-run

# Agent mode — JSON + compact + no prompts in one flag
oneword-domains-pp-cli domains search --tld ai --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store.

Set `ONEWORD_DOMAINS_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
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

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
oneword-domains-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `oneword-domains-pp-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/oneword-domains-pp-cli/config.toml`; `--home`, `ONEWORD_DOMAINS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `oneword-domains-pp-cli doctor` to check credentials
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **domains search returns 401 Unauthorized** — Sign in to oneword.domains in Chrome with a lifetime-pass account, then run `oneword-domains-pp-cli auth login --chrome`
- **domains check returns HTTP 500 for a name** — The site only knows dictionary words; run oneword-domains-pp-cli words list --prefix sm to find the indexed spelling, or oneword-domains-pp-cli gpt generate --tld com for invented names
- **gpt generate returns 429 or 'usage limit'** — Anonymous callers get 10 generations; check `oneword-domains-pp-cli gpt usage`, sign in via `auth login --chrome` for the larger quota, or set ONEWORD_DOMAINS_GPT_TOKEN
- **tlds get prints an empty record with null prices** — The TLD is not one of the 93 the site tracks; run `oneword-domains-pp-cli tlds list --sort-alpha` to see the supported slugs
- **listings rank --ending or listings watch --ending-within returns no rows** — The aftermarket feed currently lists auctions whose end dates have already passed; drop the window flag and sort with --sort popularity or --sort price-x instead

## Discovery Signals

This CLI was generated with browser-captured traffic analysis.
- Target observed: https://oneword.domains/tlds
- Capture coverage: 55 API entries from 56 total network entries
- Reachability: standard_http (65% confidence)
- Protocols: rest_json (75% confidence), browser_rendered (70% confidence), html_scrape (55% confidence)
- Auth signals: chrome_mcp_session
- Generation hints: requires_js_rendering
- Candidate command ideas: create_generate — Derived from observed POST /gpt/generate traffic.; create_view — Derived from observed POST /api/tlds/ai/view traffic.; list_ai — Derived from observed GET /api/tlds/ai traffic.; list_ai.json — Derived from observed GET /_next/data/qzfSin9peIEieIakzu-ki/tlds/ai.json traffic.; list_api_key — Derived from observed GET /api/auth/api-key traffic.; list_com — Derived from observed GET /api/tlds/com traffic.; list_count — Derived from observed GET /api/domains/count traffic.; list_domains — Derived from observed GET /api/domains traffic.

Warnings from discovery:
- error_status_cluster: Endpoint cluster only observed error HTTP statuses.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**domain-goat**](https://github.com/mvanhorn/printing-press-library) — Go (2037 stars)
- [**mcp-domain-availability**](https://github.com/imprvhub/mcp-domain-availability) — Python (59 stars)
- [**FastDomainCheck-MCP-Server**](https://github.com/bingal/FastDomainCheck-MCP-Server) — Python (40 stars)
- [**domainr-cli**](https://github.com/MichaelThessel/domainr-cli) — Go (25 stars)
- [**find-my-domain**](https://github.com/idimetrix/find-my-domain) — TypeScript (16 stars)
- [**domainr-mcp-server**](https://github.com/danohn/domainr-mcp-server) — TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
