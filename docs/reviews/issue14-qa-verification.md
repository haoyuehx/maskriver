# Issue #14 independent QA verification

- **QA context:** fresh independent subagent (`delegate`), model
  `opencode-go/deepseek-v4.1-flash`, read-only (verification harness ran in a
  throwaway copy; repo untouched).
- **Environment:** Linux x86_64, `go version go1.26.3 linux/amd64`.
- **Verified implementation head:** `09612f8` (review-fix delta `e9ae004`
  re-checked separately).
- **Verdict:** **PASS — no P0/P1.**

## Static gates

- `gofmt -l cmd internal pkg tools` -> no output, PASS.
- `go vet ./...` -> PASS.
- `go build ./...` -> PASS.

## Package and focused suites

```text
go test -count=1 ./internal/db/ ./internal/runner/ ./cmd/maskriver/
ok internal/db        21.174s
ok internal/runner    64.002s
ok cmd/maskriver      33.165s
```

- `go test -run 'TestRecovery' ./internal/db/` -> 11/11 PASS: schema
  install/idempotent/version, reserved names, same-transaction batch,
  checkpoint-failure rollback, commit-unknown after a real commit, no-op
  progress, tampered progress visible (not repaired), legacy outbox, schema
  shape tamper, read-only reader, manifest-state CAS.
- `go test -run 'TestRecovery' ./internal/runner/` -> 15/15 PASS: resume
  idempotency, NULL/multi-table, commit-unknown reconcile, tampered target,
  changed source/settings, revalidate after interrupted validation,
  crash-before/after-commit, all-NULL page, validation-failed never complete,
  fixed-point refusal, wrong key, late external change, non-recoverable
  strategy, manifest ahead of progress.
- `go test -run 'TestE2ERecoveryCLI' ./cmd/maskriver/` -> PASS: real child
  processes at 1/10/100/500 batches, after-validate, before-commit, and refusal
  paths.
- `go test -race -count=1 ./internal/db/ ./internal/runner/` -> PASS
  (30.1 s / 72.0 s).

## Independent observation (real CLI, `/tmp/mrqa`)

- Interrupted apply: exit 70, one committed batch (manifest=1, progress=1,
  receipts=1).
- `status`: exit 0, `state=running`, checkpoint 1, 2 rows committed,
  `resume_eligible=true`, `commit_unknown=false`, `conflict=false`; target bytes
  unchanged by `status`.
- `resume`: exit 0, `state=complete validation=passed`, 6 rows, 3 batches,
  `audit=consistent`.
- `validate --strict`: exit 0, `pass=true tables=1 rows=6 issues=0`.
- A/B masked values byte-identical to an uninterrupted baseline; no double
  masking.
- Second `resume`: exit 0, target bytes unchanged, receipt/progress counts
  unchanged.
- Final `status`: `state=complete`, `validation=passed`, `audit=complete`.

## Claim checks

- No double masking: proven by A/B value comparison and unchanged receipts on
  repeat resume.
- Commit-unknown not retried: DB real-commit-then-error test plus runner
  reconciliation test assert the committed batch is skipped.
- No-op/NULL progress: progress-only events write no receipts; boundary sets
  `business_complete`; post-complete writes are refused.
- Manifest/checkpoint atomicity: same-transaction test; checkpoint failure rolls
  back business rows, receipt and progress; stale CAS/terminal tests.
- Status read-only: file hash and (after the P2 fix) sidecar checks, CLI digest
  observation.

## Residual P2 (accepted)

- A resume prints the plan hash of its own run; an A/B baseline on a different
  target path has a different `Plan.ID` because `TargetID` is the target path
  digest. Cosmetic observability only; content binding and validation passed.
- Fault injection is installed only by test code; no test asserts a production
  binary ignores `MASKRIVER_TEST_FAULT` (read-verified only).
- The opt-in scale matrix is evidence, not a CI gate.

## Review-fix delta

`e9ae004` is the P2 follow-up (allowlist before manifest, eligibility,
final-ordinal guard, `init-recovery` requires outbox, extra tests). A fresh QA
recheck executed `TestRecovery` (DB/runner), `TestE2ERecoveryCLI` and the whole
`cmd/maskriver` package at `e9ae004`: all PASS; the new final-ordinal, sidecar and
eligibility tests genuinely ran. The recheck found one environment-gated
regression: the opt-in scale harness still called `init-recovery` without
`init-outbox`, so the evidence path failed when enabled while the normal package
run skipped it. `e1f2242` fixes the harness to provision outbox first (re-run at
100 rows: PASS, A/B digest equal, strict validate PASS) and updates two stale
comments. CI on `e1f2242` covers Go ubuntu/windows, race, isolated MySQL and the
benchmark workflow.
