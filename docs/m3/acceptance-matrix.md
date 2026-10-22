# M3 acceptance matrix

| Work package | Status |
|---|---|
| M3-1 security & quality | COMPLETE (merged cc178aa; independent Arch/Security/QA PASS) |
| M3-2 MySQL CLI | COMPLETE after merge: real 8.0.46 CI E2E Scan/Preview/Approve/Apply/Strict Validate; streaming/recovery unsupported |
| M3-3 UX/docs | COMPLETE after merge: `help <command>`, examples, guide index, troubleshooting/operations runbooks |
| M3-4 detection/masking evaluation | COMPLETE after merge: 345-case generated holdout (210 positive/135 negative) + 100 values for five masking formats; external/human adjudication still not claimed |
| M3-5 cross-platform | COMPLETE after merge: Linux runtime+MySQL CI; Windows/macOS non-streaming M1 CI; Linux cross-builds; streaming/recovery unsupported |
| M3-6 stable release | RELEASE CANDIDATE: v1.0.0 PR; final CI/review/tag/asset verification pending |

## M3-1 detail

- Strict full-scan refusal: PASS (`verify` tests).
- Strict integrity capability/failure: PASS for SQLite (`verify` + database-wide FK test).
- MySQL strict integrity: implemented in M3-2 as CHECK TABLE ... QUICK status rows plus visible-FK orphan scans; verified on 8.0.46 CI. Not a whole-server consistency check.
- Writer non-NULL -> NULL refusal: PASS (`TestWriteTxRefusesNonNullToNullReplacement`).
- Approval clock skew aligned: PASS (runner tests + 5-minute bound).
- Independent reviews: PASS (Architecture, Security, QA); records in `docs/reviews/m3-1-architecture.md`, `m3-1-security.md`, `m3-1-qa.md`.
