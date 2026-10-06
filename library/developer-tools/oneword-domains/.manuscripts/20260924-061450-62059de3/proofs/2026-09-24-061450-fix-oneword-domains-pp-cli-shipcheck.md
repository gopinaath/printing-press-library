# Shipcheck: oneword-domains-pp-cli

Run 1 log: `2026-09-24-061450-shipcheck-run1.log` · Run 2 log: `2026-09-24-061450-shipcheck-run2.log` · Standalone scorecard (45 s probe): `2026-09-24-061450-scorecard-live45.log`

## Legs (run 2, umbrella)
| Leg | Result |
|---|---|
| verify (mock, --fix) | PASS, 39/39, 0 critical |
| validate-narrative --strict --full-examples | PASS (13 ok, 2 unsupported side-effectful auth examples) |
| dogfood | PASS (WARN verdict: 4 generator dead helpers, config field regex mismatch) |
| workflow-verify | workflow-pass |
| verify-skill | PASS (exit 0) |
| scorecard --live-check | PASS, 83/100 Grade A |
| umbrella | PASS 7/7, exit 0 |

## Top blockers found and fixed
1. Run 1 failed validate-narrative: a troubleshooting tip used the placeholder `--prefix <text>`, which the validator executed literally and hit the site's 500. Fixed in research.json (concrete `--prefix sm` example); dogfood re-synced README/SKILL.
2. Root help drifted from the narrative headline because the spec also carried `cli_description`; removed from the spec and regenerated.
3. Cookie import required two NextAuth cookie names when only `__Secure-next-auth.session-token` exists on https; spec trimmed, regenerated; `auth login --cookies-file` then imported the session (exported from Brave, where the user is signed in; `auth login --chrome` scans Google Chrome profiles only).

## Before / after
- verify pass rate: 100% → 100% (39/39 both runs)
- scorecard total: 83 → 83 (umbrella); 87/100 standalone with a 45 s probe budget and unverified dims (path_validity, auth_protocol) omitted from the denominator
- narrative: 1 failed example → 0
- live sample probe: 3/6 (10 s budget) → 4/6 (45 s budget; brainstorm completes in about 15 s because DomainsGPT itself takes 10 to 15 s)

## Live sample probe residue (not CLI bugs)
- `domains intersect` and `tlds inventory` exit 4 inside the scorecard probe because the probe sandbox does not pass the `ONEWORD_DOMAINS_CONFIG` session override (the same override works in a sandboxed HOME when set directly: `domains count --tld ai` returns 2225). Both commands were verified live with the imported session: inventory returns per-TLD available counts (io 269, ai 150, com 0 for positive words); intersect scans 2 pages per TLD and returns the intersection (empty for 4 to 6 letter positive words on com plus ai; per_tld_counts ai 5, com 0). Phase 5 live dogfood uses the documented config override so the matrix exercises them for real.
- Dead code 2/5 and auth_protocol 2/10 are generator/scorer shape issues for cookie-auth CLIs (retro candidates).

## Ship threshold
shipcheck exit 0 · verify PASS · dogfood wiring clean · workflow-pass · verify-skill 0 · scorecard 83 at or above 65 · every approved feature returns correct output live (6 novel, 5 absorbed hand-built, and gpt generate verified with real responses; pass-gated ones with the imported session).

**Final ship recommendation: ship**
