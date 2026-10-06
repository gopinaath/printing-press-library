# Philonet CLI Brief

## API Identity
- Domain: philonet.ai — social reading/"thinking" network. Users share links/PDFs, highlight quotes, post "thoughts" (conversation starters) on articles, react ("Insightful"), award stars, DM, and track reading time with friends.
- Backend: REST JSON at https://api.typepilot.app/v1 (web app is Next.js at philonet.ai; Firebase identity for sign-in; API uses its own HS256 JWT).
- Users (observed on the live product):
  - The daily-reader: reads 1-3 articles every morning, keeps a reading streak, checks "Friends thinking today" standings (who read how long) before starting.
  - The discussant: scans friends' fresh thoughts on articles, replies in threads, hands out stars/"Insightful", watches notifications and friend requests.
  - The curator/researcher: searches articles/thoughts/people on a topic (e.g. "ai"), saves read-later items and bookmarks, wants to find what smart peers (verified alma mater/employer badges) said.
- Data profile: articles (url, title, summary, tags, category, reading time), thoughts/comments (quotes, replies, reactions), people (friendship, mutual connections), reading stats (streaks, thinking time), notifications.

## Reachability Risk
- Low. Plain HTTPS + bearer token; no bot-gating seen. Azure front door, `access-control-allow-origin: *`.
- Not an official public API: endpoints undocumented and may change. Only /v1/public/blog is unauthenticated.

## Auth
- `Authorization: Bearer <accessToken>` (JWT, ~180d expiry). Web app keeps accessToken/refreshToken in localStorage. `/v1/token` exists (refresh). CLI: env `PHILONET_TOKEN` (never stored in repo).

## Top Workflows
1. Browse "For You" / discover / friends feed from the terminal (feed2/forme, room/feed/unified).
2. Search articles, thoughts, and people (room/search/{articles,thoughts,people}).
3. Read a thought thread on an article (room/articles/{id}/comments/{cid}, subcommentsnewthreadedv2).
4. Check reading stats, streak, friends standings, profile metrics.
5. Inbox: notifications/activity, friend requests, invitations, DMs, unread counts.

## Table Stakes
- No competing CLI/MCP/SDK found; web UI is the incumbent.

## Data Layer
- Primary entities: articles, thoughts (conversation starters/comments), users, friend edges, activity, reading days.
- Sync cursor: page/pageSize (feeds), limit/offset (search, inbox), next_cursor (activity, DMs).
- FTS/search: local SQLite over synced feed articles + thoughts for offline search.

## User Vision
- Study www.philonet.ai and webapp via Chrome DevTools MCP using the existing logged-in Chrome session, then build the CLI.

## Product Thesis
- Name: philonet-pp-cli
- Why: terminal/agent access to a reading network with no public API — feed, search, threads, stats, offline search of everything you've seen.

## Build Priorities
1. Auth (token env/config), feed, search (articles/thoughts/people), thread read.
2. Reading stats/standings/profile/metrics, notifications, friend requests, bookmarks/read-later.
3. Local store + offline search, read-only first; write ops (react, bookmark, friend request, add thought) behind explicit commands.
