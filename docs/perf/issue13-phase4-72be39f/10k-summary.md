# 10,000-row SQLite Runner stress run (single observation per worker count)

Synthetic `@example.invalid` values; exact SHA and environment are in `10k-environment.json`. Fixture time excludes workflow; Apply is a substage of workflow. All runs used signed approval, Complete receipts, full independent Validate, unchanged source, exact row counts, and row-by-row equality to workers=1. Connections and fixture directories were released after each run. One observation/configuration is descriptive only; no significance or speedup claim.

| workers | fixture ms | workflow ms | apply ms | workflow rows/s | CPU user/system ms | RSS before→after KB | goroutines before→after | open reader/writer before close | after close |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 59.86 | 1910.83 | 1445.34 | 5233 | 1014.59/120.17 | 18980→24960 | 4→4 | 1/1 | 0/0 |
| 2 | 58.28 | 1947.36 | 1472.52 | 5135 | 1050.41/324.11 | 26132→25720 | 4→4 | 1/1 | 0/0 |
| 4 | 59.61 | 1956.88 | 1476.74 | 5110 | 1243.97/181.07 | 26260→25552 | 4→4 | 1/1 | 0/0 |
| 8 | 52.10 | 1949.32 | 1472.31 | 5130 | 1296.07/183.89 | 26604→26768 | 4→4 | 1/1 | 0/0 |
| 16 | 46.53 | 1945.22 | 1468.80 | 5141 | 1418.90/217.26 | 27012→27076 | 4→4 | 1/1 | 0/0 |
