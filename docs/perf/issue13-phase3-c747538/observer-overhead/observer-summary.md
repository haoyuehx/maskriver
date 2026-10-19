# SQLSpy observer overhead (paired, 256 rows; descriptive only)

Each pair uses independent fresh signed SQLite fixtures, randomized mode order, equal per-key outputs and independent Validate. `plain` uses the production SQL path; `instrumented` adds SQLSpy plus diagnostic writer counters. Apply wall time includes all observer work. This estimates total observer perturbation for these test fixtures; it does not imply production throughput or statistical significance.

| Batch | pairs | plain Apply median (ms) | instrumented Apply median (ms) | paired delta median (%) | paired delta min–max (%) |
|---:|---:|---:|---:|---:|---:|
| 16 | 10 | 129.25 | 131.33 | 0.99 | -6.21–9.26 |
| 64 | 10 | 39.56 | 40.12 | 4.08 | -5.59–5.59 |
| 256 | 10 | 16.29 | 16.42 | 1.44 | -0.58–14.15 |
