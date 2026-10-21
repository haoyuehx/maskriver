# Issue #14 WP0 second-context security recheck

- **Reviewer context:** fresh independent subagent (`delegate`), model
  `opencode-go/deepseek-v4.1-flash`, read-only.
- **Reviewed:** implementation @ `09612f8` + design parent `86ec313`.
- **Verdict:** **PASS — no P0/P1.**

## P1 closure verified

The manifest now stores `key_check`, a KCV computed as
`HMAC-SHA256(key, "maskriver-recovery-v1/key-check\0" || key_id)`. It is required
to be 64 lowercase hex, is verified in `verifyManifestBinding` before
`SetManifestState(running)` and before any business transaction, and a wrong key
with the same `KeyID` is refused with `ErrUnsafe` even when the committed prefix
is empty. Regression test:
`TestRecoveryResumeRefusesWrongKeyBeforeFirstBatch` (target unchanged). A
recomputable-strategy allowlist is enforced for resume and before manifest
creation.

## Other controls re-verified

- Manifest/progress contain identifiers, digests, counts, states and timestamps
  only; no values, plaintext PKs, keys, seeds or DSNs.
- Approval is loaded from the signed store and re-verified (signature, trust,
  expiry, future clock, revocation, canonical content digest, exact approval
  equality) before writes; revoked/expired resume refuses with exit 8.
- External target changes are never overwritten: prefix/suffix recomputation,
  per-page source==target comparison, and optimistic expected-value UPDATEs. The
  late-tamper test proves the external value survives.
- Ambiguous commit is never retried from the error; the authoritative progress
  and receipts are re-read, and a committed batch is skipped.
- Double masking is prevented by recomputing from the immutable source and
  skipping the committed prefix.
- Terminal-state CAS and monotonicity are enforced; a stale finaliser gets
  `ErrConflict`.
- `status` opens `mode=ro`, runs no DDL and no session-PRAGMA write. The
  read-only test now also asserts no `-journal`/`-wal`/`-shm` sidecars appear.
- The recovery migration never alters outbox v1; a partial/wrong shape fails
  closed. `pkg/contracts`, `m1a-v1`, `m2-stream-v1` and the release are unchanged.

## P2 findings and disposition

1. Allowlist only on resume -> now also enforced before manifest creation on a
   fresh run.
2. `resume_eligible=true` for `validation_failed` -> eligibility now excludes
   `validation_failed` in both the runner report and `status`.
3. KCV is an offline oracle; key strength is only length-checked -> documented:
   the key file must be a CSPRNG-generated >=32-byte secret, not a passphrase,
   and the KCV travels with the masked artifact.
4. `business_complete` not bound to the final table ordinal -> resume now
   rejects `business_complete` before the last planned table and the check is
   unit-tested.
5. Read-only evidence only hashed the main DB file -> the test now also asserts
   no journal/sidecar files appear.
6. `init-recovery` could add tables to an arbitrary SQLite file -> it now
   requires an already-provisioned outbox v1 schema; operators run `init-outbox`
   first, and the CLI tests do the same.
7. Dangling evidence paths -> this file and the final design status line are
   committed with the acceptance evidence.
