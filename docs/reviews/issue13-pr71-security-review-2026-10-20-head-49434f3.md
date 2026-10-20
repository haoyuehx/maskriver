# PR #71 Independent Security Design Review — head 49434f3

- **Reviewed head:** `49434f3d33cf55f97970e544517d1f08b656f5bb`, SHA supplied by Main. The complete base-to-head diff was unavailable to reviewer tools; the review covered current ADR/design/history/reports and relevant contracts/runner/DB source.
- **Verdict:** **P0=0, P1=0, P2=1.** No tests, DB/anchor/crash probes, or CI query were run. This is design review only.

## Four prior Security P1 dispositions

| Finding | Classification | Limit |
|---|---|---|
| Commit→durable receipt gap | **RESOLVED at design level only** | Same-target transaction requires business update and `BatchCommitted`; current code still has no outbox implementation or transaction probe. |
| Trust anchor/run discovery | **RESOLVED within ADR-11 scope at design level only** | Independent enrollment/catalog and trusted-runner per-batch attestations; excludes pre-enrollment rewrite, host/Runner compromise and anchor rollback. Owner approval remains pending. |
| Arbitrary Consumer blocking/cancellation | **RESOLVED at design level only** | Arbitrary callbacks/channels removed; synchronous internal outbox. Runtime and permanently stuck I/O behavior unverified; no hard-exit guarantee. |
| Independent receipt-file path | **PARTIALLY RESOLVED** | Separate path removed; target DB path identity, permissions and durability remain implementation gates. |

## P2 — target storage/path/durability support remains unproven

**Location:** ADR A3 requirements and persistent storage/file-system boundary. The proposed backend matrix and fail-closed checks remain to be operationalized. Existing SQLite stat-then-open is not path pinning; MySQL outbox/CLI support has no evidence. Before implementation or support claims, define the exact OS/filesystem/backend durability matrix, implement fail-closed preflight, and test path races and SQLite/MySQL durability/failure behavior.

## Other boundaries

`COMPLETE_VALID` requires an exact COMPLETE seal outcome; verifier precedence is defined. A3 requires same-transaction outbox; external anchor failures stop next writes and cannot trigger restart reconstruction or business retry. Approval policy/DTO compatibility remains Owner/contract review; metadata leakage is acknowledged. #14 recovery/replay is not authorized. No implementation or Issue #13 AC pass is claimed.

Full reviewer transcript: `/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/72a66e80-350f-4d8a-8865-cfe5caf17e5b_go-reviewer_output.md` (run ID `72a66e80-350f-4d8a-8865-cfe5caf17e5b`).
