# PR #71 Independent Architecture Review — head 49434f3

- **Reviewed head:** `49434f3d33cf55f97970e544517d1f08b656f5bb`, task-supplied SHA; reviewer watchdog confirmed checkout based on that exact SHA and empty working-tree diff.
- **Scope/limits:** current design and relevant source; full committed diff from base unavailable to reviewer tools. No commands/tests, database probe, CI query, or GitHub fetch. No production change was examined because the supplied change list was documentation-only.
- **Verdict:** **P0=0, P1=0, P2=1.**

## P2 — COMMIT_ATTEMPTED row permits querying before run stop

**Location:** ADR §“P1-1 state machine and failure table”, line 134 on reviewed head.

The table states verifier outcomes are only assessed after execution stops, but the active “Underlying Commit attempt in progress” row says `UNKNOWN if queried before authoritative resolution`. That is inconsistent with the table's own rule that a live run is `N/A`, and it improperly describes a verifier query while execution is not stopped.

**Correction:** Describe the injection as a stopped/interrupted commit attempt whose outcome remains unresolved after authoritative recovery; keep the last Runner state `COMMIT_ATTEMPTED`; Verifier outcome `UNKNOWN` after stop/readback.

## Confirmed corrections

The preceding issue has `COMPLETE_VALID` require `seal.outcome == COMPLETE`; outcome precedence is explicit; the failure matrix distinguishes INCOMPLETE prefixes from catalog-backed missing data. BatchPrepare/ack is inside each loop iteration; event identity differs from batch identity with role-scoped uniqueness; ADR-11 is an implementation prerequisite. Original live AC is quoted with provenance and no API/heap-only scope is silently inferred. Target storage/durability remains a clearly stated implementation gate.

Full reviewer transcript: `/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/270e21a6-869a-4916-85b3-820919bda728_reviewer_output.md` (run ID `270e21a6-869a-4916-85b3-820919bda728`).

This is design review only. No implementation, durability proof or Issue #13 AC completion is claimed.
