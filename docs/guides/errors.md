# Error Reference

CLI exit codes: `0` success/complete, `1` refused/incomplete, `2` usage or
configuration, `3` recoverable/cancelled/hot journal, `4` conflict, `5`
commit-unknown, `6` unsupported, `7` invalid/missing manifest, `8` unauthorized,
`9` validation failed.

Key categories: `ErrInvalid` (malformed request), `ErrUnsafe` (unsafe state or
unauthorized write), `ErrUnsupported` (dialect/feature not implemented, e.g.
MySQL strict, MySQL recovery), `ErrConflict` (target/source drift or integrity
violation), `ErrCommitUnknown` (ambiguous commit; never auto-retried),
`ErrIncomplete` (coverage/validation incomplete), `ErrNotFound` (missing record
or run), `ErrClosed`.

A refusal is fail-closed. Do not retry commit-unknown automatically; reopen and
reconcile the authoritative database.
