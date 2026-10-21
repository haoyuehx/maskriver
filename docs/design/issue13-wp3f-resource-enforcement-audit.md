# Issue #13 WP3-F — ResourceBudget enforcement audit

**Head audited:** `e02fa8e595a5ff4b008a173ed8e146e75794d4b6` (DB paging `de2751a3`)
**Scope:** streaming (`m2-stream-v1`) path only. The legacy m1 path keeps its documented O(K)
`ApplyReport` behaviour and is not reinterpreted here.

A numeric `Validate()` check is never counted as enforcement. Each row below names the actual
consumption site and whether the bound is applied before allocation, before a write, or only after a
resource was already used.

## Enforcement matrix

| Field | Consumption site | Check timing | Can allocate past the bound? | Fail closed | Evidence | Verdict |
|---|---|---|---|---|---|---|
| MaxBatchSize | `runner.validateStreamingStructure`; page `Limit` in `streamingRun.applyTable`; `Outbox.BeginBatch` | before any DB page read / before transaction | No (page size is derived from it; contract `Validate` also rejects larger) | Yes | runner streaming tests; WP2 `BeginBatch` validation | **ENFORCED** |
| MaxWorkers | `validateStreamingStructure` (`cfg.Workers <= budget`); `maskBatchValues` 1..16 pool | before any page is masked | No | Yes | worker 1/2/4/8/16 equivalence test | **ENFORCED** |
| MaxRowBytes | `db.PageWithin` reads keys/costs then the accepted prefix in one read transaction; `pagePayloadBytes` re-checks decoded values; `changePayloadBytes` checks masked replacement rows; `boundedValidationReader` bounds final validation | **before the value is materialised** (oversized first row, later row and lookahead all reject in phase 1), and again before each write | No projected value over the budget crosses the driver boundary; SQLite evaluates `length()` without copying the value to Go | Yes | `TestPageWithinExactBoundaryAndLegacyPage`, multi-column, UTF-8/NUL/BLOB/NULL/types, huge TEXT PK, huge lookahead, `TestPageWithinPreMaterialisationMemory` (8 MiB rejected with ~13 KiB total alloc vs ~33.5 MiB legacy), runner static/late-batch/source/target/replacement tests | **ENFORCED** on the supported Linux/SQLite path |
| MaxColumns | `contracts.ResourceBudget.ValidatePlan` via `StreamingApplyRequest.Validate` | before I/O | Plan metadata only; not an allocation guard | Yes | contract tests + runner request validation | **VALIDATED ONLY** |
| MaxTables | same as MaxColumns | before I/O | Plan metadata only | Yes | same | **VALIDATED ONLY** |
| MaxHistoryEntries | `history.Store.eachEntry` used by streaming ambiguity scan | during directory walking; chunk of 64 | No full directory materialisation; fails on overflow | Yes | `TestBoundedHistoryEnumeration`; CLI passes the budget into store config | **ENFORCED** (streaming lookup) |
| MaxHistoryNameBytes | `history.eachEntry` name check | before opening a candidate record file | No unbounded name allocation; candidate name over the bound fails | Yes | bounded-enumeration tests | **ENFORCED** — semantics are one record file name, not directory-name total |
| MaxReceiptPage | request validation, `db.OpenOutbox(MaxReceiptPage)`, `Outbox.Page`/reader | before I/O and on every page call | No (limit+1 look-ahead only) | Yes | WP2 outbox tests + runner summary validation | **ENFORCED** |
| MaxIssueCounts | `runner.boundedIssues` after the independent Validate | after validation, before summary return | The verifier's own issue slice is bounded by table/column categories, not rows (`verify.addIssue` aggregates by status/code/column) | Yes (category overflow fails the run) | verify code audit; runner budget tests | **ENFORCED** |
| MaxQueue | `validateStreamingStructure` | before I/O | No internal sink exists; any non-zero value is refused | Yes | `TestStreamingProcessAndDiskAdmission/queue` | **ENFORCED** as `0`-only |
| MaxProcessBytes | `validateProcessBudgetCombination` (worst case `MaxBatchSize*MaxRowBytes*4 + 16 MiB`) and `checkProcessAdmission` (actual page bytes ×4 + 16 MiB) | before I/O and before each page's masking; rows are now already bounded by PageWithin | **Yes** — it is an admission estimate, not an OS RSS cap | Yes for the run | `TestStreamingProcessAndDiskAdmission`; 1M CLI peak RSS ≈ 39.5 MiB | **PARTIALLY ENFORCED** (admission bound, not a hard process limit) |
| MaxDiskBytes | `checkDiskAdmission` before each transaction; `checkDiskAfterCommit` after each commit | before `BeginBatch`, and after the commit returns | **Yes** — SQLite can grow between check and commit; rollback journal/temp space is not fully attributable | Pre-check yes; post-commit reports failure and stops | `TestStreamingProcessAndDiskAdmission/disk_admission`, `TestCheckDiskAfterCommit`; CLI outbox growth 63–66 MiB at 1M | **PARTIALLY ENFORCED** |

## Semantic conflicts recorded, not papered over

1. **MaxRowBytes** is now a real pre-materialisation read bound on the supported SQLite path, and it
   also covers masked replacement rows and streaming validation. It is not a bound on SQLite's
   internal page-cache behaviour or on an arbitrary non-SQLite adapter; readers without the bounded
   capability are refused with `ErrUnsupported`. The frozen `contracts.Reader.Page` keeps its old
   semantics.
2. **MaxProcessBytes** cannot be a strict process-memory guarantee in application code without OS
   resource isolation. ADR-08 approves it explicitly as an admission bound.
3. **MaxDiskBytes** cannot include SQLite internal temporary space or guarantee a commit cannot fail
   for ENOSPC; such a commit remains `ErrCommitUnknown` and is never retried. ADR-08 approves it as an
   admission/post-commit bound.

## Other intermediate-collection audit

| Site | Bound | Verdict |
|---|---|---|
| Scan samples | `SampleLimit` (≤10000) plus 1 MiB/value and 16 MiB/column sample caps; oversized DISTINCT values are replaced by a flag and fail closed in one statement | Bounded |
| Plan/schema metadata | tables and columns limited by `MaxTables`/`MaxColumns` | Bounded, validated |
| Preview / dry run | `visit` pages capped at 128 rows, per-table budget; the **streaming** dry run additionally uses `PageWithin(MaxRowBytes)` | Bounded; streaming pre-materialisation |
| Streaming final validation | `boundedValidationReader` routes verifier pages through `PageWithin(MaxRowBytes)` | Bounded; legacy Validate unchanged |
| Verify issues | `addIssue` aggregates by status/code/column; no per-row list | Bounded by categories, not rows |
| Worker queue | fixed `workers` channel capacity, ≤16 | Bounded |
| Outbox reader | `limit+1` look-ahead, no receipt cache | Bounded |
| CLI output | summary lines and counters only | Bounded |
| Error paths | fixed sentinels/categories, no accumulated error slices | Bounded |
| History directory | 64-entry chunks; streaming entry/name caps; legacy keeps old semantics by design | Bounded memory; streaming caps fail closed |

## Conclusion

On the approved Linux/SQLite streaming path, `MaxRowBytes` is enforced **before** an oversized
projected value is materialised, including keys, the lookahead row, masked replacements and final
validation. `MaxProcessBytes` remains an admission bound and `MaxDiskBytes` an admission/post-commit
bound; ADR-08 records those exact meanings and cannot convert them into OS RSS or filesystem quotas.
MySQL/Windows streaming, G5 handle-level TOCTOU, G7 receipt-time ENOSPC/fsync and G9 mid-commit kill
remain unverified and are disclosed in the final acceptance matrix.
