# 10,000-row SQLite Runner stress run (single observation per worker count)

Synthetic `@example.invalid` values; exact SHA and environment are in `10k-environment.json`. Fixture time excludes workflow; Apply is a substage of workflow. All runs used signed approval, Complete receipts, full independent Validate, unchanged source, exact row counts, and row-by-row equality to workers=1. Connections and fixture directories were released after each run. One observation/configuration is descriptive only; no significance or speedup claim.

| workers | fixture ms | workflow ms | apply ms | workflow rows/s | CPU user/system ms | RSS before→after KB | goroutines before→after | open reader/writer before close | after close |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 51.14 | 1928.64 | 1447.20 | 5185 | 1065.55/110.57 | 18972→25316 | 4→5 | 1/1 | 0/0 |
| 2 | 46.13 | 1967.61 | 1474.46 | 5082 | 1228.39/206.78 | 25940→25996 | 4→4 | 1/1 | 0/0 |
| 4 | 52.33 | 1980.84 | 1489.17 | 5048 | 1317.45/176.49 | 26620→26380 | 4→4 | 1/1 | 0/0 |
| 8 | 55.89 | 1973.55 | 1483.76 | 5067 | 1402.83/172.46 | 27196→26116 | 4→4 | 1/1 | 0/0 |
| 16 | 50.50 | 1978.74 | 1481.42 | 5054 | 1439.59/206.46 | 26772→27024 | 4→5 | 1/1 | 0/0 |
