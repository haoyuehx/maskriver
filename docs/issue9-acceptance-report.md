# Issue #9 验收证据报告（AC1–AC5）

- 日期：2026-10-18
- 关联：[Issue #9](https://github.com/haoyuehx/maskriver/issues/9)、[#11](https://github.com/haoyuehx/maskriver/issues/11)
- 基线：`origin/main` = `4ac8fd4891a6a2206a891f49662cff75be5c2c4a`（PR #56 merge）
- 被取证提交：`c5cbeb0107c953231d929965f8d8693e5f2e3839`（PR #56 head）。`git diff HEAD origin/main` 为空，两者工作树逐字节相同；本文所有命令都在该工作树上执行。
- 冻结契约：`APIRevision=m1a-v1`，本次未改动 `pkg/contracts`、`go.mod`、`go.sum` 或任何 Go 源码。
- 数据：仅本项目自编合成 fixture（`example.invalid` 地址、`synthetic-*` 文本）。文中只出现计数、文件大小与 SHA-256；不出现原始值、主键值、DSN、密钥字节或本机绝对路径（统一写作 `<repo>`、`<tmp>`）。
- 本文性质：证据记录，不是安全批准，也不是 M1 发布结论。方法：真实二进制子进程 + 真实 SQLite 驱动 + 驱动级 SQL Spy + Writer 能力 Spy + 文件树（含 sidecar）哈希；包内探针通过 `go test -overlay` 从 `<tmp>` 编译，**不新增仓库文件、不改产品代码**。

## 0. 结论摘要

| AC | 结论 | 一句话依据 |
|---|---|---|
| AC1 功能 | **PASS** | Scan/Preview/签名审批/新进程 Apply/严格 Validate 在单表与两表真实 SQLite 上退出码 0；无 `--apply` 时退出 0 且目标树不变；`--apply` 是唯一写意图；UNKNOWN、截断、无稳定主键、UNIQUE/CHECK/FK/触发器、WAL、审批缺失/伪造/过期/撤销/跨计划全部 fail-closed（退出 1 或 2）。`--strict` 现状见 §9，未升级。 |
| AC2 测试 | **PASS** | `go test -count=1 ./...` 退出 0（298 PASS / 0 FAIL / 3 SKIP）；`go test -race -count=1 ./...` 退出 0；真实 SQLite 端到端、Preview/Apply 取值一致、文件树 + SQL Spy 零写、扫描故障/取消/提交未知注入均有实跑用例。 |
| AC3 安全性 | **PASS** | 所有 CLI 输出经脱敏断言（值/路径/密钥）；可读操作驱动级 SQL Spy `writes=0`；源快照在 Apply 前后字节不变；Preflight/审批/证据门禁都在写之前。信任边界（本地签名，非 RBAC）已在代码注释与 README 声明，不能据此宣称抗内部人。 |
| AC4 文档/Feature Matrix | **PASS** | `docs/feature-matrix.md:32,65,76,123,136`、`docs/test-plan.md` P2 表（八个：P2-1…P2-8）、`docs/roadmap.md:11-13` 的每项陈述都能对应到本次实跑的用例；`docs/m1-runner-acceptance.md` 已改为带日期说明的历史记录。 |
| AC5 最终交付证据 | **PASS** | 本文件即交付物：CLI 逐命令退出码与行数、零副作用文件树 + SQL Spy + Writer 计数、23 项拒绝矩阵、故障/事务证据、UNVERIFIED 清单。 |

FAIL 数量：**0**。AC1–AC5 全部 PASS；但 AC 之外仍有 10 类范围项 UNVERIFIED（§10），其中 MySQL CLI、checkpoint、exactly-once、生产级检测准确率、macOS CI 与 `--strict` 独立语义不在本次证据能力内。

## 1. 环境与静态检查

| 项 | 值 | 来源命令 |
|---|---|---|
| 提交 | `c5cbeb0107c953231d929965f8d8693e5f2e3839` | `git log -1` |
| 目标分支 | `origin/main` = `4ac8fd4891a6a2206a891f49662cff75be5c2c4a`，树相同 | `git diff HEAD origin/main`（空） |
| Go | `go1.26.3` | `go version` |
| OS/arch | Linux `7.0.0-38-generic` / `x86_64` | `uname -srm` |
| SQLite 驱动 | `modernc.org/sqlite v1.60.1` | `go.mod` |
| 工作树 | 取证开始时 `git status --porcelain` 为空 | `git status --porcelain` |

| 检查 | 结果 | 证据 |
|---|---|---|
| `gofmt -l internal cmd pkg tools` | 退出 0，未列出任何文件 | 空输出 |
| `go test -count=1 ./...` | **退出 0**：10 个包 `ok`，`tools/fixture` 无测试 | 逐包 `ok` 行 |
| `go test -count=1 -v ./...` | **退出 0**：`--- PASS` 298、`--- FAIL` 0、`--- SKIP` 3 | 见下 |
| `go vet ./...` | 退出 0 | 空输出 |
| `go test -race -count=1 ./...` | **退出 0**：10 个包 `ok` | 逐包 `ok` 行 |
| `git diff --check` / `git diff --cached --check` | 均退出 0 | 空输出 |

3 个 SKIP 的原因：`TestCLIProcessChild`（子进程复用入口，仅在设置了子进程环境变量时运行）、`TestMySQLAdapter`、`TestMySQLFixture`（本机未提供隔离 MySQL socket）。SKIP 不是通过项。

## 2. CI 真实结论（该 SHA）

`gh run list --commit <sha> --json`：

| head SHA | 运行 | 事件 | 结论 | 作业与逐步结论 |
|---|---|---|---|---|
| `c5cbeb01…` | 39127514031 | pull_request | **success** | `Go (ubuntu-latest)` success（Format/Tests/Vet/**Race** 全 success）；`Go (windows-latest)` success（Format/Tests/Vet success，**Race skipped**，步骤条件为 `if: runner.os == 'Linux'`）；`Isolated MySQL fixture and adapter acceptance` success |
| `4ac8fd48…` | 39127793131 | push (main) | **success** | 同上三个作业，同上结论 |

该 diff 上**未运行**的工作流及原因：

- `Synthetic performance benchmarks`（`bench.yml`）：仅 `pull_request` 且路径过滤为 `internal/runner/**`、`internal/mask/**`、`cmd/maskriver/**`、`scripts/bench_*.py`、`.github/workflows/bench.yml`。本 PR 的改动只有 `README.md` 与 `docs/**`（`git diff --name-only` 确认），因此该工作流不适用，不是“应跑而未跑”，也不能当作通过项。
- `bench.yml` 的 `bench` 作业因此同样未运行；仓库不存在 macOS 作业，故 macOS 无 CI 覆盖（见 §10）。
- 结论边界：CI 只在 Ubuntu/Windows 上执行了 `go test -count=1 -v ./...`、`go vet ./...`、Ubuntu race 与隔离 MySQL 适配器；CI 不执行本报告的 CLI 取证脚本，CLI 证据来自 §3–§7 的本机实跑。

## 3. CLI 实录（单表与两表）

每个 `CMD` 行都是**独立进程**：编译后的二进制 `maskriver` 每次单独 `exec`。审批在 `approve` 进程签发，写入在另一个 `mask` 进程读取 deployment trust file 完成，`validate` 又是第三个进程。等价的多进程链路还由 `TestE2EPlanIssueNewProcessApply`（`cmd/maskriver/approval_e2e_test.go:201`，真实 re-exec 子进程）固定。

### 3.1 单表（25 行，`people.email`，非 UNIQUE 规划列）

```text
CMD scan       exit=0  scan: complete=true columns=1 issues=0
CMD preview    exit=0  preview: dry-run=true rows=25 complete=true (no row values shown)
CMD approve    exit=0  approve: stored=true scopes=1 valid_for=24h0m0s
ROWS source-post-approve  rows=25 distinct_email=25
ROWS target-pre-apply     rows=25 distinct_email=25
CMD apply      exit=0  apply: complete=true committed_batches=1 committed_rows=25 outcome_unknown=false
                       apply: table 1 batches=1 rows=25
ROWS target-post-apply    rows=25 distinct_email=25
CMD validate   exit=0  validate: pass=true tables=1 rows=25 issues=0
ROWS source-post-validate rows=25 distinct_email=25
```

文件树（含 sidecar）变化：只有 `target.db` 变为 `size=16384 sha256=8fb8711ddb3ee58cfc4c56d9117d54d36920823ca75180d5851e14cf30e430bc`（写前为 `bb9d03e754d33b2cb43fa5066a7fecad519734dad83148bb099a8fc099801845`，`size=16384`），并新增 1 个审批记录 `history/<plan-id>.decision`（`size=922 sha256=ea7925bb531ac8d857dd74b143e23a90492adc84a3853e26b9457a1b97d2365a`）。`source.db` 未出现在差异里（字节不变）；整个过程**没有**出现 `-wal`、`-shm`、`-journal` sidecar。行数与 `COUNT(DISTINCT email)` 均为 25 → 写后仍是 25 个不同值，没有把唯一性压掉（本列的 UNIQUE 在 `handle` 上，规划列只有普通索引）。

### 3.2 两表（`people.email` + `contacts.email`，各 25 行）

```text
CMD scan2      exit=0  scan: complete=true columns=2 issues=0
CMD preview2   exit=0  preview: dry-run=true rows=50 complete=true (no row values shown)
CMD approve2   exit=0  approve: stored=true scopes=2 valid_for=24h0m0s
ROWS target2-pre-apply  people=rows=25 distinct_email=25 contacts=rows=25 distinct_email=25
CMD apply2     exit=0  apply: complete=true committed_batches=2 committed_rows=50 outcome_unknown=false
                       apply: table 1 batches=1 rows=25
                       apply: table 2 batches=1 rows=25
ROWS target2-post-apply people=rows=25 distinct_email=25 contacts=rows=25 distinct_email=25
CMD validate2  exit=0  validate: pass=true tables=2 rows=50 issues=0
```

文件树变化：只有 `target.db`（`12288`，`cb9f0ab85149e96d61517641ded19766f7ef30f623f1b6975da698c8fc6a1add` → `b66fb6fbf566b7e835207df20bab5db0ca7b862f5bafe4c3475e7b15bf45ebbe`）与新增的 1 个 `history/<plan-id>.decision`（`size=999 sha256=22ad6b4ee30b312748f6729b0d6b86c84485d78196486144f418486f9b041e1f`）。`source.db` 不变，无 sidecar。整计划语义：1 条记录覆盖 2 个 scope，`committed_batches=2`。

`--strict` 在两次 validate 中都退出 0 且 `pass=true`；它的证明范围见 §9。

## 4. 撤销负例（目标仍是未脱敏状态）

目的：证明“撤销”这道闸门本身会拒绝写入，而不是用“已脱敏目标被再次 apply 拒绝”来冒充。

| 步骤 | 命令/观测 | 结果 |
|---|---|---|
| 1 | 从初始 fixture 复制一份**未脱敏**目标（`target.db` = `bb9d03e7…`，与 `source.db` 字节相同） | 目标未脱敏 |
| 2 | `scan`、`mask`（preview） | 均退出 0；目标树不变 |
| 3 | `approve`（独立进程签发） | 退出 0，`history/<plan-id>.decision` 出现 |
| 4 | **撤销前** `history.Store.Lookup`（history 包自带 fixture 的计划，3 个 scope；同一存储实现的读取路径） | `ok=true`，`revoked_at_nil=true` |
| 5 | `revoke`（独立进程） | 退出 0；记录 `revoked_at` 被写入（payload `revoked_at` 字段存在） |
| 6 | **撤销后** 同一 `Lookup` | 失败，`errors.Is(err, errRevoked)=true`，`errors.Is(err, errBinding)=false` |
| 7 | **同一未脱敏目标** + 同一 trust/history 执行 `mask --approved --apply`（新进程） | **退出 1**，`apply: complete=false committed_batches=0 committed_rows=0` |
| 8 | 目标树（含 sidecar）对比 | **完全相同**：`target.db` 仍是 `size=16384 sha256=bb9d03e7…`；无 sidecar；history 文件前后同字节（`size=967 sha256=ca9f2d48…`） |
| 9 | 对照：同 fixture、`approve` 后不 `revoke` 直接 apply | **退出 0**，`committed_rows=25`，目标变为 `8fb8711d…` |
| 10 | 反驳“环境问题”：`revoke` 后**重新** `approve` 再 apply | **退出 0**，`committed_rows=25`，目标变为 `8fb8711d…` |

`revoke` 只改那一个 decision 文件：写入前/后 store 目录都只有 1 个文件，文件名集合相同（`revoke-file-set before=1 after=1 same_names=true`，后缀 `.decision`）。第 7 步的 Writer 能力计数来自包内探针（`decision-revoked`）：`opens=0 begins=0 updates=0 commits=0 rollbacks=0`，`target_changed=no`。

闸门归属：第 7 步拒绝来自 `Engine.checkApproval`（`internal/runner/approval.go`）对 `history.Store.Lookup` 的调用；`Lookup` 的 cause 由探针命名为 `errRevoked`（`internal/history/record.go:288,386`）。第 9/10 步证明同一命令形态在同一 fixture 上会成功，因此第 7 步的差异只可能是撤销。

## 5. 零副作用证据

### 5.1 方法

- **文件树**：对每个场景，执行前后各遍历一次 case 目录全部文件，按区域分组（`source*`、`target*`、`history/*`、`aux`），记录文件名、字节数与完整 SHA-256。`-wal`、`-shm`、`-journal` 都会以独立文件出现，因此 sidecar 被计入（这正是测试计划 P2-5 指出的 CLI 用例 `targetDigest` 忽略 sidecar 的补足）。
- **驱动级 SQL Spy**：`internal/db.SQLSpy` 是实例级连接工厂，观测真实驱动收到的语句并分类写/事务意图。
- **Writer 能力 Spy**：包内 `writerSpy` 记录 `opens/begins/updates/commits/rollbacks`。

### 5.2 初始 fixture 摘要（单表，未脱敏；多数场景都由它复制）

| 文件 | 字节 | SHA-256 |
|---|---|---|
| `source.db` | 16384 | `bb9d03e754d33b2cb43fa5066a7fecad519734dad83148bb099a8fc099801845` |
| `target.db` | 16384 | `bb9d03e754d33b2cb43fa5066a7fecad519734dad83148bb099a8fc099801845` |
| `masking.key` | 40 | `063c3b3c94c509b15e8d425f89aa608ffe52740593b0e5c2cb3a6d627fb3e06e` |
| `reviewer.key` | 64 | `896f3325e7a5854600b51623b36d5804e22a64d2a6909352d4bcbdf7220c6d4e` |
| `trust.json` | 156 | `c26520fd5615ff27544906014095ca4bf5ce291f485643ec4a58149786e5b587` |
| `history/` | 空目录 | — |

### 5.3 场景结果（每个场景独立 case 目录，执行前后逐区域比较）

| 场景 | 退出码 | source | target | history | 结论 |
|---|---|---|---|---|---|
| `scan` | 0 | 相同 | 相同 | 无文件 | 零持久副作用 |
| `mask`（preview，无 `--apply`） | 0 | 相同 | 相同 | 无文件 | 零持久副作用 |
| `validate --strict`（目标未脱敏） | 1（`pass=false issues=1`） | 相同 | 相同 | 无文件 | 失败也不写 |
| `mask --apply`（无 `--approved`） | 2 | 相同 | 相同 | 无文件 | 配置层拒绝，未到 DB |
| `mask --approved --apply`（无记录） | 1 | 相同 | 相同 | 无文件 | 审批闸门拒绝，不写 |
| 静态预检：规划列 UNIQUE | 1 | 相同 | 相同 | 相同（仅 1 条既有记录） | 预检拒绝，不写 |
| 静态预检：规划列 CHECK | 1 | 相同 | 相同 | 相同 | 同上 |
| 静态预检：规划列外键 | 1 | 相同 | 相同 | 相同 | 同上 |
| 静态预检：规划表触发器 | 1 | 相同 | 相同 | 相同 | 同上 |
| 扫描失败（目标缺表） | 1 | 相同 | 相同 | 无文件 | 读取阶段失败，不写 |
| apply + 篡改签名记录 | 1 | 相同 | 相同 | 相同 | 审批闸门拒绝，不写 |
| apply + 过期记录（`--valid-for 1s`，sleep 2s） | 1 | 相同 | 相同 | 相同 | 审批闸门拒绝，不写 |
| apply + 已撤销记录 | 1 | 相同 | 相同 | 相同 | 撤销闸门拒绝，不写 |
| apply + 跨计划记录 | 1 | 相同（2 个文件） | 相同（2 个文件） | 相同 | 审批闸门拒绝，不写 |
| apply + 源快照已变 | 1 | 相同（2 个文件） | 相同 | 相同 | 审批闸门拒绝，不写 |
| apply + scope 换序 | 1 | 相同 | 相同 | 相同 | 审批闸门拒绝，不写 |
| apply + 只批一张表 | 1 | 相同 | 相同 | 相同 | 审批闸门拒绝，不写 |
| 未知策略 | 1 | 相同 | 相同 | 无文件 | Plan 拒绝，未到写 |
| `null` 策略 apply | 1 | 相同 | 相同 | 相同 | dryRun 拒绝，未到 Writer |
| UNKNOWN 证据 | 1 | 相同 | 相同 | 无文件 | Plan 拒绝，未到写 |
| 证据不足（5 distinct） | 1 | 相同 | 相同 | 无文件 | Plan 拒绝，未到写 |
| 截断扫描（`--sample-limit 20`） | 1 | 相同 | 相同 | 无文件 | Plan 拒绝，未到写 |
| Describe 失败（目标缺表） | 1 | 相同 | 相同 | 无文件 | Plan 拒绝，未到写 |
| schema 不一致 | 1 | 相同 | 相同 | 无文件 | Plan 拒绝，未到写 |
| 无稳定主键 | 1 | 相同 | 相同 | 无文件 | Plan 拒绝，未到写 |
| **WAL 源** | 1 | **不同**（新增 `source.db-shm`、`source.db-wal`） | **不同**（新增 `target.db-shm`、`target.db-wal`） | 无文件 | 主文件字节不变，但 SQLite 打开连接时创建了 sidecar，见 §11-F1 |
| 对照：合法审批 + apply | 0 | 相同 | **不同**（`8fb8711d…`） | 相同 | 唯一会写目标的路径 |
| 对照：撤销后重新审批 + apply | 0 | 相同 | **不同**（`8fb8711d…`） | 相同 | 同上 |

所有“相同”的区域都是**字节级**相同（文件名、字节数、完整 SHA-256 三元组逐项相等），不是“计数为 0”的替代品。每个场景的完整三元组清单由取证脚本输出保存（§12）；发生变化的文件如下，其余均与 §5.2 的初始清单逐项相等：

| 场景 | 变化的文件 | 字节 | SHA-256 |
|---|---|---|---|
| 对照：合法审批 + apply | `target.db` | 16384 | `8fb8711ddb3ee58cfc4c56d9117d54d36920823ca75180d5851e14cf30e430bc` |
| 对照：撤销后重新审批 + apply | `target.db` | 16384 | `8fb8711ddb3ee58cfc4c56d9117d54d36920823ca75180d5851e14cf30e430bc` |
| WAL 源 | `source.db` / `target.db` | 8192 / 8192 | `2b90d3b0016f0e70c1e8ecb143ba0e694d42cf390b7b118480eb4aebb639a6c7`（两者相同，未变） |
| WAL 源 | 新增 `source.db-shm` / `target.db-shm` | 32768 / 32768 | `fd4c9fda9cd3f9ae7c962b0ddf37232294d55580e1aa165aa06129b8549389eb`（两者相同） |
| WAL 源 | 新增 `source.db-wal` / `target.db-wal` | 0 / 0 | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`（空文件） |

### 5.4 驱动级 SQL Spy（真实 SQLite 驱动收到的语句）

探针让 Scan/Plan/Preview/Validate 通过 spy-backed Reader 执行，统计累计语句：

| 操作 | 累计语句 | SELECT | 写/事务意图 | 目标树变化 |
|---|---|---|---|---|
| `Scan` | 16 | 7 | **0** | 无 |
| `Plan` | 33 | — | **0** | 无 |
| `Preview` | 90 | 43 | **0** | 无 |
| `Validate` | 133 | 64 | **0** | 无 |

同样的断言由仓库用例固定：`TestPreviewIssuesNoWriteSQLThroughSQLSpy`（`internal/runner/preview_sqlspy_test.go:17`）、`TestMultiTablePreviewIssuesNoWriteSQLThroughSQLSpy`（`internal/runner/multi_execute_test.go:286`）、`TestSQLSpyReadOnlyOperationsIssueNoWrites`（`internal/db/sqlspy_test.go:57`）。

### 5.5 Writer 能力计数（包内探针，真实 SQLite Writer）

| 场景 | 阶段 | 错误类别 | opens | begins | updates | commits | rollbacks | 目标树变化 |
|---|---|---|---|---|---|---|---|---|
| 无审批 seam | apply | `ErrUnsafe` | 0 | 0 | 0 | 0 | 0 | 无 |
| 无记录 | apply | `ErrNotFound` | 0 | 0 | 0 | 0 | 0 | 无 |
| 记录过期 | apply | `ErrUnsafe` | 0 | 0 | 0 | 0 | 0 | 无 |
| 记录已撤销 | apply | `ErrUnsafe` | 0 | 0 | 0 | 0 | 0 | 无 |
| 记录属于别的计划 | apply | `ErrNotFound` | 0 | 0 | 0 | 0 | 0 | 无 |
| 签名相对 trust 不可信 | apply | `ErrUnsafe` | 0 | 0 | 0 | 0 | 0 | 无 |
| seam 返回换序 scope | apply | `ErrUnsafe` | 0 | 0 | 0 | 0 | 0 | 无 |
| seam 返回部分 scope | apply | `ErrUnsafe` | 0 | 0 | 0 | 0 | 0 | 无 |
| 只批子集计划 | apply | `ErrNotFound` | 0 | 0 | 0 | 0 | 0 | 无 |
| 缺 `ExplicitApply` | apply | `ErrUnsafe` | 0 | 0 | 0 | 0 | 0 | 无 |
| `Approved=false` | apply | `ErrUnsafe` | 0 | 0 | 0 | 0 | 0 | 无 |
| Plan ID 被改 | apply | `ErrConflict` | 0 | 0 | 0 | 0 | 0 | 无 |
| 预检：UNIQUE | apply | `ErrUnsafe` | 0 | 0 | 0 | 0 | 0 | 无 |
| 预检：CHECK | apply | `ErrUnsupported` | 0 | 0 | 0 | 0 | 0 | 无 |
| 预检：触发器 | apply | `ErrUnsupported` | 0 | 0 | 0 | 0 | 0 | 无 |
| 预检：外键 | apply | `ErrUnsupported` | 0 | 0 | 0 | 0 | 0 | 无 |
| `null` 替代（dryRun） | apply | `ErrUnsafe` | 0 | 0 | 0 | 0 | 0 | 无 |
| 目标行在 Plan 后被改 | apply | `ErrConflict` | 0 | 0 | 0 | 0 | 0 | 无 |
| 源在 Plan 后被改（重算不一致） | apply | `ErrConflict` | 0 | 0 | 0 | 0 | 0 | 无 |

SQL Spy 只适用于**只读**工作流（§5.4）：被拒绝的 Apply 在打开 Writer 之前就结束，不存在可供观测的写连接，因此这些行的仪器是 Writer 能力计数（全 0），独立校验是含 sidecar 的文件树——而不是“SQL Spy 里没有写语句”（后者在无写连接时是空断言）。

Plan 阶段闸门（UNKNOWN、截断、无主键、schema 不一致、未知策略）在探针里单独打印并标注 `writer_reachable=no`：Apply 从未被调用，Writer 计数不构成这些闸门的证据，它们只证明 Plan 拒绝（这也是测试计划 P2-2 的告警，本文不重犯）。

### 5.6 `approve`/`revoke` 确实写历史——与只读结论分开

`approve` 退出 0 后在 `history/` 新增恰好 **1** 个文件（`<plan-id>.decision`，`size=922`/`967`/`999` 取决于 scope 数与是否撤销）；`revoke` 只重写同一个文件，文件名集合不变。除此之外，`approve`/`revoke` 都不改源/目标 `.db`（§4 步骤 3、5 的树比较）。历史目录在纯只读命令（scan/preview/validate）执行后仍无任何文件，`TestE2EReadOnlyCommandsDoNotTouchHistory`（`cmd/maskriver/approval_e2e_test.go:478`）固定同一行为。

## 6. 拒绝矩阵

列含义：**实际触发闸门**是探针/代码路径确认的那一道；“Writer 打开”列一律来自上表的实例级计数（Plan 阶段闸门除外）。`N/A` 表示该行只有组件级证据、没有 CLI 路径。所有行的源/目标/历史树在 CLI 侧都已比较过（§5.3），除 WAL 行外全部字节相同。

| # | 触发 | 期望闸门 + 错误类别 | 实际结果 | Writer 打开 | DB 变化 | 用例 / 位置 |
|---|---|---|---|---|---|---|
| 1 | UNKNOWN：值不匹配任何规则 | Plan 证据闸门 → `ErrUnsafe` | 探针 `stage=plan ErrUnsafe`；CLI `mask` 退出 1（脱敏拒绝） | 否（Apply 未调用） | 否 | `TestPlanRefusesNonSensitiveColumnEvenWhenApproved` `internal/runner/scan_gate_test.go:45` |
| 2 | 证据不足：25 行仅 5 distinct，`MinSamples=20` | Plan 证据闸门 → `ErrUnsafe`（证据原因 `InsufficientEvidence`） | 探针/CLI 同上；CLI 退出 1 | 否 | 否 | `TestDetectMinSamplesBoundary` `internal/detect/detect_test.go:139`；`TestPlanRefusesNonSensitiveColumnEvenWhenApproved` 同路径 |
| 3 | 截断扫描：25 distinct、`--sample-limit 20` | Plan 完整性闸门 → `ErrIncomplete` | 探针 `stage=plan ErrIncomplete`；CLI 退出 1 | 否 | 否 | `TestPlanRefusesTruncatedSampleEvenWhenApproved` `scan_gate_test.go:72`；`TestPlanRefusesTruncatedSampleInOneScope` `multi_scan_test.go:214` |
| 4 | Describe/Sample 失败（目标缺表；Sample seam 报错） | Plan/gather → `ErrNotFound`（缺表）/ 原样传播 | 探针 `stage=plan ErrNotFound`；CLI `scan` 退出 1 | 否 | 否 | `TestScanFailsClosedOnEvidenceSeamError` `scan_fault_test.go:103`；`TestCLIScanFailureExitsNonZeroWithoutWrite` `cmd/maskriver/main_test.go:609` |
| 5 | 取消 | `gather` ctx 检查 / `visit` / rollback | **CLI 无路径**（命令内置 2 分钟超时，无法从外部可靠触发）；包内用例退出码 N/A | 取消发生在批次内时已打开 1 个 Writer，随后 rollback+close | 未提交批次回滚 | `TestScanFailsClosedOnMidScanCancellation` `scan_fault_test.go:147`；`TestApplyRollsBackBeginBatchFailuresAndCancellation` `workflow_acceptance_test.go:386`；`TestApplyCancelledInSecondTableRollsBackAndReleasesWriter` `multi_apply_test.go:427` |
| 6 | 无稳定主键（`keyless` 表） | `describe` 要求非空 PK → `ErrIncomplete` | 探针 `stage=plan ErrIncomplete`；CLI 退出 1 | 否 | 否 | `TestPlanRefusesMissingOrPrimaryKeyScope` `multi_scan_test.go:235`（scope 为主键时 `ErrUnsafe`） |
| 7 | 规划列 UNIQUE | 静态预检 → `ErrUnsafe` | 探针 `opens=0`；CLI `--apply` 退出 1 | 否 | 否 | `TestPreflightRefusesUniqueConstraintOnModifiedColumn` `preflight_test.go:141`；`TestCLIRefusesUniquePlannedColumn` `main_test.go:381` |
| 8 | 规划列 CHECK | 静态预检 → `ErrUnsupported` | 探针 `opens=0`；CLI 退出 1 | 否 | 否 | `TestPreflightRefusesCheckOnModifiedColumn` `preflight_test.go:161` |
| 9 | 规划列外键（含入向引用） | 静态预检 → `ErrUnsupported` | 探针 `opens=0`；CLI 退出 1 | 否 | 否 | `TestPreflightRefusesForeignKeyOnModifiedColumn` `preflight_test.go:118`；`TestPreflightRefusesIncomingReferenceFromUnplannedTable` `preflight_test.go:440` |
| 10 | 规划表触发器 | 静态预检 → `ErrUnsupported` | 探针 `opens=0`；CLI 退出 1 | 否 | 否 | `TestPreflightRefusesTriggerOnPlannedTable` `preflight_test.go:62` |
| 11 | WAL 源 | 快照身份闸门（Plan 内 `requireQuiescentSnapshot`）→ `ErrIncomplete` | CLI 退出 1；**副作用：两侧新增 `-shm`/`-wal` sidecar**（§11-F1） | 否 | 主 `.db` 字节不变 | `TestWALSourceIsRefusedBySnapshotIdentity` `snapshot_identity_test.go:82`；`TestWALSourceWithoutActiveWriterIsStillRefused` `:175` |
| 12 | 不支持的 journal mode（memory/off） | 同一快照身份闸门 → `ErrIncomplete` | **CLI UNVERIFIED**：这两种模式是连接级设置，不能持久在文件里，CLI 无法构造稳定 fixture；组件级断言成立 | 否 | 否 | `TestQuiescentJournalModes` `internal/db/capability_test.go:98`；`TestNonQuiescentJournalSourceIsRefusedBeforePlanAndWriter` `snapshot_identity_test.go:258` |
| 13 | 快照不一致（approve 后源文件变化） | CLI：审批闸门 → `ErrNotFound`（plan ID 变化，查无记录）；若 seam 回显旧计划则预检 `checkPlan` → `ErrConflict` | CLI 退出 1；探针 `stale-plan-preflight-recheck` = `ErrConflict`、`opens=0` | 否 | 否 | `TestE2EDecisionBoundFieldChangesRefused` `approval_e2e_test.go:280`；`TestCheckPlanBindingRejectsMismatchedScopes` `preflight_test.go:318` |
| 14 | schema 不一致（源/目标同表不同类型） | `describe` 要求源目标 schema 深比较相等 → `ErrIncomplete` | 探针 `stage=plan ErrIncomplete`；CLI 退出 1 | 否 | 否 | `internal/runner/workflow.go:111-129`（`describe`）；`TestCheckPlanBindingRejectsMismatchedScopes` `preflight_test.go:318` 覆盖计划与实时 schema 不一致 |
| 15 | 未知策略 | Plan 策略注册表（在扫描之前）→ `ErrInvalid` | 探针 `stage=plan ErrInvalid`；CLI 退出 1 | 否 | 否 | `Test_NewStrategy_UnknownErrInvalid` `internal/mask/mask_test.go:91` |
| 16 | `null` 策略（非 NULL→NULL） | Apply dryRun → `ErrUnsafe`（Plan 可构建） | 探针 `opens=0`；CLI 退出 1 | 否 | 否 | `TestApplyRefusesNullReplacementBeforeWriter` `null_replacement_test.go:17`；`TestCLINullStrategyRefusedBeforeWrite` `main_test.go:643` |
| 17 | 缺少审批 | `checkApproval` → `ErrNotFound`/`ErrUnsafe` | 探针 `ErrNotFound`（空 store）、`ErrUnsafe`（无 seam）；CLI：无 `--approved` 退出 2，有 `--approved` 无记录退出 1 | 否 | 否 | `TestApplyRequiresApprovalQuery` `approval_test.go:182`；`TestApplyRefusesEveryUnauthenticatedDecision` `:211` |
| 18 | 签名不对（记录被篡改 / 签发者不在 trust） | 审批闸门 → `ErrUnsafe` | 探针 `decision-untrusted-signer` = `ErrUnsafe`；CLI 篡改记录后 apply 退出 1 | 否 | 否 | `TestLookupRejectsForgedSignature` `internal/history/authenticity_test.go:20`；`TestApplyRefusesEveryUnauthenticatedDecision` 的“tampered/forged”子例 |
| 19 | 记录过期 | 审批闸门 → `ErrUnsafe` | 探针 `decision-expired` = `ErrUnsafe`；CLI（`--valid-for 1s` + 2s）退出 1 | 否 | 否 | `TestLookupRejectsExpiredRecord` `authenticity_test.go:74`；`TestApplyRefusesEveryUnauthenticatedDecision` “expired record” |
| 20 | 记录已撤销 | 审批闸门（cause `errRevoked`）→ `ErrUnsafe` | 探针 `decision-revoked` = `ErrUnsafe`、计数全 0；CLI 退出 1（§4） | 否 | 否 | `TestRevokeMakesRecordUnusable` `authenticity_test.go:86`；`TestApplyRefusesEveryUnauthenticatedDecision` “revoked record” |
| 21 | 跨计划（为 A 签发、对 B 使用） | 审批闸门 → `ErrNotFound` | 探针 `decision-foreign-plan`/`subset-plan-only-record` = `ErrNotFound`；CLI 退出 1 | 否 | 否 | `TestLookupRejectsRecordForAnotherPlan` `internal/history/binding_test.go:180` |
| 22 | scope 换序 | 两种防御：plan 摘要变化（CLI 表现为 `ErrNotFound`）；记录内容换序由 store 绑定校验与 runner 复核拒绝（`ErrConflict`/`ErrUnsafe`） | 探针 `seam-returns-reordered-scopes` = `ErrUnsafe`；`TestLookupRejectsScopeReorder` = `ErrConflict`；CLI 退出 1 | 否 | 否 | `TestLookupRejectsScopeReorder` `binding_test.go:47`；`TestApplyRefusesRecordFailingRunnerRecheck` `approval_test.go:403` |
| 23 | 部分审批（只批一张表） | store 绑定校验（构造记录）与 CLI 计划摘要（`ErrNotFound`） | 探针 `seam-returns-partial-scopes` = `ErrUnsafe`；`TestLookupRejectsPartialMultiTableApproval` = `ErrConflict`；CLI 退出 1 | 否 | 否 | `TestLookupRejectsPartialMultiTableApproval` `binding_test.go:16`；`TestE2EPartialDecisionDoesNotAuthorizeWholePlan` `approval_e2e_test.go:269` |

**闸门归属说明（避免把早先闸门当成被测闸门）**：

- 第 1/2/3/4/6/11/12/14/15 行是 **Plan 阶段**闸门。CLI 在这些用例上从未进入 `Apply`；`opens=0` 在这里不是“Apply 的写前检查有效”的证据，而是“Plan 拒绝 + 只有 Apply 才构造 Writer”（`internal/runner/workflow.go:1-2` 与 `execute.go:505`）的结果。按 P2-2，本文不把扫描阶段的空 Writer 计数当作扫描闸门回归的证明。
- 第 7–10、16 行是 **Apply 写阶段**闸门，且都在 `openWriter` 之前；为排除“审批闸门先拒绝”，这些 CLI 用例都先执行了 `approve`（退出 0），并且 `null` 策略的 Plan 也能构建成功。
- 第 17–23 行是 `checkApproval` 闸门，顺序在预检与 dryRun 之前（`internal/runner/execute.go:505-545`）。因此这些行的 `opens=0` 属于审批闸门本身。
- 第 13 行在 CLI 上先撞到审批闸门（plan ID 变了，查不到记录）。只有用回显旧计划的 seam 时才能证明 `checkPlan` 的 `ErrConflict`（探针 `stale-plan-preflight-recheck`），本文把两者分开陈述。

## 7. 事务与故障

| 性质 | 证据（实跑通过） | 关键观测 |
|---|---|---|
| 第二张表失败保留先前已提交 receipt | `TestApplyStopsInSecondTableAndKeepsCommittedReceipts` `internal/runner/multi_apply_test.go:335` | 故障注入在 contacts 第 2 批：`updates=4 commits=3 rollbacks=1 closes=1`；`report.Committed` 为 people#1、people#2、contacts#1 三条；contacts 第 2 行保持原值；**仪器证据**：`spy.begun` = people,people,contacts,contacts，证明执行真的到达第 4 批而不是被预检提前拒绝 |
| `Complete=false` 与已提交内容一致 | `TestApplyRetainsReceiptForEarlierCommittedBatch` `workflow_acceptance_test.go:473` | 第 2 批 Commit 失败：`commits=2 rollbacks=1 closes=1`；`Committed` 仅 1 条（ordinal 1、rows 1）；真实回读显示第 1 行已改动、第 2 行未改动 |
| CommitUnknown 不重试、不声称回滚 | `TestApplyStopsAtUnknownCommitInSecondTable` `multi_apply_test.go:383`；`TestApplyWrappedCommitUnknownIsNotReportedAsRolledBack` `workflow_acceptance_test.go:432`；`TestApplyWrappedCommitUnknownAfterDurableCommitReportsUnknown` `:449` | 第 3 批 Commit 返回包装的 `ErrCommitUnknown`：`commits=3 updates=3 rollbacks=0 closes=1`，`spy.begun` 只有 3 个表/批（没有后续批次、没有重试）；`CommitOutcomeUnknown=true`、`Complete=false`；“真实提交后丢失确认”用例证明报告保守（数据已落盘但只确认前面批次） |
| 取消/回滚释放连接 | `TestApplyCancelledInSecondTableRollsBackAndReleasesWriter` `multi_apply_test.go:427` | `rollbacks=1 closes=1`；rollback 使用未取消且有 deadline 的独立 context（`rollbackContextErr=nil`，剩余 deadline > 0）；contacts 两行均未变；测试随后用一个**新的**写连接成功执行 `UPDATE contacts SET id=id`，证明写锁/连接已释放 |
| 乐观并发变更不被静默覆盖 | `TestApplyPreflightFailuresDoNotOpenWriter` 的 “preview mismatch” 子例 `workflow_acceptance_test.go:205`；`TestSQLiteWriteTransactions` `internal/db/sqlite_test.go:168` | 目标行在 Plan 后被改：Apply 失败且 `opens=0`（探针 `target-changed-after-plan` = `ErrConflict`）；DB 层 `Update` 用 `Expected` 行值做乐观条件，`ErrConflict` 时整事务中止，同批前一行也未提交（回读仍为空串） |
| 无跨批全局原子性 | 上述保留 receipt 用例 + `docs/contracts.md` §6 | 每批一个事务；首批提交后第二批失败即留下已提交前缀，报告为 `Complete=false`，不做跨批补偿 |
| 表在运行中被清空 | `TestApplyRefusesTableEmptiedBeforeItsTurn` `apply_table_count_test.go:68`；`TestApplyRefusesTableEmptiedAfterItsStartCount` `:103` | 前后 Count 不一致 → `ErrConflict`，不会把“写入 0 行”当成功 |
| 第二张表的约束类别不会污染第一张表 | `TestApplyRefusesSecondTableConstraintClass` `multi_apply_test.go:273` | 先断言第一张表 `checkTableCapabilities` 返回 nil，再断言整体 Apply 被拒——证明拒绝来自第二张表，不是预检顺序造成的假象 |

## 8. AC 矩阵（逐条引用 Issue #9 原文）

AC 原文来自 `gh issue view 9 --json body`（2026-10-18 读取，状态 OPEN，labels `priority:P1 phase:M1 type:feature status:blocked`）。

### AC1（原文）

> - [ ] **功能**: `mask` without `--apply` performs a preview with zero persistent side effects; `--apply` is the only write intent; a config file cannot enable writes; incomplete scan, unresolved `UNKNOWN`, missing stable key, and unverified output all fail closed; `validate` returns non-zero on failure and, with `--strict`, also on anything unverified.

**结论：PASS**（附两条限定）

- 证据路径 §3（transcripts，退出码与行数）、§5.3（零副作用）、§6（fail-closed 矩阵）、§9（strict 范围）。
- 命令：本报告 §3/§5/§6 的二进制子进程调用；`go test -count=1 ./...`（退出 0）。
- CI SHA：`4ac8fd48…` / `c5cbeb01…` 的 CI 全部 success（Ubuntu/Windows 测试与 vet、Ubuntu race）；CLI 证据来自 §3–§7 的本机实跑，不在 CI 内。
- 限定 1：AC 所述 “a config file cannot enable writes” 在本 SHA 上是**真空成立**——项目根本不读取配置文件（`internal/config/config.go` 只有 `Options{Apply}` 且注释写明 “File loading is not implemented”；`docs/feature-matrix.md:34` 记为“计划中”）。因此这不是“配置校验拦截了写入”的证据，而是“不存在配置文件这条写路径”。
- 限定 2：`--strict` 的独立语义见 §9，本文不把它升级为“已具备严格语义门禁”。

### AC2（原文）

> - [ ] **测试**: Real SQLite end-to-end test covering scan → preview → apply → strict validate on synthetic fixtures; preview/apply value agreement; a dry-run zero-side-effect proof (file snapshot plus SQL spy, not just a `rows_written=0` counter); failure injection for scan error, cancellation and commit ambiguity; `go test -race ./...`.

**结论：PASS**

- 证据路径：`TestCLIRealSQLiteWorkflow` `cmd/maskriver/main_test.go:233`、`TestCLIMultiTableWorkflow` `:477`、`TestE2EPlanIssueNewProcessApply` `approval_e2e_test.go:201`；取值一致 `TestWriterAcceptsPreviewChangesAndAgreesOnValues` `internal/runner/apply_columns_test.go:113`；零副作用 §5.3–5.5；故障注入 §7。
- 命令：`go test -count=1 ./...`（退出 0；298 PASS/0 FAIL/3 SKIP）、`go test -race -count=1 ./...`（退出 0）。
- CI SHA：同上；Ubuntu CI 的 Race 步骤 success，Windows 的 Race 步骤按条件 skipped（不计为通过）。
- 风险：`--strict` 与非 strict 在数据库层面不可区分（§9）；MySQL 路径不在 AC2 内，另列 UNVERIFIED。

### AC3（原文）

> - [ ] **安全性**: No original value, sensitive key, DSN, or seed in stdout/stderr/logs/reports; preview leaks nothing by default; the source snapshot is taken before apply; no unrelated diagnostic changes merged into `main`.

**结论：PASS**

- 证据路径：`TestCLIRealSQLiteWorkflow`、`TestCLIApplySummaryIsSanitized` `main_test.go:561`、`TestCLIFlagParseErrorsAreSanitized` `:154`、`TestPlanRefusalNeverLeaksValues` `internal/runner/scan_gate_test.go:150`、`TestApplyPathTakesNoReviewerPrivateKey` `approval_e2e_test.go:634`；本报告所有 CLI 输出均为脱敏摘要（只有计数、位置、布尔与窗口）。
- 源快照在 apply 前取得：`Plan` 计算 `SourceSnapshotID`（整个 `.db` 的 SHA-256）后才产生权威记录，且 §3 中 `source.db` 在 Apply 前后字节不变。
- 无关诊断改动：本 SHA 的三个 #9 文档提交只触及 `README.md` 与 `docs/**`（`git diff --name-only` 已核对），未夹带代码；更早历史不在本次审计范围。
- 风险：`ReviewerID` 是自填标签，trust file 与审核私钥若由同一操作者控制则工具无法阻止（代码注释、README“安全提示”均明确声明）。这是本地签名边界，不是 RBAC。

### AC4（原文）

> - [ ] **文档/Feature Matrix**: `docs/feature-matrix.md` and `docs/test-plan.md` updated to reflect exactly which workflow paths are verified end to end; `docs/roadmap.md` M1 status updated.

**结论：PASS**

- 证据路径：`docs/feature-matrix.md:32`（CLI 行：SQLite 多表已测、MySQL CLI 未实现）、`:65`（扫描 fail-closed）、`:76`（`null` 策略 CLI 拒绝 + P2-1）、`:123`（退出码）、`:136`（strict 语义 inert）；`docs/test-plan.md` 的“#9 真实 SQLite CLI 与签名审批（当前证据）”与 P2 表；`docs/roadmap.md:11-13`（M1-B 已合并、M1-C 收尾中、M1-D 待 #11）。
- 核对方式：本文逐项实跑了文档点名的用例，测试名与文件位置全部存在且通过（§6/§7 的引用列）；文档中“未验证”的表述与 §10 一致。
- `docs/m1-runner-acceptance.md` 是**历史记录**：文件顶部有 2026-10-18 的时效说明与“本文原始结论 → 当前状态”对照表（CHECK 覆盖、CLI 参数泄漏、非 NULL→NULL、WAL 源、多表/SQL Spy 等已被后续 PR 改变）。本文不把该文件的旧发现当作当前缺陷，也不要求改写其原始结论。
- 风险：文档只能与“已存在的测试 + 本次实跑”对齐；文档本身不构成运行时保证。

### AC5（原文）

> - [ ] **最终交付证据**: end-to-end test log, CLI transcript with exit codes, dry-run zero-side-effect evidence, and the list of still-unverified items (MySQL, remaining strategies).

**结论：PASS**

- 证据路径：本文件 §3（CLI 转录 + 退出码）、§5（零副作用：文件树 + SQL Spy + Writer 计数）、§10（UNVERIFIED 清单，含 MySQL 与剩余策略）、§1/§2（测试日志与 CI 结论）。
- 命令：§1 六条静态/测试命令、§2 `gh run list --json`、§3–§7 的二进制调用与探针。
- 风险：一切仅限 Linux amd64 + Go 1.26.3 + `modernc.org/sqlite v1.60.1` + 合成 SQLite；Windows 只有 CI 的 test/vet（无 CLI 取证），macOS 无 CI。

## 9. `--strict` 证据的准确范围

**能证明**：

1. `validate` 接受 `--strict` 并把 `Strict=true` 放进 `ValidateRequest`（`cmd/maskriver/main.go` 的 `validationRequest`），由 `TestCLIStrictFlagWiring`（`main_test.go:680`）在真实命令路径上观测。
2. 退出码由 `report.Passed(strict)` 决定：`strict` 下只有 `Warning` 的报告被判失败，非 strict 下同一报告通过（`TestValidationPassedHonorsStrictWarning` `main_test.go:731`）。
3. 在真实 SQLite 上，`validate --strict` 对已正确脱敏目标退出 0（`pass=true`，§3），对未脱敏目标退出 1（`pass=false issues=1`，§5.3）。

**不能证明**（不得升级）：

- 不能证明存在“strict 专属的数据库层面严格语义”。`internal/verify` **完全忽略** `ValidateRequest.Strict`（该包内没有对 `Strict` 的任何引用）；而且所有 Warning 路径已经把 `Coverage.Complete` 置为 false，`Passed(false)` 也同样失败。
- 因此没有任何 fixture 能在“strict vs 非 strict”的退出码上产生差异；CLI 用例只证明 flag 解析、转发与判决函数，不证明额外的严格检查。
- `go vet`/`gofmt`/race 都不能把这一点变成“已具备严格门禁”。

## 10. UNVERIFIED 清单（含原因）

| # | 项 | 原因 | 现有最强证据 |
|---|---|---|---|
| 1 | MySQL CLI 端到端 | CLI 只实现 SQLite；`internal/db` 的 MySQL 适配器测试存在，但用户命令面没有 MySQL 路径 | CI 的隔离 MySQL 8.0.46 作业执行 `TestMySQLAdapter`（success）；本机无 socket 时 SKIP |
| 2 | 断点恢复 / checkpoint / 跨进程幂等重放 / exactly-once | 未实现；批次事务与审批记录都不能推出恢复协议 | 无。批次语义仅有“保留已提交前缀 + `Complete=false`” |
| 3 | 生产级检测准确率 | 仅 74 条自编样本（38 正 / 36 负，8 条超覆盖）；开发集即评估集；姓名规则 P=0.500/R=0.750 | `internal/detect/eval_test.go`（`docs/test-plan.md` 已如实记录） |
| 4 | macOS CI | 仓库 CI 矩阵只有 ubuntu-latest / windows-latest | 无（`ci.yml` 无 macOS 作业） |
| 5 | `--strict` 的独立严格语义 | verify 忽略 `Strict`，且 Warning 路径已置 `Complete=false`（§9） | flag 接线 + 判决函数 + 真实 SQLite 退出码 |
| 6 | 不支持 journal mode（memory/off）的 **CLI** 拒绝 | 两种模式是连接级设置，无法持久在文件里，CLI 无法构造稳定 fixture | 组件级 `TestQuiescentJournalModes`、`TestNonQuiescentJournalSourceIsRefusedBeforePlanAndWriter` |
| 7 | 取消的 **CLI** 路径 | 命令内置固定 2 分钟 context，外部无法在确定阶段注入取消 | runner 级 `TestApplyCancelledInSecondTableRollsBackAndReleasesWriter` 等 |
| 8 | 剩余/CN 策略经 CLI 全链路的 Apply 与严格验证、SQL Spy 专项、跨列唯一/PK-FK、持久映射 | 超出 #9 声明范围；CLI 本次只实跑了 `fake_email` 与 `null`（拒绝）两种策略的装配 | 组件级 PR #29/#30 证据；不构成 CLI 证据 |
| 9 | Windows/macOS 上的 CLI 取证 | 本次取证只在 Linux 实跑；Windows 仅有 CI 的 test/vet | CI 结论（`c5cbeb01…` / `4ac8fd48…`） |
| 10 | 并行 Worker 与 SQL 读写的真实并发快照 | 多 Worker 结果一致性已有 CLI 用例（串行/并行目标字节一致），但不是并发快照或吞吐证明 | `TestCLIRealSQLiteWorkflow` 的串行/并行比较 |

计数：**10 项 UNVERIFIED**（需求点名的 MySQL、checkpoint、exactly-once、生产检测准确率、macOS CI 全在其中）。

## 11. 残余风险与发现

**F1（本次新观察到，尚未登记编号）**：被拒绝的 WAL 源仍会产生 SQLite sidecar 文件。对 `PRAGMA journal_mode=WAL` 的源执行 `mask`（Plan 阶段被快照身份闸门拒绝，退出 1）后：

| 文件 | 执行前 | 执行后 |
|---|---|---|
| `source.db` | `8192` `2b90d3b0016f0e70c1e8ecb143ba0e694d42cf390b7b118480eb4aebb639a6c7` | 相同（未变） |
| `source.db-shm` | 不存在 | `32768` `fd4c9fda9cd3f9ae7c962b0ddf37232294d55580e1aa165aa06129b8549389eb` |
| `source.db-wal` | 不存在 | `0` `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`（空文件） |
| `target.db` / `-shm` / `-wal` | 同源 | 同样新增 32768 与 0 字节 sidecar |

主 `.db` 字节与逻辑内容未变，也没有写入既有的 WAL 数据，但“零持久副作用”在 WAL 源上不能表述为“没有新增文件”。这与测试计划 P2-5（CLI e2e 的 `targetDigest` 只哈希主文件、可能看不见 WAL 提交）方向一致；本报告用含 sidecar 的树哈希把它暴露出来。建议单独登记为一条 P2（是否登记由仓库所有者决定，本文不改登记表）。

**F2**：`--strict` 语义 inert（§9）。这意味着 AC1 的“`--strict` 下对未验证项也非零”目前靠“所有未验证路径都已令 `Complete=false`”实现，而不是靠 strict 分支；后续若新增只在 Warning 中表达的未验证路径，strict 与非 strict 的差异就会变得真实而未被覆盖。

**F3**：审批信任边界是本地签名边界。`approve`/`revoke` 需要审核私钥文件，`apply` 只读 trust file；但同一操作者若同时控制 trust file 与审核私钥，工具不会阻止。`ReviewerID` 只是自填标签。这不是 RBAC，不能作为独立审批的证明。

**F4**：历史存储不是透明日志。能写 store 目录者可恢复旧的有效记录或撤销前文件；`docs/test-plan.md` 的 P2-6 已登记。过期窗口仍限制影响，但本文不声称防篡改审计。

**F5**：快照身份只证明主文件字节；rollback-journal 模式下仍不能排除并发外部写入（`internal/db/capability.go:40-43` 已声明）。隔离副本仍是部署前提。

**已知并接受的 P2 登记（八个，`docs/test-plan.md` §“已知残余缺陷（已接受 P2）”，行 165–172）**：

| 编号 | 摘要 |
|---|---|
| P2-1 | `internal/db` 写入层仍接受非 NULL→NULL（CLI 已在上游拒绝） |
| P2-2 | 部分扫描拒绝测试的 `spy.opens != 0` 断言无法失败（扫描阶段本就不打开 Writer） |
| P2-3 | 取消测试未单独覆盖 `gather` 循环顶部的 `ctx.Err()` |
| P2-4 | `main.go` 为测试 seam 引入两个可变包级函数变量（不可并行） |
| P2-5 | CLI e2e 的 `targetDigest` 忽略 sidecar |
| P2-6 | 历史存储不检测整体回滚/撤销回滚 |
| P2-7 | typed-nil seam 防护不完整（`internal/db` 注入式连接 seam） |
| P2-8 | Runner 侧审批窗口无时钟偏移容差（与 history 的 5 分钟容差语义不同） |

**本文相关但未改动的既有结论**：`docs/m1-runner-acceptance.md` 的历史发现（CHECK 覆盖缺失、flag 解析回显、非 NULL→NULL 写入后 validate 失败、WAL 未绑定）均已被后续 PR 修正，按该文件顶部的日期说明阅读，不作为当前缺陷。

## 12. 复现方式

所有路径都用 `<tmp>` 占位；fixture 与历史目录都在 `<tmp>/ac5` 下，仓库内不落任何取证文件（取证过程中 `git status --porcelain` 除本报告外始终为空）。

```sh
# 1. 环境
go version && uname -srm
gofmt -l internal cmd pkg tools
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...

# 2. 构建并跑 CLI 转录（§3）
go build -o <tmp>/ac5/bin/maskriver ./cmd/maskriver
bash <tmp>/ac5/transcripts.sh

# 3. 生成合成 fixture（§3–§6），通过 overlay 编译包内探针，仓库不新增文件
AC5_ROOT=<tmp>/ac5 go test -overlay <tmp>/ac5/overlay_build.json \
  -run '^TestAC5BuildFixtures$' ./cmd/maskriver
python3 <tmp>/ac5/harness.py            # 每个场景一个独立进程 + 含 sidecar 的树哈希
go test -overlay <tmp>/ac5/overlay_runner.json  -run '^TestAC5Probe' -v ./internal/runner
go test -overlay <tmp>/ac5/overlay_history.json -run '^TestAC5Probe' -v ./internal/history

# 4. CI
gh run list --commit c5cbeb0107c953231d929965f8d8693e5f2e3839 --json databaseId,workflowName,conclusion
gh run view 39127514031 --json jobs
```

复核要点：任一条 CLI 行的退出码、`apply:` 摘要的计数、`validate: pass=` 与 §3/§5 一致；树比较必须包含 `-wal`/`-shm`/`-journal`；Plan 阶段闸门不得被当作 Apply 写前检查的证据。
