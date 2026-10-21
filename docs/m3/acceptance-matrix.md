# M3 acceptance matrix

| Work package | Status |
|---|---|
| M3-1 security & quality | In progress |
| M3-2 MySQL CLI | Not started |
| M3-3 UX/docs | Not started |
| M3-4 detection/masking evaluation | Not started |
| M3-5 cross-platform | Not started |
| M3-6 stable release | Not started |

## M3-1 detail

- Strict full-scan refusal: PASS (`verify` tests).
- Strict integrity capability/failure: PASS for SQLite (`verify` + database-wide FK test).
- MySQL strict integrity: UNVERIFIED/unsupported (fail closed).
- Writer non-NULL -> NULL refusal: PASS (`TestWriteTxRefusesNonNullToNullReplacement`).
- Approval clock skew aligned: PASS (runner tests + 5-minute bound).
- Independent reviews: pending.
