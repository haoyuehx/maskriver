# Issue #13 WP2 — SQLite transactional receipt outbox

**Status:** `WP2 IMPLEMENTED — PENDING INDEPENDENT REVIEW AND MERGE`.
**Scope:** an explicit, opt-in SQLite database capability only. It is not wired into the Runner or
CLI, it is not a complete Streaming Apply, and it does not satisfy the Issue #13 memory acceptance
criterion by itself.

- Baseline `main`: `24877565e39c312c59a628cf0f054fe33dc79b8e` (WP1 merged).
- Contract dependency: `pkg/contracts/streaming.go` (`m2-stream-v1`), `APIRevision=m1a-v1` unchanged.
- Decision record: `docs/design/issue13-owner-direction-2026-10-20.md`.

## 1. What the capability is

`internal/db` gains an explicit streaming outbox:

```go
type OutboxOptions struct {
    contracts.DatabaseOptions
    Protocol       string // must be contracts.StreamingProtocolRevision
    JournalMode    string // only "delete" in this version
    MaxReceiptPage int    // > 0; no unbounded default
}

outbox, err := db.OpenOutbox(ctx, opts)            // verifies path, PRAGMAs, schema
err = outbox.InitializeRun(ctx, db.OutboxRun{...}) // separate autocommit statement
tx, err := outbox.BeginBatch(ctx, runID, plan, receipt)
n, err := tx.Update(ctx, changes)                  // existing write contract
err = tx.Commit(ctx)                               // INSERT receipt + one Commit
reader, err := outbox.Reader(runID)                // bounded page reader
page, err := reader.Page(ctx, limit)
```

`BeginBatch` returns a `contracts.WriteTx`; `Commit` inserts the receipt in the same transaction
and then performs the only `Commit`. `Update`, `Rollback` and error categories reuse the existing
write contract, so WP2 does not redefine the business-write DTOs or add a second receipt model.

The legacy path is untouched: `OpenWriter`, `WriteTx`, `ApplyReport` and all `m1a-v1` semantics are
unchanged and never require the outbox.

## 2. Schema (pre-provisioned, versioned)

Migration: `internal/db/outbox_schema_v1.sql`, applied only by
`db.InstallOutboxSchema` (explicit call, one transaction, no DDL at apply time).

| Table | Purpose | Key constraints |
|---|---|---|
| `maskriver_outbox_schema` | schema version marker | single-row contract; mismatch refused |
| `maskriver_outbox_run` | run identity (IDs/digests/timestamps only) | 32-hex run ID, bounded label lengths, PK `run_id` |
| `maskriver_outbox_batch` | one receipt per committed batch | PK `(run_id,batch_sequence)`, `UNIQUE(run_id,batch_id)`, `CHECK(outcome=1)`, `CHECK(receipt_version='m2-stream-v1')`, FK to run |

Reserved names are exposed as `db.OutboxTables()` / `db.IsOutboxTable`; business scopes and plans
must reject them (runner integration is WP3). The tables contain identifiers, counts, outcome and
timestamps — never source values, replacements, primary keys, keys or DSNs.

Install behaviour: equal version = no-op; different version, partially created reserved tables, or
a differently shaped schema = `ErrConflict`/`ErrUnsupported`, never automatic repair.

## 3. Same-transaction atomicity (G1)

One `*sql.Conn` is pinned for the outbox session; every batch begins `sql.Tx` on that connection.
`Commit` performs, in order:

1. `INSERT INTO maskriver_outbox_batch ...` through the same `*sql.Tx`;
2. `tx.Commit()` exactly once;
3. outcome classification.

`TestOutboxSameTransactionCommit` proves on a real SQLite file that a fresh connection sees both
the business UPDATE and the receipt row after success. `TestOutboxReceiptFailureRollsBackBusiness`
proves a failing receipt INSERT (real UNIQUE violation) aborts the business UPDATE.
`TestOutboxUpdateFailureLeavesNoReceipt` proves a failing UPDATE leaves no receipt.

## 4. Commit / rollback / CommitUnknown

- Known failure before the commit attempt (validation, UPDATE, receipt INSERT, cancellation):
  rollback, sanitized error, no success claim.
- Error returned by `tx.Commit()` itself: always `contracts.ErrCommitUnknown`; no retry, no
  guessing from message text, no post-hoc "probably committed" claim.
- A committed batch always has its receipt in the same database; an interrupted/uncommitted batch
  has none.

`TestOutboxCommitUnknownIsConservative` injects a post-commit error while the real commit happened
and confirms the caller still receives `ErrCommitUnknown` and exactly one receipt exists (no retry
duplication).

## 5. PRAGMAs, durability and platform

- `journal_mode`: must already be `delete`; the outbox never changes it. WAL/truncate/persist/
  memory/off are `ErrUnsupported` (only DELETE verified for this capability).
- `synchronous`: the pinned session requires `FULL`; if the session reports otherwise it sets
  `PRAGMA main.synchronous=FULL` on that session only (connection-local, not persistent) and
  re-reads to confirm. This is not a claim of proven power-loss durability; it is a verified
  session setting.
- `foreign_keys`: must be ON; `query_only`: must be OFF.
- Platform: Linux only. Other GOOS returns `ErrUnsupported`; Windows keeps the legacy feature set
  and CI but the streaming outbox is unsupported.
- The outbox requires `MaxOpenConns >= 2` because the write connection is pinned and paged reads
  use the pool.

## 6. Path safety (G5) — supported boundary

Preflight (Linux): absolute path, existing regular file, `Lstat` not a symlink, `os.SameFile`
consistent, hard-link count 1, parent directory exists and is not world-writable. Identity is
rechecked before each run/batch. This detects simple path replacement between open and batch.

**Not proven:** modernc SQLite reopens the database by pathname, so a concurrent attacker able to
replace the path between the stat and SQLite's open could still redirect the handle. The supported
threat boundary is the ADR-11 trusted local directory; no claim of TOCTOU-proof binding is made.
If that guarantee is required, this capability must be re-designed with an OS-handle-pinned open.

## 7. Bounded reads

`Outbox.Page` uses keyset pagination on `(run_id, batch_sequence)` with `LIMIT ?+1`, so a page holds
at most `limit` receipts and one look-ahead row. `OutboxReceiptReader` implements
`contracts.ReceiptReader`, advances a per-reader cursor, rejects oversized pages, unknown runs and
use-after-close, and propagates context cancellation. No full-run slice is materialized.

`TestOutboxPagingThousandBatches` commits 1,000 batches and pages them 32 at a time (32 pages,
never more than 32 receipts in memory); the outbox accumulates no in-process receipt cache.
Disk usage still grows with the number of batches: **bounded memory is not bounded disk**.

## 8. Fault matrix (real SQLite unless stated)

| Scenario | Test | Result |
|---|---|---|
| UPDATE + receipt success | `TestOutboxSameTransactionCommit` | PASS — both visible after commit |
| UPDATE failure | `TestOutboxUpdateFailureLeavesNoReceipt` | PASS — no receipt, no change |
| Receipt INSERT failure (real UNIQUE) | `TestOutboxReceiptFailureRollsBackBusiness` | PASS — business UPDATE rolled back |
| Duplicate batch ID / sequence | `TestOutboxDuplicateBatchIdentityRejected` | PASS — `ErrConflict`, no extra receipt |
| Cancel before commit | `TestOutboxCommitCancelBeforeAttempt` | PASS — canceled, nothing persisted |
| Commit error after real commit | `TestOutboxCommitUnknownIsConservative` | PASS — `ErrCommitUnknown`, one receipt |
| Pre-receipt failure (hook) | `TestOutboxHooksPreventReceipt` | PASS (simulated seam) — rollback |
| Missing schema / wrong version | `TestOutboxSchemaFailClosed` | PASS — `ErrUnsupported` |
| WAL journal mode | `TestOutboxJournalModeFailClosed` | PASS — `ErrUnsupported`, mode not rewritten |
| Symlink / hardlink / world-writable parent | `TestOutboxPathPreflight` | PASS — `ErrUnsafe` |
| SQLite page limit (SQLITE_FULL) | `TestOutboxFullDiskFailsClosed` | PASS — real storage failure, no change/receipt |
| Process exit before commit | `TestOutboxCrashBeforeCommit` | PASS — SQLite rollback journal restores |
| Process exit immediately after commit | `TestOutboxCrashAfterCommit` | PASS — data + receipt survive |
| Process kill *during* commit | — | **UNVERIFIED** (no deterministic seam without a hostile fixture) |
| ENOSPC injected exactly at receipt INSERT | — | **UNVERIFIED** (full-disk was exercised at UPDATE; receipt-time ENOSPC is not deterministically producible) |
| Legacy M1 DB without outbox | `TestOutboxLegacyDatabaseWithoutSchema` | PASS — legacy writer unaffected; outbox refuses |
| Legacy writer with outbox present | `TestOutboxLegacyWriterUnaffected` | PASS — no receipt rows written |

## 9. Environment and evidence

- Go `go1.26.3`, Linux `7.0.0-38-generic` x86_64, AMD Ryzen 9 7940H.
- Driver `modernc.org/sqlite v1.60.1`.
- Observed test database: `journal_mode=delete`, session `synchronous=FULL` (verified by the test
  fixture; see `TestOutboxSameTransactionCommit` path).
- Commands run: `gofmt`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, `go test ./internal/db -run TestOutbox -v`, `git diff --check`,
  plus `GOOS=windows go build ./...` and `GOOS=darwin go build ./...` compile checks.

## 10. Explicit non-claims

No streaming Runner/CLI integration, no end-to-end memory AC, no 10k/100k/1M streaming runs, no
MySQL outbox, no Windows streaming support, no anchor/seal/tamper evidence, no automatic recovery,
no checkpoint/resume, no exactly-once, and no unconditional power-loss durability proof. Issue #13
remains OPEN.
