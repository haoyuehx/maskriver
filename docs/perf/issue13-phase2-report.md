# Issue #13 第二阶段：基线修复、正确性与资源证据

本文件是**实现方自述**，不是独立审查结论。独立结论见 `docs/reviews/`。

## 1. 上一版（v1）永久作废

v1 的 `~78 rows/s` 与「每行 12.7 ms、线性」把 fixture 建库计入了计时：
`benchFixture` 在 `b.N` 循环内，`ns/op` 中约 97% 是建库，阶段耗时只占 1.7–3.1%。
该结论已从证据目录与 PR 说明中撤回，v1 的 BLOCK 审查报告保持原样，未改写为 PASS。

## 2. 本版改了什么

| 问题 | 处理 |
|---|---|
| 阶段耗时取「最后一次迭代」，与 `ns/op` 的多次平均不可相加 | 每次迭代单独记录总耗时与五个阶段；阶段指标改为**同一批迭代的中位数** |
| 聚合口径混用 | `samples.json`（原始逐次）为唯一真源，`results.csv`/`stats.csv`/`summary.md`/`environment.json` 全部由它生成 |
| 数字靠人工抄写 | `TestPerfArtifactsMatchSamples` 重新计算并与磁盘文件逐值比对；手改数字会失败（已验证：改一个数即失败） |
| 在打开的连接下覆盖 target 文件（上一版 `copyFile` 重置） | **删除重置逻辑**；每次迭代新建独立环境、独立 engine、独立数据库，并在迭代结束时关闭连接后再建下一个 |
| `internal/db/stats.go` 未接入 | 新增 `Engine.PoolStats()`，报告 **engine 自己**的 reader/writer 连接池统计，不建立第二个连接 |
| 资源指标 UNVERIFIED | 采集单次迭代期间的进程 RSS 增量、用户态/系统态 CPU、goroutine 数、连接池计数；fixture 成本单独计时 |

## 3. 本版结果（5 次迭代/配置，共 60 条样本）

| rows | shape | workers | total median | rows/s (mean) | apply share | apply median |
|---|---|---|---|---|---|---|
| 128 | long | 1 | 35.3 ms | 3801 | 57% | 20.2 ms |
| 128 | long | 4 | 35.5 ms | 3582 | 56% | 20.3 ms |
| 128 | short | 1 | 34.5 ms | 3758 | 58% | 20.2 ms |
| 128 | short | 4 | 35.4 ms | 3674 | 58% | 20.3 ms |
| 512 | long | 1 | 109.3 ms | 4683 | 69% | 74.3 ms |
| 512 | long | 4 | 109.5 ms | 4636 | 69% | 76.2 ms |
| 512 | short | 1 | 105.4 ms | 4898 | 70% | 74.3 ms |
| 512 | short | 4 | 109.6 ms | 4706 | 69% | 74.7 ms |
| 2048 | long | 1 | 405.2 ms | 5017 | 72% | 295.6 ms |
| 2048 | long | 4 | 426.5 ms | 4305 | 76% | 304.4 ms |
| 2048 | short | 1 | 393.6 ms | 5202 | 74% | 290.6 ms |
| 2048 | short | 4 | 407.4 ms | 5044 | 74% | 298.4 ms |

- 吞吐 **3 582–5 202 rows/s**，规模越大越高（固定开销被摊薄）。
- **`apply` 占 56–76%**，是工作流的绝对主成本。
- `workers=4` 在 8/12 组略慢、4/12 组略快，量级 1–5%，且 2048/long/w4 出现一次离群（mean 491.8 ms vs median 426.5 ms）。**不声称存在统计显著的并发差异**；上一版「workers=4 全部更慢」的说法同样不被本版数据支持。

## 4. Apply 成本来源（只测量，未优化）

`TestApplyPathDiagnosis` 在 256 行、workers=4 下测得：

| batch | receipts | statements | UPDATE | total | 每批 | 每行 |
|---|---|---|---|---|---|---|
| 16 | 16 | 368 | 256 | 126 ms | 7.9 ms | 494 µs |
| 64 | 4 | 284 | 256 | 38 ms | 9.6 ms | 150 µs |
| 256 | 1 | 263 | 256 | 16 ms | 16.3 ms | 64 µs |

- **每行恰好一条 `UPDATE`**（256 条），批内没有按行集合合并写。
- 每批固定约 **7 条额外语句**（368−256=112=16×7）。
- 每行成本随批大小下降 7.7 倍（494→64 µs），而每批成本基本恒定（7.9–16.3 ms）→ **成本由「批数 × 每次提交」主导，不是由每行计算主导**。
- 交叉验证：基准用 `BatchSize=64`，2048 行 = 32 批 × 9.6 ms ≈ 307 ms，实测 apply 中位数 290–304 ms，**吻合**。
- 同一台机器上 fixture 的 autocommit 插入为 **12 ms/行**（2048 行 fixture 约 24.7 s），与「每次提交成本约 8–12 ms」一致。

这些是下一阶段优化假设的依据（减少提交次数、合并行写入），本轮**未修改** BatchSize 默认值、事务策略、journal/synchronous 配置或 Worker 数量。

## 5. 正确性与安全测试（新增）

`internal/runner/workflow_concurrency_safety_test.go`，全部走**真实**审批存储与真实签名：

1. `TestWorkerCountNeverChangesResultData` — workers 1 与 4 的最终数据按主键逐行比较一致，行数正确，且无一行保持原值。
2. `TestApplyReceiptsAndValidationAgree` — 回执行数、报告 `Complete`、独立 `Validate` 与目标实际行数/唯一值一致；source 摘要不变。
3. `TestPreviewDoesNotWriteTarget` — preview 前后目标文件摘要一致。
4. `TestUnapprovedApplyCannotWrite` — 未审批的 Apply 在 Writer 前失败（`ErrNotFound`）且目标不变；**变异验证**：移除 `checkApproval` 后该测试失败（Apply 写入 5 行），证明其有承载力。
5. `TestWorkersStopAndDoNotLeak` — 结束后 goroutine 回落到基线。
6. `TestCancellationStopsEveryWorker` — 取消即刻返回错误，之后引擎仍可完成完整 Apply（证明 worker 已排空、无死锁）。
7. `TestWorkflowLeavesNoSidecarFiles` — 运行后无残留非空 `-wal`/`-shm`/`-journal`。

未覆盖（诚实记录）：goroutine 数量峰值、`sql.DB` 是否在迭代间复用同一池（每迭代新建引擎，池随引擎销毁）、多批次 CommitUnknown 的专门新测试（沿用既有测试）。

## 6. 资源口径

- **进程级**：RSS 取 `/proc/self/status` VmRSS（单次迭代前后差），CPU 取 `getrusage` 用户态/系统态差。
- **工作流级**：同一次迭代的 `total`。
- **单次 Apply 级**：`apply_ns`。
- **fixture 级**：单独计时，见 `fixture median`，**从不**计入工作流数字。
- 非 Linux 平台这些列输出 `UNVERIFIED`，不填 0。

实测（中位数）：128 行 RSS ≈ 20–21 MB、用户态 CPU ≈ 17–22 ms；2048 行 RSS ≈ 23–24 MB、用户态 CPU ≈ 236–271 ms；连接池在 apply 结束后 reader=1、writer=1。

## 7. 未验证 / SKIP

- 统计显著性：**未做**（`stats.csv` 给出 stddev/min/max，但未做检验）。
- 10k/100k/1M 行、workers 2/8/16：**未运行**。
- 非 Linux 平台的 RSS/CPU：**UNVERIFIED**。
- CLI 子进程 E2E 性能：**未测量**（本轮仍是 Runner 级）。
- MySQL 性能：**未测量**。
