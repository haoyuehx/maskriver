# Phase-4 baseline safety inventory (no production changes)

This is not new fault-injection evidence for each matrix run. The observed baseline trials all completed with successful commit receipts and independent Validate; tests below provide the repository's existing regression evidence. No candidate exists, so no candidate-specific delta is claimed.

- Existing regression files: `internal/runner/apply_diagnosis_test.go`, `internal/runner/workflow_acceptance_test.go`, `internal/runner/approval_test.go`, `internal/runner/workflow_concurrency_safety_test.go`, `internal/db/write_test.go`.
- They cover injected Update/Begin failures and rollback, wrapped and durable-but-unconfirmed CommitUnknown (no retry/success receipt), prior committed-prefix receipts, cancellation, preview zero-write behavior, explicit Apply intent and authenticated approval failures, worker output determinism, resource close, NULL preservation, expected-value optimistic predicate, exactly one affected row, and PK/plan column validation.
- Batch-size consequence: at 256 rows observed page/commit counts are 16/4/1 for batch 16/64/256; the maximum update rows in a single transaction grows with the configured batch cap (up to 1000). Failure rollback/unknown-commit exposure correspondingly grows. No BatchSize default or transaction boundary has changed.
- This phase did not add injected fault points for every enumerated request position, did not execute 128/256 batch × 10k × workers 1/4 repeated scale matrix, and did not measure lock duration/WAL growth or 100k rows. These remain SKIP/UNVERIFIED, not passes.
