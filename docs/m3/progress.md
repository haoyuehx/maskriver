# M3 progress

- Baseline: `b1433d5197c6f0dbf1dfff25d2894a564f76c802` (v0.2.0-preview.1)
- Milestone: M3 (#1)
- Issues: #85 (M3-1), #86 (M3-2), #87 (M3-3), #88 (M3-4), #89 (M3-5), #90 (M3-6)
- M3-1: COMPLETE, merged as `cc178aa09c32fcdb9b9ef670a52b151e62827c4c`; independent Architecture/Security/QA PASS.
- M3-2: implementation and real MySQL 8.0.46 CI E2E (`TestMySQLCLIWorkflow`) passing on PR #93. Independent architecture/security/QA rechecks PASS; final code head `0f3c380`; merge SHA recorded in the post-merge M3-3 update.
- Current: merge #93, then M3-3 UX/docs, M3-4 independent holdout, M3-5 cross-platform and M3-6 v1.0.0.
- M3-1 delivered on branch `m3/security-quality`: real strict full-scan + adapter integrity checks, Writer NULL-replacement refusal, approval clock-skew alignment, docs/tests.
- Tests: full `go test -count=1 ./...` PASS; M3-1 race and independent reviews PASS; M3-2 CI includes Go race, Windows and isolated MySQL 8.0.46.
- Known limits: local Docker is permission-blocked, so MySQL evidence comes from the authorized GitHub Actions isolated container. MySQL streaming/recovery remains unsupported. No v1.0.0 claim until M3-3/M3-4/M3-5/M3-6 pass.
- Next: merge #93, then M3-3; M3-4/M3-5/M3-6 remain.
