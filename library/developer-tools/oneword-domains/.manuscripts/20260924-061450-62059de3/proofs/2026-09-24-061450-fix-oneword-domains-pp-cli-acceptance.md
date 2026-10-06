# Acceptance Report: oneword-domains

Level: Full Dogfood (live, signed-in session via config override)
Tests: 169/169 passed (123 runner-standard skips: no positional fixture, mutating commands without --allow-destructive, non-id positionals)
Novel features: 12/12 passed a live happy path (brainstorm, recheck, listings rank, domains intersect, words mine, tlds inventory, check, compare, hacks, tlds drift, listings watch, gpt generate)
DomainsGPT spend: 8 generations for the matrix runs (user approved the spend)

## Loop 1 failures (14) and fixes
- CLI fix: pp:happy-args on brainstorm, gpt generate, check, words mine, domains intersect, listings rank used spaces; runner splits on semicolons. Rewritten with semicolons.
- CLI fix: tlds get returned exit 0 with an empty shell for an unknown TLD (site answers 200). Added owdTLDDetailMissing; tlds get now exits 2 with the unknown-TLD error and JSON envelope.
- CLI fix: doctor, sync, workflow archive --dry-run --json lacked dry_run:true and action; added both.
- CLI fix: feedback list, profile list --dry-run --json printed a bare array; now emit the standard dry-run object.

## Printing Press issues (retro)
- Generator emits space-separated pp:happy-args for spec-derived endpoint mirrors (domains search had "--tld=ai --sort=popularityDesc"), but the runner splits on semicolons; the malformed value passed silently.
- Framework templates (doctor, sync, workflow archive, feedback list, profile list) fail the runner's own dry-run JSON checks on a fresh print.
- Generated endpoint mirrors have no hook for "200 with empty body means not found" APIs.

Gate: PASS
