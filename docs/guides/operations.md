# Operations

## Provisioning

- Use dedicated, isolated, disposable databases or file copies. MySQL should use
  a least-privilege account; the source account needs SELECT and metadata
  visibility, the target needs UPDATE on planned columns. The tool adds a
  read-only session guard, but database grants remain the primary control.
- Keep the masking key, reviewer Ed25519 private key, deployment trust file and
  history directory outside the database and outside the release package.
- Record the tag/commit and archive SHA-256 used for every run.

## Backup and rollback

- Take an independent backup before Apply. MaskRiver does not provide backup,
  restore, cross-store rollback or exactly-once recovery.
- SQLite streaming recovery can resume a known deterministic run, but it does
  not undo committed batches. MySQL has no recovery protocol and no streaming
  support.
- A whole-history-directory rollback or a revoked-record rollback is not
  detected. Keep externally authenticated copies of decision digests if this
  matters.

## Approval hygiene

- Issue approvals in a separate process from Apply when possible; the Apply
  process reads the trust file and a read-only decision store, never the
  reviewer private key.
- Revoke decisions that are no longer valid. Expiry, revocation, foreign plan
  and future-issued checks are enforced, but the local signature boundary is
  not RBAC.

## Logging and evidence

- CLI output contains counts and status only. Do not enable verbose driver or
  shell tracing that prints DSNs/row values.
- Preserve the sanitized command, exit code, CI run/head SHA and archive SHA-256
  as release evidence. The M1 CLI does not print a plan ID; if needed, record
  the plan ID as stored in the history directory, not from stdout.
- Report MySQL evidence separately from SQLite; the pinned 8.0.46 CI image is
  not a statement about other versions.
