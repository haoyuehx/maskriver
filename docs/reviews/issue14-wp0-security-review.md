# Issue #14 WP0 security review (independent context)

- **Reviewer context:** fresh independent subagent (`delegate`), model
  `opencode-go/deepseek-v4.1-flash`, read-only.
- **Reviewed:** `docs/design/issue14-recovery-protocol.md` @ `2251823`.
- **Baseline:** `main` `aee0b801c738b89a6267e06c575c95878f785c7f`.
- **Verdict:** **CHANGES REQUIRED** (no P0; one P1).

## Verified controls

- Prefix/suffix recomputation is a strengthening of the existing dry run, not a
  gate reduction.
- Manifest/progress carry identifiers, digests, ordinals, counts and timestamps
  only; no original values, replacements, plaintext PKs, keys or DSNs.
- `TargetID` is a path digest, so installing recovery DDL does not invalidate the
  plan/approval identity.
- Business UPDATE + receipt + progress share one SQLite transaction; the
  optimistic `Expected` compare and exactly-one-row rule are preserved.
- The design correctly does not claim generic exactly-once or cross-store
  atomicity.

## P1 finding

**Key material was not bound.** The manifest stored only `key_id`, and the
prefix/suffix verification is vacuous when `rows_processed = 0`. A wrong key file
with the same approved `KeyID` could therefore resume a run that had not yet
committed anything, irreversibly writing replacements derived from unapproved key
material.

**Fix:** add a key-check value (KCV) `HMAC-SHA256(key, "maskriver-recovery-v1/key-check\0" || key_id)`
to the manifest and verify it before any write. The reviewer accepted the KCV
offline-oracle trade-off for a high-entropy >=32-byte key; the design documents
it as a non-claim.

## P2 findings

- Scope "any divergence" to planned columns plus row/key correspondence.
- Keep the optimistic `Expected` compare as a hard protocol invariant.
- CAS/monotonic terminal state updates.
- Enumerate manifest/outbox-run divergence and progress/receipt tamper cases.
- State that `status` uses SQLite `mode=ro`.
- State that the recovery transaction adds two statements (protocol unchanged).
- Document retention of recovery metadata in the masked artifact.
- State the one-shot approval-validity check limitation.
- Gate recovery on a recomputable-strategy allowlist.
- Do not call the document "FROZEN" before independent review.

## P1 disposition

Applied with the KCV field, allowlist and additional tests; disposition verified
in `issue14-wp0-security-recheck.md`.
