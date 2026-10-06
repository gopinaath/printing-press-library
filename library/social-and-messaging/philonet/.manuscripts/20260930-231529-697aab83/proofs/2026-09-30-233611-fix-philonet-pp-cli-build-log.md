Manifest transcendence rows: 7 planned, 7 built. Phase 3 passed: all 7 ship.

# Philonet CLI build log

## Built
- Foundation: generator output (36 read endpoints + 10 write endpoints = 46 typed commands across feed, find, thread, me, friends, inbox, invite, post, react, blog), sync/search/analytics/tail/doctor, MCP server.
- Shared helpers: internal/cli/philonet_common.go (tolerant JSON access, JWT uid decode, history tables pn_feed_cards / pn_reading_days / pn_snapshots).
- Novel commands (all hand-coded, each with --dry-run and read-only MCP annotation): today (live), rhythm (auto: live fetch + local history), digest (auto), voices (auto), owed (live), queue (live), resonance (live).
- Tests: internal/cli/philonet_novel_test.go (parsers, zero-week reporting, friend filter, badge filter, unanswered-thread logic, queue fit filter, resonance grouping, empty results).
- Write commands (user-requested): post thought/reply/link, friends send-request/respond/withdraw, invite send/users, react star/bookmark. All get generated --dry-run.

## Deviations from the manifest wording
- owed, queue, resonance call live endpoints instead of reading the synced store (the feed/thought endpoints are not in the generic sync set and live reads are more accurate). Command paths and behavior are as approved.
- rhythm/digest/voices keep their own history in the local DB because Philonet exposes only the last week of reading days and ephemeral feed pages.

## Deferred / limits
- Request bodies for write endpoints were derived from server validation errors and the web bundle, never sent with real data. Optional post fields (quote, highlights) not modeled.
- inbox discussions item shape unverified (account inbox is empty); owed only reads its unread counter.
- Sync pagination warning: inbox and inbox-conversations declare limit without offset (first page only).
- Live verification of owed/queue/resonance limited by an account with no thoughts, bookmarks or read-later items.

## Generator limitations found
- Sniff analyzer mislabeled POST /v1/room/subcommentsnewthreadedv2 as /v1/room/{room_id}; corrected by hand in the spec.
