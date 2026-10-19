# PR #66 独立评审 — Issue #13 第一阶段 SQLite 性能基线

- **范围**：`perf/issue13-sqlite-baseline-20261019` vs base `main` = `1d56e4b`，head `50d22b4`。
- **评审者**：独立 QA + Security 上下文；未参与该 PR 的任何实现。非作者评审，符合 Issue #13 的“非作者评审”要求。
- **评审机**：同机复核，linux/amd64，`go1.26.3`，AMD Ryzen 9 7940H，16 核（与 `environment.json` 一致）。
- **只读**：除本文件外未改动工作区；未提交、未推送、未访问 GitHub、未启动子代理。
- **方法**：读 diff + 读真实调用链 + 复跑基准（`-benchtime=3x`）+ CPU profile + `strace -c` 计数 + 全套门禁。

## 结论摘要

真实路径**成立**：Scan / Plan / Preview / 真实 Ed25519 签名 Approve / Apply 全部执行，`checkApproval`、preflight、dryRun 三个门禁都在，`cfg.Approved` 是生产门禁输入而非绕过，`testApproval` 桩未被使用。

但**交付的度量无效**：`ns/op`、`rows/s` 以及 `summary.md` 的三条主要结论，测的是**每轮重建 fixture 的开销**，不是工作流。工作流本体只占 `ns/op` 的 **1.68%–3.14%**。因此这不是一条可用于决策的基线，且 `summary.md` 有两条可被自身数据直接证伪的结论。

| 门禁 | 命令 | 退出码 |
|---|---|---|
| 格式 | `gofmt -l internal cmd scripts` | 0（无输出） |
| 测试 | `go test -count=1 ./...` | 0（全部 ok） |
| 静态检查 | `go vet ./...` | 0 |
| 空白/补丁完整性 | `git diff --check 1d56e4b..HEAD` | **2**（`raw.txt:4` 行尾空白） |

---

## P0

### P0-1 `ns/op` 与 `rows/s` 测的是 fixture 重建，不是工作流

**位置**：`internal/runner/workflow_bench_test.go:209`（`b.N` 循环包住整个 `benchWorkflow`）、`:75`（`benchFixture` 在循环内）、`:135`（`total` 计算后**从未上报**）、`:218`（`rows/s` 用 `b.Elapsed()`）、`:158`（fixture 逐行 `INSERT`）。

```go
209: for i := 0; i < b.N; i++ {
210:     total, stages, rows := benchWorkflow(b, cfg)   // fixture 建库、Open、建 history store 都在这一次调用里
...
218:     b.ReportMetric(float64(lastRows)*float64(b.N)/b.Elapsed().Seconds(), "rows/s")
```

`benchWorkflow` 在 `start := time.Now()`（:92）之前先做 `benchFixture`（:75，两个库各 `rows` 次单条 `INSERT`）、`Open`（:79）、`benchStore`（:87）。这些都在 `ns/op` 的计时窗口内；只有 `stages` 把 fixture 排除在外。`total`（:135，工作流总时长）被返回却只用于 `total <= 0` 检查（:212），从未 `ReportMetric`，所以最终输出的是 `b.N` 的墙钟时间而不是注释（:199-201）承诺的 “the workflow total”。

**复现与证据**（同一份 `results.csv` 即可自证）：

```
配置              ns/op    阶段和(µs)  阶段和/ns-op   上报 rows/s   阶段和推算 rows/s
128/short/w1    1.632s     51 201       3.14%          78.42          2 500
512/short/w1    6.558s    145 581       2.22%          78.07          3 517
2048/short/w1  25.972s    572 003       2.20%          78.86          3 580
```

12 组配置全部落在 **1.68%–3.14%**。即 `ns/op` 里 **97% 以上**不是工作流。

1. **CPU profile**（`go test -bench 'BenchmarkWorkflow/rows=128/shape=short/workers=1$' -benchtime=3x -cpuprofile=/tmp/cpu.prof`，再 `go tool pprof -list=benchFixture`）：

   ```
   740ms / 910ms samples (81.32%) 在 benchFixture 的 INSERT 循环（:158）
   database/sql.(*DB).execDC 累计 740ms，调用者 100% 来自 benchFixture
   ```

2. **`strace -f -c` 计数**（挂钟被放大，但调用次数精确）：
   - rows=128 一次迭代：`fsync` **1042** 次 ≈ 4 ×（2×128 + 2 条 `CREATE`/`INSERT`）= 1032。
   - rows=512 一次迭代：`fsync` **4138** 次 ≈ 4 ×（2×512 + 2）= 4104。
   - 真实工作流（apply 2–32 个 batch、一个 Writer、preview/dryRun 无写）只贡献约 **10–34** 次。

   即每轮迭代 99% 的持久化提交来自 fixture，不是 Apply。

3. `rows/s` **不携带新信息**：`rows*b.N/b.Elapsed()` 与 `rows/ns_per_op` 代数恒等；对 `results.csv` 12 行逐行核对，最大偏差 0.0048。它把 fixture 时间算进分子分母，因此比工作流真实行吞吐低 **30–60 倍**（78 vs 2 500–4 770 rows/s）。

**影响**：`docs/perf/issue13-1d56e4b/{summary.md,results.csv,results.json,throughput.svg}` 的 headline 数字（`ns/op`、`rows/s`，SVG 标题写作 “runner workflow ns/op”）都不能代表 runner 工作流，也不能作为后续优化对照的基线。

**修复方向**（不扩大范围的最小改法）：把 fixture 建在 `b.N` 循环之外并用 `b.ResetTimer()`（若必须每轮重建，则循环内 `b.StopTimer()`/`b.StartTimer()` 包住 `benchFixture`+`Open`+`benchStore`），然后把 `total` 作为“workflow”指标上报，`rows/s` 用 `rows/total` 而非 `b.Elapsed()`。同时把 `Open`/`benchStore` 明确排除或单列。

---

## P1

### P1-1 `summary.md:11` “apply 阶段占据绝大部分时间”与其自身数据矛盾

`summary.md:11`：2048 行 `apply` 约 **341–399 ms/批**，“占据绝大部分时间”。

- `results.csv`：2048 行 `apply` = 340 732–398 782 µs = **0.34–0.40 s**，而同一行 `ns/op` = **25.9–26.2 s** → `apply` 占 **1.3%–1.5%**，不是“绝大部分”。
- 若以阶段和为分母（0.55–0.61 s），`apply` 占 60%–70%，这才是“多数”成立的口径，但 `summary.md` 写的是“绝大部分时间”，未限定分母。
- “ms/**批**”是量纲错误：`BatchSize=64`（`workflow_bench_test.go:77`），2048 行是 **32 个 batch**，341–399 ms 是整个工作流的 apply 总时长，按批读会夸大 32 倍。

结论“优化方向应优先看 Apply 的 SQL 与提交策略”本身在阶段和口径下可以成立，但当前表述会把读者引向错误的数量级。

### P1-2 `summary.md:9`/`:12` 的 workers 结论不可复现，且超出该度量能分辨的范围

`summary.md:9`：“12 组配置全部落在 76.8–80.9 rows/s … 多数配置下 workers=4 略慢”，`:12`：“workers=4 不带来收益，改 Worker 数量不会改善吞吐”。

作者数据里 w4 相对 w1 的 `ns/op` 差异只有 **+0.3%～+1.7% / −3.3% / −0.1%**（3/6 对里 w4 更快）。同机复核（`-benchtime=3x`，`-count=1`）把方向翻转：

| 配置 | PR `ns/op` | 本次复核 | 偏差 | 本次 w4 vs w1 |
|---|---|---|---|---|
| 128/short/w1 | 1.632s | 1.746s | +7.0% | — |
| 128/short/w4 | 1.649s | 1.657s | +0.5% | **w4 快 5.1%** |
| 128/long/w1 | 1.645s | 1.700s | +3.3% | — |
| 128/long/w4 | 1.651s | 1.637s | −0.8% | **w4 快 3.7%** |
| 512/short/w1 | 6.558s | 6.961s | +6.1% | — |
| 512/short/w4 | 6.669s | 6.434s | −3.5% | **w4 快 7.6%** |

同一台机器、同一命令，128 组里 **3/4 配置 w4 更快**，与“多数配置下 workers=4 略慢”相反；run-to-run 噪声（±7%）**大于**被比较的效应（<2%）。因此 `:9` 的方向性说法不可复现。

更重要的是 **`:12` 与 `:9` 的推断越界**：`Workers` 在生产代码里只影响写路径的 `maskBatch`（`internal/runner/execute.go:188`，全仓唯一非测试引用），而写路径只占 `ns/op` 的 1%–2%（P0-1）。也就是说该 harness 对 workers 的可探测上界约 1%–2%，远低于噪声。**数据支持的最强表述是“本 harness 无法分辨 worker 数量对吞吐的影响”，而不是“worker pool 不是瓶颈”。** 后者需要先把 P0-1 修掉。

### P1-3 `bench.yml` 没有覆盖本 PR 的新基准，“bench job 通过”不能作为本工作的证据

`.github/workflows/bench.yml` 的基准步骤只有：

```
go test -run '^$' -bench '^BenchmarkMaskBatchWorkers$' -benchmem -count=3 -benchtime=300ms ./internal/runner
```

`BenchmarkWorkflow`（`internal/runner/workflow_bench_test.go:202`）**不在**该正则内，CI 从不运行它；`ci.yml` 也没有任何 `-bench`。该 workflow 确实会因 `internal/runner/**` 路径变更被触发（即“跑了且绿”），但**跑的是旧的 128-value 内存基准**，与本 PR 的 SQLite 工作流数字无关。

另需注意：即便把正则改宽，`-benchtime=300ms` + `timeout-minutes: 20` 也容纳不下 12 组 × 26 s 的配置（约 9–15 分钟的下限，且 2048 组单次 op 就 26 s，`300ms` benchtime 语义下只会取 1 次）。**这条 CI 证据必须视为“未覆盖”，不能计入本 PR 的验收。**

### P1-4 `summary.md` 结果表含未填占位符与错误数值

`docs/perf/issue13-1d56e4b/summary.md:18` 起：

- `:18` `scan µs = 5 262`，而 `raw.txt` / `results.csv` / `results.json` 三处一致为 **3 633**。
- `:22` `22 13x`、`:23` `19 98x`、`:25` `7x xxx`、`:26` `8x xxx` —— 未填完的占位符，直接进入已提交的结论文档。

`:27` 有“精确数据以 CSV/JSON 为准”的免责声明，但一张同时含**错误值**与**占位符**的表格放在“结论”之后，读者无法判断哪些格子可信。把表格换成 CSV 全列或删掉表格，成本极低。

---

## P2

- **P2-1 未使用的导出生产 API**：`internal/db/stats.go:14 func DBStats` 在**全仓库零引用**（含测试），`summary.md` 同时声明连接统计 UNVERIFIED（“本轮未实现”）。这是为新度量预留但从未接线的生产符号，属于推测性脚手架；且 `internal/db` 不在“基准与证据文档”的显式范围内，是否越出本任务授权的文件集合需 Orchestrator 核对。该函数本身无行为副作用（不取锁、不发 SQL，单一构造点保证 `db` 非 nil），安全风险为零。
- **P2-2 门禁失败**：`git diff --check 1d56e4b..HEAD` 退出码 **2**，`docs/perf/issue13-1d56e4b/raw.txt:4` 行尾空白（从 `go test` 输出直接粘贴所致）。
- **P2-3 估计量描述不实**：`workflow_bench_test.go:200` 写 “per-stage **medians**”，`summary.md:5` 写“3 次的合并结果；仅报**中位数**意义上的单值”。实际 `go test` 报的是 3 次的**算术平均**，而 `us/scan…us/apply` 只取**最后一次迭代的单样本**（`lastStages`，:221-222），既不是中位数也不是 3 次汇总。另 `:70` 注释称 fixture “由 `acceptanceFixture` 构建”，实际是 `benchFixture`。
- **P2-4 无正确性断言**：基准不检查 `applied.Complete`，也不检查 `Committed` 行数是否覆盖全部计划行（只要求 `> 0` 才上报，:216）。`rowsMasked` 取自 receipt，因此分子本身诚实，但“跑通”缺少显式断言。

---

## 已经站得住的正面结论（同样经独立验证）

1. **真实路径成立，无门禁旁路**——逐条核对调用链：
   - `Open`（`internal/runner/workflow.go:35`）→ 生产 Engine，两个真实 Reader。
   - `Scan` → 生产 scan/plan 证据路径（`cfg.Approved=true` 不豁免扫描门禁，见 `internal/runner/scan_gate_test.go` 的既有回归）。
   - `Preview`（`execute.go:200`）→ 先跑同一 `preflight`（:208）再 `visit` 全量干跑。
   - `Approve` → **真实** `history.Store`（临时目录）+ **真实** Ed25519：`ed25519.GenerateKey` + `history.NewTrustedKeys` + `history.Signer`（`workflow_bench_test.go:184-191`），签名与验签都跑。
   - `Apply`（`execute.go:505`）→ 形状校验 → `ExplicitApply && cfg.Approved`（:516）→ `checkApproval`（:526，`approval.go:60` 经 `store.Lookup` 真实验签/有效期/范围绑定，`recordAuthorizes` 复检）→ `preflight`（:529）→ `dryRun`（:543）→ 真实 Writer。
   - 未使用测试桩：`approval_test.go:33` 的 `testApproval` 在基准中**无引用**；`testReviewerKeyID/testReviewerLabel` 只是共享的字符串常量。
   - 任一环节失败都会 `tb.Fatalf`，所以“基准跑绿”本身就证明门禁真的执行了。
2. **没有为数字改产品行为**：diff 仅 **8 个新增文件**（6 份 `docs/perf/**` 证据 + 1 个基准测试 + 1 个未接线的 `internal/db/stats.go`）。**无任何既有文件被修改**；未触碰并发、SQL、事务或门禁代码；`pkg/contracts` 未变（`APIRevision=m1a-v1` 未动）。
3. **“目标被二次脱敏”的隐患不存在**：fixture 在 `benchWorkflow` 内每轮重建（:75），`b.N` 迭代不会复用已脱敏的 target。（代价正是 P0-1。）
4. **阶段计时没有被 fixture 污染**：`start` 在 fixture/Open 之后（:92），stage 列是真实工作流耗时——这是本 PR 唯一可直接使用的数据，只是单样本。
5. **诚实的 UNVERIFIED / SKIP 标注**：`summary.md:44-46` 把进程 RSS、运行期采样、goroutine、`sql.DB` 连接、CPU 时间标为 **UNVERIFIED**，把 10k/100k/1M 行与 workers 2/8/16 标为 **SKIP**；`raw.txt` 里确实没有这些行，标注与实际一致，没有虚报。48 字符 long 形态被 `ErrUnsafe` 拒绝、记为“未测量”也是诚实处理。

---

## 证据证明什么、不证明什么

**证明**：门禁齐全的真实 Runner 工作流能在真实 SQLite 上跑通 128/512/2048 行 × 2 种值形态 × 1/4 workers；其**单样本阶段耗时**为 scan 3.6–57.7 ms、plan 3.9–63.2 ms、preview 6.3–81.9 ms、approve 3.3–13.0 ms、apply 24–399 ms（rows=128 最慢 30 ms，2048 最慢 399 ms）；阶段和占总时间 1.68%–3.14%。

**不证明**：任何 runner/CLI 的吞吐或延迟基线（`ns/op` 97% 是 fixture）；worker 数量与吞吐的关系（信号 1%–2% vs 噪声 ±7%）；10k 行以上的任何外推；CPU/RSS/连接数；MySQL；跨机器可比性（本次复核为同机）；也没有证明 `bench.yml` 覆盖了这些数字。

## 本 PR 实际满足的 Issue #13 验收项

| Issue #13 验收项（见 `docs/performance.md:42-49`） | 状态 |
|---|---|
| 真实 SQLite 端到端（多行数/多分布）吞吐与延迟对比 | **部分**：矩阵与真实路径 harness 建立（结构性满足）；**数字无效**（P0-1），结论不可用 → 不满足 |
| CPU / RSS / 连接在持续整库负载下的度量 | **未满足**（诚实标为 UNVERIFIED，未实现） |
| 非作者评审 | **满足**（本文件） |
| “任何改进必须由实测数据支持，而非假定并行加速” | **部分**：没有声称加速（好）；但 P1-1/P1-2 反向声称了数据不支持的因果结论 |
| 真实路径、门禁不被旁路 | **满足** |

## 建议处置

1. 修 P0-1（fixture 移出计时或 `StopTimer`/`StartTimer`，上报 `total`），重跑 12 组，重新生成 csv/json/svg/summary。
2. 修 P1-1/P1-2 的表述，或在新数据上重写结论；workers 相关只保留“本 harness 不可分辨”。
3. 删掉 `summary.md` 表格里的占位符与错误值（P1-4）；去掉 `raw.txt` 的行尾空白（P2-2）。
4. `bench.yml` 增加 `^BenchmarkWorkflow$` 的独立 job（或不宣称 CI 覆盖），并明确其 benchtime 预算（P1-3）。
5. `internal/db/stats.go` 要么接线到基准并报连接数，要么移出本 PR（P2-1）。

---

PR #66 verdict: BLOCK
