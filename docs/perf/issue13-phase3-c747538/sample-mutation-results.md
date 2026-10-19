# Artifact mutation checks at source SHA `8ffca9b`

The offline `TestPerfArtifactsMatchSamples` test was run independently against five corrupted copies of phase-2 `samples.json`. All returned exit code 1 with the expected validator error:

| Mutation | Result |
|---|---|
| Set `rows_masked=0` | `128/short/w1 iteration 1 rows_masked=0, want 128` |
| Set each positive stage to 0.6% of total (3% aggregate) | `128/short/w1 iteration 1 stage sum is 3.0% of total, expected 80-120%` |
| Set workers to 3 | `illegal configuration 128/short/w3` |
| Delete one record | `configuration 2048/long/w4 has 4 samples; matrix requires 5 iterations` |
| Duplicate one record | `duplicate sample 128/short/w1/iteration=1` |

A corrupted phase-3 summary (`129.90 / 130.26` changed to `229.90 / 230.26`) was rejected by `TestPhase3ArtifactConsistency`: exit 1, `summary.md is not reproducible from samples.json`. The unchanged phase-2 and phase-3 artifacts passed their offline validators (exit 0). `TestValidatePhase3TrialsRejectsInconsistentThroughput` and `TestValidatePhase3TrialsRejectsNegativeDurations` pass by rejecting inconsistent throughput and negative driver/batch timings. All scratch mutation copies were removed.
