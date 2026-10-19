# Apply-window fsync evidence

- Code under test: `a9b6fb267a414b83f4fa10a758de504843763bf0` (`TestPhase3StraceApply`).
- Host: Linux x86_64, kernel `7.0.0-38-generic`; host MemTotal 15,187 MiB (not a cgroup limit); `strace 6.8`; Go `go1.26.3`; SQLite `modernc.org/sqlite v1.60.1`.
- Command: `strace -f -ttt -T -yy -e trace=fsync,fdatasync,write -o /tmp/... env MASK_RIVER_STRACE_APPLY=1 go test -run '^TestPhase3StraceApply$' -count=1 -v ./internal/runner/` (actual trace: `/tmp/maskriver-phase3-a9b6.strace.log`; selected raw syscalls are preserved in `fsync-syscalls.txt`).
- Marker write interval: start `1792415432.605978`, end `1792415432.673407` (67.429 ms wall time, **strace-instrumented; not a benchmark result**). The test completed Apply with `Complete=true` and four receipts, then validated success.
- Inside the marked interval: **16 successful `fsync` calls, 0 `fdatasync` calls**. `-yy` associates 8 calls with this fixture's `target.db-journal`, 4 with its containing directory, and 4 with `target.db`. One fsync was split into an unfinished/resumed pair by `strace`; the resume returned 0 and it is counted once.
- The 16 calls appear as four sequential groups of four (journal, directory, journal, database), temporally aligned with the four successful Apply receipts/commits. This is strong SQLite-file correlation, **not proof that every `fsync` is exclusively caused by a single Commit**: no per-Commit kernel marker was injected, and sync semantics are the SQLite driver's/platform implementation.
- This supports the direct driver-level timing result but must not be added to transaction timing: syscall durations can overlap scheduler activity, and strace materially perturbs wall time. No production journal/synchronous setting was changed.
