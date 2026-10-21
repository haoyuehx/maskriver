# ADR-08 — final resource-budget decision

**Status:** `ADR-08 APPROVED — DELEGATED TECHNICAL DIRECTION (2026-10-21)`
**Scope:** explicit M2 streaming path (`m2-stream-v1`) on Linux amd64, local SQLite, trusted
local directory, isolated synthetic copy.
**Authority:** the repository owner's 2026-10-21 task direction explicitly delegates the final
numeric budget decision and the conditional Issue #13 closure to the AI technical lead, subject to
independent review and real evidence. This document records that **delegated technical direction**.
It is **not** a GitHub Owner signature and must not be presented as one. No repository rule required
a separate GitHub approval that could not be satisfied here.

**Measured code heads:**
- pre-materialisation DB paging: `de2751a3adb34824b291ee725d5b586710330451` (PR #76)
- runner integration and masked-row bound: `e02fa8e595a5ff4b008a173ed8e146e75794d4b6` (PR #77)

## Approved values

| Field | Value | Guarantee in this implementation | Enforcement point |
|---|---:|---|---|
| MaxBatchSize | 256 | hard page size | request validation + every page request |
| MaxWorkers | 16 | hard worker cap | runtime config + request validation |
| MaxRowBytes | 1 MiB | **pre-materialisation** projected-row read bound; also covers the masked replacement row and streaming validation | `internal/db.PageWithin` (length preflight before values), `changePayloadBytes`, `boundedValidationReader` |
| MaxColumns | 256 | plan-shape validation | `ResourceBudget.ValidatePlan` before I/O |
| MaxTables | 32 | plan-shape validation | `ResourceBudget.ValidatePlan` before I/O |
| MaxHistoryEntries | 4096 | hard streaming-history directory entry cap | chunked lookup, fail closed |
| MaxHistoryNameBytes | 4096 | hard cap on **one candidate file name**, not the directory total | streaming record name check |
| MaxReceiptPage | 128 | hard receipt page cap | request, outbox and reader |
| MaxIssueCounts | 32 | hard cap on aggregated issue categories | summary construction |
| MaxQueue | 0 | no asynchronous receipt queue; only synchronous delivery | structure validation rejects non-zero |
| MaxProcessBytes | 2048 MiB | **admission bound** (`max(batch*row*4, page*4) + 16 MiB`), not an OS RSS cap | pre-I/O combination check + per-page admission |
| MaxDiskBytes | 8 GiB | **admission bound plus post-commit check** on main DB + journal/WAL; not a filesystem quota and not commit-time space reservation | per-batch projection + post-commit check |

`ResourceBudget.Validate` still rejects zero/negative values: there is no "unlimited" encoding.
Arithmetic is overflow-checked (`int64` guards); SQLite costs that overflow to REAL are rejected.

## What each guarantee actually means

### MaxRowBytes — now pre-materialisation
`PageWithin` runs one read transaction per page:

1. phase 1 selects only per-row byte-cost expressions (no keys, no values);
2. any row over budget, including the `LIMIT+1` lookahead row, rejects the page with `ErrUnsafe`;
3. phase 2 fetches only the accepted prefix from the same SQLite snapshot.

TEXT/BLOB/date lengths are `length(CAST(col AS BLOB))` (UTF-8 bytes, embedded NUL counted), INTEGER is
the exact decimal text, REAL/BOOL use proven upper bounds, NULL is zero. The Go driver receives no
oversized value. The decoded payload is re-checked before return, and the runner also checks the
projected **masked replacement** row before the outbox transaction, because a strategy may lengthen a
value.

Evidence: `TestPageWithinPreMaterialisationMemory` — 8 MiB field rejected with ~13 KiB total
allocation versus ~33.5 MiB for the legacy `Reader.Page`; 8 MiB lookahead excluded with ~10 KiB;
2 MiB TEXT primary key rejected without preflight key allocation; runner evidence ~8 KiB.
The frozen `contracts.Reader.Page` signature and legacy semantics are unchanged.

### MaxProcessBytes — admission only
It refuses a configuration or page whose modelled working set cannot fit, before masking/writing. It
does **not** cap RSS, Go heap, SQLite page cache or the OS. Measured 1M CLI peak RSS is ~39 MiB for
the approved 1 MiB/256 configuration, but that is an observation, not a guarantee for arbitrary data.

### MaxDiskBytes — admission / post-commit only
Before each transaction it projects current main+journal/WAL plus `pageBytes*3`; after commit it
re-checks. An overrun after commit is reported as incomplete, keeps committed receipts, and is never
retried or presented as success. It is not a quota and cannot reserve commit-time space; a commit-time
ENOSPC remains `ErrCommitUnknown`.

### MaxHistoryNameBytes
One candidate record file name. It is not a total-directory byte budget; the entry count is bounded
by `MaxHistoryEntries` and reading is chunked.

## Supported environment

- Linux amd64.
- Local SQLite files (`modernc.org/sqlite v1.60.1`, SQLite 3.53.4), rollback-journal mode for the
  outbox, trusted local directory, isolated copy.
- Explicit `mask --streaming --apply` with a real `m2-stream-v1` Ed25519 approval bound to canonical
  plan content and a pre-installed outbox.
- The CLI requires an explicit `--budget-file` and prints `budget_status=experimental`; these values
  do not change legacy M1 defaults (BatchSize 64, Workers 1).

## Preserved limits and unverified areas

- MySQL streaming, Windows streaming runtime, Windows outbox: not implemented/verified.
- G5: trusted-local-directory boundary; file-handle TOCTOU is not defended against.
- G7: UPDATE-stage `SQLITE_FULL` and receipt failure rollback verified; receipt-time ENOSPC/fsync
  unverified.
- G9: exits before/after commit verified; exact mid-commit kill unverified; CommitUnknown is never
  retried and this ADR does not claim #14 recovery.
- Power-loss durability, p95 latency, arbitrary concurrent writers: not claimed.

## Evidence

- `docs/perf/issue13-wp3g-e02fa8e/` — 27 real CLI subprocess samples (10k/100k/1M × 3 configs × 3),
  apply peak RSS 24.4/37.7/39.0 MiB, batch 256 ~2.5–2.8× faster than 64.
- `docs/design/issue13-wp3f-resource-enforcement-audit.md` — per-field enforcement matrix.
- `docs/reviews/issue13-final-acceptance.md` — final AC matrix with test/code evidence.
- PRs #76 and #77 with independent Architecture/Security/QA reviews.

## Decision

The values above are approved for the explicit M2 streaming path in the supported environment, with
the guarantee levels stated here. Any stronger claim (OS RSS cap, filesystem quota, MySQL/Windows,
power-loss recovery, exactly-once) is outside this decision.
