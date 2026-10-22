# M3 risk register

| ID | Risk | State |
|---|---|---|
| R1 | `--strict` was inert | Open -> ADR-M3-1 |
| R2 | `internal/db` accepts non-NULL -> NULL | FIXED (M3-1) |
| R3 | MySQL CLI not implemented/verified | FIXED (M3-2; real 8.0.46 CI E2E; recovery still unsupported) |
| R4 | MySQL recovery not supported | Accepted/Unsupported |
| R5 | Persistent mapping not supported | Accepted/Unsupported |
| R6 | Windows/macOS streaming/recovery runtime unverified; macOS/Windows non-streaming M1 is CI-verified | Narrowed (M3-5) |
| R7 | History rollback/revocation rollback undetectable | Accepted P2 |
