# M3 platform matrix

| Platform | M1 CLI | Streaming | Recovery | Status |
|---|---|---|---|---|
| Linux amd64 | verified (real SQLite + MySQL 8.0.46 E2E) | verified (SQLite only) | verified (SQLite only) | supported |
| Windows amd64 | CI full non-streaming suite | not supported | not supported | M1 via CI; streaming/recovery UNVERIFIED/unsupported |
| macOS arm64/amd64 | not run | not supported | not supported | UNVERIFIED; no CI/runtime available |
| Linux arm64/other | not built | not supported | not supported | UNVERIFIED/unsupported |

Cross-compilation passing is not runtime acceptance. Only Linux amd64 is
released. MySQL CLI is Linux CI-verified on the pinned 8.0.46 image; MySQL
streaming/recovery and `m2-recovery-v1` on MySQL are not implemented.
