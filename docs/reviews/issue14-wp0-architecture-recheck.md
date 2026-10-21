# Issue #14 WP0 second-context architecture recheck

- **Reviewer context:** fresh independent subagent (`delegate`), model
  `opencode-go/deepseek-v4.1-flash`, read-only.
- **Reviewed:** `docs/design/issue14-recovery-protocol.md` + implementation @
  `a057fbc`. The subsequent P2 fixes at `09612f8` (per-receipt reconciliation,
  BUSY/LOCKED classification, extra tests, doc order) are covered by the local
  test suites and the second-context security recheck.
- **Verdict:** **PASS — no P0/P1.**

## P1 closure verified

- **P1-1:** `verifyRecoveryRow` classifies a prefix row by equality with the
  recomputed replacement; the write path refuses a fixed-point non-NULL
  replacement before commit. Doc §5 matches code.
- **P1-2:** the progress INSERT and manifest CAS share the business transaction;
  `RowsAffected != 1` rolls back with `CONFLICT`. Resume requires a contiguous
  `1..N` log and `manifest == max(progress)`, plus receipt cross-checks.
- **P1-3:** `InitializeRun` strictly precedes `CreateManifest`; the manifest has
  an FK to the outbox run; a run without a manifest is reported as not
  recoverable, never `new` or resumable.

## Additional controls verified

Same-transaction UPDATE+receipt+progress+CAS; CAS-guarded terminal states; KCV
key binding before any write; recomputable-strategy allowlist; bounded pages;
multi-table and all-NULL handling; dense independent batch sequence; read-only
`status`; legacy outbox-only behavior unchanged; `pkg/contracts` untouched.

## Remaining P2 (accepted / addressed)

1. Per-receipt reconciliation was aggregate-only → now reconciles every batch
   event with the same-ordinal receipt (id, rows, run-scoped batch id).
2. Doc overstated "verify all tables before any write" → now states that later
   unvisited tables are verified page-by-page immediately before each write.
3. Gate order in the doc differed from code → doc §9 now matches the code order.
4. Dead `unmasked` verification branch → remains as a defensive/unreachable
   branch; no safety impact (accepted).
5. `SQLITE_BUSY/LOCKED` mapped to `ErrDatabase` → now mapped to `ErrConflict`.
6. `validation_failed` re-validation wording → doc now states it is terminal for
   resume.
7. Evidence gaps: manifest-ahead-of-log test and manifest-state CAS stale test
   were added; the fixed-point guard is proven at unit level; scale evidence is
   opt-in and recorded in the acceptance report.
