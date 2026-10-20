# Issue #13 WP3-F — ResourceBudget enforcement audit

**Head audited:** `41e96b18130d9becfabb15d2bca502054331c1cd`
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
| MaxRowBytes | `pagePayloadBytes` over the page returned by `Reader.Page` | after the reader materialised the row, **before masking/SQL/transaction** | **Yes — the driver allocates the full field first** | Yes (run aborts, no write) | `TestStreamingRowBytesExactAndOver`, multi-column, late-large-row, UTF-8, NULL; `TestStreamingRowBytesPostMaterializationEvidence` shows an 8 MiB field is already in the heap before the check | **PARTIALLY ENFORCED** |
| MaxColumns | `contracts.ResourceBudget.ValidatePlan` via `StreamingApplyRequest.Validate` | before I/O | Plan metadata only; not an allocation guard | Yes | contract tests + runner request validation | **VALIDATED ONLY** |
| MaxTables | same as MaxColumns | before I/O | Plan metadata only | Yes | same | **VALIDATED ONLY** |
| MaxHistoryEntries | `history.Store.eachEntry` used by streaming ambiguity scan | during directory walking; chunk of 64 | No full directory materialisation; fails on overflow | Yes | `TestBoundedHistoryEnumeration`; CLI passes the budget into store config | **ENFORCED** (streaming lookup) |
| MaxHistoryNameBytes | `history.eachEntry` name check | before opening a candidate record file | No unbounded name allocation; candidate name over the bound fails | Yes | bounded-enumeration tests | **ENFORCED** — semantics are one record file name, not directory-name total |
| MaxReceiptPage | request validation, `db.OpenOutbox(MaxReceiptPage)`, `Outbox.Page`/reader | before I/O and on every page call | No (limit+1 look-ahead only) | Yes | WP2 outbox tests + runner summary validation | **ENFORCED** |
| MaxIssueCounts | `runner.boundedIssues` after the independent Validate | after validation, before summary return | The verifier's own issue slice is bounded by table/column categories, not rows (`verify.addIssue` aggregates by status/code/column) | Yes (category overflow fails the run) | verify code audit; runner budget tests | **ENFORCED** |
| MaxQueue | `validateStreamingStructure` | before I/O | No internal sink exists; any non-zero value is refused | Yes | `TestStreamingProcessAndDiskAdmission/queue` | **ENFORCED** as `0`-only |
| MaxProcessBytes | `validateProcessBudgetCombination` (worst case `MaxBatchSize*MaxRowBytes*4 + 16 MiB`) and `checkProcessAdmission` (actual page bytes ×4 + 16 MiB) | before I/O and before each page's masking | **Yes** — it is an admission estimate, not an OS RSS cap; a single pre-check row can already be allocated | Yes for the run | `TestStreamingProcessAndDiskAdmission`; 1M CLI peak RSS ≈ 39 MiB | **PARTIALLY ENFORCED** (admission/observability, not a hard process limit) |
| MaxDiskBytes | `checkDiskAdmission` before each transaction; `checkDiskAfterCommit` after each commit | before `BeginBatch`, and after the commit returns | **Yes** — SQLite can grow between check and commit; rollback journal/temp space is not fully attributable | Pre-check yes; post-commit reports failure and stops | `TestStreamingProcessAndDiskAdmission/disk_admission`, `TestCheckDiskAfterCommit`; CLI outbox growth 63–66 MiB at 1M | **PARTIALLY ENFORCED** |

## Contract conflicts that must not be papered over

1. **MaxRowBytes** promises a row-size bound, but `contracts.Reader.Page` has no byte-aware fetch. The
   current path validates after materialisation. A 500 MiB field would already be allocated before the
   refusal. This needs a DB-layer read capability (for example a per-page `length()` preflight query
   evaluated by SQLite before rows are materialised) under a separate DB authorization.
2. **MaxProcessBytes** cannot be a strict process-memory guarantee in application code without OS
   resource isolation. The implementation treats it as an explicit admission bound and documents
   that meaning. Either the contract wording must be revised by an authorized contract change, or the
   field must be documented as admission-only.
3. **MaxDiskBytes** cannot include SQLite internal temporary space or guarantee a commit cannot fail
   for ENOSPC; such a commit remains `ErrCommitUnknown` and is never retried.

## Other intermediate-collection audit

| Site | Bound | Verdict |
|---|---|---|
| Scan samples | `SampleLimit` (≤10000), per declared scope | Bounded |
| Plan/schema metadata | tables and columns limited by `MaxTables`/`MaxColumns` | Bounded, validated |
| Preview / dry run | `visit` pages capped at 128 rows, per-table budget | Bounded |
| Verify issues | `addIssue` aggregates by status/code/column; no per-row list | Bounded by categories, not rows |
| Worker queue | fixed `workers` channel capacity, ≤16 | Bounded |
| Outbox reader | `limit+1` look-ahead, no receipt cache | Bounded |
| CLI output | summary lines and counters only | Bounded |
| Error paths | fixed sentinels/categories, no accumulated error slices | Bounded |
| History directory | 64-entry chunks; streaming entry/name caps; legacy keeps old semantics by design | Bounded memory; streaming caps fail closed |

## Conclusion

The streaming path's *retained* working set is a function of `BatchSize`, the projected row size,
worker count and receipt page size, and the 1M CLI evidence shows flat RSS. It is **not** a guarantee
that an oversized field is rejected before allocation: the source and target pages are materialised
first, so peak allocation is not bounded by `MaxRowBytes`. `MaxProcessBytes` and `MaxDiskBytes` are admission/observability bounds,
not OS-level hard limits. ADR-08 must record these meanings; formal approval cannot convert them into
stronger guarantees.
