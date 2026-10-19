# 10,000-row SQLite Runner stress run (single observation per worker count)

Synthetic `@example.invalid` values; exact SHA and environment are in `10k-environment.json`. Fixture time excludes workflow; Apply is a substage of workflow. All runs used signed approval, Complete receipts, full independent Validate, unchanged source, exact row counts, and row-by-row equality to workers=1. Connections and fixture directories were released after each run. One observation/configuration is descriptive only; no significance or speedup claim.

| workers | fixture ms | workflow ms | apply ms | workflow rows/s | CPU user/system ms | RSS before→after KB | goroutines before→after | open reader/writer before close | after close |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 60.52 | 1917.32 | 1437.95 | 5216 | 1059.28/129.60 | 19320→25752 | 4→4 | 1/1 | 0/0 |
| 2 | 52.93 | 1963.28 | 1469.87 | 5094 | 1247.05/170.45 | 25880→25632 | 4→4 | 1/1 | 0/0 |
| 4 | 62.16 | 1950.69 | 1459.10 | 5126 | 1324.92/147.70 | 26308→25952 | 4→4 | 1/1 | 0/0 |
| 8 | 51.86 | 1971.46 | 1475.61 | 5072 | 1386.27/165.28 | 26664→26096 | 4→4 | 1/1 | 0/0 |
| 16 | 59.70 | 1969.10 | 1485.27 | 5078 | 1452.91/193.69 | 27196→27824 | 4→4 | 1/1 | 0/0 |
