# Issue #13 Phase 3 — Independent Security Follow-up

- Date: 2026-10-19
- Reviewer: `go-reviewer`, independent read-only context
- Reviewed HEAD: `c49b9ceee75d4cc6c8e56f32c23ef9d4740e4186`
- Follow-up base: `1e5b5b0728ea833063b959fd0d7c2f98c9ad5503`
- Repository: `/home/haoyue/Project/maskriver`

## Scope and method

Reviewed the supplied `.review-phase3-followup.patch`, current relevant source, initial Security/QA reports, governing contracts and architecture/test/feature documentation, attribution documentation, and selected measurement artifacts.

The SHA identification relies on the supplied review metadata; this reviewer did not execute Git commands, tests, network operations or writes. Artifact inspection was selective, not an independent numerical recomputation. Initial reports remain unchanged.

## Initial findings disposition

### P2-1 — Historical artifact verification depends on checker HEAD/host: CLOSED in code

**Locations:** `internal/runner/phase3_artifacts_test.go:29–77`; `internal/runner/phase3_validation_test.go:82–108`.

**Original reproduction:** Verify committed measurements from a later checkout or another host.

**Evidence:** Environment metadata is now validated separately rather than byte-compared against regenerated checker-host metadata. Derived artifacts retain byte-consistency checks. Validation checks SHA shape, required provenance fields, sample count/repetitions and SQLite settings. The regression explicitly permits a different historical SHA.

This resolves the original false-failure mechanism without overwriting measurement provenance. It validates metadata consistency, **not authenticity or Git ancestry**. Exact-HEAD execution remains an independent QA responsibility.

### P2-2 — Inconsistent derived throughput accepted: CLOSED in code

**Locations:** `internal/runner/phase3_experiment_test.go:555–619`; `internal/runner/phase3_validation_test.go:55–80`.

**Original reproduction:** Change only a sample’s finite positive `rows_per_second`.

**Evidence:** Validation now recomputes rows/s from the fixed row count and Apply duration, checks a defined floating-point tolerance, rejects negative component/batch durations and requires workflow duration to cover Apply. Mutation regressions cover inconsistent throughput and representative negative timings.

The original contradictory-throughput input is now rejected. Test execution claims are recorded in the supplied evidence, not independently reproduced here.

### P2-3 — Repeated whole-log copies bias Apply: CLOSED for the reported mechanism

**Locations:** `internal/db/sqlspy.go:66–98`; `internal/runner/apply_diagnosis_test.go:76–81,131–159`; `internal/runner/phase3_observer_test.go:34–118`.

**Original reproduction:** Increase batch count while diagnostic Begin/Commit repeatedly copy growing logs.

**Evidence:** Begin now obtains constant-time cursors; Commit copies and scans only the relevant suffix. A randomized plain/instrumented paired experiment uses fresh signed fixtures, Complete receipts, full independent Validate and per-key output equality.

The report explicitly includes observer work in Apply measurements and limits the measured overhead ranges to these fixtures. SQLSpy still stores growing logs and imposes instrumentation cost; the fix does not establish bounded production logging or an overhead correction valid at other scales.

**Recommendation:** Retain the test-only boundary and diagnostic-throughput caveats.

### Additional initial note — strace lacked independent Validate: CLOSED in code

**Location:** `internal/runner/phase3_strace_test.go:48–62`.

Full Validate now runs after END, requiring complete coverage and `Passed(true)`. Validation therefore remains outside the marked syscall interval.

## New findings and P0/P1 disposition

**No new P0/P1/P2 defect identified in the reviewed follow-up.**

The initial security review identified no P0/P1; none requires closure in this follow-up. This conclusion does not close repository-wide findings outside the assigned diff.

## SQLSpy production surface and safety

- SQLSpy is ordinary compiled Go source, not build-tag-isolated test code. Its additional exported methods extend internal test-support surface.
- Production `db.OpenReader`/`OpenWriter` retain the nil-factory path (`internal/db/db.go:43–48`). Inspected SQLSpy instantiations are tests; no CLI references were found.
- Cursor/suffix methods are mutex-protected and return slice copies. They introduce no global registration or connection-policy change.
- `SpyEvent` contains operation label, duration and error boolean—not bound values or raw driver errors.
- Existing `SpyStatement.SQL` retains SQL text (`internal/db/sqlspy.go:35–38,146–150`). This is **not safe general-purpose production logging**: SQL literals or identifiers can be sensitive. The follow-up does not add parameter capture or publish statement logs.

No production changes were found to default Dry Run, CLI write intent, approval binding, scan fail-closed/UNKNOWN handling, typed outputs, mapping scope/collision policy, transaction/pagination/cancellation semantics, or validation’s bidirectional-key coverage. Frozen contracts and dependencies are unchanged.

## Claims versus evidence

The revised report appropriately distinguishes:

- Measurement SHA `c747538edcc0fb52fb16a0eb796953cec1062c94`.
- Later tested source SHA `8ffca9b42ee3841d0d11e329b21a09e7c45aa682`.
- This review’s HEAD `c49b9ceee75d4cc6c8e56f32c23ef9d4740e4186`.

Supplied local results report tests, race, vet, fuzz, benchmark smoke and artifact checks passing at the documented source revisions. They do not establish independently executed final-HEAD QA.

The report correctly treats ten randomized repeats and observer ranges as descriptive, 10k worker results as single observations, and goroutine snapshots as insufficient to prove absence of leaks. Windows evidence is cross-compilation only. MySQL/CLI performance, non-Linux resources, 100k/1M scale and statistical significance remain unverified.

The inspected syscall excerpt contains 16 successful fsync entries associated with synthetic target/journal/directory paths. It does not include START/END marker writes or the complete trace, so completeness of the interval and absence of omitted calls remain supplied evidence claims. Exclusive per-Commit causality is appropriately disclaimed.

Inspected artifacts contain synthetic measurements and temporary fixture paths; no real row values, credentials or private keys were identified. This was not an exhaustive secret scan.

No dependency, licence or author-attribution changes appear in the supplied patch. Attribution documentation retains MIT project ownership and third-party licence obligations. Independent provenance or Git-history ownership cannot be established from this read-only inspection alone.

## Verdict

**Security/code review: OK with notes.** All three initial P2 mechanisms are addressed; no new P0/P1/P2 identified.

Required final-HEAD independent executed QA and remote CI evidence were not available to this reviewer. Earlier local results and this static review do not satisfy those merge gates. No remote CI success or merge is asserted.

**Merge verdict: BLOCK** — pending required merge-gate evidence, not an unresolved security-code finding.