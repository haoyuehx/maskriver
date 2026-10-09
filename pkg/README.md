# Shared development contracts

`pkg/contracts` is exclusively Main-owned. APIRevision `m1a-v1` freezes component DTOs and interfaces for the next worker wave; it is not a stable external SDK guarantee.
See [contracts](../docs/contracts.md) and [parallel tasks](../docs/parallel-tasks.md).
Workers must import these types, not duplicate or modify them. Implementations remain in `internal/`.
