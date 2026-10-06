# Phase 4.95 local code review findings: oneword-domains-pp-cli

Review path chosen: direct reviewer-subagent dispatch (correctness, security, maintainability) over internal/cli, internal/store, internal/client, internal/mcp (excluding cobratree), cmd; hand-authored owd_* files and implemented scaffolds prioritized.

## Round 1 findings (in scope, routed to autofix)
- Correctness: listings watch re-reports gone listings forever (high); tlds drift skips the daily min-price snapshot when detail rows exist; boundCtx caps whole loops at the per-request timeout; watch new[] rows camelCase vs snake_case siblings; non-upsert insert on owd_listing_seen; snapshot writes swallow errors from goroutines; owdSchemaOnce process cache; inventory cheapest_registrar null without detail rows; intersect --taken-on page cap semantics; spurious quota warning.
- Security: user input concatenated into request paths in owdCheckDomain/owdFetchTLDDetail (medium; reachable via recheck --file and brainstorm --tld); raw DomainsGPT client without redirect guard; unscrubbed error body; --file exposed over MCP (mitigated by charset validation).
- Maintainability: duplicated fan-out/precheck/index/normalise/window-parse blocks, two dedupers, two word-file readers, magic numbers (100, 4, dogfood caps, 93), inconsistent fetch_failures shape and popularity precision, dead fields, stale comments, DATETIME scanning three ways, test gaps (owdRecordListings, 500 to exit 3, DomainsGPT httptest).

## Template-shape retro candidates (not patched in place)
- internal/cli/auth.go: the Chrome cookie-DB probe copies -wal/-shm siblings with default perms into the system temp dir (os.Create); copy into an os.MkdirTemp 0700 dir or open with O_EXCL 0600.
- internal/client/client.go: JSON response bodies are read with an unbounded io.ReadAll; cap with a LimitReader.
- internal/cli/helpers.go boundCtx is documented as the request timeout but is the only ctx helper offered to hand-authored commands, inviting whole-loop caps on multi-request commands; provide a per-request helper or guidance.
- internal/cli/auto_refresh.go readCommandResources over-declares resource x verb paths (domains get/list, gpt get/list/search, listings-filters*, tlds search, words search) that do not exist; README/SKILL Freshness sections render them (pruned by hand in this run).
- Generator dead helpers (handleBinaryResponseDelivery, retainCLIQueryParams, retainExplicitQueryParams, successfulNoop) and the config write/read field regex mismatch reported by dogfood on every run.
- Scorecard auth_protocol scores cookie-auth CLIs 2/10 (no scheme prefix to match); the scorecard live probe strips the <API>_CONFIG override so session-gated novel commands cannot be sampled; the umbrella probe timeout (10 s) is not configurable from shipcheck.
- The generated JSON client rejects the DomainsGPT concatenated-object stream (a hand-built gpt generate was needed); the syncer treats every resource as single-page when the spec declares no pagination; generate --force re-emits TODO scaffolds for research.json novel features that already have hand-built implementations under other constructor names (alias file needed); spec descriptions containing periods (for example 1.38M or e.g.) are truncated in the SKILL command reference and root Short; auth login --chrome scans Google Chrome channels only (the user was signed in via Brave).

## Out-of-scope (generator-reserved) retro candidates
- internal/mcp/cobratree mirrors local-file-path flags (--file, --cookies-file) into MCP tool schemas with no file-access policy.
- internal/cliutil: nothing found.

## Autofix summary
- Round 1: 10 correctness + 4 security + 38 maintainability findings autofixed in place by one worker pass (no git in the working tree; the changed files are listed in the build log). Extra bug found by the new watch end-to-end test: the generated GET cache hid changes between watch runs; fixed with NoCache on that command.
- Round 2 re-review: 9/10 correctness and 3/3 security round-1 items confirmed closed; residuals and new findings (page-capped watch scans deleting rows, tlds drift --registrars no-op, dogfood cap bypass on 0, DomainsGPT request attaching credentials to any base-URL host, file-line errors echoing content, no check --max-checks, unscrubbed table cells, one error-mapping path, naming/placement splits) autofixed by a second worker pass.
- Round 3 re-review: every round-2 item confirmed closed; survivors were one medium (typed errors raised outside owdAPIErr skip the JSON envelope), two low security items (credential masking in DomainsGPT error bodies; token-shaped file lines), and dead double wiring plus small tidy-ups. These were fixed in a targeted post-round pass with unit tests instead of a fourth full review (round cap reached).

## Surface-to-user findings
- None required a product decision; no approved feature was descoped.

## Convergence outcome
- In-scope findings cleared at round 3 plus one targeted post-round fix pass (see the post-round worker report in the build log); template-shape and generator-reserved items filed above for retro.

## Review path chosen
- Direct subagent dispatch: correctness, security, maintainability reviewers, three rounds, followed by a Claude Code /simplify pass over the hand-authored files.

## /simplify pass (post-convergence)
Four cleanup reviewers (reuse, simplification, efficiency, altitude) ran over the hand-authored set; one worker applied the deduped worklist (groups G1-G11).
- Schema moved into store extras hook (migrateExtras); EnsureOWDSchema removed; words.slug index added.
- N+1 removed: batched dictionary lookups, batched latest-check window query, one transaction per snapshot writer.
- Domain-check fan-out consolidated (owdChecksByDomain); single snapshot timestamp per run.
- Dead double error mapping removed; owdSourceErr derives from command metadata; GPT request validation shared; config loaded once per gpt command.
- Skipped: test-fixture reuse of generated runRootArgs, words-paging unification (behavior change), fan-out parallelism, trivial double index.
Gate: gofmt clean, build/vet ok, go test ./... ok, TestOwd|TestNovel 100 pass.
