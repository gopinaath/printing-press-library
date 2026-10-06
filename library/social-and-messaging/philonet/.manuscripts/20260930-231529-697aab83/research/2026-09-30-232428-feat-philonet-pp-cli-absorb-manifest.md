# Philonet CLI Absorb Manifest

> **Superseded in the shipped build:** `owed`, `queue` and `resonance` read live endpoints (not the synced store); `digest`, `voices` and `rhythm` keep account-keyed history that refreshes from the API on each run; `resonance` joins thoughts with awards. See the build log's Deviations section.

## Absorbed (match or beat everything that exists)
Incumbent is the philonet.ai web app (no competing CLI/MCP/SDK found on npm or GitHub).
| # | Feature | Best Source | Our Implementation | Added Value |
|---|---------|-------------|--------------------|-------------|
| 1 | For You / friends feed | philonet web | (generated endpoint) feed for_me | --json/--select, offline via sync |
| 2 | Discover feed, moment, suggested | philonet web | (generated endpoint) feed unified | agent-native output |
| 3 | Search articles | philonet web | (generated endpoint) find articles | scriptable, --csv |
| 4 | Search thoughts | philonet web | (generated endpoint) find thoughts | scriptable |
| 5 | Search people | philonet web | (generated endpoint) find people | scriptable |
| 6 | Read a thought | philonet web | (generated endpoint) thread get | JSON |
| 7 | Thread replies | philonet web | (generated endpoint) thread replies | JSON |
| 8 | Article detail + members | philonet web | (generated endpoint) thread article | JSON |
| 9 | Article impact | philonet web | (generated endpoint) thread impact | JSON |
| 10 | Reading stats | philonet web | (generated endpoint) me reading_stats | snapshot-able |
| 11 | Friends standings | philonet web | (generated endpoint) me standings | snapshot-able |
| 12 | Streak/profile stats | philonet web | (generated endpoint) me profile_stats | snapshot-able |
| 13 | Verification badge | philonet web | (generated endpoint) me badge | JSON |
| 14 | Metrics | philonet web | (generated endpoint) me metrics | JSON |
| 15 | Profile | philonet web | (generated endpoint) me profile | JSON |
| 16 | My thoughts | philonet web | (generated endpoint) me thoughts | offline |
| 17 | Bookmarks | philonet web | (generated endpoint) me bookmarks | offline |
| 18 | Engagements | philonet web | (generated endpoint) me engagements | offline |
| 19 | Awards | philonet web | (generated endpoint) me awards | offline |
| 20 | Spotlights | philonet web | (generated endpoint) me spotlights | offline |
| 21 | Read later | philonet web | (generated endpoint) me read_later | offline |
| 22 | Onboarding state | philonet web | (generated endpoint) me onboarding | JSON |
| 23 | Top friends | philonet web | (generated endpoint) friends top | JSON |
| 24 | Friends thinking list | philonet web | (generated endpoint) friends thinking | presence data |
| 25 | Friend requests | philonet web | (generated endpoint) friends requests | JSON |
| 26 | Requests preview | philonet web | (generated endpoint) friends requests_preview | JSON |
| 27 | Friends as stories | philonet web | (generated endpoint) friends stories | JSON |
| 28 | Unread counters | philonet web | (generated endpoint) inbox unread | exit-code friendly |
| 29 | Unread discussions | philonet web | (generated endpoint) inbox unread_conversations | JSON |
| 30 | Notifications | philonet web | (generated endpoint) inbox activity | JSON |
| 31 | Discussions inbox | philonet web | (generated endpoint) inbox discussions | JSON |
| 32 | Direct messages | philonet web | (generated endpoint) inbox dms | JSON |
| 33 | Space invitations | philonet web | (generated endpoint) inbox invitations | JSON |
| 34 | Public blog | philonet web | (generated endpoint) blog list | no auth |
| 35 | Offline search over synced data | generator | philonet-pp-cli search | FTS5 |
| 36 | Health check | generator | philonet-pp-cli doctor | token validity |
| 37 | Post a thought on an article | philonet web (user request) | (generated endpoint) post thought | --dry-run, --stdin, scriptable |
| 38 | Reply to a thought | philonet web | (generated endpoint) post reply | --dry-run |
| 39 | Add a link/article | philonet web | (generated endpoint) post link | --dry-run |
| 40 | Send friend request | philonet web (user request) | (generated endpoint) friends send_request | --dry-run |
| 41 | Accept/decline/withdraw friend request | philonet web | (generated endpoint) friends respond | --dry-run |
| 42 | Invite someone to think on an article | philonet web (user request) | (generated endpoint) invite send | --dry-run |
| 43 | Find users to invite | philonet web | (generated endpoint) invite users | JSON |
| 44 | Star / bookmark a thought | philonet web | (generated endpoint) react star | --dry-run |
| 45 | Search news (articles by topic) | philonet web (user request) | (generated endpoint) find articles | covered |

### Transcendence (only possible with our approach)
| # | Feature | Command | Buildability | Why Only We Can Do This | Long Description |
|---|---------|---------|--------------|------------------------|------------------|
| 1 | Today briefing | today | hand-code | Live join of streak, reading time, friends thinking, unread and requests that the web app spreads over several screens | Use this command for a single morning status snapshot. Do NOT use it for reading history over time; use 'rhythm' instead. |
| 2 | Reading rhythm | rhythm | hand-code | Web app shows only today; needs dated local snapshots of stats/standings | Use this command for trends and history. Do NOT use it for today's live status; use 'today' instead. |
| 3 | Friends digest | digest | hand-code | Joins synced feed cards with top friends, grouped per friend and time-windowed | Use this command to see what top friends said recently. Do NOT use it to find people by credential or topic; use 'voices' instead. |
| 4 | Owed replies | owed | hand-code | Local join of my thoughts, replies and discussions inbox to find threads awaiting my answer | none |
| 5 | Voices | voices | hand-code | Aggregates starter profiles + verification badges over synced cards with tag/FTS filter | Use this command to rank people by credential and topic from your synced data. Do NOT use it for a keyword search across thoughts; use 'find thoughts' instead. |
| 6 | Reading queue | queue | hand-code | Joins read-later and bookmarks with reading time and friend-thought counts; --fits <minutes> | none |
| 7 | Resonance | resonance | hand-code | Joins my thoughts with engagements, awards, spotlights to rank what landed | none |

Hand-code: 7 of 7 (today, rhythm, digest, owed, voices, queue, resonance). Stubs: none. Write actions added per user request (post thought/reply/link, friend request, invite, star, bookmark). Request shapes come from server validation errors and the web bundle; live write-testing needs your explicit OK per action (dry-run only otherwise).
