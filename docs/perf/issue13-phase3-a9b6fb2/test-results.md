# Verification results

Code SHA tested: `a9b6fb267a414b83f4fa10a758de504843763bf0`. Host: Linux x86_64, kernel `7.0.0-38-generic`; Go `go1.26.3`; AMD Ryzen 9 7940H (16 CPU cores); host MemTotal 15,187 MiB (not container limit).

| Command | Result |
|---|---|
| `gofmt` on changed Go files | PASS (formatted before commit) |
| `go test -count=1 ./...` | PASS, exit 0 |
| `go test -race -count=1 ./...` | PASS, exit 0 |
| `go vet ./...` | PASS, exit 0 |
| `go test -run '^$' -fuzz '^FuzzMaskBatchDeterminism$' -fuzztime=10s -parallel=2 ./internal/runner/` | PASS, 10-second fuzz run, exit 0 |
| `GOOS=windows go test -c -o /tmp/maskriver-phase3-final.test.exe ./internal/runner` | PASS, Windows test binary cross-compiled; not executed on Windows |
| `go test -run '^$' -bench '^BenchmarkWorkflow/rows=128/shape=short/workers=1$' -benchmem -benchtime=2x ./internal/runner/` | PASS, benchmark smoke, exit 0 |
| `git diff --check` | PASS, exit 0 |

Separately, the 30-trial phase-3 experiment, its artifact consistency test, the 10k × five-worker run, and the marked Apply-only strace test each passed; exact commands and raw evidence are captured alongside this file. Windows execution, non-Linux resource measurement, MySQL performance, and CLI E2E performance remain UNVERIFIED.
