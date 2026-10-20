# PR #71 Independent Security Design Review — head 45f754d

- **Reviewed head:** `45f754d47e753ef4798050feb6a961b90bfa4687`, Main-supplied SHA; reviewer could not independently verify Git HEAD or read complete base diff. Read current ADR/design/history and relevant source.
- **Verdict:** **P0=0, P1=0, P2=1; Merge verdict OK with notes**, for design documentation only.
- **Limits:** No tests, shell commands, CI/GitHub query, DB/anchor/crash probe or implementation validation.

## Four historical Security P1 dispositions

1. Commit→receipt gap: **resolved at design level only** (same target DB transaction required); current code remains in-memory receipts appended after Commit, no outbox/test.
2. Trust anchor/run discovery: **resolved within ADR-11 scope at design level only**; pre-enrollment rewrite, Runner/host compromise and anchor rollback excluded and Owner approval pending.
3. Arbitrary Consumer blocking: **resolved at design level only**; synchronous internal outbox/no callback, but stuck driver/kernel I/O has no hard-exit proof.
4. Separate receipt path: **partly resolved**; independent file removed; target DB path identity, permissions, platform and durability remain implementation gates.

## P2 — target storage/path/durability

ADR `:71–80,159–175`: exact SQLite/MySQL, OS/filesystem support matrix, fail-closed path/config preflight and crash/durability testing are not implemented or demonstrated. Before implementation/support claims, define supported configurations, implement preflight and verify path replacement, permissions, disk/commit faults and SQLite/MySQL durability. Same-transaction atomicity alone is not power-loss durability.

The current status `DESIGN READY — PENDING OWNER APPROVAL` is correctly limited; no Owner approval, production authorization or Issue #13 AC pass is claimed. The corrected Commit/verifier matrix distinguishes live state from post-stop/readback UNKNOWN. Explicit write/approval gates, no retry/reconstruction, privacy caveats and #14 boundary remain explicit. Legacy receipt API is O(K).

Full reviewer transcript: `/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/518239d3-ce5c-4372-b848-c12dc379ed2d_go-reviewer_output.md` (run ID `518239d3-ce5c-4372-b848-c12dc379ed2d`).
