# M3 acceptance matrix

| Work package | Status |
|---|---|
| M3-1 security & quality | COMPLETE (merged cc178aa; independent Arch/Security/QA PASS) |
| M3-2 MySQL CLI | BLOCKED: no MySQL CLI implementation; MySQL strict UNVERIFIED |
| M3-3 UX/docs | PARTIAL: quickstart/tutorials/guides added; CLI UX unchanged |
| M3-4 detection/masking evaluation | PARTIAL: existing 74-case synthetic metrics recorded; independent holdout NOT COMPLETED |
| M3-5 cross-platform | PARTIAL: Linux verified; Windows M1 via CI; macOS/Windows streaming UNVERIFIED |
| M3-6 stable release | BLOCKED: v1.0.0 requirements not met; v0.2.1-preview only |

## M3-1 detail

- Strict full-scan refusal: PASS (`verify` tests).
- Strict integrity capability/failure: PASS for SQLite (`verify` + database-wide FK test).
- MySQL strict integrity: UNVERIFIED/unsupported (fail closed).
- Writer non-NULL -> NULL refusal: PASS (`TestWriteTxRefusesNonNullToNullReplacement`).
- Approval clock skew aligned: PASS (runner tests + 5-minute bound).
- Independent reviews: pending.
