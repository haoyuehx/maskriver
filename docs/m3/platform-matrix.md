# M3 platform matrix

| Platform | M1 CLI | Streaming | Recovery | Status |
|---|---|---|---|---|
| Linux amd64 | verified (real SQLite + MySQL 8.0.46 E2E) | verified (SQLite only) | verified (SQLite only) | supported |
| Windows amd64 | CI full non-streaming SQLite/M1 suite | not supported | not supported | SQLite M1 via CI; streaming/recovery unsupported |
| macOS arm64 | CI full non-streaming SQLite/M1 suite (GitHub `macos-latest`, observed arm64) | not supported | not supported | SQLite M1 via CI; streaming/recovery unsupported |
| macOS amd64 | cross-build only | not supported | not supported | built, runtime UNVERIFIED |
| Linux arm64 | cross-build only | not supported | not supported | CLI binary built, runtime UNVERIFIED |
| Linux other arches | not built | not supported | not supported | unsupported/UNVERIFIED |

CI runs `gofmt`, `go test ./...`, and `go vet ./...` on Ubuntu, Windows and
macOS; Linux additionally runs the full race suite. The Linux job cross-builds
the `maskriver` CLI binary for darwin/arm64, darwin/amd64, windows/amd64 and
linux/arm64, and emits `go env GOOS GOARCH` for the test runner, but cross-build
is not runtime acceptance. The macOS row was observed as arm64 on
`macos-latest`; if GitHub changes that label, the row must be re-evaluated. Only Linux amd64 is released. MySQL CLI is Linux
CI-verified on the pinned 8.0.46 image; MySQL streaming/recovery and
`m2-recovery-v1` on MySQL are not implemented. Windows/macOS runtime streaming
and recovery remain unsupported.
