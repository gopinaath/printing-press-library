---
name: pp-philonet
description: "Your Philonet reading life in the terminal: feed, threads, streaks and friends, plus a local history the web app does not keep. Trigger phrases: `check my philonet`, `what are my friends reading on philonet`, `philonet reading streak`, `search philonet for`, `post a thought on philonet`, `invite a friend to think on philonet`, `use philonet`, `run philonet`."
author: "Som Samantray"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - philonet-pp-cli
    install:
      - kind: go
        bins: [philonet-pp-cli]
        module: github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/cmd/philonet-pp-cli
---

# Philonet — Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `philonet-pp-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install philonet --cli-only
   ```
2. Verify: `philonet-pp-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.6 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/cmd/philonet-pp-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Philonet has no public API, so this CLI replays the web app's own backend calls with your token. Run today for a morning snapshot, rhythm to build up reading history, and digest or owed to catch up on friends and unanswered replies. It can also post, react, send friend requests and invite people; every write supports --dry-run.

## When to Use This CLI

Use for reading or checking a user's Philonet feed, search, threads, stats, friends and notifications, for local analysis of data this CLI has stored, and for explicitly requested actions: posting a thought or reply, reacting, sending a friend request or inviting someone. Write commands act on the user's real account, so run them with --dry-run first and send only after the user confirms. Good for morning briefings and weekly recaps.

## Anti-triggers

Do not use this CLI for:
- Posting, replying, reacting, friend-requesting or inviting unless the user explicitly asked for that exact action
- Other social platforms (Twitter/X, LinkedIn, Reddit)
- Reading another person's private data
- Anything needing an official, stable API; this is an unofficial client of an undocumented backend

## Unique Capabilities

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

## Data sources and limits

- `today`, `owed`, `queue` and `resonance` read live endpoints only. They reject `--data-source local` with a clear error.
- `rhythm`, `digest` and `voices` keep account-keyed history in a per-account local file. By default they refresh from the API first. `--no-refresh` or `--data-source local` reads stored history without any API call (a token is still needed to identify the account). `--data-source live` always refreshes and fails instead of serving stored data; it conflicts with `--no-refresh`.
- History is keyed by the account in the token, so switching tokens never mixes history and a refreshed token keeps it. History saved by an earlier build at the previous default location is imported into the per-account file once. `rhythm` history starts at your first run because Philonet only reports the last week.
- This is an unofficial client of an undocumented API; endpoints can change without notice.

## Discovery Signals

This CLI was generated with browser-observed traffic context.
- Capture coverage: 38 API entries from 38 total network entries
- Protocols: rest_json (75% confidence)
- Auth signals: chrome_devtools_session
- Candidate command ideas: create_article — Derived from observed POST /v1/room/article traffic.; create_articles — Derived from observed POST /v1/room/search/articles traffic.; create_inboxnew — Derived from observed POST /v1/room/conversation/inboxnew traffic.; create_mainprofilenew — Derived from observed POST /v1/room/mainprofilenew traffic.; create_people — Derived from observed POST /v1/room/search/people traffic.; create_room — Derived from observed POST /v1/room/{room_id} traffic.; create_thoughts — Derived from observed POST /v1/room/search/thoughts traffic.; get_comments — Derived from observed GET /v1/room/articles/{article_id}/comments/{comment_id} traffic.

## Command Reference

**blog** — Public blog

- `philonet-pp-cli blog` — Philonet public blog posts (no auth needed)

**feed** — Article and thought feeds

- `philonet-pp-cli feed for-me` — Personalized 'For You' / friends feed (feed2 engine). filter=forme|friends
- `philonet-pp-cli feed moment` — Featured 'moment' card
- `philonet-pp-cli feed suggested` — Suggested curiosity-session feed
- `philonet-pp-cli feed unified` — Unified feed. filter=discover|friends

**find** — Live search across articles, thoughts and people

- `philonet-pp-cli find articles` — Search articles (POST body: query, sort_by, limit, offset)
- `philonet-pp-cli find people` — Search people
- `philonet-pp-cli find thoughts` — Search thoughts/comments

**friends** — Friends, requests and activity

- `philonet-pp-cli friends requests` — Friend requests (type=received|sent|all)
- `philonet-pp-cli friends requests-preview` — Friend requests preview
- `philonet-pp-cli friends respond` — Accept or decline a friend request (action=accept|decline)
- `philonet-pp-cli friends send-request` — Send a friend request
- `philonet-pp-cli friends stories` — Friends activity as stories
- `philonet-pp-cli friends thinking` — Friends/contacts thinking list with presence
- `philonet-pp-cli friends top` — Top friends
- `philonet-pp-cli friends withdraw` — Withdraw a sent friend request

**inbox** — Notifications, unread counts, discussions and DMs

- `philonet-pp-cli inbox activity` — Notifications / activity log
- `philonet-pp-cli inbox discussions` — Discussions inbox (POST, read-only)
- `philonet-pp-cli inbox dms` — Direct-message conversations
- `philonet-pp-cli inbox invitations` — Space invitations
- `philonet-pp-cli inbox unread` — Unread counters (notifications, invitations, total)
- `philonet-pp-cli inbox unread-conversations` — Unread discussion counters

**invite** — Invite people to think about an article

- `philonet-pp-cli invite send` — Invite a user to share thoughts on an article
- `philonet-pp-cli invite users` — Search users to invite

**me** — Your profile, reading stats and saved items

- `philonet-pp-cli me awards` — Awards/stars received
- `philonet-pp-cli me badge` — Verification / top-learner badge for a user
- `philonet-pp-cli me bookmarks` — Bookmarked thoughts
- `philonet-pp-cli me engagements` — My engagements
- `philonet-pp-cli me metrics` — My verified categories, stars, streak
- `philonet-pp-cli me onboarding` — Onboarding state and interests
- `philonet-pp-cli me profile` — Full profile for a user (POST, read-only)
- `philonet-pp-cli me profile-stats` — Streak and reading totals for a user
- `philonet-pp-cli me read-later` — Read-later saved articles
- `philonet-pp-cli me reading-stats` — My reading stats: last 7 days, goals, personal bests
- `philonet-pp-cli me spotlights` — Spotlights
- `philonet-pp-cli me standings` — Friends thinking-time leaderboard
- `philonet-pp-cli me thoughts` — A user's thoughts (paged)

**post** — Post thoughts and replies (write actions)

- `philonet-pp-cli post link` — Add a link as an article so you can post a thought on it
- `philonet-pp-cli post reply` — Reply to a thought
- `philonet-pp-cli post thought` — Post a thought (comment) on an article by article id

**react** — Star and bookmark thoughts (write actions)

- `philonet-pp-cli react bookmark` — Toggle bookmark on a thought
- `philonet-pp-cli react star` — Star a thought

**thread** — Articles and thought threads

- `philonet-pp-cli thread article` — Get article detail, members and people (POST, read-only)
- `philonet-pp-cli thread get` — Get one thought (comment) with article context and star quota
- `philonet-pp-cli thread impact` — Reader impact stats for an article
- `philonet-pp-cli thread replies` — List replies in a thought thread (POST, read-only)


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
philonet-pp-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match — fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

## Auth Setup

Philonet uses a bearer JWT held in your browser. Copy the accessToken from philonet.ai (DevTools > Application > Local Storage) and run 'philonet-pp-cli auth set-token' (or export PHILONET_TOKEN); 'auth status' shows what is configured and 'auth logout' clears it. The token lasts about six months. Philonet is an unofficial client of an undocumented API and can change without notice.

Run `philonet-pp-cli doctor` to verify setup.

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
  philonet-pp-cli blog --agent --select articleId,author,contentUrl
  ```
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — sync/search commands can use the local SQLite store when available
- **Non-interactive** — never prompts, every input is a flag
- **Explicit confirmation** — `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Explicit retries** — use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

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

- Use `--home <dir>` for one invocation, or set `PHILONET_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `PHILONET_CONFIG_DIR`, `PHILONET_DATA_DIR`, `PHILONET_STATE_DIR`, `PHILONET_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `PHILONET_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `philonet-pp-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `PHILONET_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `PHILONET_HOME`, or `doctor` will not find credentials left under the former root.

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
philonet-pp-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "philonet-pp-cli learnings confirm 12"] }
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
       materially more, record the divergence via `philonet-pp-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `philonet-pp-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities — direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `philonet-pp-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall — argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
philonet-pp-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
philonet-pp-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
philonet-pp-cli teach-playbook \
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
philonet-pp-cli playbook amend \
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

`philonet-pp-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `PHILONET_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
philonet-pp-cli feedback "the --since flag is inclusive but docs say exclusive"
philonet-pp-cli feedback --stdin < notes.txt
philonet-pp-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `PHILONET_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `PHILONET_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
philonet-pp-cli profile save briefing --json
philonet-pp-cli --profile briefing blog
philonet-pp-cli profile list --json
philonet-pp-cli profile show briefing
philonet-pp-cli profile delete briefing --yes
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
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `philonet-pp-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/social-and-messaging/philonet/cmd/philonet-pp-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add philonet-pp-mcp -- philonet-pp-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which philonet-pp-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   philonet-pp-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `philonet-pp-cli <command> --help`.
