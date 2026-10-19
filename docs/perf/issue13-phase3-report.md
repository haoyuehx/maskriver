# Issue #13 — Phase 3 measurement report

**Implementation branch:** `perf/issue13-phase3-transaction-cost-20261019`

**Measurement code SHA:** `c747538edcc0fb52fb16a0eb796953cec1062c94`

**Latest tested source SHA:** `8ffca9b42ee3841d0d11e329b21a09e7c45aa682` (only adds post-marker Validate to the strace test)

**Evidence:** [`issue13-phase3-c747538/`](issue13-phase3-c747538/)

**Issue state:** keep #13 OPEN; phase-3 measurements do not complete acceptance or authorize #14.

## Scope and safety

Measurement-only work. No production transaction, SQL, journal/synchronous, BatchSize default, worker-pool, approval, validation, or write authorization behavior was changed. All fixtures use synthetic `@example.invalid` values, independent SQLite source/target files, and real test Ed25519 approval. MySQL/CLI/non-Linux/100k+ performance remain unverified.

## Follow-up fixes and independent review

The four inherited phase-2 P2 notes were addressed: benchmark timer boundaries now exclude fixture/teardown/bookkeeping; online and offline sample validation share physical/matrix checks; transaction calls are measured through a real SQLite driver spy; and per-iteration temp directories are removed after closing the engine.

The initial independent review/QA at `1e5b5b0` found three additional P2s and the exact-head artifact check failed. Follow-up fixes at `c747538`:

1. `environment.json` is immutable measurement-time provenance. Offline regeneration now validates its schema, sample matrix, code-SHA shape and SQLite settings, while regenerating only sample-derived artifacts; it no longer substitutes the checker’s current HEAD/host. Exact evidence consistency passed at the later checkout.
2. The phase-3 validator now recomputes rows/s from row count and Apply duration and rejects negative durations/components. Mutation tests cover both.
3. SQLSpy now uses constant-time log cursors and suffix-only copies, avoiding repeated copies of the growing entire statement/event log. A paired plain-vs-instrumented Apply experiment quantifies observer perturbation; Apply/throughput is explicitly labeled diagnostic and not production throughput.

The strace test now performs independent full Validate after the END marker, keeping validation outside the syscall window. Initial BLOCK review reports were preserved unchanged in `docs/reviews/`. Final independent QA at `c49b9ce` passed locally; the independent Security follow-up was OK with notes and found no P0/P1/P2. Reports: `docs/reviews/issue13-phase3-qa-followup-2026-10-19.md` and `docs/reviews/issue13-phase3-security-followup-2026-10-19.md`. Remote final-head CI and PR merge gates remain outstanding.

## Repeated transaction experiment

Linux x86_64, kernel `7.0.0-38-generic`, AMD Ryzen 9 7940H, 16 cores, host MemTotal 15,187 MiB (not a container limit), Go 1.26.3, `modernc.org/sqlite v1.60.1`, journal `DELETE`, synchronous `2`. Thirty fresh-database trials: 256 rows, workers=4, 10 randomized trials each at BatchSize 16/64/256. Every trial completed, independently validated, and reconciled exact receipts and rows. See raw `samples.json`, `apply-transactions.csv`, and `apply-batches.csv`.

| Batch | successful commits/trial | instrumented Apply mean (median), ms | driver Commit mean / commit | driver Commit total / Apply | UPDATE driver total, ms | other SQL call total, ms | transaction windows, ms | unassigned Apply, ms |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16 | 16 | 129.90 (130.26) | 6.36 ms | 101.732 ms / 78% | 3.296 | 1.945 | 112.88 | 17.01 |
| 64 | 4 | 38.86 (38.54) | 5.91 ms | 23.626 ms / 61% | 1.277 | 0.406 | 28.13 | 10.74 |
| 256 | 1 | 16.10 (16.28) | 6.17 ms | 6.166 ms / 38% | 0.477 | 0.048 | 7.96 | 8.14 |

Every batch has one UPDATE per row, five other SQL statements, plus BEGIN/COMMIT. BatchSize=64 has Commit as its largest directly timed component. At BatchSize=256 the unassigned Apply remainder is about half the observed Apply; it includes observer work, page reads, masking/receipt/framework work, and is not assigned solely to production. Driver call times are nested in transaction windows: do not add the columns. Ten repeats are descriptive; no significance claim is made. No tuning is authorized by these results.

### Observer perturbation

Ten randomized paired plain/instrumented SQLite Apply comparisons per batch, each with a fresh signed fixture, exact per-key output equality, Complete receipts, and independent Validate. Median paired wall-time delta (`SQLSpy`+diagnostic counters vs production SQL path) was **+0.99%** for batch 16 (range −6.21% to +9.26%), **+4.08%** for 64 (−5.59% to +5.59%), and **+1.44%** for 256 (−0.58% to +14.15%). These are noisy descriptive ranges, not a statistical correction factor. Direct driver call timers are separately measured inside the wrapper. Full raw pair rows and regenerated summary are in `observer-overhead/`.

## 10k-row workers matrix

One true SQLite Runner run for each workers=1/2/4/8/16, 10,000 rows each. All five had 157 receipts covering 10,000 rows, Complete, full independent Validate, unchanged source, and exact row-by-row output agreement with workers=1. Reader/writer connections were 0/0 after close. Workflow elapsed 1.929–1.981 s; Apply 1.447–1.489 s; throughput 5,048–5,185 rows/s. A single observation per worker count supports no speedup/significance conclusion. The sampled goroutine count was 4→5 for workers 1 and 16 and 4→4 for the other three; do not claim the snapshots alone prove no leak. The separate `TestWorkersStopAndDoNotLeak` polls after close and passes its baseline+2 bound.

## SQLite sync calls

Apply-only marker window, BatchSize=64 and four receipts, `strace -f -ttt -T -yy -e trace=fsync,fdatasync,write`: 16 successful `fsync`, 0 `fdatasync`; 8 calls associated with `target.db-journal`, 4 with its directory, 4 with `target.db`. Full Validate passed after the marker. This is strong file/timing correlation, not exclusive proof that every sync syscall was caused by an individual Commit. The marked trace wall time is perturbed and is not a benchmark.

## Verification and status

At source SHA `8ffca9b` and independent QA HEAD `c49b9ce`: `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go vet ./...`, 10-second `FuzzMaskBatchDeterminism`, Windows runner test cross-compilation, benchmark smoke, `git diff --check`, phase-2 sample validation, phase-3 artifact regeneration, and observer artifact regeneration all passed locally. The high-load measurements and their validators are listed in `test-results.md` and the environment JSON.

A push attempt to `origin` failed with `gnutls_handshake() failed: The TLS connection was non-properly terminated`. No alternative transport/retry was used. Therefore no PR, final-head GitHub CI, or merge exists; the merge gate is **not satisfied**. Issue #13 was not changed by this local work and must remain OPEN.
