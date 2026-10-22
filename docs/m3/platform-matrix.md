# M3 platform matrix

| Platform | M1 CLI | Streaming | Recovery | Status |
|---|---|---|---|---|
| Linux amd64 | verified (real SQLite + MySQL 8.0.46 E2E) | verified (SQLite only) | verified (SQLite only) | supported |
| Windows amd64 | CI full non-streaming suite | not supported | not supported | M1 via CI; streaming/recovery UNVERIFIED/unsupported |
| macOS arm64 | CI full non-streaming suite (GitHub `macos-latest`) | not supported | not supported | M1 via CI; streaming/recovery unsupported |
| macOS amd64 | cross-build only | not supported | not supported | built, runtime UNVERIFIED |
| Linux arm64/other | cross-build only | not supported | not supported | built, runtime UNVERIFIED |

CI runs `gofmt`, `go test ./...`, and `go vet ./...` on Ubuntu, Windows and
macOS; Linux additionally runs the full race suite. The Linux job cross-builds
darwin/arm64, darwin/amd64, windows/amd64 and linux/arm64, but cross-build is not
runtime acceptance. Only Linux amd64 is released. MySQL CLI is Linux
CI-verified on the pinned 8.0.46 image; MySQL streaming/recovery and
`m2-recovery-v1` on MySQL are not implemented. Windows/macOS runtime streaming
and recovery remain unsupported.
