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

## 8. 第三阶段补充：四项 P2 已处理，直接事务成本证据（2026-10-19）

> 本补充不改写以上阶段二的历史记录。代码与原始证据 SHA、环境和逐次数据见 [`issue13-phase3-a9b6fb2/`](issue13-phase3-a9b6fb2/)。该轮仍是测量，不是优化授权；Issue #13 保持 OPEN。

| 遗留 P2 | 第三阶段处理/核验证据 |
|---|---|
| Benchmark 定时边界可能把 bookkeeping 算入或 fixture 误算 | `BenchmarkWorkflow` 每轮显式 Start/Stop，仅括住 `Workflow.run`；fixture、engine 生命周期、关闭/清理在计时外。退出路径有 Stop 清理。 |
| 样本物理约束/完整矩阵只靠单一生成路径 | 新测量和离线重建共用完整 `validateSamples`；`sample-mutation-results.md` 保存 5 种独立负变异（零 masked 行、3% 阶段和、非法 worker、缺样本、重复样本）实际失败码与错误；摘要篡改也由再生一致性测试拒绝。 |
| 提交数/成本只是间接推断 | 真实 SQLite `Writer` 与 value-free SQL driver spy 直接测 Begin/Commit/Rollback、UPDATE、其他 SQL 调用及每个 batch 的事务窗口/语句数/回执；CommitUnknown 与 Update 失败/成功 Rollback 路径单独验证。Apply 的 Commit driver 调用与 fsync 证据分开记录，未把嵌套计时相加。 |
| benchmark TempDir 随迭代累积 | 一个 benchmark 根目录下每次只建一个独立迭代目录；Engine 显式关闭后目录删除；资源回归测试验证无遗留，后续迭代可以继续。 |

### 第三阶段发现（256 行、workers=4；BatchSize 16/64/256 各 10 次随机次序）

| Batch | 每轮 Commit 成功数 | Apply 均值（中位数） | driver Commit 时间均值（每轮） | UPDATE driver 时间均值 | 其他 SQL driver 时间均值 | Begin driver 时间均值 | 事务窗口均值 | 未归属 Apply 均值 |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16 | 16 | 131.59 ms（131.40） | 102.231 ms | 3.472 ms | 1.989 ms | 0.070 ms | 113.88 ms | 17.72 ms |
| 64 | 4 | 39.43 ms（39.50） | 24.744 ms | 1.303 ms | 0.461 ms | 0.016 ms | 29.37 ms | 10.05 ms |
| 256 | 1 | 16.17 ms（16.24） | 5.885 ms | 0.527 ms | 0.068 ms | 0.003 ms | 7.86 ms | 8.31 ms |

独立 batch 记录显示每批恰有 `UPDATE × 行数 + 5 条其他 SQL + BEGIN/COMMIT`；每个 batch 都有正确行数的回执。**默认 BatchSize=64 的测量 Apply 中，Commit driver 时间是最大已直接计时项（约 63%）；** 每次 Commit 约 6.2 ms。SQLite `DELETE` journal / `synchronous=2` 下的独立 Apply-only strace 窗口关联到目标 `target.db`/journal/目录的 16 次成功 `fsync`（四组、4 个回执），无 `fdatasync`。同步调用与 Commit 时序一致，但没有逐 Commit 内核标记，故不把所有 fsync 因果独占地归给 Commit。

**不把 Commit 说成全部 Apply 成本。** BatchSize=256 时，driver Commit 约占 Apply 36%，未归属部分约 8.31 ms（约 51%）；未归属包括 page/read、遮盖/校验和框架等，未被单独证明。UPDATE driver 时间仅占小部分；其他 SQL 的逐批数与耗时另列 `apply-batches.csv`。计时嵌套，`driver` 列不可再与事务窗口求和。十次重复仅作描述统计，不声称显著性。未改 BatchSize、SQL、事务、journal/synchronous 或 worker pool。

### 10k 行 Runner 压测（单次/worker）

真实 SQLite Runner 对 workers=1/2/4/8/16 各运行一次；所有运行 `Complete`、157 个回执累计 10k 行、独立 Validate、源未变，逐行结果一致；关闭后 reader/writer 连接均为 0/0。Workflow 约 1.92–1.97 秒、Apply 约 1.44–1.49 秒，吞吐约 5.07–5.22k rows/s；workers 增大没有显示一致提速。CPU、RSS、goroutine 抽样及每次结果见 `10k-workers.csv`（本轮 goroutine 观测均为 4→4）；独立 `TestWorkersStopAndDoNotLeak` 同样轮询关闭后回落至基线+2 内。未运行 100k/1M 行或多轮显著性分析。

最终主机证据：Linux `7.0.0-38-generic` x86_64、AMD Ryzen 9 7940H、16 cores、host `/proc/meminfo` MemTotal 15,187 MiB（不代表容器限额）、Go 1.26.3、modernc SQLite 1.60.1。完整 SQLite 样本与报告在 `docs/perf/issue13-phase3-a9b6fb2/`；CLI E2E、MySQL 性能、非 Linux 资源仍是 **UNVERIFIED**。Issue #13 未关闭，也未授权开始 #14。

## 9. 第三阶段独立审查修复与最终更正

保留上述第 8 节作为其生成时的 phase-3 快照，不覆盖其历史记录。独立 Security Review/QA 在 review HEAD `1e5b5b0` 发现环境 SHA 的离线重建不一致、吞吐率缺少物理复核，以及 SQLSpy 批量扫描开销可能随 batch 数增长；QA 确认当时的 artifact consistency gate 失败。后续代码提交 `c747538` 将环境摘要作为测量时 provenance 单独验证、加入吞吐率/负时长校验、SQLSpy O(1) cursor/suffix 观察和 10 组随机配对 observer-overhead 测量；`8ffca9b` 将完整 Validate 放在 strace END marker 之后。修复后的 evidence consistency、观测器配对测试及全套本机测试均通过，准确命令与 SHA 见 `docs/perf/issue13-phase3-c747538/test-results.md`。

**第 8 节中基于 `a9b6fb2` 的表格是较早诊断快照，不是最终值。** 当前最终 Phase 3 结果、观测器误差范围、系统调用证据和 scope 限制以 [`issue13-phase3-report.md`](issue13-phase3-report.md) 与其版本化数据目录 `docs/perf/issue13-phase3-c747538/` 为准。新的独立 QA/Security follow-up 已在 `c49b9ce` 完成（QA PASS；Security OK with notes，无 P0/P1/P2），报告见 `docs/reviews/`。PR #68 已创建，最终 Head 的必需 CI（Ubuntu、Windows、MySQL、bench）均已通过；合并仍须遵守仓库门禁，本地报告不自行构成合并授权。
