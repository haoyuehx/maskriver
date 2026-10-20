# PR #71 Independent Security Design Review — head e1faa58

- **Reviewed SHA:** `e1faa582ca5fa2d07478f081418c968101049f97`, supplied by Main; reviewer could not independently verify Git.
- **Scope:** current design/source review of ADR/design/review docs and relevant contracts/Runner/DB/History/CLI. The requested `/tmp` diff was inaccessible under repo-only reviewer tools, so the complete committed diff/file list was not independently checked. No tests, commands, DB probes, anchor tests, crash injection, CI/GitHub query, or performance measurement were run.
- **Verdict:** **P0=0, P1=0, P2=1.** This is design only; no production implementation or Issue #13 acceptance.

## Four original Security P1 dispositions

| Finding | Classification at this head | Limit |
|---|---|---|
| Commit→durable receipt window | **RESOLVED at design level** | Same target transaction requires business update and `BatchCommitted`; current implementation still appends an in-memory report after Commit. No durability/crash test. |
| Envelope/seal/trusted anchor and full-run discovery | **RESOLVED at design level within ADR-11** | Independent registration/catalog and per-batch trusted Runner attestations, plus no post-restart reconstruction. Excludes pre-enrollment rewrite, compromised Runner/host, and anchor rollback; Owner has not approved scope. |
| Arbitrary Consumer block/cancel | **RESOLVED at design level** | Arbitrary callback/channel removed; internal synchronous outbox; no hard-exit promise for stuck driver/kernel. Runtime unverified. |
| Separate receipt-path filesystem safety | **PARTIALLY RESOLVED** | No independent receipt path; target SQLite identity/TOCTOU/permissions/durability remain unimplemented gates. |

## P2 — durability/path gates are not executable or proven

**Location:** ADR §A3 and §“persistent storage/file-system boundary”.

Before implementation or platform-support claims, specify supported backend/OS/filesystem matrix, exact durability/preflight checks, and fail-closed behavior. Existing SQLite `Stat` then pathname open does not prove path pinning; MySQL outbox/CLI capability is not established. Require native path-race and SQLite/MySQL durability fault tests. This remains an implementation gate and is not misrepresented as a completed protection.

## Other assessed boundaries

- Verification outcome precedence now prevents an INCOMPLETE seal from returning COMPLETE_VALID; no implementation proves the design.
- Same-DB/same-transaction outbox is required; external anchor is a separate failure domain. Missing batch acknowledgment stops later writes; no post-restart attestation reconstruction or business retry.
- Append-only event roles distinguish event and batch identity; backend DDL/grants remain future work.
- Existing Apply authorization, signed approval and Plan binding must remain; audit policy binding and new DTO need Owner/contract review.
- No raw values, replacements, keys or DSNs in audit event design; table names/stable digests may still expose metadata.
- Issue #14 checkpoint/resume/exactly-once is out of scope.

**Status:** no new P1 within the narrow ADR-11 threat model, but Owner scope approval and durability/path implementation evidence remain outstanding. The review does not authorize implementation or merge and does not establish #13 AC.

Full reviewer transcript: `/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/28be4513-7402-4fde-a246-93e02b1afcc6_go-reviewer_output.md` (run ID `28be4513-7402-4fde-a246-93e02b1afcc6`).
