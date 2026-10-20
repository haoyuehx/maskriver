# PR #71 Independent Security Design Review — head 46b2cf1

- **Reviewed SHA:** `46b2cf1127905eeeda6d3b2c4a953e85d6980f87`, supplied by Main; reviewer could not independently verify Git.
- **Verdict:** **P0=0, P1=0, P2=3.** Suitable for Owner consideration at design level only; not implementation readiness or Issue #13 acceptance.
- **Scope/evidence:** read AGENTS, architecture/test-plan/contracts/feature-matrix, both design docs, review history and relevant contracts/runner/DB/history/config/CLI source. No shell/Git/tests/DB probes/crash injection/CI/GitHub query. No diff artifact for this review. Main stated `git diff --check` PASS and CI pending; neither is implementation proof.

## Prior four Security P1 dispositions

| Finding | Classification on this head | Limits |
|---|---|---|
| P1-1 Commit→durable receipt window | **RESOLVED at design level only** | Same-target transaction requires business update + `BatchCommitted`; code still appends an in-memory report after Commit. Durability/crash behavior not implemented/tested. |
| P1-2 envelope/seal/trusted history | **RESOLVED at design level only within ADR-11 scope** | Independent pre-write enrollment, enumerable catalog, per-batch trusted-runner commit attestation, terminal seal and no post-restart reconstruction close the first-submission/deleted-known-run argument within the stated trust boundary. Owner has not approved this narrowed scope. |
| P1-3 arbitrary Consumer block/cancel | **RESOLVED at design level only** | No arbitrary callbacks/channels/detached goroutine; synchronous internal outbox, no hard deadline for stuck driver/kernel. Runtime unverified. |
| P1-4 independent receipt path | **PARTIALLY RESOLVED** | Separate receipt path removed. Target SQLite safe-open/path identity/ACL/durability remain explicit but unimplemented launch gates. |

## P2 findings

1. **Durability/platform prerequisites remain non-executable** (ADR §A3, filesystem boundary): exact SQLite sync settings/drift checks, MySQL authoritative-primary/durability checks, and OS path identity/ACL matrix deferred. Existing stat-then-open and SameFile are point-in-time only. Require supported OS/filesystem/backend matrix, fail-closed probes and native race/durability tests before implementation authorization.
2. **Sequence diagram moves BatchPrepare outside the loop** (`docs/design/issue13-bounded-receipts-design.md` lines 118–120 on this head), contrary to ADR's per-batch prepare. Move it inside the loop before each data transaction.
3. **Implementation authorization checklist omits ADR-11** (`Design` line 198): require ADR-01..11, including trust boundary/catalog/enrollment, before separate implementation authorization.

## Trust assessment and remaining boundaries

No new P1 was found within the explicitly narrowed threat model. Run enrollment and independent catalog plus trusted live-process attestation address first-submission and disappeared-run risks; after Commit but before anchor acknowledgment, the batch may be committed while the run remains incomplete/unknown. The process must stop before a later business batch and cannot reconstruct the missing attestation after restart. This intentionally trades availability for trust.

Assumptions explicitly exclude never-enrolled runs, pre-enrollment rewrite, host-root/Runner process-identity compromise, or anchor rollback/compromise. The anchor certifies audit history, not that current business rows remain unchanged. These exclusions require Owner approval. MySQL outbox/CLI workflow remains unverified. Existing outbox schema authorization, approval binding, PII/metadata exposure and target DB security are future gates.

**Original P1 disposition must not be described as tested production fixes.** Current code has no A3/anchor implementation; current m1a remains unchanged; no Issue AC is met by this design review.

Full reviewer output artifact: `/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/544ea5bb-509e-4dc5-a527-2175d8485e56_go-reviewer_output.md` (run ID `544ea5bb-509e-4dc5-a527-2175d8485e56`).
