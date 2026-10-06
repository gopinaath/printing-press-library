---
date: 2026-10-02
target_cli: game-goat-pp-cli
amend_run_id: amend-2026-10-02T2130
deferred_count: 1
---

## F9 — thin demo view over Steam
- category: add-command
- classification: feature
- rationale: "free demos on Steam right now" filterable by title, tag/genre and release, plus a demo-availability annotation on existing game output (ratings/games get).
- evidence: "(e) [DEFERRED — do not build in this run] The thin demo view: \"free demos on Steam right now\", filterable by title, tag/genre and release, plus demo-availability annotation on existing game output."
- reason-deferred: user chose A-then-B sequencing; this run is tier A only.
- implementation hint: build on `steam browse --type demo --free --tag ...` (IStoreQueryService/Query) and `StoreItem.DemoAppIDs` / `ParentAppID` from IStoreBrowseService/GetItems.
- still_relevant: unknown
