# Phase 4.85 agentic output review
Status: WARN (warnings only, per Wave B policy). The reviewer's sampled runs were unauthenticated (401), so content-quality checks (relevance, ranking) could not be judged from them; live authenticated samples were reviewed separately by the operator (today, rhythm, digest, voices, owed, queue, resonance, find articles, feed for-me all returned well-formed, relevant output).
Findings, all fixed:
1. digest/voices printed a filter-blaming note when every fetch had failed -> now exit non-zero with the real error when nothing is stored.
2. rhythm reported refreshed:false with a misleading "--no-refresh" hint and no failure detail -> now returns refresh_error, errors when nothing is recorded, note no longer mentions --no-refresh.
Also fixed after live sampling: find articles required --limit (sniffer marked body fields required); optional fields now have defaults.
