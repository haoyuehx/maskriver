# Issue #13 Phase 4 — candidate analysis

Baseline/report commits: see `environment.json`; measurements were performed from base source `72be39f0ddc7a68b4e37796ecff7bd8086b744bd`. The test-only matrix extension adding 128 is committed separately as `1f71a05aadd46e5467a00a96cbf99dc0eddcce2b`.

## Apply path and safe optimization boundary

`internal/runner/execute.go` runs approval authentication, static preflight, and full read-only dry-run before opening a writer. `applyTable` compares source/target pages and `writeBatch` opens one WriteTx per nonempty page. Receipts are appended only after successful Commit. Commit errors become CommitUnknown, stop execution, and are never replayed; other failure paths attempt bounded rollback. Prior confirmed batches remain receipted after a later failure.

`internal/db/write.go:85-95` Begin re-describes the live schema/validates plan before `BeginTx`. Update validates each row, discovers assigned columns and builds parameterized optimistic UPDATE (`:100-145,203-240`); the WHERE compares PK and all planned expected values (`IS ?` on SQLite, `<=> ?` on MySQL). It checks exactly one affected row. NULL-only rows issue no UPDATE. Commit/Rollback close transaction state (`:243-286`).

Runner preflight rejects triggers, modified UNIQUE/CHECK/FK columns and incoming references (`internal/runner/execute.go:220-470`). These gates, approval, dry-run and optimistic source/target comparisons must not be cached or bypassed.

Each transaction page repeats schema validation at Begin; each changed row has one UPDATE statement and parameter list. The phase-4 repeated SQLSpy observation shows 256 UPDATEs at each size and 16/4/2/1 Commit receipts for batch sizes 16/64/128/256. It does not establish native production Prepare counts or preparation time; SQLSpy's wrapper can introduce Prepare fallback. Thus no evidence justifies prepared statement caching/reuse. Larger BatchSize lowers commit count but expands transaction duration and per-batch rollback/unknown-commit/cancellation/lock exposure. No change to production BatchSize/default or boundaries is justified by this descriptive sample.

## Evidence status

The 40 raw 256-row trials in `samples.json` are 10 independent real-SQLite fixtures per batch 16/64/128/256, workers=4, randomized order, synthetic fake_email, real Ed25519 approval and independent Validate. Fixture timing is outside workflow/Apply timing. Aggregates are reproducible with `TestPhase3ArtifactConsistency` against these samples. `baseline-results.csv` and other aggregates derive from this four-size run. It has a fresh transaction-evidence run, not uninstrumented production throughput; the paired SQLSpy comparison samples are in `observer-paired-samples.csv`.

10k rows at batch64 was performed as three one-off worker matrices; `10k-workers.json`/`10k-workers.csv`, `10k-workers-run2.json`/`10k-workers-run2.csv`, and `10k-workers-run3.json`/`10k-workers-run3.csv` retain each distinct run. All runs had correct row receipts, source unchanged, per-key agreement, independent Validate, and closed connections. Not 10 repetitions per configuration; not statistically conclusive.

Rows=2048, 10k batch128/256, 100k, MySQL/CLI, Windows runtime, peak RSS, cancellation timing by batch, lock duration, WAL/journal growth, and candidate-code A/B are UNVERIFIED/SKIP. No interpolation is used.

## Decision

No production optimization was selected or implemented. Prepared-statement reuse lacks cost proof; batch-size increases are a transaction safety tradeoff rather than a production optimization automatically licensed by this result. **Verdict: INSUFFICIENT EVIDENCE.** Issue #13 remains OPEN; do not start #14.
