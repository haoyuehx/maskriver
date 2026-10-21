# Security Model

- Default preview is read-only; business writes need explicit `--apply` plus a
  valid Ed25519 approval bound to the exact plan.
- Source is opened read-only; target must be a distinct file. SQLite WAL sources
  are refused by the snapshot identity gate.
- Masking is deterministic with a keyed HMAC. It is not anonymisation and does
  not guarantee collision-free or unlinkable output.
- Local trust/key files are a local signature boundary, not RBAC. An operator
  controlling both can approve their own plan.
- Recovery is local SQLite only and supports deterministic strategies. It does
  not provide distributed exactly-once or cross-store atomicity.
- Strict validation is full-scan and requires adapter integrity checks; MySQL
  strict is unsupported and fails closed.
- Logs/reports contain counts and controlled status only, not raw values, keys,
  DSNs or plaintext primary keys.
