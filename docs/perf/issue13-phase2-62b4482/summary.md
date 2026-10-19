# Issue #13 真实 SQLite Runner 性能基线

本文件由测量程序生成，不是手工填写。原始逐次样本在同目录 `samples.json`，聚合值在 `results.csv`；`TestPerfArtifactsMatchSamples` 会重新计算并比对，手改数字会让测试失败。

## 口径

- **样本**：每个配置每次迭代是一条 `samples.json` 记录，含工作流总耗时与该次迭代内五个阶段的耗时。
- **total**：单次迭代内 `Scan→Plan→Preview→Approve→Apply` 的墙钟耗时，不含 fixture 构建、连接关闭、target 重建。
- **stage/total**：同一次迭代内阶段之和除以该次总耗时。它是污染检查：fixture 若混入计时区间，该比值会明显偏离 100%。
- **apply share**：同一次迭代内 `apply` 除以总耗时。
- **rows/s**：单次迭代 `rows_masked / total`，不是用平均值反推。
- **median/mean/min/max/stddev**：全部逐项记录在 `stats.csv`；下表 total 用 mean 与 median，rows/s 与占比用 mean，apply 用 median，均明确标注。
- **fixture**：单独计时，`fixture_ns_median` 列，不计入任何工作流数字。

## 环境

- 平台：linux/amd64，Go go1.26.3，SQLite 驱动 modernc.org/sqlite v1.60.1（纯 Go）
- 首配置：128 行，末配置：2048 行；配置 12 组，逐次样本 60 条

## 工作流结果

| rows | shape | workers | iterations | total mean | total median | rows/s (mean) | stage/total | apply share | apply median |
|---|---|---|---|---|---|---|---|---|---|
| 128 | long | 1 | 5 | 33.8 ms | 35.3 ms | 3801 | 100% | 57% | 20.2 ms |
| 128 | long | 4 | 5 | 35.8 ms | 35.5 ms | 3582 | 100% | 56% | 20.3 ms |
| 128 | short | 1 | 5 | 34.1 ms | 34.5 ms | 3758 | 100% | 58% | 20.2 ms |
| 128 | short | 4 | 5 | 34.9 ms | 35.4 ms | 3674 | 100% | 58% | 20.3 ms |
| 512 | long | 1 | 5 | 109.4 ms | 109.3 ms | 4683 | 100% | 69% | 74.3 ms |
| 512 | long | 4 | 5 | 110.5 ms | 109.5 ms | 4636 | 100% | 69% | 76.2 ms |
| 512 | short | 1 | 5 | 104.6 ms | 105.4 ms | 4898 | 100% | 70% | 74.3 ms |
| 512 | short | 4 | 5 | 108.8 ms | 109.6 ms | 4706 | 100% | 69% | 74.7 ms |
| 2048 | long | 1 | 5 | 408.3 ms | 405.2 ms | 5017 | 100% | 72% | 295.6 ms |
| 2048 | long | 4 | 5 | 491.8 ms | 426.5 ms | 4305 | 100% | 76% | 304.4 ms |
| 2048 | short | 1 | 5 | 393.7 ms | 393.6 ms | 5202 | 100% | 74% | 290.6 ms |
| 2048 | short | 4 | 5 | 406.1 ms | 407.4 ms | 5044 | 100% | 74% | 298.4 ms |

## 资源与 fixture（口径见 environment.json）

RSS/CPU 是**单次迭代**期间的进程级增量，不含 fixture 构建；`fixture median` 是单独计时的 fixture 成本，**不计入**上面的工作流数字。

| rows | shape | workers | fixture median | RSS after median | CPU user median | CPU system median | reader open | writer open |
|---|---|---|---|---|---|---|---|---|
| 128 | long | 1 | 1554.3 ms | 21432 KB | 19 ms | 2 ms | 1 | 1 |
| 128 | long | 4 | 1550.3 ms | 21440 KB | 22 ms | 3 ms | 1 | 1 |
| 128 | short | 1 | 1555.7 ms | 20300 KB | 17 ms | 6 ms | 1 | 1 |
| 128 | short | 4 | 1552.5 ms | 20952 KB | 22 ms | 1 ms | 1 | 1 |
| 512 | long | 1 | 6182.0 ms | 22524 KB | 64 ms | 1 ms | 1 | 1 |
| 512 | long | 4 | 6176.5 ms | 22564 KB | 71 ms | 5 ms | 1 | 1 |
| 512 | short | 1 | 6183.7 ms | 21932 KB | 61 ms | 2 ms | 1 | 1 |
| 512 | short | 4 | 6178.0 ms | 22512 KB | 71 ms | 3 ms | 1 | 1 |
| 2048 | long | 1 | 24676.1 ms | 23888 KB | 240 ms | 1 ms | 1 | 1 |
| 2048 | long | 4 | 24987.8 ms | 23728 KB | 271 ms | 35 ms | 1 | 1 |
| 2048 | short | 1 | 24693.1 ms | 23320 KB | 236 ms | 0 ms | 1 | 1 |
| 2048 | short | 4 | 24695.2 ms | 23596 KB | 266 ms | 5 ms | 1 | 1 |

## 未测量 / UNVERIFIED

- 显示为 UNVERIFIED 的列在本机或本平台无法采集（例如非 Linux 上的 /proc 统计）。
- 10k/100k/1M 行、workers 2/8/16 与本轮矩阵之外的平台：**未运行**。
- 本文件不声明统计显著性；看 `stats.csv` 的 `stddev_pct` 与 min/max 自行判断。
