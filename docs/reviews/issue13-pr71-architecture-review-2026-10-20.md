# PR #71 Independent Architecture Review — 2026-10-20

- **Reviewed SHA:** `c44a7c38785b07cfe4e5f18d0e85c2ba7a282c9a`, verified from worktree `.git`/`HEAD`; watchdog reported no worktree changes at that review point.
- **Scope:** Current design/source review of bounded receipts, ADR, review history, contracts, performance/test-plan/architecture/feature-matrix docs and related Runner/DB/History/Verify/CLI/tests. No committed base-to-head diff artifact; this was not a full PR diff/source ownership verification. The supervisor supplied Issue #13's active AC excerpt; reviewer could not independently fetch GitHub or run git commands.
- **Verdict:** **P0=0, P1=0, P2=3.** Proposed disposition: `DESIGN READY — PENDING OWNER APPROVAL` on that reviewed design, not production readiness. No current-head CI/outbox probe/tests/fault injection were independently verified.
- **AC scope:** correctly distinguished S1 all-API, S2 new path only, S3 cap; global Issue wording remains authoritative pending Owner decision. Legacy full-receipt result remains Ω(K). All ADR-01..10 were pending on reviewed head; ADR-11 was added afterward.
- **A3 direction:** feasible in principle only if data and outbox share one DB transaction. Existing frozen `WriteTx` has Update/Commit/Rollback only; concrete transaction is private, SQLite target durability probes are missing, MySQL A3 and Runner configuration are not established. A3 needs additive reviewed DB/contract capability and real adapter/workflow evidence.
- **Other validated design constraints:** explicit schema effects, CommitUnknown no retry, no hard cancellation deadline for stuck I/O, O(H)/L/T/C/D/R resource terms, old API O(K), Plan/Approval/APIRevision compatibility, and #14 exclusion are represented accurately. Runtime/apply data working-set is conditional, not a proven process RSS bound.

## P2 findings

1. **Event storage model ambiguous** (`ADR:79–81,135` at reviewed head): permitted in-place PREPARED→COMMITTED conflicts with append-only grants; batch ID uniqueness not distinguished from event identity. **Fix:** append-only only, event ID distinct from batch ID, explicit unique constraints, and backend-specific grant limits.
2. **Hash-chain rewrite test missing anchoring precondition** (`ADR:91–106`, failure row 12): local history rewritten before first external commitment cannot be rejected by a not-yet-existing trusted digest. **Fix:** condition the test on already anchored enrollment/events; document pre-anchor limits and trusted submission assumptions.
3. **Historical implementation guidance remains current-looking** (`bounded-receipts-design.md:188,194–197,229`): caller Sink, later outbox, global API revision bump conflict with ADR. **Fix:** remove/mark stale and align with internal outbox/additive version proposal.

## Historical Security P1 dispositions (as architecture reviewer assessed)

- P1-1 commit→receipt gap: resolves at design level conditionally under same-transaction outbox; implementation/durability unverified.
- P1-2 seal/anchor: partly resolves; anchor assumptions and timing need explicit clarification; no implementation evidence.
- P1-3 arbitrary consumer: resolves at design level by excluding callbacks; runtime cancellation unverified.
- P1-4 separate sink path: separate path is N/A after A3; target DB path/security prerequisites remain unproven.

This review does not declare tested production fixes or close any production-readiness finding.

## Evidence and limits

- Commands/tests executed: none; read-only inspection.
- Current head verified; no base-to-head diff, live GitHub Issue check, CI query, database probe, anchor implementation, or native platform test.
- The external original Issue body was not independently fetched; supervisor-supplied exact AC excerpt was used and the reviewer explicitly retained scope as unverified outside that excerpt.
- Full original transcript: `/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/eb3ffd95-119b-4422-aa19-858295b23d5b_reviewer_output.md` (run ID `eb3ffd95-119b-4422-aa19-858295b23d5b`).

This report applies only to `c44a7c3`; subsequent documentation changes require review at their new exact SHA. It does not authorize implementation, mean Issue #13 AC passes, close #13, start #14, or supersede Security Review.
