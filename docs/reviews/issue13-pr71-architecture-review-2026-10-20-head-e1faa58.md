# PR #71 Independent Architecture Review — head e1faa58

- **Reviewed SHA:** `e1faa582ca5fa2d07478f081418c968101049f97` as supplied; reviewer could verify only the abbreviated checkout HEAD `e1faa582ca5f…`, and reported a clean worktree.
- **Scope:** current design/source review; both requested `/tmp` committed-diff artifacts were unavailable to this reviewer role, so no independent full diff/change-list verification. Read relevant docs/source and tests; no commands/tests/GitHub query/DB probe.
- **Verdict:** **P0=0, P1=0, P2=1.** Corrected `COMPLETE_VALID` outcome requirement, event sequencing/uniqueness, ADR-11 prerequisite, and Issue AC scope were accepted. No implementation or AC pass follows.

## P2 — Failure-matrix status vocabulary conflates execution state and verifier outcome

**Location:** ADR §“P1-1 state machine and failure table”, reviewed lines 129–136.

Runner report phrases such as “Failed before commit”, `AUDIT_INCOMPLETE / UNKNOWN`, and “incomplete/UNKNOWN” were not consistently mapped to the declared execution states or the separate one-of-six verifier outcomes. Slash combinations leave unclear whether one or both statuses should be emitted.

**Correction:** distinguish Runner execution/report state from post-stop verifier outcome; use the declared tokens, map each specified injection to exactly one verifier result after recovery/readback, and use N/A only when no stopped run exists. Do not combine verifier categories with slash alternatives.

Full reviewer transcript: `/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/1fa9df07-f643-4ba1-a7ee-ced25b6177a6_reviewer_output.md` (run ID `1fa9df07-f643-4ba1-a7ee-ced25b6177a6`).

## Correctness and boundaries

- `COMPLETE_VALID` now requires `seal.outcome == COMPLETE`; outcome precedence and failure row 9 are explicit.
- `BatchPrepare` and its anchor acknowledgment are inside each per-batch loop; event ID and batch ID differ; batch role uniqueness permits one prepare and one terminal result.
- Owner checklist includes ADR-01..11 and trust scope. Live Issue AC is quoted and S2/heap-only interpretation remains Owner decision.
- No production files changed, no code tests or DB/anchor/durability probes run. CI evidence was supplied by Main, not independently queried.

A P2-only review is not approval of implementation, API changes, Issue #13 AC, or merge. Corrections require a new exact-head review.
