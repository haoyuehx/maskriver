# Approval Guide

- `approve` signs a plan with an Ed25519 reviewer key and stores a decision.
- `apply` loads only the deployment trust file; it never receives the reviewer
  private key.
- A decision binds the plan ID, source/target/schema/config digests, strategy,
  version and key ID, plus the ordered scope list.
- Expired, revoked, future-issued (beyond 5 minutes) and foreign-plan records are
  refused.
- `--approved` is a declaration, not authorization; `--apply` is intent. Both
  plus a valid decision are required.
- The local trust file + reviewer key boundary is not RBAC: an operator holding
  both can approve their own plan.
- History files are local; whole-file rollback or revocation rollback is not
  detected. Keep externally authenticated copies of decision digests if this
  matters.
