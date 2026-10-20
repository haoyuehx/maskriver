# Issue #13 WP3 — independent Architecture review record

**Scope:** PR #74, read-only. **Reviewer context:** independent AI reviewer, not the implementer.
**Reviewed heads:** `24c1b49d994137434fc3f520580d433e92389856`, `af5ebf4d5d692671967e5638bfd172318f25f70a`
(base `97e95c8be712ad12491bdfe7ada1cea41f85eec7`).
**Evidence:** supplied sanitized diffs plus repository sources. No tests were run by this reviewer.

## Verdicts

| Head | P0 | P1 | P2 | Verdict |
|---|---|---|---|---|
| `24c1b49` | 0 | 0 | 1 | OK with notes — legacy directory bound changed |
| `af5ebf4` | 0 | 0 | 0 | OK |

## Finding and disposition

- **P2 — legacy history behaviour change.** The first version routed the legacy ambiguity check
  through the new 4096-entry bound. Fixed: the legacy path uses chunked, constant-memory reading
  with no entry/name cap; only the streaming lookup enforces `MaxEntries`/`MaxNameBytes`.
  Convergence review confirmed the streaming bound is not weakened.

## Confirmed at the reviewed code

- `pkg/contracts` untouched; legacy Plan/approval/Apply and defaults unchanged; no #14 semantics.
- One separate streaming approval record and lookup; legacy and streaming records are disjoint by
  suffix and binding revision.
- Canonical plan-content digest covers table/PK/column/type/Scope/NormalizationVersion/order.
- `InitializeRun` occurs only after approval, live preflight and full dry run; preview creates no
  run.
- Existing worker pool reused; one serialized commit per batch through the WP2 outbox; no receipt
  accumulation; summary and issue aggregation are bounded.
- Reserved outbox tables are refused in scan/plan/streaming scopes.
- Scale document labels the 1M cell as a single sample and does not claim RSS/p95 or production
  throughput.

## Limits

Reviews are code/docs inspection only. They do not establish MySQL/Windows runtime behaviour,
power-loss durability, TOCTOU-proof binding, or that the experimental budget is approved.
