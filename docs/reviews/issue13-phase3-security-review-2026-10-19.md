# Issue #13 Phase 3 — Independent Security Review

- Date: 2026-10-19
- Reviewer: `go-reviewer`, independent read-only context
- Base: `2cb42ad2da23431e102a01f38a98aad9822865ca`
- Head: `1e5b5b0728ea833063b959fd0d7c2f98c9ad5503`
- Branch: `perf/issue13-phase3-transaction-cost-20261019`

## Scope and method

Read `AGENTS.md`, contracts, architecture, test plan, feature matrix and attribution documentation; reviewed the supplied exact base-to-head patch and relevant source/evidence files.

No commands, tests, network operations or file writes were performed by this reviewer. Supplied test-result documents are evidence claims, not independently executed results. Generated artifacts were inspected selectively, not exhaustively independently recomputed.

## Findings

### P2-1 — Historical artifact verification depends on the verifier’s HEAD and host

**Locations:** `internal/runner/phase3_artifacts_test.go:27–39`; `internal/runner/phase3_experiment_test.go:458–479,507–513`.

**Trigger / reproduction:** At the reviewed HEAD, point `TestPhase3ArtifactConsistency` at the committed `docs/perf/issue13-phase3-a9b6fb2` directory.

**Evidence:** The consistency test regenerates and byte-compares `environment.json`. Generation reads the current Git SHA, Go runtime, CPU, kernel and memory. The committed environment records `a9b6fb267a414b83f4fa10a758de504843763bf0`, not the reviewed HEAD. Thus unchanged historical artifacts fail verification after a documentation-only commit, even on the original host; another host introduces further differences.

**Impact:** False verification failures undermine reproducibility. The documented earlier PASS does not establish that this check passes against the committed evidence at the reviewed HEAD.

**Recommendation:** Preserve measurement-time environment as explicit input. Validate its schema/provenance separately; regenerate derived results from samples without replacing historical metadata with verifier-host metadata.

### P2-2 — Phase-3 validator accepts inconsistent derived throughput

**Locations:** `internal/runner/phase3_experiment_test.go:214,349,375,557–559`.

**Trigger / reproduction:** Change a sample’s `rows_per_second` to another finite positive number while retaining its original row count and `apply_ns`, then invoke `validatePhase3Trials`.

**Evidence:** Generation computes throughput from rows and Apply duration, but validation checks only positivity and finiteness. Report renderers consume the stored throughput directly.

**Impact:** Internally contradictory raw samples can pass validation and produce mutually reproducible but incorrect reports. Byte consistency alone does not catch this if all derived artifacts are regenerated. No evidence was found that the committed throughput values were manipulated.

**Recommendation:** Recompute throughput from authoritative counts/durations, or check equality within a defined floating-point tolerance. Add mutation tests for inconsistent throughput and negative component timings.

### P2-3 — Instrumentation adds batch-dependent work to measured Apply

**Locations:** `internal/runner/apply_diagnosis_test.go:76–81,130–165`; `internal/db/sqlspy.go:53–64`.

**Trigger / reproduction:** Compare different BatchSizes using the diagnostic writer.

**Evidence:** Each Begin copies the complete accumulated statement/event logs to obtain their lengths. Each Commit copies them again before examining its suffix. These allocations and copies occur within measured Apply; Commit wrapper timing also includes diagnostic processing. More batches cause more repeated copying of growing logs.

**Impact:** This biases measured Apply and residual costs by BatchSize. Driver-call timing remains a distinct useful observation, but instrumentation overhead has not been quantified, so the measured residual cannot be attributed solely to production framework/read/masking work.

**Recommendation:** Expose constant-time cursors and suffix-only snapshots, or aggregate events incrementally. Measure observer overhead against an otherwise equivalent uninstrumented run and document the remaining measurement perturbation.

## Security and production assessment

**No new P0/P1 was identified in the reviewed diff. No P0/P1 remains from this review.** This does not close previously documented repository-wide issues.

**Production CLI behavior did not change in the reviewed diff.** The only changed non-test Go implementation is SQLSpy instrumentation. `internal/db/db.go:43–47` retains the production nil-factory path; identified SQLSpy call sites are tests. SQLSpy’s injected behavior does change through timing, event storage and wrapped rows; that is not a production transaction-policy change.

The diff does not alter:

- Default Dry Run or explicit CLI Apply authorization.
- Scan fail-closed, UNKNOWN handling or approval binding.
- Typed masking, mapping scope or collision policy.
- Production transaction, pagination, cancellation or commit-unknown handling.
- Production validation coverage or bidirectional-key comparison.

Added fault tests distinguish failed Update/rollback from a durable Commit whose acknowledgement is reported unknown. They do not establish checkpoint recovery, automatic retries or whole-workflow atomicity.

New exported timing events contain operation labels, durations and error booleans, not parameters or raw driver errors. Existing SQL statement storage remains test support and must not be treated as generally safe production logging. Inspected artifacts contain synthetic aggregate measurements and temporary fixture paths; no real sensitive values or credentials were identified.

No dependency, license, frozen-contract or author-history changes appear in the supplied diff. Source provenance cannot be independently established from code inspection alone.

## Evidence limitations and notes

- Supplied local results report tests, race, vet, fuzz and benchmark smoke passing at code SHA `a9b6fb2`; this reviewer did not execute them.
- Windows evidence is cross-compilation only. MySQL performance, CLI E2E performance and non-Linux resource measurements remain unverified.
- The added stress job is manual-only; the existing PR benchmark job remains.
- `phase3_strace_test.go:47–55` checks Apply completion and four receipts, but does not call `Engine.Validate`. The fsync evidence wording should distinguish checking successful Apply from independent Validate.
- Selected fsync lines support the recorded target/journal/directory correlation. The committed excerpt alone does not independently establish the full marker window or absence of omitted calls.
- Remote CI at the reviewed HEAD and independent executed QA results were not available in this review.

## Verdict

**Security/code review: OK with notes.** Three non-blocking P2 findings remain; no new P0/P1 identified.

Repository merge authorization is separate: required final-HEAD CI and independently executed QA evidence must be satisfied under `AGENTS.md`; supplied implementer results do not substitute for these gates.

**Merge verdict: BLOCK** — pending required merge-gate evidence, not a newly identified P0/P1 code defect.