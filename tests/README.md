# Integration test reservation

Current executable tests are colocated with `cmd/maskriver`, `internal/config`, `internal/testenv` and `pkg/contracts`.
This directory will hold cross-module SQLite/MySQL integration tests and synthetic fixtures.

No integration test exists yet: the database adapters are not implemented.
See [test plan](../docs/test-plan.md). Never use real personal data or committed credentials.
