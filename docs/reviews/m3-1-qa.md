# M3-1 QA review

- **Head:** `b61e76b` (merged as `cc178aa`)
- **Context:** independent read-only subagent, model `opencode-go/deepseek-v4.1-flash`.
- **Verdict:** PASS for SQLite scope; no P0/P1.

Executed `gofmt`, `go vet`, full `go test -count=1 ./...`, and race for
`internal/db`, `internal/runner`, `internal/verify`, `cmd/maskriver`. Mutation
probes confirmed removing the strict RowLimit refusal, the FK violation check or
the IntegrityCheck capability makes the new tests fail. Non-strict behavior is
unchanged.

UNVERIFIED: MySQL strict E2E (no local MySQL/Docker; CI adapter job does not
assert IntegrityCheck); full repository race was run in CI only; Windows/macOS
runtime streaming/recovery.
