# M3 progress

- Baseline: `b1433d5197c6f0dbf1dfff25d2894a564f76c802` (v0.2.0-preview.1)
- Milestone: M3 (#1)
- Issues: #85 (M3-1), #86 (M3-2), #87 (M3-3), #88 (M3-4), #89 (M3-5), #90 (M3-6)
- M3-1: COMPLETE, merged as `cc178aa09c32fcdb9b9ef670a52b151e62827c4c`; independent Architecture/Security/QA PASS.
- Current: preparing v0.2.1-preview. M3-2 MySQL CLI, M3-4 independent holdout, M3-5 macOS/Windows streaming and M3-6 v1.0.0 are not complete.
- M3-1 delivered on branch `m3/security-quality`: real strict full-scan + adapter integrity checks, Writer NULL-replacement refusal, approval clock-skew alignment, docs/tests.
- Tests: full `go test -count=1 ./...` PASS; race and independent reviews pending.
- Known blockers: no local MySQL/Docker; M3-2 and MySQL strict E2E must run in CI or remain UNVERIFIED. No v1.0.0 claim until M3-2/M3-5/M3-6 pass.
- Next: push PR #85, independent Architecture/Security/QA review, merge after CI; then decide M3-2 feasibility.
