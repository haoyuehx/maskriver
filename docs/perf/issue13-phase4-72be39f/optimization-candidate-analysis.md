# Issue #13 Phase 4 — candidate analysis and baseline (base 72be39f)

## Candidate analysis

`internal/runner/execute.go` runs approval authentication, static preflight, and full read-only dry-run before opening a writer. `applyTable` compares source/target pages, transforms rows, then `writeBatch` opens one `WriteTx` per nonempty page. It appends the receipt only after Commit returns success. Commit errors are classified `ErrCommitUnknown`, set `CommitOutcomeUnknown`, stop execution, and are never replayed; other failure paths attempt bounded rollback. Context is checked before pages and by the DB transaction operations. Earlier committed batches remain receipted if a later batch fails.

`internal/db/write.go:85-95` `Begin` performs live schema description and plan validation before `database/sql.BeginTx`. In `Update` (`:100-145`), each row is contract-validated, assigned columns are discovered, then `updateSQL` (`:203-240`) allocates assignment/predicate/argument slices and builds parameterized optimistic UPDATE SQL. WHERE contains PK and *all planned columns* with expected-value comparisons (SQLite `IS ?`, MySQL `<=> ?`). Each changed row calls `tx.ExecContext` and checks `RowsAffected()==1`; NULL-only rows issue no UPDATE. Commit/Rollback close transaction state at `:243-286`.

The static gate performs schema/plan binding, rejects triggers on both snapshots, modified UNIQUE/CHECK/FK columns, and incoming FK references (`runner/execute.go:220-470`). These checks and approval checks must not be cached or bypassed. Runner dry-run and source/target comparison are also required.

Phase-3 instrumentation on 256 rows observed 256 UPDATE statements at every batch size; successful commits/receipts are 16, 4, and 1 at batch sizes 16, 64, and 256. Begin/commit/UPDATE counts therefore move with batch partitioning, but no repeated Prepare measurement exists in phase 3; SQLSpy uses driver interfaces that force a Prepare fallback to observe statements, so its statement count must not be interpreted as the native production path's Prepare count. Candidate Prepared Statement reuse cannot be justified solely by current evidence and may be unsafe across driver implementations or statement lifetime. The best controlled next step is measurement across explicit batch sizes; do not alter production defaults or transaction boundaries without evidence.

BatchSize affects page allocations, number/duration of transactions, max rows in a rollback/unknown-outcome batch, and cancellation/lock exposure. It does not alter SQL shape per row. Larger pages increase the per-batch failure exposure; there is no global transaction.

## Reproduced baseline (no production changes)

Date/environment was captured by the test harness; see `environment.json`. This rerun at base SHA 72be39f used the existing opt-in, instrumented 256-row matrix (10 randomized trials each at 16/64/256), real SQLite independent source/target databases, fake_email synthetic values, real Ed25519 approval and independent Validate. Fixture setup is outside Apply timing. It is not a new 16/64/128/256 matrix and does not measure uninstrumented throughput.

| Batch | n | Apply median (ms) | mean (ms) | min–max (ms) | CV | mean rows/s | successful commits/trial | driver Commit mean (ms) | UPDATE calls |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
|16|10|125.31|124.92|122.13–128.35|1.70%|2,050|16|97.71|256|
|64|10|38.54|39.00|38.17–40.73|2.59%|6,569|4|24.72|256|
|128|SKIP|—|—|—|—|—|—|—|—|
|256|10|16.35|16.63|14.26–18.38|7.29%|15,472|1|6.03|256|

Apply and driver timings here include SQLSpy/diagnostic observer work. The separate paired observer run is recorded below; medians show instrumented/plain delta -1.78% (range -6.15..6.67%) for 16, -0.03% (-5.56..0.61%) for 64, and -0.42% (-13.87..28.33%) for 256. This is noisy, not a correction factor.

10k Batch64 real Runner checks reproduced for workers 1/2/4/8/16; all 157 receipts represented 10k rows, exact rows matched the one-worker output, source was unchanged, independent Validate passed, and connections closed. Apply ranged ~1.445–1.477 s (about 6,770–6,920 Apply rows/s). Also separately repeated workers 1/4. Each cell is one observation; no speedup claim.

## Phase-4 decision

No production optimization is selected or implemented at this point. The measurement supports a batch-count/commit-cost hypothesis, but changing BatchSize would alter transaction and failure scope, and no evidence yet shows that such a change is safe or appropriate as a default. Prepared-statement reuse is unproven. The initial sample suite lacks 128 and uninstrumented 256/2048/10000-row crossed measurements, as well as repeated 10k BatchSize/worker matrices. These are material gaps, so the final optimization verdict cannot be ACCEPT on these data alone. No production transaction behavior has changed.
