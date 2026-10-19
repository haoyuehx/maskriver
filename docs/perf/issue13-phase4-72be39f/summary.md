# Issue #13 Phase 4 — controlled Apply optimization investigation

## Decision

**Verdict: INSUFFICIENT EVIDENCE.** No production optimization was selected or shipped. No production code changed. Candidate Prepared Statement reuse is not supported by evidence that production preparation is repeated or material. BatchSize changes transaction size, confirmed commits, cancellation/lock window, and failure blast radius; existing timing does not justify changing a default. Keep Issue #13 OPEN and do not start #14.

## Baseline findings

At baseline source `72be39f0ddc7a68b4e37796ecff7bd8086b744bd`, the randomized 256-row instrumented matrix included batch 128 and used 10 independent SQLite fixtures per batch 16/64/128/256, workers=4. All 40 runs reported Complete, exact receipts/rows, and independent Validate. See `samples.json`, `baseline-results.csv`, `apply-cost-breakdown.csv`, `environment.json`.

Median Apply was 124.23 ms (batch 16), 38.31 ms (64), 24.24 ms (128), and 16.38 ms (256); successful commits were 16, 4, 2, and 1 per trial. There were 256 UPDATE calls in each trial. This demonstrates fewer commits at larger explicit batch sizes, not a candidate production optimization. Apply and component timings include SQLSpy/diagnostic instrumentation. Paired plain-vs-instrumented comparisons were noisy: median deltas -1.78%, -0.03%, -0.42%, with ranges crossing zero for all sizes. See raw pairs in `observer-paired-samples.csv`; they are not a correction factor.

Rows=2048 is **SKIP**. The 10k crossed batch64 × workers1/4 was repeated twice as single observations, not ten repeats; 10k × batch128/256 is **SKIP**. Each observation in the worker matrix is one sample and is not statistical evidence. 100k is **SKIP**. No estimates are extrapolated.

## 10k results and resources

Real SQLite Runner runs at 10000 rows and batch64, workers1/4, each had 157 receipts, 10000 rows, unchanged source, exact per-key agreement, full Validate, and pools closed to 0/0. Across two runs, Apply ranged 1445–1477 ms (roughly 6.77k–6.92k Apply rows/s); workers1 Apply was 1445–1455 ms and workers4 1463–1476 ms, with no demonstrated improvement. RSS snapshots are process-level and noisy; they do not establish peak memory. Resource details are in `resource-results.csv`.

## Safety and limitations

All sampled workflows used synthetic `@example.invalid`, fake_email, isolated real SQLite source/target, and genuine Ed25519 approval. Existing regression tests cover invalid approvals, preview zero writes, rollback, cancellation, durable/uncertain Commit semantics, and receipt accuracy; see `transaction-safety-results.md`. This report does not claim every requested failure position was newly injected in phase 4. WAL/journal size, lock duration, cancellation latency by batch, MySQL/CLI performance, and 100k rows were not measured. Historical phase-3 fsync evidence is not re-attributed to this run. No statement-cache optimization was implemented, so resource/leak or semantic claims about such an optimization are inapplicable.

No changed production behavior, contract, dependency, default batch, worker setting, approval, journal/synchronous setting, or release artifact. No #13 acceptance completion claim.
