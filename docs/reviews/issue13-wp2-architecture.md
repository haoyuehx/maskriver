# Issue #13 WP2 — independent Architecture review record

**Scope:** PR #73 (`feat/db: add SQLite transactional receipt outbox`), read-only review.
**Reviewer context:** independent AI reviewer, separate from the implementer; no edits or merges.
**Reviewed heads:** `a3708f865808a9dbcd955403e5e454dc7e6231f0`, `100f71d839169bf7592d7938c6325017b844fc17`,
`297c57b20df15f1e8facd00a321c75e18ca78d3a` (base `24877565e39c312c59a628cf0f054fe33dc79b8e`).
**Artifacts:** reviewer outputs in the session artifact store; evidence is the supplied diff plus the
repository sources. No tests were run by this reviewer.

## Verdicts

| Head | P0 | P1 | P2 | Verdict |
|---|---|---|---|---|
| `a3708f8` | 0 | 1 | 0 | BLOCK — schema marker did not prove table shape |
| `100f71d` | 0 | 1 | 0 | BLOCK — shape check skipped hidden/generated columns |
| `297c57b` | 0 | 0 | 0 | OK |

## Findings and dispositions

1. **P1 — version marker was not schema evidence** (`internal/db/outbox.go`). A database could keep
   the `maskriver-outbox-v1` row while the run/batch tables were altered. Fixed in `100f71d`:
   `verifyOutboxTableShape` compares `PRAGMA table_xinfo` column name/type/NOT NULL/PK order against
   the expected definition, on both open and idempotent install.
2. **P1 — hidden columns skipped** (`verifyOutboxTableShape`). A generated column could be added
   without detection. Fixed in `297c57b`: any `hidden != 0` column returns `ErrUnsupported`, and a
   generated-column tamper test was added.

## Confirmed properties at the final reviewed code

- One pinned `*sql.Conn`; `BeginBatch` starts the transaction and `Commit` inserts the receipt through
  the same `*sql.Tx` before the single commit attempt.
- The `write.go` extraction is behavior-preserving; legacy `Begin`/`Commit`/`Rollback` semantics and
  `OpenWriter` are unchanged.
- WP1 DTOs are reused (`StreamingBatchReceipt`, `BatchOutcome`, `OutboxCapabilities`, `ReceiptPage`,
  `ReceiptReader`); no duplicate receipt model.
- Reads use `LIMIT ?+1` and a per-reader cursor; no full-run slice or background queue.
- No #14 recovery/checkpoint semantics.

## Limits

Reviews are documentation/code inspection only. They do not establish runtime behavior on MySQL or
Windows, do not prove power-loss durability, and do not validate TOCTOU-proof path binding.
