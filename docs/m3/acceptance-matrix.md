# M3 acceptance matrix

| Work package | Status |
|---|---|
| M3-1 security & quality | COMPLETE (merged cc178aa; independent Arch/Security/QA PASS) |
| M3-2 MySQL CLI | COMPLETE after merge: real 8.0.46 CI E2E Scan/Preview/Approve/Apply/Strict Validate; streaming/recovery unsupported |
| M3-3 UX/docs | COMPLETE after merge: `help <command>`, examples, guide index, troubleshooting/operations runbooks |
| M3-4 detection/masking evaluation | COMPLETE after merge: 210-case generated holdout + 100-value-per-format masking invariants; external/human adjudication still not claimed |
| M3-5 cross-platform | PARTIAL: Linux verified; Windows M1 via CI; macOS/Windows streaming UNVERIFIED |
| M3-6 stable release | BLOCKED: v1.0.0 requirements not met; v0.2.1-preview only |

## M3-1 detail

- Strict full-scan refusal: PASS (`verify` tests).
- Strict integrity capability/failure: PASS for SQLite (`verify` + database-wide FK test).
- MySQL strict integrity: implemented in M3-2 as CHECK TABLE ... QUICK status rows plus visible-FK orphan scans; verified on 8.0.46 CI. Not a whole-server consistency check.
- Writer non-NULL -> NULL refusal: PASS (`TestWriteTxRefusesNonNullToNullReplacement`).
- Approval clock skew aligned: PASS (runner tests + 5-minute bound).
- Independent reviews: pending.
