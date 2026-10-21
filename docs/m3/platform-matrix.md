# M3 platform matrix

| Platform | M1 CLI | Streaming | Recovery | Status |
|---|---|---|---|---|
| Linux amd64 | verified (real SQLite E2E) | verified | verified | supported |
| Windows amd64 | CI full non-streaming suite | not supported | not supported | M1 via CI; streaming/recovery UNVERIFIED/unsupported |
| macOS arm64/amd64 | not run | not supported | not supported | UNVERIFIED; no CI/runtime available |
| Linux arm64/other | not built | not supported | not supported | UNVERIFIED/unsupported |

Cross-compilation passing is not runtime acceptance. Only Linux amd64 is
released. `m2-recovery-v1` is not extended to other platforms.
