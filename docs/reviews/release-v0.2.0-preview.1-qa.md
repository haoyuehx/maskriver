# v0.2.0-preview.1 — independent QA review

- **Reviewer context:** fresh read-only subagent, model `opencode-go/deepseek-v4.1-flash`.
- **Reviewed candidate:** `7266a540f41e544954316ae87024108a9b2d5775`, package
  `/tmp/candidate/maskriver-v0.2.0-preview.1-linux-amd64`.
- **Verdict:** **CHANGES REQUIRED** (narrow, no data-safety violation), then fixed.

## Results

- `gofmt`, `go vet`, `go test -count=1 ./...` PASS.
- `go test -race -count=1 ./internal/db/ ./internal/runner/` PASS.
- Clean clone `scripts/test-release.sh` PASS; two builds byte-identical.
- Package `sha256sum -c` PASS; binary byte-identical to a clean rebuild; VCS
  commit/version/platform match.
- M1, M2 streaming, status, resume, strict validate, A/B equality, idempotent
  second resume and source immutability PASS.
- Independent interruption harness: real SIGKILL with process alive, partial
  commits, resume to complete/passed PASS.

## Findings (fixed)

1. **Concurrent read-only status could abort an in-flight apply.** Without a
   SQLite busy timeout, the single writer's `Commit` could return
   `SQLITE_BUSY`, classified as `CommitUnknown`; each of 8/8 status-reader
   trials aborted (no data loss, resume reconciled). Fix: bounded
   `busy_timeout(5000)` on SQLite connections.
2. **Release smoke was flaky** because of (1) and because it did not assert the
   process was alive before SIGKILL. Fix: batch size 1, 2000 rows, 5 ms polling,
   alive-before-kill and `-9` assertions.

After the fixes, the smoke passed 5/5 consecutive runs. A second-context QA
recheck is recorded in the release acceptance comment.

## Second-context recheck

A fresh targeted QA recheck at `208fd23`/`82d8948` passed: release package
build, five smoke runs, deterministic hot-journal status/resume, concurrent
`status` during a live apply (status rc=0, no CommitUnknown), strict validate,
and idempotent second resume. The earlier P1 (smoke nondeterminism from a
transient reader lock) and the hot-journal false-positive status were both
fixed. The committed regression tests for the hot-journal reader/recovery and
status exit code were added and pass.

**Recheck verdict: PASS.**
