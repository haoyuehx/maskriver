# Issue #14 WP0 architecture review (independent context)

- **Reviewer context:** fresh independent subagent (`delegate`), model
  `opencode-go/deepseek-v4.1-flash`, read-only.
- **Reviewed:** `docs/design/issue14-recovery-protocol.md` @ `2251823`.
- **Baseline:** `main` `aee0b801c738b89a6267e06c575c95878f785c7f`.
- **Verdict:** **CHANGES REQUIRED** (no P0; three P1).

## Verified as deliverable

- The same-transaction seam exists in `internal/db/outbox.go` and one commit is
  performed; the progress INSERT + manifest CAS can be added there.
- `pageChanges` has no mapping-store seam, so the commit-gap premise holds.
- Source snapshot digest semantics are inherited from the existing Plan builder.
- Commit-unknown reconciliation from the authoritative DB is sound because the
  progress row shares the business transaction.
- Outbox v1 DDL/data and `m1a-v1`/`m2-stream-v1` remain untouched.
- Bounded receipt paging already exists and the progress paging follows it.

## P1 findings

1. **Prefix rule vs fixed-point replacement** (`§5`). Requiring a committed
   non-NULL target to "differ from source" could misclassify a legitimate
   fixed-point replacement as a conflict. Fix: verify equality with the
   recomputed replacement; the apply path already refuses unchanged non-NULL
   replacements.
2. **CAS rowcount and manifest/log agreement** (`§4`, `§9`, `§10`). The CAS must
   affect exactly one row, and resume must require
   `manifest.checkpoint_sequence == max(progress)` plus a contiguous log.
3. **Metadata creation crash window** (`§11`). Freeze the order
   `InitializeRun` → `CreateManifest`, note the FK, and state that a manifest
   without its outbox run row is impossible/not recoverable.

## P2 findings

- Freeze `rows_processed` as cumulative-since-boundary.
- Define a no-op page as zero pending `RowChange`s; mixed pages use
  `kind=batch` with `affected_rows < rows_processed`.
- Freeze `batch_sequence` as dense and independent of `checkpoint_sequence`;
  no receipts for progress/boundary events.
- A wrong-shape/partial recovery schema must fail closed during ordinary apply.
- State the approval re-issue semantics (plan content binding, not record id).
- State that recovery verification uses the same bounded reader/budget.
- Add the source-digest TOCTOU caveat.
- Add a completeness rule (`rows_processed <= source rows`; final boundary count).
- Freeze the DDL constraints, not only columns.

## P1 disposition

All three P1 items were applied to the design and implementation before the
implementation PR. See `issue14-wp0-architecture-recheck.md`.
