# Issue #13 WP3 — streaming scale evidence

**Head:** `a4a2e415e662c5fa2e4c6c312cec3e1d4895f37e`
**Status:** experimental, synthetic, runner-level. Not an approved production budget.

## What ran

| Matrix | Rows | Workers | Batch | Repeats | Evidence |
|---|---|---|---|---|---|
| quick | 128 / 512 / 2048 / 10000 / 100000 | 1,2,4,8,16 (10k all; others sampled) | 64; 10k also 16 and 256 | 3 | `streaming-scale.csv` |
| 1M smoke | 1000000 | 4 | 256 | 1 | `streaming-scale-1m-single.csv` |

All 36 quick runs and the single 1M run reached `RunStateComplete` with
`AuditStateConsistent`; the internal independent Validate passed in every run.

## Headline results (median of 3 unless noted)

| Rows | Workers | Batch | Rows/s | Apply ms | Batches | Heap after (MiB) | Heap Sys (MiB) |
|---|---|---|---|---|---|---|---|
| 128 | 1 | 64 | ~4,500 | 28 | 2 | ~2.4 | ~7.4 |
| 512 | 2 | 64 | ~6,200 | 82 | 8 | ~1.0 | ~11.2 |
| 2,048 | 4 | 64 | ~7,200 | 285 | 32 | ~2.5 | ~11.2 |
| 10,000 | 1..16 | 64 | ~7,300–7,600 | ~1,320–1,370 | 157 | ~1.4–3.8 | ~14–15 |
| 10,000 | 4 | 16 | ~2,100 | ~4,740 | 625 | ~1.6–3.3 | ~14–15 |
| 10,000 | 4 | 256 | ~20,300 | ~490 | 40 | ~2.1–3.0 | ~14–15 |
| 100,000 | 8 | 64 | ~7,400 | ~13,470 | 1,563 | ~2.9–3.4 | ~15 |
| 1,000,000 (single) | 4 | 256 | ~21,100 | 47,325 | 3,907 | ~1.5 | ~11.0 |

## Interpretation, restricted

- **Heap is flat in batch count.** End-of-run `HeapAlloc` stays around 1–4 MiB
  from 128 to 1,000,000 rows and from 2 to 3,907 committed batches. `TotalAlloc`
  grows with rows (per-row churn), which is expected and is not retained state.
- **Workers do not raise throughput here.** The single serialized SQLite commit
  and receipt INSERT dominate; 1–16 workers produce the same result and roughly
  the same rate. Worker count affects only in-page transformation, and the
  result is required to be identical across workers (runner test).
- **Batch size matters.** 256 is ~2.7× faster than 64 on the 10k fixture and
  much faster than 16; this is commit/fsync amortisation, not a memory change.
  The default remains 64 and was not changed.
- **Peak RSS is unavailable** (`VmHWM` unreadable here), so no RSS claim is made.
  p50/p95 are not reported: three samples are too few for a percentile claim.
- **Disk outbox grows with batches/rows** (one row per committed batch) and is
  not covered by the heap bound.

## Not verified

- MySQL streaming, Windows streaming runtime, and CLI subprocess scale beyond
  the 10k E2E test.
- Power-loss durability, mid-commit kill, receipt-time ENOSPC.
- Any production memory budget: ADR-08 numbers remain unapproved, and the CLI
  requires an explicit experimental budget file rather than a default.
