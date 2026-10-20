# ADR-08 — final resource-budget proposal

**Status:** `ADR-08 PENDING APPROVAL`
**Technical head:** `41e96b18130d9becfabb15d2bca502054331c1cd`
**Authority:** this document proposes numbers and meanings; it is not an Owner signature and does not
approve production defaults.

The contract type `contracts.ResourceBudget` is frozen (`APIRevision=m1a-v1`). This proposal therefore
distinguishes what the implementation can actually enforce from what it can only check or observe.

## Recommended values

| Field | Proposed | Meaning in this implementation |
|---|---|---|
| MaxBatchSize | 256 | hard page size; measured ~2.7× faster than 64 at 10k/100k/1M |
| MaxWorkers | 16 | hard worker cap; no throughput gain observed, kept as a safety limit |
| MaxRowBytes | 1 MiB | **post-materialisation** projected-row check, fail closed before masking/write |
| MaxColumns | 256 | plan-shape validation before I/O |
| MaxTables | 32 | plan-shape validation before I/O |
| MaxHistoryEntries | 4096 | hard cap on streaming lookup directory entries |
| MaxHistoryNameBytes | 4096 | hard cap on **one candidate record file name**, not the directory total |
| MaxReceiptPage | 128 | hard receipt page cap (request, outbox and reader) |
| MaxIssueCounts | 32 | hard cap on aggregated issue categories in the summary |
| MaxQueue | 0 | only synchronous receipt delivery is implemented |
| MaxProcessBytes | 2048 MiB | **admission bound** (`MaxBatchSize*MaxRowBytes*4 + 16 MiB` worst case and actual-page ×4 + 16 MiB); **not** an OS RSS cap |
| MaxDiskBytes | 8 GiB | pre-transaction projection (current main + journal/WAL + page×3) and post-commit check; **not** a filesystem quota |

Measured evidence (real CLI subprocess, 33 samples): peak RSS 24 MiB at 10k, ~38 MiB at 100k,
~39 MiB at 1M; outbox 63–66 MiB at 1M. 1M × 3 samples per configuration. See
`docs/perf/issue13-wp3f-41e96b1/summary.md`.

## Why these numbers

- 256 is the largest batch in the measured matrix and is well below the contract's 1000 page cap; it
  gives the best throughput without changing the default (the CLI default remains 64).
- 1 MiB keeps the worst-case page working set (256 × 1 MiB × 4) near 1 GiB, which fits the proposed
  2 GiB admission bound with the fixed baseline.
- 4096 history entries and 4096 name bytes are far above real approval directories and still bound.
- 8 GiB disk is over 100× the measured 1M outbox and leaves room for journal growth while remaining a
  meaningful refusal point.

## Requirements before approval

1. **DB read preflight for MaxRowBytes.** Without a byte-aware page fetch, an oversized field is
   allocated before rejection. This requires a separate DB-module authorization and PR (for example a
   SQLite `length()`-based page preflight evaluated before materialisation). Until then MaxRowBytes is
   `PARTIALLY ENFORCED` and cannot be advertised as a pre-allocation memory guarantee.
2. **Contract wording or documented semantics for MaxProcessBytes.** The frozen field name suggests a
   hard process limit. The implementation and this proposal define it as an admission bound. An
   authorized contract revision, or an explicit Owner decision to keep it admission-only, is required.
3. **Owner (or formally delegated decision body) approval of the table above.** No self-signature.

## What approval does not mean

- It does not make MaxProcessBytes an RSS hard cap.
- It does not make MaxDiskBytes a filesystem quota or guarantee commit-time space.
- It does not verify power-loss durability, mid-commit kill, receipt-time ENOSPC, MySQL or Windows
  streaming.
