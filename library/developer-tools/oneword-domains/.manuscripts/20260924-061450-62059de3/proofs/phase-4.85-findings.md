# Phase 4.85 agentic output review findings (Wave B: warnings)

Status: WARN, 3 findings, all fixed in-session.

1. `words mine --category positive,tech` (documented example) always returned `[]` because multi-category is ALL-of and no word carries both tags; the hint blamed dictionary size. Fixed: examples now use `adjectives,positive` (a real pairing) and show `--any`; a zero-row ALL-mode result prints a stderr hint naming `--any`.
2. `listings watch` reported months-old auctions as live `new` rows and `ending_soon` could never fire against the stale upstream feed. Fixed: the result carries an `expired` count and a stderr note when every listing's end date has passed.
3. `compare` (and `check`) human mode used the generic card renderer, hiding false booleans and reordering fields. Fixed: fixed-column table (domain, available, premium, price, popularity, min price, registrar, note).

Samples that checked out: compare (identical tldCount for two words is genuine upstream data), recheck (empty on a fresh store, 16/16 unchanged later), hacks --tld art, no HTML entities or mojibake in any sample.
