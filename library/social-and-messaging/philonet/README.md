# Philonet CLI

**Your Philonet reading life in the terminal: feed, threads, streaks and friends, plus a local history the web app does not keep.**

Philonet has no public API, so this CLI replays the web app's own backend calls with your token. Run today for a morning snapshot, rhythm to build up reading history, and digest or owed to catch up on friends and unanswered replies. It can also post, react, send friend requests and invite people; every write supports --dry-run.

Learn more at [Philonet](https://api.typepilot.app).

Created by [@SomSamantray](https://github.com/SomSamantray) (Som Samantray).

## Install

The recommended path installs both the `philonet-pp-cli` binary and the `pp-philonet` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install philonet
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install philonet --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install philonet --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install philonet --agent claude-code
npx -y @mvanhorn/printing-press-library install philonet --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.6 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/cmd/philonet-pp-cli@latest
```

This installs the CLI only — no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/philonet-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install philonet --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-philonet --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-philonet --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install philonet --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/philonet-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `PHILONET_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/cmd/philonet-pp-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "philonet": {
      "command": "philonet-pp-mcp",
      "env": {
        "PHILONET_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Philonet uses a bearer JWT held in your browser. Copy the accessToken from philonet.ai (DevTools > Application > Local Storage) and run 'philonet-pp-cli auth set-token' (or export PHILONET_TOKEN); 'auth status' shows what is configured and 'auth logout' clears it. The token lasts about six months. Philonet is an unofficial client of an undocumented API and can change without notice.

## Quick Start

```bash
# Verify-safe health check without a network call
philonet-pp-cli doctor --dry-run


# See your streak, unread counts and friends reading now
philonet-pp-cli today --agent


# Browse the For You feed compactly
philonet-pp-cli feed for-me --agent --select article.title,article.url


# Search articles by topic
philonet-pp-cli find articles --query ai --agent


# Start recording reading history; it grows with every run
philonet-pp-cli rhythm --weeks 4 --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Daily habit

- **`today`** — One snapshot of your streak, reading time, friends thinking now, unread counts and pending requests.

  _Reach for this first when an agent needs a morning status of the user's Philonet reading habit._

  ```bash
  philonet-pp-cli today --agent
  ```
- **`rhythm`** — Weekly and monthly reading minutes, longest streak and rank-vs-friends trend, accumulated from your own runs (history starts at the first run).

  _Use for recap or goal questions over time, not live status._

  ```bash
  philonet-pp-cli rhythm --weeks 4 --agent
  ```

### Conversations

- **`digest`** — Friends' thoughts this CLI first saw recently, grouped by friend; 'recently' is relative to when the CLI saw them.

  _Catch up on friends' takes without scrolling the feed._

  ```bash
  philonet-pp-cli digest --since 24h --agent
  ```
- **`owed`** — Threads among your recent thoughts where someone replied and you have not answered yet.

  _Use to find conversations waiting on the user._

  ```bash
  philonet-pp-cli owed --agent
  ```
- **`resonance`** — Which of your thoughts earned stars and insightful reactions, by tag and week.

  _Tell the user what kind of thoughts land with people._

  ```bash
  philonet-pp-cli resonance --by week --agent
  ```

### Discovery

- **`voices`** — Rank people from your stored For You feed cards by thoughts on a topic, filtered by alma mater or employer badge.

  _Find credentialed voices on a topic when the web search cannot filter by badge._

  ```bash
  philonet-pp-cli voices --topic ai --badge professional --agent
  ```
- **`queue`** — Read-later and bookmarks that fit your free minutes (items with unknown length are dropped unless --include-unknown), flagged when discussed.

  _Pick what to read next given available time._

  ```bash
  philonet-pp-cli queue --fits 10 --agent
  ```

## Recipes


### Morning briefing

```bash
philonet-pp-cli today --agent
```

Streak, unread and friends reading now in one record.

### Compact feed scan

```bash
philonet-pp-cli feed for-me --agent --select article.title,article.reading_time.text,conversation_starters.content
```

Narrows large feed cards to titles, read time and top thought.

### Catch up on friends

```bash
philonet-pp-cli digest --since 24h --agent
```

Friends' thoughts this CLI first saw in the last day; the first run treats the whole feed as new.

### Pick something to read

```bash
philonet-pp-cli queue --fits 10 --agent
```

Saved items that fit ten minutes.

### Post a thought

```bash
philonet-pp-cli post thought --article-id 29018 --content "Interesting take" --dry-run
```

Preview the request first; removing --dry-run sends a real post to the user's account, so only do it after they confirm.

## Usage

Run `philonet-pp-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `PHILONET_CONFIG_DIR`, `PHILONET_DATA_DIR`, `PHILONET_STATE_DIR`, or `PHILONET_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `PHILONET_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export PHILONET_HOME=/srv/philonet
philonet-pp-cli doctor
```

Under `PHILONET_HOME=/srv/philonet`, the four dirs resolve to `/srv/philonet/config`, `/srv/philonet/data`, `/srv/philonet/state`, and `/srv/philonet/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "philonet": {
      "command": "philonet-pp-mcp",
      "env": {
        "PHILONET_HOME": "/srv/philonet"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `PHILONET_DATA_DIR` overrides an explicit `--home` for that kind. Use `PHILONET_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `PHILONET_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `philonet-pp-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### blog

Public blog

- **`philonet-pp-cli blog`** - Philonet public blog posts (no auth needed)

### feed

Article and thought feeds

- **`philonet-pp-cli feed for-me`** - Personalized "For You" / friends feed (feed2 engine). filter=forme|friends
- **`philonet-pp-cli feed moment`** - Featured "moment" card
- **`philonet-pp-cli feed suggested`** - Suggested curiosity-session feed
- **`philonet-pp-cli feed unified`** - Unified feed. filter=discover|friends

### find

Live search across articles, thoughts and people

- **`philonet-pp-cli find articles`** - Search articles (POST body: query, sort_by, limit, offset)
- **`philonet-pp-cli find people`** - Search people
- **`philonet-pp-cli find thoughts`** - Search thoughts/comments

### friends

Friends, requests and activity

- **`philonet-pp-cli friends requests`** - Friend requests (type=received|sent|all)
- **`philonet-pp-cli friends requests-preview`** - Friend requests preview
- **`philonet-pp-cli friends respond`** - Accept or decline a friend request (action=accept|decline)
- **`philonet-pp-cli friends send-request`** - Send a friend request
- **`philonet-pp-cli friends stories`** - Friends activity as stories
- **`philonet-pp-cli friends thinking`** - Friends/contacts thinking list with presence
- **`philonet-pp-cli friends top`** - Top friends
- **`philonet-pp-cli friends withdraw`** - Withdraw a sent friend request

### inbox

Notifications, unread counts, discussions and DMs

- **`philonet-pp-cli inbox activity`** - Notifications / activity log
- **`philonet-pp-cli inbox discussions`** - Discussions inbox (POST, read-only)
- **`philonet-pp-cli inbox dms`** - Direct-message conversations
- **`philonet-pp-cli inbox invitations`** - Space invitations
- **`philonet-pp-cli inbox unread`** - Unread counters (notifications, invitations, total)
- **`philonet-pp-cli inbox unread-conversations`** - Unread discussion counters

### invite

Invite people to think about an article

- **`philonet-pp-cli invite send`** - Invite a user to share thoughts on an article
- **`philonet-pp-cli invite users`** - Search users to invite

### me

Your profile, reading stats and saved items

- **`philonet-pp-cli me awards`** - Awards/stars received
- **`philonet-pp-cli me badge`** - Verification / top-learner badge for a user
- **`philonet-pp-cli me bookmarks`** - Bookmarked thoughts
- **`philonet-pp-cli me engagements`** - My engagements
- **`philonet-pp-cli me metrics`** - My verified categories, stars, streak
- **`philonet-pp-cli me onboarding`** - Onboarding state and interests
- **`philonet-pp-cli me profile`** - Full profile for a user (POST, read-only)
- **`philonet-pp-cli me profile-stats`** - Streak and reading totals for a user
- **`philonet-pp-cli me read-later`** - Read-later saved articles
- **`philonet-pp-cli me reading-stats`** - My reading stats: last 7 days, goals, personal bests
- **`philonet-pp-cli me spotlights`** - Spotlights
- **`philonet-pp-cli me standings`** - Friends thinking-time leaderboard
- **`philonet-pp-cli me thoughts`** - A user's thoughts (paged)

### post

Post thoughts and replies (write actions)

- **`philonet-pp-cli post link`** - Add a link as an article so you can post a thought on it
- **`philonet-pp-cli post reply`** - Reply to a thought
- **`philonet-pp-cli post thought`** - Post a thought (comment) on an article by article id

### react

Star and bookmark thoughts (write actions)

- **`philonet-pp-cli react bookmark`** - Toggle bookmark on a thought
- **`philonet-pp-cli react star`** - Star a thought

### thread

Articles and thought threads

- **`philonet-pp-cli thread article`** - Get article detail, members and people (POST, read-only)
- **`philonet-pp-cli thread get`** - Get one thought (comment) with article context and star quota
- **`philonet-pp-cli thread impact`** - Reader impact stats for an article
- **`philonet-pp-cli thread replies`** - List replies in a thought thread (POST, read-only)


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`philonet-pp-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`philonet-pp-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`philonet-pp-cli learnings list`** - Inspect taught rows
- **`philonet-pp-cli learnings forget <query>`** - Undo a teach
- **`philonet-pp-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`philonet-pp-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`philonet-pp-cli teach-pattern`** - Install a query/resource template up front
- **`philonet-pp-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `PHILONET_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `philonet-pp-cli` opens the database, older binaries refuse it with a version error — upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
philonet-pp-cli blog

# JSON for scripting and agents
philonet-pp-cli blog --json
# Filter to specific fields
philonet-pp-cli blog --json --select articleId,author,contentUrl

# Dry run — show the request without sending
philonet-pp-cli blog --dry-run

# Agent mode — JSON + compact + no prompts in one flag
philonet-pp-cli blog --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
philonet-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `philonet-pp-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/philonet-pp-cli/config.toml`; `--home`, `PHILONET_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `PHILONET_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `philonet-pp-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Write commands (they act on your real account)

`post thought|reply|link`, `friends send-request|respond|withdraw`, `invite send` and `react star|bookmark` change your real Philonet account. Run each with `--dry-run` first; it prints the exact request and sends nothing. Their request shapes were derived from the web app, so treat them as best-effort.

## Known limits

- Unofficial client of an undocumented API (`api.typepilot.app`); endpoints can change without notice.
- `rhythm`, `digest` and `voices` keep their own history in a per-account local file, keyed by the account in your token, so switching tokens never mixes history and a refreshed token keeps the same history. History saved by an earlier build at the previous default location is imported into the per-account file once. `--data-source local` (or `--no-refresh`) reads that history without contacting the API; a token is still needed to identify the account. `--data-source live` refreshes from the API and fails rather than falling back to stored data.
- `today`, `owed`, `queue` and `resonance` are live-only and reject `--data-source local`. `owed` scans only your most recent thoughts (`--max-scan`); `resonance` scans a bounded number of pages of your thoughts (`--max-scan-pages`); `queue` merges up to 50 read-later and 50 bookmarked items (`--fits`, `--limit`, `--include-unknown`).

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `philonet-pp-cli doctor` to check credentials
- Verify the environment variable is set: `echo $PHILONET_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct

### API-specific

- **HTTP 401 or 403** — Your token expired; copy a fresh accessToken from philonet.ai local storage and run philonet-pp-cli auth set-token
- **Empty digest output** — digest only lists friends' thoughts first seen within --since; widen --since, add --all, or raise --max-scan-pages
- **Empty voices output** — voices ranks cards stored from your For You feed; loosen --topic or --badge, or raise --max-scan-pages
- **rhythm shows few days** — Philonet reports only the last week, so history starts at your first rhythm run; run it regularly

## Discovery Signals

This CLI was generated with browser-captured traffic analysis.
- Target observed: https://philonet.ai
- Capture coverage: 38 API entries from 38 total network entries
- Reachability: standard_http (65% confidence)
- Protocols: rest_json (75% confidence)
- Auth signals: chrome_devtools_session
- Candidate command ideas: create_article — Derived from observed POST /v1/room/article traffic.; create_articles — Derived from observed POST /v1/room/search/articles traffic.; create_inboxnew — Derived from observed POST /v1/room/conversation/inboxnew traffic.; create_mainprofilenew — Derived from observed POST /v1/room/mainprofilenew traffic.; create_people — Derived from observed POST /v1/room/search/people traffic.; create_room — Derived from observed POST /v1/room/{room_id} traffic.; create_thoughts — Derived from observed POST /v1/room/search/thoughts traffic.; get_comments — Derived from observed GET /v1/room/articles/{article_id}/comments/{comment_id} traffic.

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
