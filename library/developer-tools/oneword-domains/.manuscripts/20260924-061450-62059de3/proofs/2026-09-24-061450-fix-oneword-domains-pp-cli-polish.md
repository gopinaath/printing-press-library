## Polish results: oneword-domains-pp-cli (mid-pipeline, `STANDALONE_MODE=false`)

The CLI now scores 87/100 (up from 84) and passes every hard ship gate. One thing needs your attention first: the dogfood verdict went from WARN to **FAIL**. None of my fixes broke anything. Dogfood reports only the first rule that matches, and the dead-helper warning used to come first. Removing those helpers uncovered a config-consistency failure that was already there. That failure is a false positive in the checker (details under Remaining issues).

**Divergence check:** no clone of the public library exists locally, so I treated the internal copy as canonical. The build lock was stale (held by the parent run), so I went ahead.

**Setup advisory `[go-toolchain-old]`:** the binary wants Go 1.26.8 and 1.26.5 is installed. Go commands may download a toolchain.

```
                    Before    After     Delta
  Scorecard:        84/100    87/100    +3
  Verify:           100%      100%      +0
  Live matrix:      exercised -> exercised
  Tools-audit:      2         0         -2 pending findings
```

**Fixes applied**
- Removed 4 unused helpers from `internal/cli/helpers.go`: `successfulNoop`, `retainCLIQueryParams`, `retainExplicitQueryParams`, `handleBinaryResponseDelivery`. Dead Code score went from 2/5 to 5/5.
- Fixed the product name from "Oneword Domains" to "One Word Domains" in the root help, the `auth` Short and the MCP server name. I also added `display_name: "One Word Domains"` to `spec.yaml`; `mcp-sync` has since re-emitted the MCP `main.go` from it with the correct name.
- `hacks --tld` now lists short multi-letter stems first and single-letter stems last, so `--limit` keeps strong hacks. `--tld art` now gives ap/ch/he/qu/sm/st.art first instead of b.art, c.art, blk.art. Help text updated; tests pass.
- Changed the `check` example from `--available-only` (which returned `[]` because smart.* is taken everywhere) to `check smart --tld com,io,ai --json`. Edited in `research.json` and in the rendered README and SKILL. The `check` live-check sample now passes.
- tools-audit: accepted 2 thin-short findings (`profile list`, `learnings list`). Both are in generated files, so they need a fix in the generator template.

**Skipped findings**
- **gosec (53 raw issues):** none are in hand-written novel-feature code. They are all in generated files or generator framework files (`platform/migration.go`, `cliutil/testenv`). Retro candidates.
- **Live-check failures, all environmental:**
  - `brainstorm` and `gpt generate` hit the anonymous DomainsGPT quota.
  - `domains intersect` and `tlds inventory` need a signed-in lifetime-pass session.
  - `listings rank` (about 9.6s) and `tlds drift` (about 23s, 93 TLD fetches) hit the 10s sampling timeout.
- **Auth Protocol 2/10:** the scorer doesn't model cookie auth. Structural.
- **Dogfood novel-feature false positives:**
  - "check registered as domains check": a top-level `check` exists.
  - "gpt generate hand-rolled": it calls `owdGenerate` in a helper file the check doesn't look at.
- **`compare` ordering** (from the output review): it lists available domains first, cheapest first, then alphabetical. That is intended and documented.
- **Marking `listings watch` and `brainstorm` as `mcp:local-write`:** I tried and reverted it. Live-check then skipped both as "mutating", which loses live coverage. Other commands that write local history, like `check`, stay read-only, so leaving these two read-only is consistent.
- **Unplanned `mcp-sync` run:** I ran it once by mistake. It re-emitted the MCP and cliutil support files at the same generator version. Build, vet, tests and MCP parity all still pass.

**Remaining issues**
- **Dogfood FAIL from a config-consistency false positive.** The checker's regex reads the empty string literals in `cfg.SaveTokens("", "", token, "", ...)` (generated `internal/cli/auth.go:401`) as a field named `", token, "`. It then finds no matching read field in `config.go:363`. I didn't edit the generated line only to satisfy the checker. This is a retro candidate for dogfood `checkConfigConsistency` and for its first-match rule order, which let a WARN hide a FAIL. If the parent pipeline gates on the dogfood verdict, it will need to override this.

```
---POLISH-RESULT---
scorecard_before: 84
scorecard_after: 87
verify_before: 100
verify_after: 100
dogfood_before: WARN
dogfood_after: FAIL
dogfood_live_matrix_before: exercised
dogfood_live_matrix_after: exercised
govet_before: 0
govet_after: 0
gosec_before: 0
gosec_after: 0
tools_audit_before: 2 pending
tools_audit_after: 0 pending
publish_validate_before: skipped (mid-pipeline)
publish_validate_after: skipped (mid-pipeline)
fixes_applied:
- Removed 4 dead helper functions from internal/cli/helpers.go (Dead Code 2/5 -> 5/5)
- Corrected product name to "One Word Domains" in root help, auth Short, MCP server name; added display_name to spec.yaml
- hacks --tld now ranks short multi-letter stems first, single-letter stems last, so --limit keeps the strongest hacks
- Replaced the empty-result check novel-feature example (research.json, README, SKILL) with one that returns rows
- Accepted 2 tools-audit thin-short findings in generated files (profile list, learnings list) as generator-template retro items
skipped_findings:
- gosec 53 raw findings: all in generated/framework files, none in hand-authored novel code; generator retro candidates
- live-check failures: DomainsGPT anonymous quota (brainstorm, gpt generate), signed-in session required (domains intersect, tlds inventory), 10s sampling timeout on network-heavy listings rank / tlds drift; environmental
- auth_protocol 2/10: scorer does not model cookie auth; structural
- dogfood "check registered as domains check" and "gpt generate hand-rolled": checker false positives (top-level check exists; owdGenerate lives in a helper file)
- compare row ordering (output review): available-first, cheapest-first, then alphabetical is intended design
- mcp:local-write for listings watch/brainstorm: tried and reverted; it made live-check skip them as mutating and would be inconsistent with other history-writing commands
remaining_issues:
- dogfood verdict FAIL from checkConfigConsistency false positive on generated auth.go:401 (SaveTokens("", "", token, ...) read as field ", token, "); previously hidden by the dead-functions WARN via first-match rule ordering; dogfood retro candidate, parent must override if it gates on dogfood verdict
ship_recommendation: ship
further_polish_recommended: no
further_polish_reasoning: All hard gates pass. The only open item is a dogfood regex false positive in generated auth code, which needs a printing-press fix, not another polish pass.
---END-POLISH-RESULT---
```