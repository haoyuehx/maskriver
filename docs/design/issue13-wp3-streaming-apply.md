# Issue #13 WP3 — bounded streaming Apply and CLI integration

**Status:** `WP3 IMPLEMENTED — PENDING RESOURCE APPROVAL` (ADR-08 numbers are experimental;
production streaming stays opt-in and is not enabled by default).
**Base main:** `97e95c8be712ad12491bdfe7ada1cea41f85eec7`
**Implementation head:** `a4a2e415e662c5fa2e4c6c312cec3e1d4895f37e`

WP1 defined the `m2-stream-v1` DTOs; WP2 implemented the SQLite transactional outbox.
WP3 connects them through an explicit, protocol-bound path and a real Ed25519 approval chain.
It does not change `pkg/contracts`, the legacy `m1a-v1` Plan/approval semantics, the default
`mask` behaviour, BatchSize/Workers defaults, or the release.

## 1. Execution chain

```
Scan/Plan (existing) -> Preview (read-only) -> approve --streaming (separate process)
  -> mask --streaming --apply --budget-file ... (new process)
       structure -> reserved-scope check -> canonical/Ed25519 approval
       -> live preflight -> full dry run
       -> OpenOutbox (platform/path/journal/schema gates)
       -> InitializeRun (separate autocommit; not business success)
       -> per page: BeginBatch -> Update -> receipt INSERT -> one Commit
       -> independent Validate -> bounded StreamingApplySummary
```

Both the old and new paths build the plan through the same `Engine.Plan`; no second plan format or
duplicate DTO exists.

## 2. Approval binding

The legacy record cannot express per-column `Scope`/`NormalizationVersion` and must not authorize
M2, so streaming approvals are a separate signed file:

- file suffix `.streaming-decision`, magic `maskriver-history-streaming-record-v1`,
  version `history-streaming-record-v1`;
- signed payload binds `m2-stream-v1`, `m2-approval-binding-v1`, every m1a digest, the strategy/key
  tuple and a **canonical plan-content digest** covering every table, primary key, full column
  reference, type, Scope, NormalizationVersion and column order;
- verification re-derives that digest from the live plan, so editing Scope/type/order while keeping
  `Plan.ID` is refused;
- Ed25519 verification, trusted key set, expiry, future-issued skew and revocation are the history
  store's responsibility; the runner re-checks the record it receives and refuses a nil verifier;
- a legacy decision is never read by the streaming lookup and a streaming record is never read by the
  legacy lookup (disjoint suffix and binding revision).

All approval checks happen before the outbox is opened, so a refusal cannot start a transaction.

## 3. Outbox integration

`StreamingApply` uses the merged WP2 API unchanged: `db.OpenOutbox`, `Outbox.Capabilities`,
`Outbox.InitializeRun`, `Outbox.BeginBatch` (which re-validates the table plan),
`Outbox.Page`/`Reader`. The business UPDATEs and the receipt INSERT share one transaction and one
commit. Any commit error surfaces as `ErrCommitUnknown` and stops the run; it is never retried and
never reported as success.

`InitializeRun` runs only after structure, approval, preflight and dry run pass. Preview never calls
it. The outbox schema is installed only by the explicit `init-outbox` command; Apply never runs DDL.

## 4. Bounded execution

- Keyset paging per table; each page is transformed by the existing worker pool
  (1–16 workers, deterministic index mapping, context cancellation) — the pool is reused, not
  redesigned.
- Only constant-size counters are retained per batch (`BatchesTotal/Committed/Unknown/Failed`,
  `RowsCommitted`, bounded `IssueCounts`). No `[]StreamingBatchReceipt`, no accumulated channel, map
  or error list.
- Batch sequences start at 0 and increase across the run; `BatchID` is run-scoped.
- Zero-row pages (all planned columns NULL) produce no statement and no receipt.
- The first failure stops later batches; previously committed batches remain in the outbox and are
  visible through the reader.
- Validation is a separate read-only pass after the last commit; failure forces an incomplete state.

## 5. CLI

```
maskriver init-outbox --target <existing-db>
maskriver approve --streaming ... --trust-file ... --history-dir ... --reviewer-key-id ... --reviewer-key-file ...
maskriver revoke  --streaming ...
maskriver mask --streaming --apply --approved --budget-file <json> --receipt-page-size N \
    --source ... --target ... --scope table.column --key-file ...
```

- `--streaming` without `--apply` is refused.
- `--streaming --apply` requires a complete explicit `ResourceBudget` JSON (there are no defaults)
  and refuses when `BatchSize`/`Workers`/page size exceed it.
- No environment variable or config file can enable writes.
- Missing outbox schema, wrong schema version, WAL, unsupported platform or a reserved
  `maskriver_outbox_*` scope fail closed; there is no fallback to the legacy path.
- Output is aggregate-only: run/plan IDs, protocol, state, audit state, counters, page size and
  `budget_status=experimental`. `commit_unknown=true retry=never` is printed explicitly.

## 6. Bounded history enumeration

`Store` now enumerates the review directory in fixed 64-entry chunks with `MaxEntries` and
`MaxNameBytes` bounds (context-aware, fail closed on overflow) while preserving the legacy
"more than one file claiming this plan is ambiguous" rule. The streaming run wires its budget's
history limits into the store.

## 7. ADR-08 status

No numeric budget is approved. The CLI has no production default and requires an explicit file; the
experimental values used for tests are recorded only as evidence, not as a commitment. Proposed
starting points (to be approved or replaced by the Owner after reviewing
`docs/perf/issue13-wp3-a4a2e41/summary.md`):

| Field | Experimental proposal |
|---|---|
| MaxBatchSize | 256 |
| MaxWorkers | 16 |
| MaxRowBytes | 1 MiB |
| MaxColumns | 256 |
| MaxTables | 32 |
| MaxHistoryEntries | 4096 |
| MaxHistoryNameBytes | 4096 |
| MaxReceiptPage | 128 |
| MaxIssueCounts | 32 |
| MaxQueue | 0 (synchronous receipt delivery) |
| MaxProcessBytes | 512 MiB |
| MaxDiskBytes | 8 GiB |

These are **not** approved defaults, and no memory-AC claim depends on them.

## 8. Residual risks (unchanged)

- **G5:** path preflight plus identity recheck, but no file-handle-level TOCTOU binding; the
  supported boundary remains the ADR-11 trusted local directory.
- **G7:** real `SQLITE_FULL` at UPDATE is tested; receipt-time ENOSPC and fsync failure remain
  unverified.
- **G9:** exit before/after commit is tested; kill *during* commit remains unverified.
- MySQL streaming, Windows streaming runtime, recovery, checkpoint/resume, exactly-once and
  tamper-evidence anchors are out of scope.
