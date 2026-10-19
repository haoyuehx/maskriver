# PR #67 独立评审 — Issue #13 第二阶段：基线修复、正确性与资源证据

- **范围**：`perf/issue13-sqlite-baseline-20261019`，head `712a2e0`。真实 base = `62b4482`（`origin/main` 上的 PR #66 合并点），即本 PR 只含 `dcdce29..712a2e0` 两个提交（`e1615f6`、`712a2e0`）。
  - 注意：任务描述里的 base `main` 在本机是陈旧的 `b34ed0b`，`git diff main...HEAD` 会把 PR #55–#66 已合并的约 6000 行算进来。因此本评审对 `git diff 62b4482...HEAD` 定界。
- **评审者**：独立上下文，未参与本 PR 任何实现；实现方自述（`docs/perf/issue13-phase2-report.md`）不作为证据，只作为待核对的主张。
- **评审机**：同机，linux/amd64，`go1.26.3`，AMD Ryzen 9 7940H，16 核（与 `environment.json` 一致）。
- **只读**：除本文件外未改动工作区（临时目录在 `/tmp/perfrev`、`/tmp/perfregen`、`/tmp/execute.go.orig*`，均在仓库外）。无提交、无推送、无 GitHub、无子代理。收尾时 `git status --porcelain` 为空。
- **预算约束**：按任务要求未跑全矩阵、未重测基线；复用提交的 `samples.json` 做再生比对，只跑了两个定向变异与一次诊断测试。

## 结论摘要

第二阶段**修的是真问题，且证据链自洽**：fixture 构建、连接关闭、每轮重建确实都在计时区间外；`fixture_ns` 从不进入 `total`/`rows/s`；产物由 `samples.json` 单源派生，手改数字会让测试失败，从 `samples.json` 再生与提交版**逐字节一致**；两个被选中的正确性/审批测试在变异后都**真实失败**。范围干净，无契约、默认值或断言放宽。

未发现 P0/P1。遗留 4 条 P2，都不改变已发布的数字或结论。

---

## P0

无。

## P1

无。

### 旧 P1（在打开的句柄下覆盖 `target.db`）是真正修掉，不是改写措辞

**证据**：`internal/runner/workflow_bench_test.go` 中 `copyFile`、`pristine.db`、`benchEnv.reset`、`benchEnv.source/target/pristine` 字段**整段删除**（`git diff 62b4482...HEAD -- internal/runner/workflow_bench_test.go`，删除 55 行中的主体就是这套重置逻辑）。现在每次迭代 `benchEnvFor` → 新建 `tb.TempDir()`（Go 保证每次调用返回唯一目录）→ 建两个库 → `Open` 新 engine → 迭代结束 `env.close()` 后才进入下一次迭代（`workflow_bench_test.go:228-238`、`perf_baseline_test.go:103-142`，其中 `env.close()` 在 `:140`，即样本装配完之后）。

即：**不存在跨迭代复用的 target 文件**，旧的“复制覆盖 + 计时期间重建”路径已不存在。这不是措辞修改。

---

## P2

### P2-1 每轮簿记落在计时区间内（`workflow_bench_test.go:234-238`）

```go
232: b.StopTimer()
233: env.close()
234: b.StartTimer()          // ← 计时重新开始
235: rowsMasked = rows
236: for name, d := range stages {
237:     stageSamples[name] = append(stageSamples[name], float64(d.Microseconds()))
238: }
```

被点名要求核对的三件事（fixture 构建 `:229`、engine close `:233`、每轮重建 `:229`）**确实都在计时区间外**；但计时器在簿记之前就重启了，所以 `rowsMasked` 赋值与 5 条 map/append 被算进 `ns/op`。

- 量级：微秒级 vs 34–490 ms，**< 0.1%**，不改变任何结论，因此 P2。
- 修正：把 `b.StartTimer()` 移到 `:238` 之后（或循环开头），即 `StopTimer → close → 簿记 → StartTimer`。
- 复现：读 `:228-240`；`-benchtime=3x` 下 `ns/op` 略含簿记，无法从产物分辨，属静态缺陷。

阶段耗时与 total 的**同迭代一致性成立**：`stages` 与 `total` 都来自 `env.run` 的同一次调用（`perf_baseline_test.go:118-120`），阶段指标是同批迭代的中位数而非“最后一次”（`workflow_bench_test.go:245-248`），这正是 v1 P0-2 的修复点。

### P2-2 不变量断言只存在于测量路径，不在校验路径（`perf_baseline_test.go:484-493` vs `:502-560`）

`stage_sum_ratio 80–120%`（`:484`）与 `RowsDone == Rows`（`:489`）只在 `TestPerfBaseline` 的测量分支里执行。`TestPerfArtifactsMatchSamples` 只比对“文本是否可由 `samples.json` 重新渲染”，不校验样本本身的物理不变量。后果：一份 `rows_masked=0` 或 `stage_sum_ratio=3%` 的 `samples.json` 会渲染出自洽的 summary，测试照样通过。

- 影响：当前提交的样本没这个问题（12 组 `stage_sum_ratio` 99.98–100.00%，`rows_masked` 与 `rows` 逐组一致），是**回归防线缺口**而非现存错误。
- 建议：把 `:484-493` 的两个检查抽成 `validateSamples(samples)`，两条路径都调用。
- 复现：`MASK_RIVER_PERF_DIR=<dir> go test -run TestPerfArtifactsMatchSamples`，把 `samples.json` 里某条 `rows_masked` 改成 0 并再生，测试通过。

### P2-3 “成本由提交次数主导”里，“提交次数”是推断而非实测（`apply_diagnosis_test.go:44-63`、`:122-132`）

`spyBatchWriter` 统计了 `begin/commit/rollback`，但 `diagnoseApply` 从不读取这三个计数，测试也不断言 `commit == receipts`。报告里“由批数 × 每次提交主导”的因果，实际靠的是 `receipts == ceil(rows/batchSize)`（`:139`）这一契约断言，加上“同一 256 行、时间差 7.5 倍”这一现象。

- 现象本身**支持结论方向**：我本机复跑 `TestApplyPathDiagnosis`（`-v`）：batch16 = 122.4 ms / 478 µs/行，batch64 = 38.5 ms / 150 µs/行，batch256 = 16.3 ms / 64 µs/行；`updates == 256` 恒定 → 每行工作量不变而总时间差 7.5 倍，**足以否证“成本由每行脱敏计算主导”**。
- 但不能证明“就是 commit”：每批的固定开销里 `BEGIN/COMMIT`、receipt 写入、语句预编译混在一起（batch16 多出 112 条 = 16×7 条语句）。
- 另：报告称“每批成本基本恒定（7.9–16.3 ms）”过头了，实测同一批固定成本从 7.65 ms 涨到 16.3 ms（2.1 倍），稳定的是“**批数**”这个因子，不是“每批成本”。
- 建议：`diagnoseApply` 记录并 `t.Logf` `spy.commit`，断言 `commit == receipts`，把因果变成实测；报告改成“每批固定开销（含提交）主导”。

### P2-4 每迭代一个新 `TempDir` 在大 `-benchtime` 下无上限（`workflow_bench_test.go:229`）

每次迭代新建目录，`tb.TempDir` 的清理延迟到子基准结束。按提交的 fixture 成本（2048 行 ≈ 24.7 s / 次，见 `results.csv:fixture_ns_median`），`-benchtime=200x` 会留下约 200 份 2048 行库直到收尾。当前文档化的 `3x` 与 CI 的 `-benchtime=300ms` 都没问题，属使用风险提示，不影响交付数字。

---

## 证据能证明什么、不能证明什么

**能证明**

1. **fixture 不在 `total`/`rows/s` 里**，而且这条不靠读代码、靠数据本身：
   - 128 行 `fixture_ns_median` = 1.554 s，`total` 中位数 35.3 ms。若 fixture 混入，128 行的 `rows/s` 会是 `128/1.588 ≈ 81`，正好复现 v1 被撤回的 `~78 rows/s`；实测 3801，比 v1 高 48.7 倍，与“扣掉 97.9% 建库时间”一致。
   - `stage_sum_ratio_pct` 12 组全部 99.98–100.00%，即阶段之和几乎等于 total，没有第三种成本藏在里面。
2. **`rows_masked == rows`**：`perf_baseline_test.go:489` 断言，12 组样本逐条满足（`results.csv` 的 `rows_per_s_mean` 反推亦一致）。
3. **产物单源可再生产**：把 `docs/perf/issue13-phase2-62b4482` 复制到 `/tmp/perfregen`，只放 `samples.json`，`MASK_RIVER_PERF_REGEN=1` 跑 `TestPerfBaseline` → `summary.md`、`results.csv`、`stats.csv`、`environment.json`、`samples.json` 五个文件与提交版 `diff -q` **全部相同**。
4. **产物防手改**：`/tmp/perfrev` 里把 `summary.md` 的 `512/short/w1` rows/s 从 `4898` 改成 `5898`，`go test -run TestPerfArtifactsMatchSamples` **FAIL（exit 1）**，报错 `perf_baseline_test.go:524`。这条测试有承载力。
5. **算术自洽**：`rows/s(mean)` 与 `rows/total` 逐样本再平均的结果一致（如 128/long/w1：3801 vs 128/33.79 ms = 3788，差异来自“先比值再平均”的 Jensen 项，已在口径里写明）；`apply share` 与 `apply median / total median` 一致（2048/short/w1：73.97% vs 290.6/393.6 = 73.8%）；报告的交叉验证 `32 批 × 9.624 ms = 307 ms` 与实测 apply 中位数 290.6–304.4 ms 相差 1–5.6%，属同向吻合。
6. **测试有承载力（两个真实变异）**
   - 审批：`internal/runner/execute.go:522` 把 `if err := e.checkApproval(...); err != nil` 改成 `...; false && err != nil` → `TestUnapprovedApplyCannotWrite` **FAIL**：`Apply() succeeded, want a refusal; ... Complete:true`，未审批也写入了 5 行。已还原。
   - 正确性：`internal/runner/execute.go:739` 把 receipt 的 `Rows: n` 改成 `Rows: n - 1` → `TestApplyReceiptsAndValidationAgree` **FAIL**：`workflow_concurrency_safety_test.go:215: receipt for {"maskriver" "main" "people"} reported 0 rows`。已还原。
   - 还原后 `git status --porcelain` 为空，未留下任何改动。
7. **范围干净**：`git diff 62b4482...HEAD --stat` 只有 14 个文件：`.github/workflows/bench.yml`、`docs/perf/*`、`internal/runner/{perf_baseline_test,perf_cpu_linux_test,perf_cpu_other_test,apply_diagnosis_test,workflow_concurrency_safety_test,workflow_bench_test}.go`、`internal/runner/workflow.go`。**没有** `pkg/contracts`、`internal/db`、`internal/mask`、`internal/verify` 改动；`workflow.go` 只多了只读的 `Engine.PoolStats()`（`db.DBStats` 观测，不建第二条连接、不发 SQL）；日志/序列化里无 DSN、密钥或真实 PII（扫描 `/home/`、`password`、`dsn=`、`secret`、`BEGIN ... PRIVATE` 均无命中；样本值是 `@example.invalid` 合成值）；BatchSize/事务/journal/worker 默认值未改（`apply_diagnosis_test.go` 文件头明确声明“只测量”）。
8. **门禁**：`gofmt -l internal cmd scripts` → 无输出，exit 0；`go vet ./...` → exit 0；`git diff --check` → exit 0；`git diff 62b4482...HEAD --check` → exit 0（v1 的 `raw.txt:4` 行尾空白已不复存在）。

**不能证明**

1. **不证明本机之外的复现性**：全部数字来自同一台 Ryzen 9 7940H / go1.26.3 / linux-amd64，`samples.json` 是 5 次迭代的结果，`stddev_pct` 0.62–21.03%（2048/long/w4 有一次离群，mean 491.8 vs median 426.5 ms）。跨机、跨平台未测。
2. **不证明统计显著性**：没有做检验，报告也已明说。`workers=1 vs 4` 的“8/12 略慢、4/12 略快、1–5%”只在噪声量级内，不能当并发结论用。
3. **不证明成本模型是“提交”**（见 P2-3），只是否证了“每行脱敏计算主导”。
4. **不证明规模外推**：只测到 2048 行；10k/100k/1M 与 workers 2/8/16 未运行。报告“规模越大 rows/s 越高（固定开销摊薄）”在 128→2048 区间成立，不能外推到 1M。
5. **不证明 CLI 端到端性能**：仍是 Runner 级；flag 解析与密钥读取不在数字内（`workflow_bench_test.go` 文件头已声明）。
6. **不证明 MySQL 侧**：完全未测。

## 本 PR 满足的 #13 AC（按本 PR 交付物判断）

- **真实路径基线（Runner 级，SQLite）**：满足。Scan/Plan/Preview/真实 Ed25519 签名 Approve/Apply 全走生产门禁（`checkApproval` 在 `execute.go:522` 生效，变异测试证明其承载）。
- **计时区间正确（fixture/关闭/重建在外）**：满足（P2-1 的簿记残留 <0.1%，不构成违反）。
- **数字不可手改、单源派生**：满足（变异后 FAIL，再生逐字节一致）。
- **资源/连接池证据**：满足，口径写清（进程级 RSS/CPU 单迭代增量、engine 自有连接池计数、非 Linux 输出 UNVERIFIED 而非 0）。
- **正确性与安全回归**：满足，7 个真实签名测试；抽查 2 个经变异验证有承载力。
- **不改默认值、不夹带优化**：满足（无 BatchSize/事务/journal/worker 改动，无 `pkg/contracts` 变更）。
- **非作者独立评审**：由本文件满足。
- **统计显著性 / 全矩阵规模 / MySQL / CLI E2E**：不满足，报告已标为未验证，属后续阶段。

## UNVERIFIED（本轮未验证）

- `go test ./...` 全量套件（超出时间预算）。被改动文件的包编译已被定向测试实际编译并执行，其它包的回归未验证。
- `go test -race ./...`：**跳过**（任务允许在 5 分钟内跑完才做；未尝试）。
- 未重跑基准矩阵（按指示复用提交产物），因此 `results.csv`/`summary.md` 的数值本身可复现性未被独立重测，只验证了“由 `samples.json` 可精确再生”。
- 非 Linux 路径（`perf_cpu_other_test.go`，`712a2e0` 的 Windows 可编译性修复）未在非 Linux 上验证；本机只能确认 linux 构建与测试通过。
- CI（`.github/workflows/bench.yml` 新增的 `MASK_RIVER_PERF_ITERS=1` 步骤）未实际触发，仅静态阅读；其产物上传依赖已有的 `path: bench-out/`（`bench.yml:77`），看着成立但未运行。

## 我没有检查的（明确列出）

- `internal/db`、`internal/mask`、`internal/verify`、`internal/history` 内部实现：本 PR 未改它们，我也未审其正确性。
- `workflow_concurrency_safety_test.go` 中另外 5 个测试（`TestWorkersStopAndDoNotLeak`、`TestCancellationStopsEveryWorker`、`TestWorkflowLeavesNoSidecarFiles`、`TestWorkerCountNeverChangesResultData`、`TestPreviewDoesNotWriteTarget`）未做变异验证，只审读了断言。
- `perf_cpu_linux_test.go` / `perf_cpu_other_test.go` 的 `getrusage` 具体实现，只看接口一致性与 Windows 构建标签。
- `scripts/`、`docs/feature-matrix.md`、README 等文档一致性（本 PR 未改它们，除 `docs/perf/*` 与 `docs/reviews/`）。
- 提交签名/作者身份、PR 元数据（不允许访问 GitHub）。

## 建议（不阻塞合并）

1. `workflow_bench_test.go:234`：把 `b.StartTimer()` 移到簿记之后（P2-1）。
2. 抽出 `validateSamples`，让 `TestPerfArtifactsMatchSamples` 也执行 `rows_masked == rows` 与 `stage/total` 区间（P2-2）。
3. `diagnoseApply` 记录/断言 `spy.commit == receipts`，把“提交次数”从推断变实测；同时把报告里的“每批成本基本恒定”改成“每批固定开销（含提交）主导”（P2-3）。
4. 下一阶段：减少提交次数或合并行写入、10k 行档、workers 2/8/16、MySQL 与 CLI E2E。

---

PR #67 verdict: OK with notes
