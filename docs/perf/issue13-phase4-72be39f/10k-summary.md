# 10,000-row SQLite Runner stress run (single observation per worker count)

Synthetic `@example.invalid` values; exact SHA and environment are in `10k-environment.json`. Fixture time excludes workflow; Apply is a substage of workflow. All runs used signed approval, Complete receipts, full independent Validate, unchanged source, exact row counts, and row-by-row equality to workers=1. Connections and fixture directories were released after each run. One observation/configuration is descriptive only; no significance or speedup claim.

| workers | fixture ms | workflow ms | apply ms | workflow rows/s | CPU user/system ms | RSS before→after KB | goroutines before→after | open reader/writer before close | after close |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 55.47 | 1916.64 | 1448.05 | 5217 | 909.95/234.61 | 18544→25148 | 4→4 | 1/1 | 0/0 |
| 2 | 51.92 | 1949.99 | 1471.15 | 5128 | 1215.52/163.41 | 26376→26152 | 4→4 | 1/1 | 0/0 |
| 4 | 60.54 | 1937.18 | 1462.57 | 5162 | 1258.67/169.11 | 26492→26084 | 4→4 | 1/1 | 0/0 |
| 8 | 58.31 | 1933.99 | 1462.13 | 5171 | 1299.76/175.03 | 26756→26640 | 4→4 | 1/1 | 0/0 |
| 16 | 50.19 | 1947.91 | 1470.23 | 5134 | 1385.19/208.31 | 27196→26576 | 4→4 | 1/1 | 0/0 |
