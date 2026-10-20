# PR #71 Independent Security Design Review — head 240ce83

- **Reviewed head:** `240ce839a2a36ebb703ab4082386eb3d163fa8ab`, supplied by Main; reviewer could not independently verify Git or access the full base-to-head diff.
- **Scope:** current ADR/design/review history and reports plus related architecture, test guidance and contracts/Runner/DB source. No tests, DB/anchor/crash probes, CI query or GitHub query.
- **Verdict:** **P0=0, P1=0, P2=1; Merge verdict OK with notes** for design-only review.

## Original four Security P1 dispositions

| Finding | Classification | Limit |
|---|---|---|
| Commit→receipt crash gap | **RESOLVED at design level only** | Requires business update + `BatchCommitted` in same target transaction. No implementation/transaction test. |
| Envelope/seal/anchor/run discovery | **RESOLVED within ADR-11 scope at design level only** | Independent registration/catalog and per-batch attestation. Pre-enrollment rewrite, host/Runner compromise and anchor rollback are excluded; Owner approval pending. |
| Arbitrary Consumer blocking | **RESOLVED at design level only** | No callback/channel; synchronous internal outbox. Runtime cancellation and hard exit for stuck I/O remain unverified. |
| Separate receipt-file path | **PARTIALLY RESOLVED** | No independent path; target DB path identity/ACL/durability remain unimplemented gates. |

## P2 — target storage and durability remain implementation gates

**Location:** ADR §§A3 and persistent storage/filesystem boundary (`:71–80,159–175`). Before implementation or claims of crash-safe/tamper-evident SQLite/MySQL audit, define OS/filesystem/backend support and fail-closed checks; test path replacement, permissions and durability. Existing SQLite stat-then-open is not path pinning; MySQL outbox/CLI evidence is absent. Same-transaction atomicity alone does not prove power-loss durability.

## Additional review

The Commit/verifier table now separates live Runner state from post-stop verification; unresolved attempts after stop/readback yield UNKNOWN. Preview/write authorization, no retry/reconstruction, metadata disclosure and #14 boundary are explicit. No code or Issue #13 acceptance is verified.

Full reviewer transcript: `/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/acdf15a5-8f39-4b18-b786-52d789e459e3_go-reviewer_output.md` (run ID `acdf15a5-8f39-4b18-b786-52d789e459e3`).
