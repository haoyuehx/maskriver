# Migration Guide

v0.2.1-preview keeps `APIRevision=m1a-v1`, `m2-stream-v1`,
`m2-approval-binding-v1` and `m2-recovery-v1`. No migration of existing v0.1/v0.2
databases or approvals is required.

Changes to account for:

- `--strict` now performs a full scan and adapter integrity checks; a bounded
  `RowLimit` is refused in strict mode.
- Direct `internal/db` Writer callers can no longer replace a non-NULL value with
  NULL.
- MySQL CLI is available with `--dialect mysql` and two DSNs. MySQL strict is
  implemented; MySQL streaming/recovery remains unsupported and is refused.
- Windows/macOS streaming/recovery remain unsupported.
