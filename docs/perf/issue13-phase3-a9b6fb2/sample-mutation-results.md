# Raw-sample mutation checks

Five mutated copies of phase-2 `samples.json` were each passed through the offline `TestPerfArtifactsMatchSamples` path, which uses the same `validateSamples` as new measurements. Each failed with exit code 1 and the expected error:

| Mutation | Result |
|---|---|
| `rows_masked = 0` | exit 1 — `128/short/w1 iteration 1 rows_masked=0, want 128` |
| All five positive stage values = 0.6% of total (3% stage sum) | exit 1 — `128/short/w1 iteration 1 stage sum is 3.0% of total, expected 80-120%` |
| Worker count = 3 | exit 1 — `illegal configuration 128/short/w3` |
| Delete one sample | exit 1 — `configuration 2048/long/w4 has 4 samples; matrix requires 5 iterations` |
| Duplicate a sample at the same config/iteration | exit 1 — `duplicate sample 128/short/w1/iteration=1` |

The unmodified phase-2 source artifact passed the offline test (exit 0). Editing the phase-3 summary (`131.59 / 131.40`→`231.59 / 231.40`) failed `TestPhase3ArtifactConsistency` (exit 1, `summary.md is not reproducible from samples.json`). The unmodified final phase-3 artifacts passed consistency validation (exit 0). All scratch mutation copies were removed.
