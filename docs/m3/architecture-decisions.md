# M3 architecture decisions

## ADR-M3-1: Strict validation is database-backed and full-scan only

`--strict` requires (a) `RowLimit == 0` (a bounded run is refused, not silently
treated as complete), and (b) an adapter-provided integrity check on every
planned table on both source and target. The integrity check is an optional
internal capability, not a new frozen `pkg/contracts` interface; an adapter
without it fails closed in strict mode. Non-strict validation keeps the existing
bounded semantics. This creates a real, testable difference and never turns
incomplete coverage into PASS.

## ADR-M3-2: MySQL CLI scope (implemented)

The MySQL CLI implements Scan/Preview/Approve/Apply/Validate with two DSNs and
the shared logical schema described in [issue86-mysql-cli.md](../design/issue86-mysql-cli.md).
Plan binds source content by hashing every row of every planned table because a
MySQL server has no single snapshot file. Strict validation uses `CHECK TABLE ... QUICK` status rows plus explicit
orphan scans for every visible FK touching the planned table. Streaming/outbox/recovery remain SQLite-only and the CLI refuses them for
MySQL; no MySQL recovery protocol is claimed. Acceptance is the real
`TestMySQLCLIWorkflow` in the isolated MySQL 8.0.46 CI harness, not adapter-only
tests.
unless a separate protocol is approved and tested.

## ADR-M3-1a: P2 fixes in the same slice

- #9 P2-1 (Writer accepted non-NULL -> NULL) is closed at the writer layer.
- #9 P2-8 (runner rejected a record dated a few minutes in the future while
  history tolerated 5 minutes) is closed by using the same 5-minute skew.
- This ADR is a historical M3-1 record. #9 P2-5 target-WAL legacy apply was
  fixed by #102 (`OpenForApply` rejects non-quiescent SQLite targets); the CLI
  e2e digest sidecar limitation and #9 P2-3 (gather cancellation) remain
  documentation-only unless a regression is found.
- #11's historical two lost P2 items are not reconstructed; the M3 independent
  review covers the reviewed release candidate `b073d88` and preserves that evidence gap.

## ADR-M3-1b: MySQL strict integrity was initially not claimed (superseded by ADR-M3-2)

M3-1 implements strict integrity for SQLite (`quick_check` plus a database-wide
`foreign_key_check`). MySQL's `CHECK TABLE` only covers physical storage and does
not validate FK/CHECK semantics; an unverified partial parser would be fail-open.
The MySQL `IntegrityCheck` therefore returns `ErrUnsupported`, so strict MySQL
validation fails closed and remains **UNVERIFIED** until M3-2 implements and
tests a complete MySQL constraint check. M3-2 subsequently implemented MySQL
strict integrity (CHECK TABLE + FK orphan checks) and the MySQL CLI; MySQL
recovery remains a separate
M3-2 decisions.
