# Acceptance Report: philonet
Level: Full Dogfood (read-only; write commands dry-run only, never sent to the real account)
Tests: 243/243 passed (209 skipped by design: mutating commands, commands needing fixtures, no-positional probes)
Gate: PASS

Bugs found live and fixed in-session:
- find articles/thoughts/people required --limit (sniffer marked body fields required) -> optional with defaults.
- 17 endpoint commands lacked Examples sections -> examples + happy args added to spec.
- invite users sent `query` in the body; the API wants it in the URL -> path-embedded positional (`invite users <name>`).
- inbox discussions required unused body fields -> defaults.
- me profile required --user-id; API defaults to the caller -> optional.
- digest/voices/rhythm hid auth/fetch failures behind filter-blaming notes -> now error with the real cause.
Printing Press issues for retro: sniff analyzer marks every observed body field required and mislabels path-templated POSTs; generator cannot emit query-string params on POST (workaround: query in path); README/AGENTS boilerplate (agentcookie, `list`, `auth-status`, no write warning).
Authenticated samples reviewed: today, rhythm, digest, voices, owed, queue, resonance, find articles, feed for-me, invite users, inbox discussions all well-formed; owed/queue/resonance returned honest empty results (account has no thoughts, bookmarks or read-later items) and are covered by fixture unit tests.
