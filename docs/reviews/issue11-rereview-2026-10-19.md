# Issue #11 窄口径复核：文档收口、残余风险登记与许可合规

- 复核日期：2026-10-19
- 关联：[Issue #11](https://github.com/haoyuehx/maskriver/issues/11)、[#9](https://github.com/haoyuehx/maskriver/issues/9)、PR #58、PR #59
- 复核对象：`origin/main` = `c3907fee0c95a6d4ab9014382a52af3d4167c715`（PR #59 merge）
- 本文性质：**窄口径复核**。不重跑 #9 的 AC1–AC5，不改产品代码，不重新裁定 M1 功能范围。本文的产出是：四条幸存 P2 的验证结论、**从零重建**的残余风险登记表、许可/发行合规分析、以及发布决定。

> **证据缺口声明（必读）**：上一轮 #11 评审的逐字报告保存在临时目录（`/tmp`）并已丢失。本文**没有**恢复那份文本：它只是让证据重新落在仓库内可查的位置。上一轮 6 条 P2 中，只有 4 条有保留原文；丢失的 2 条在本文中仍标 **UNVERIFIED**，不推测、不补编号、不合并。

## 0. 结论摘要

| 项 | 结论 |
|---|---|
| P0 | **0** |
| P1 | **1**（许可/发行合规：二进制发行所需的第三方声明集合不完整，见 §5、R-08） |
| P2 | 幸存 4 条：**3 条已验证修复、1 条部分修复**（`README.md:10` 仍与 #9 CLOSED 矛盾）；丢失 2 条：**UNVERIFIED**；本轮**新增 1 条** |
| P3（仅记录，不计入 P0–P2） | 2 条（§3.1 的行标题观察、R-18） |
| 闸门 | `gofmt`/`go vet`/`git diff --check`/`go test -count=1 ./...`/`go test -race -count=1 ./...` 全部 exit 0；MySQL 本机 SKIP |
| 发布决定 | **ACCEPT WITH RESTRICTIONS — limited M1 release permitted**（限制见 §8） |
| Issue #11 是否可关闭 | **暂不可关闭**：存在未关闭 P1（§5、R-08）。P1 修复并由第二个独立上下文复核后，方可按 §9 关闭 |

## 1. 被审查目标、环境与独立性

| 项 | 值 | 核对方式 |
|---|---|---|
| 目标 `main` | `c3907fee0c95a6d4ab9014382a52af3d4167c715`（2026-10-19 10:02:18 +0800） | `git log -1 c3907fe` |
| 工作树 HEAD | `39d738f3b937d2328baedfb25170bc64e3a19f68`（PR #59 被合并的分支提交） | `git log -1` |
| 两者关系 | HEAD 是 `c3907fe` 的祖先，且 `git diff --stat 39d738f c3907fe` **为空**，即被审查树与 `origin/main` 内容完全相同 | `git merge-base --is-ancestor`、`git diff --stat` |
| 工作树状态 | 干净（`git status --porcelain` 无输出，暂存区为空） | `git status --porcelain` |
| 环境 | Go `go1.26.3`，`linux/amd64` | `go version`、`uname -srm` |
| 复核上下文 | 与 #9/#11 的所有实现者、以及 #59 文档收口作者均无重合 | 本任务指派 |

**独立性声明**：本复核由独立上下文（`deepseek/deepseek-flash`）执行，不引用实现者的成功声明作为证据；下文每条结论都给出可复现的命令或 `文件:行号`。本复核**未**重跑 #9 的 AC1–AC5 端到端证据，因此本文对 #9 的 AC 结论只作引用，不作再背书。

## 2. 实际执行的命令与退出码

下表只记录命令与真实退出码；输出中出现主机绝对路径的部分（如模块缓存路径）一律以 `<GOMODCACHE>` 代替，不粘贴进本文。

| # | 命令 | 退出码 | 结果摘要 |
|---|---|---|---|
| 1 | `git log -1` / `git status --porcelain` | 0 | HEAD `39d738f`，工作树干净，暂存区为空 |
| 2 | `gofmt -l internal cmd pkg tools` | **0** | 无输出（无未格式化文件） |
| 3 | `go vet ./...` | **0** | 无告警 |
| 4 | `git diff --check` | **0** | 无空白错误 |
| 5 | `go test -count=1 ./...` | **0** | 10 个包 `ok`，`tools/fixture` 无测试文件 |
| 6 | `go test -race -count=1 ./...` | **0** | 10 个包 `ok`（含 `cmd/maskriver` 32.7s、`internal/runner` 11.5s） |
| 7 | `go test -count=1 -v -run 'TestMySQLAdapter|TestMySQLFixture' ./internal/db ./internal/testenv` | 0 | **两条都 SKIP**：“isolated MySQL socket not provided” / “isolated MySQL not started” |
| 8 | 12 个模块 licence `cmp` 循环（见 §5.1） | **0** | 12/12 逐字节一致 |
| 9 | `grep -rn … docs README.md`（按任务指定的三段用户家目录模式：Unix、macOS、Windows） | **1**（无匹配） | 主机绝对路径已清除；模式本身不在本文重复写，以免再引入同类前缀 |

补充命令（用于建立 §5、§6 的事实）：`go list -m all`（27 个模块）、`go list -deps ./cmd/maskriver`（linux 链接 10 个模块）、`GOOS=windows go list -deps ./cmd/maskriver`（增加 `go-isatty`、`go-strftime`，并集 12 个）、`go build ./cmd/maskriver`（构建成功，`--version` 输出 `MaskRiver 0.1.0-dev`）。

## 3. 四条幸存 P2 的复核结论

| 编号 | 上一轮现象 | 本轮核对位置 | 结论 |
|---|---|---|---|
| P2-1 | `docs/feature-matrix.md` 的“Dry Run 零持久副作用”一行与 #9 的 F1（WAL 源被拒仍留 sidecar）冲突 | `docs/feature-matrix.md:119` | **已验证修复** |
| P2-2 | `docs/roadmap.md` 与 #9 已 CLOSED 的事实矛盾 | `docs/roadmap.md:12-13`、`:73-80` | **部分修复**（roadmap 正确；`README.md:10` 仍矛盾，见 R-17） |
| P2-3 | `docs/test-plan.md` 的 P2 表把 P2-8 排在 P2-7 之前 | `docs/test-plan.md:171-172` | **已验证修复** |
| P2-4 | 旧文档暴露开发者主机绝对路径 | `docs/closeout-report.md`、`docs/bootstrap-validation.md`、`docs/m1a-freeze-report.md` | **已验证修复** |

### 3.1 P2-1（feature-matrix 零副作用行）

现在的行文把原来的一句承诺拆成三项分别成立的事实，并显式写入 F1：

- ① 驱动级 SQL Spy 未观察到写/事务语句（引用 `TestPreviewIssuesNoWriteSQLThroughSQLSpy`、`TestSQLSpyReadOnlyOperationsIssueNoWrites`）；
- ② 主数据库文件执行前后字节不变；
- ③ 当次 fixture（rollback-journal）执行前后文件树未新增文件；
- F1：被拒绝的 WAL 源仍可能新增 `-shm`（32768 字节）与空 `-wal`（0 字节），目标端同样，因此不作“零文件变化”的无条件承诺。

**准确性核对**：F1 的字节数与现象与 [Issue #9 验收报告](../issue9-acceptance-report.md) 的实测记录一致（§11-F1：`source.db-shm` 32768 字节、`-wal` 0 字节；两侧同样；`TestWALSourceIsRefusedBySnapshotIdentity`）。语句“三项都不等于并发快照保证”与 `internal/runner/workflow.go:243-245` 的代码注释一致。**未发现新的过度承诺**：三项事实各自有测试证据，且明确否定了更强的“零文件变化”表述。

**残余观察（P3，不计入 P2）**：该行**标题**仍是“Dry Run 零持久副作用”，而状态列仍是“已实现（SQLite 证据）”。标题短语本身是绝对表述，正文已把它限定住；若日后有人只引用标题列，仍可能误读。建议下次文档收口时把标题改为“Dry Run 不写数据库内容”，但不构成新的 P2。

### 3.2 P2-2（roadmap 状态）——部分修复

`docs/roadmap.md:12` 已改为 **已关闭（CLOSED）**，`:13` 已改为 **已裁定 ACCEPT WITH RESTRICTIONS** 并写明 P0=0、P1=0、六条 P2 开放、**M1 未发布**；`:73-80` 新增 2026-10-19 增量节，且保留 2026-10-18 原文并注明“已被下节裁定取代”。就 `docs/roadmap.md` 本身而言，矛盾已消除。

**但同一类矛盾仍留在另一个面向读者的位置**：`README.md:10` 的状态横幅仍写“**M1 仍在收尾：#9 的 AC 待独立验收签署，尚未发布。**”，而同一文件 `README.md:46` 已写“M1-C 已关闭”。#59 只同步了 README 的“产品路线图”段，漏掉了首屏横幅。这是**同一 P2 类别的新实例**，登记为 **R-17（P2）**：读者在 README 顶部得到与 #9 CLOSED 相反的结论。

### 3.3 P2-3（test-plan 编号顺序）

`git diff 109aa8f HEAD -- docs/test-plan.md` 只显示两行的**换位**（现为 `:171` P2-7、`:172` P2-8）（`-` P2-8 / `+` P2-8 出现在 P2-7 之后），没有新增、删除或改写任何单元格内容。核对结果：P2-1…P2-8 连续无缺号，P2-7 在 P2-8 之前，内容与 #9 收尾时一致。**已验证修复**。

顺带核对（见 R-18）：P2-7 的**描述文字**与代码不完全吻合——`internal/db` 的注入 seam 是命名函数类型 `connectorFactory`（`internal/db/db.go:38-40`），`if connect == nil`（`internal/db/db.go:122`）对函数类型已是完整判空；typed-nil 风险只存在于**接口**值（如 runner 的 `ApprovalQuery`，`internal/runner/approval.go:59-72`）。这是登记条目的措辞问题（P3），不是产品缺陷。

### 3.4 P2-4（主机绝对路径）

按任务指定的三段用户家目录模式（Unix、macOS、Windows 各自的用户家目录前缀）对 `docs` 与 `README.md` 执行 `grep -rn …`，无匹配（退出码 1）；模式本身不在本文重复写出，以免再引入同类前缀。三份被点名的文档（`docs/closeout-report.md`、`docs/bootstrap-validation.md`、`docs/m1a-freeze-report.md`）改用 `<host>`/`<repo>` 占位符，证据日期、SHA-256 与结论保留。补充模式 `C:\\`、`/root/`、`/tmp/` 亦无匹配。**已验证修复**。

### 3.5 丢失的两条 P2

上一轮 P2 总数为 6，保留摘要只留下 4 条原文。**第 5、6 条仍为 UNVERIFIED**：内容、影响、分类均未知。本文不补写、不推测，也不把它们算作“已接受”。这意味着**本报告的限制清单仍可能不完整**（R-11）。

## 4. 完整残余风险登记表（从零重建）

编号规则：`R-*` 是本报告自己的序号，与 #9 的 P2-1…P2-8 登记表、以及上一轮 #11 的 P2-1…P2-4 **不共享编号空间**；引用时必须写明来源。严重度：P0/P1/P2/P3；`R-11`、`R-12` 是证据缺口，不是缺陷。

| ID | 风险 | 位置（file:line） | 触发条件 | 影响 | 类别 | 隐含的发布限制 |
|---|---|---|---|---|---|---|
| R-01 | 审批授权不是 RBAC：trust file 与审核私钥可由同一操作者控制 | `internal/history/trustfile.go:21-25`、`internal/history/store.go:57-75`、`cmd/maskriver/main.go:103-104`、`:110-112` | 同一操作者能写 trust file 且持有 `--reviewer-key-file` | 自审自批在工具层面不可阻止；无职责分离 | 设计边界（代码有意如此） | 不得宣称 RBAC、双人复核或抗内部人 |
| R-02 | `ReviewerID` 只是自填标签 | `internal/history/record.go:198`（仅校验非空与长度）、`internal/history/doc.go:31-33` | 审核方填写任意 ID | 审计记录无法证明“谁”批准 | 设计边界 | 不得把 `ReviewerID` 当作身份或追责证据 |
| R-03 | 两处时钟容差不一致（#9 P2-8） | `internal/runner/approval.go:106`（`now.Before(record.IssuedAt)` 无容差）vs `internal/history/record.go:33`、`:297`（5 分钟未来签发容差） | 审核主机时钟快于执行主机 ≤5 分钟 | 存储层认可的记录在 Runner 复核处被拒；fail-closed，不误授权，只降低可用性 | code-level（已登记，未修） | 部署必须保证 NTP 同步；不得宣称审批在任意时钟偏差下可用 |
| R-04 | WAL/journal：源侧 sidecar 副作用（F1）且目标侧 journal mode 未校验（#9 P2-5） | `internal/db/capability.go:45-66`（`Quiescent`）、`internal/runner/workflow.go:389-397`（只查源）、`docs/feature-matrix.md:119` | 源或目标为 `journal_mode=WAL` | 被拒绝的运行仍留下 `-shm`/`-wal`；目标端 WAL 提交可能落在主文件之外，`targetDigest` 看不见 | code-level + 测试缺口 | 不得宣称 Dry Run 零文件变化；不得宣称 WAL 目标已验证 |
| R-05 | `--strict` 无独立语义 | `cmd/maskriver/main.go:289-293`、`internal/verify/verify.go`（全文无 `Strict` 引用）、`pkg/contracts/contracts.go:305` | 任何 `validate` 调用 | `Strict` 只影响 `contracts.Passed` 判决，不产生额外数据库级检查 | code-level（合同字段 inert） | 不得宣称 `--strict` 提供额外严格检查 |
| R-06 | `internal/db` 写入层仍接受非 NULL→NULL（#9 P2-1） | `internal/db/write.go:183-193`（只拒绝 NULL→非 NULL；`expected == replacement` 不覆盖 NULL 替换） | 绕过 runner 直接调用 `Writer`/`WriteTx` | 可写入永远无法通过严格验证的行 | code-level（CLI 上游已拒绝） | 仅 CLI/Runner 路径有保证；不得宣称 Writer 层自证约束 |
| R-07 | MySQL 用户 CLI 结构上不可达；只有适配器组件被测 | `cmd/maskriver/main.go:1`、`:81-82`（SQLite-only 参数）、`internal/db/capability.go:49`（`Quiescent` 对非 SQLite 返回 `ErrUnsupported`）、`internal/db/mysql_test.go:22`（无 socket 即 SKIP） | 尝试用 MySQL 作为源 | 快照静默性闸门（`Quiescent`）在 MySQL 上直接返回 `ErrUnsupported`，CLI 全流程不存在 | 范围缺口 | 不得宣称 MySQL CLI 支持；MySQL 8.0.46 适配器证据只在 Ubuntu CI 成立 |
| R-08 | **发行工件许可声明集合不完整**（P1，见 §5） | `docs/attribution.md:34-35`、`:41-43`；`docs/licenses/`（12 份）；`docs/m1-acceptance-issue11.md:§8` | 按现有材料打二进制发行包 | 被编译进二进制、由 MIT/BSD 覆盖的代码缺少必须随附的声明；MPL-2.0 源码获取途径未落地 | 文档/发行流程 | **禁止二进制发行**，直到声明集合与 NOTICE 载体补齐（§5.3） |
| R-09 | 历史存储不检测整文件回滚/撤销回滚（#9 P2-6） | `internal/history/doc.go:41-50`、`internal/history/store.go:279` | 攻击者可写 store 目录 | 可恢复旧的有效审批，或回滚撤销 | code-level（本地文件存储固有） | 不得宣称审批防篡改；部署需外发摘要 |
| R-10 | 审批的 `TargetID` 只是目标**路径**摘要，不绑定目标内容 | `internal/runner/workflow.go:243-245` | 审批后同路径目标文件被替换 | 审批绑定不覆盖目标内容；内容差异只能靠 dry run 的逐行比对兜住 | code-level（已声明） | 不得宣称审批绑定了目标快照内容 |
| R-11 | 上一轮 #11 两条 P2 内容丢失 | 无（原报告随 `/tmp` 丢失） | 需要完整限制清单时 | 限制清单可能不完整 | 证据缺口 | 不得把本报告当作 #11 限制清单的最终完备版本 |
| R-12 | 历史证据为转述：CI run `39132592907` 的 job 明细、PR #58 复评报告原文均已丢失 | `docs/m1-acceptance-issue11.md:§1`、`:§3.4`、`:§10` | 需要原始证据时 | 只能依赖保留摘要 | 证据缺口 | 不得把“评审结论”复述为“原始证据” |
| R-13 | 源快照身份不防并发外部写入（rollback-journal 亦然） | `internal/runner/workflow.go:238-245` | 外部进程并发写源或目标 | 快照身份只是主文件摘要，不证明隔离 | code-level（已声明） | 只能在隔离副本上运行；不得宣称并发安全 |
| R-14 | `#9` 的两个扫描拒绝用例的 `spy.opens` 断言无法失败（#9 P2-2） | `internal/runner/scan_gate_test.go:45`、`:72` | 闸门回归 | 用例捕获不到闸门回归 | 测试缺口 | 不得把这两条用例当作闸门回归证据 |
| R-15 | `gather` 顶部 `ctx.Err()` 短路无独立用例（#9 P2-3） | `internal/runner/workflow.go:131-134`、`internal/runner/scan_fault_test.go:147` | 扫描中途取消 | 取消传播证据不完整 | 测试缺口 | 取消语义只能按已测路径声明 |
| R-16 | 测试 seam 为可变包级变量（#9 P2-4） | `cmd/maskriver/main.go:281-293` | 并行测试 | 全局状态污染，不可并行 | 测试隔离 | 不影响生产行为 |
| R-17 | **`README.md:10` 状态横幅仍与 #9 CLOSED 矛盾**（P2，§3.2） | `README.md:10` vs `README.md:46` | 读者读首屏 | 得到过时里程碑结论 | 文档矛盾 | 修正前不得把 README 首屏当作状态来源 |
| R-18 | `docs/test-plan.md` P2-7 描述与代码不符（P3） | `docs/test-plan.md:171`、`internal/db/db.go:38-40`、`:122` | 读者据此安排修复 | 会去做一个对函数类型并无意义的 typed-nil 回归 | 文档准确度 | 无发布限制 |
| R-19 | 检测准确率来自开发集、非留出评估 | `docs/test-plan.md:§UNVERIFIED`、`docs/issue9-acceptance-report.md:§10` | 宣称识别能力 | 74 条自编样本，姓名 P=0.500/R=0.750 | 范围缺口 | 不得宣称生产级识别准确率 |
| R-20 | 无持久映射、全局唯一性、匿名化、参照完整性 | `docs/feature-matrix.md`（持久映射/匿名化行）、`docs/test-plan.md:§UNVERIFIED` | 宣称脱敏等价匿名化 | 确定性≠无碰撞 | 范围缺口 | 不得宣称匿名化或无碰撞 |
| R-21 | macOS 不在 CI 矩阵 | `.github/workflows/ci.yml`、`docs/test-plan.md:§跨平台` | 在 macOS 使用 | 未验证 | 范围缺口 | 不得宣称 macOS 支持 |

**登记表自检**：R-01…R-10 是代码/流程层面的实质风险，R-11…R-12 是证据缺口，R-13…R-16 与 R-18 是测试与文档准确度，R-17 是本轮新发现的文档矛盾，R-19…R-21 是范围缺口。**没有任何一项被从上一轮降级**；R-08 是本轮**新增的 P1**。

## 5. 许可与发行合规分析

### 5.1 PR #58 的 P1 修复：逐字节复核（12/12 通过）

方法：对 `go.mod` 的每一个模块，用 `cmp` 比较 `<GOMODCACHE>/<module>@<version>/LICENSE` 与 `docs/licenses/<file>`。

| 模块 | 版本 | 仓库文件 | 字节 | 许可 |
|---|---|---|---|---|
| `github.com/go-sql-driver/mysql` | v1.10.1 | `go-sql-driver-mysql-LICENSE` | 16726 | MPL-2.0（含 Exhibit A/B） |
| `modernc.org/sqlite` | v1.60.1 | `modernc-sqlite-LICENSE` | 1489 | BSD-3-Clause |
| `modernc.org/libc` | v1.77.1 | `modernc-libc-LICENSE` | 1482 | BSD-3-Clause |
| `modernc.org/mathutil` | v1.7.1 | `modernc-mathutil-LICENSE` | 1486 | BSD-3-Clause |
| `modernc.org/memory` | v1.12.1 | `modernc-memory-LICENSE` | 1484 | BSD-3-Clause |
| `filippo.io/edwards25519` | v1.2.0 | `edwards25519-LICENSE` | 1479 | BSD-3-Clause |
| `github.com/google/uuid` | v1.6.0 | `google-uuid-LICENSE` | 1480 | BSD-3-Clause |
| `github.com/remyoudompheng/bigfft` | v0.0.0-2023… | `bigfft-LICENSE` | 1479 | BSD-3-Clause |
| `golang.org/x/sys` | v0.48.0 | `x-sys-LICENSE` | 1453 | BSD-3-Clause |
| `github.com/dustin/go-humanize` | v1.0.1 | `go-humanize-LICENSE` | 1136 | MIT |
| `github.com/mattn/go-isatty` | v0.0.24 | `go-isatty-LICENSE` | 1099 | MIT |
| `github.com/ncruces/go-strftime` | v1.0.0 | `go-strftime-LICENSE` | 1068 | MIT |

结论：**12/12 逐字节一致，无改写、无截断、无错配**；归属表（`docs/attribution.md:21-32`）与 `docs/licenses/` 是 1:1（12 行 / 12 文件）；分类 8 份 BSD-3-Clause、3 份 MIT、1 份完整 MPL-2.0，与 `docs/m1-acceptance-issue11.md:§3.3` 的记述一致。

补充事实（本轮新增，供准确性使用）：`go list -m all` 共 27 个模块，`go list -deps` 显示 **linux/amd64 只链接 10 个**（`go-isatty`、`go-strftime` 在 Linux 上不参与编译），`GOOS=windows` 时才出现，因此“12 个被编译或链接”是**各目标平台并集**，不是单一二进制的数量；这与 `docs/attribution.md:34-35` 的表述不冲突，但读者若只按 Linux 构建核对，会看到 10 个而非 12 个。

### 5.2 模块内嵌的第三方声明：**未收录且未被清单覆盖**

仅比较模块顶层 `LICENSE` 是不够的。模块缓存中还存在被编译进二进制的第三方材料：

| 文件 | 字节 | 内容 | 该文本覆盖的材料是否进入二进制 | 仓库是否收录 |
|---|---|---|---|---|
| `modernc.org/libc@v1.77.1/LICENSE-3RD-PARTY.md` | 10505 | Go（BSD-3）、musl libc（MIT）、go-netdb（MIT）、NixOS/nixpkgs（MIT） | **是**（linux/amd64 默认编译 musl 转译实现，见下） | **否** |
| `modernc.org/sqlite@v1.60.1/LICENSE-3RD-PARTY.md` | 74323 | 上游扁平化的整图声明，内含 musl/go-netdb/nixpkgs 与 “SQLite Is Public Domain” | **是**（页面内含 SQLite 公有领域声明，SQLite amalgamation 已链接） | **否** |
| `modernc.org/sqlite@v1.60.1/LICENSE-SQLITE` | 1506 | SQLite 公有领域奉献声明 | 是（amalgamation） | **否** |
| `modernc.org/sqlite@v1.60.1/LICENSE-SQLITE_VEC` | 1068 | MIT（2024 Alex Garcia） | **否**（`modernc.org/sqlite/vec` 不在 deps，二进制中无 `sqlite_vec` 符号） | 否（当前无义务） |
| `modernc.org/memory@v1.12.1/LICENSE-GO` / `LICENSE-MMAP-GO` / `LICENSE-LOGO` | 1479 / 1518 / 62 | Go BSD-3 / Evan Shaw BSD-3 / 商标图片链接 | Go、MMAP-GO 部分相关 | **否** |
| `golang.org/x/sys@v0.48.0/PATENTS` | 1303 | 附加专利授权 | 是 | **否** |
| `modernc.org/mathutil@v1.7.1/mersenne/LICENSE` | 1486 | BSD-3 | 是（mathutil 已链接） | **否** |

**musl 是否真的被编译进去？** 是。`modernc.org/libc@v1.77.1` 中 `libc_linux_amd64.go` 的头是 `//go:build !(linux && amd64)`，而 `libc_musl_linux_amd64.go` **没有 build tag**，即 `linux/amd64` 默认就使用 musl 转译实现；构建出的二进制中也出现了 `libc_musl.go`、`native_musl.go`、`etc_musl.go` 的路径字符串。因此 linux/amd64 的发行包**必须**携带 musl 的 MIT 声明，而 `docs/licenses/` 里没有。

`docs/m1-acceptance-issue11.md:§8` 只披露了 libc 一个文件（“内置第三方声明未捕获”），没有列出 `modernc.org/sqlite` 的 74323 字节声明、`LICENSE-SQLITE`、`modernc.org/memory` 的三份附加文本与 `x/sys/PATENTS`；而 §8 的措辞把三项当作**完整的发布前核查清单**（“上列三项必须在每次实际发布前逐项核查并留证”）。**控制项不完整**，所以本报告把 R-08 定为 P1，而不是把 §8 的披露当作已覆盖。

### 5.3 “仓库里有文本”≠“发行工件合规”

必须明确区分两件事：

1. **仓库状态**（可验证、本轮已部分验证）：`docs/licenses/` 有 12 份与模块缓存逐字节一致的顶层 `LICENSE`；归属表与目录 1:1。这一层**成立**。
2. **发行工件状态**（本轮**未**成立，也不可能有，因为还不存在发行工件）：合规对象是**发布出去的那个二进制包及其随附材料**。它至少要求：
   - 一份随包 `NOTICE`（或等价的第三方声明文件），**归并**所有适用文本：12 份顶层文本 + musl/go-netdb/Nixpkgs 的 MIT/BSD 文本（现成来源：`modernc.org/libc@v1.77.1/LICENSE-3RD-PARTY.md`，或 `modernc.org/sqlite@v1.60.1/LICENSE-3RD-PARTY.md` 这一份更完整的扁平化清单）+ SQLite 公有领域声明 + `x/sys` 的 `PATENTS` 声明；
   - MPL-2.0 覆盖文件（`github.com/go-sql-driver/mysql`）的**对应源码获取途径**：在随包材料中给出确切获取方式（例如固定的 module 版本 `v1.10.1` 与公开发布地址），这是 MPL-2.0 §3.2 对二进制分发的要求；`docs/attribution.md:41-43` 已声明该义务，但**尚未落地到任何工件**；
   - 若发行包附带源码或按 “source form” 提供，还需保证所提供源码与二进制对应的 **exact version**（`go.sum` 已固定，可支持这一点）；
   - 每次发布**逐项留证**（谁核对、核对了哪些文件、对应 SHA-256），而不是引用仓库目录结构。

因此，`docs/licenses/` 的存在**本身不使任何发行工件合规**；`docs/m1-acceptance-issue11.md:§8` 已正确提示这一点，本报告补充：**清单本身也不完整**（§5.2）。

## 6. VERIFIED vs UNVERIFIED 能力

**VERIFIED（本轮有真实证据）**

- 代码与文档树处于可发布检查的干净状态：`gofmt`/`go vet`/`git diff --check`/`go test`/`go test -race` 全绿（§2）。
- 四条幸存 P2 中 3 条已修复（P2-1、P2-3、P2-4），1 条部分修复（P2-2，`README.md:10` 残留）。
- 12/12 顶层许可文本逐字节一致，归属表 1:1，分类正确（§5.1）。
- 上一轮 P1 的**字面主张**（“每个被编译/链接模块的许可原文已在 `docs/licenses/`”）成立。
- `--apply` 是唯一写入口、门禁在 `openWriter` 之前（`internal/runner/execute.go:513-544`，本轮对照代码确认）。
- musl 材料确实被编译进 linux/amd64 二进制（build tag + 二进制路径字符串，§5.2）。

**UNVERIFIED（不得据此宣称）**

- 上一轮丢失的 **2 条 P2** 的内容与影响（R-11）。
- CI run `39132592907` 的 job 明细、PR #58 第二上下文复评报告原文（R-12）。
- MySQL 用户 CLI 全流程（本机两条 MySQL 用例均 SKIP；只有 Ubuntu CI 的适配器测试）R-07。
- `--strict` 的独立严格语义（R-05）。
- 目标端 WAL/journal mode（R-04）、目标内容与审批的绑定（R-10）、并发快照（R-13）。
- 断点恢复、checkpoint、跨进程幂等、exactly-once（未实现）。
- 生产级检测准确率（R-19）、持久映射/匿名化/参照完整性（R-20）、macOS（R-21）。
- **任何二进制发行工件的许可合规性**（R-08：当前不存在工件，且现有材料不足以产出合规工件）。

## 7. 有限 M1 发布可以宣称与不可以宣称

**可以宣称**

- 一个在授权、隔离的**合成 SQLite** 库上完成 `scan → preview → approve → 新进程 mask --apply → validate` 的 M1 受限版本；写入只能由显式 `--apply` 触发。
- 已接受 P2、UNVERIFIED 清单与限制条件**公开可查**（本文件、`docs/test-plan.md`、`docs/issue9-acceptance-report.md`、`docs/m1-acceptance-issue11.md`）。
- 12 份顶层第三方许可文本已与模块缓存逐字节核对一致。

**必须不宣称**

- 不得宣称 M1 已全面发布、已通过“完整安全批准”，或可处理生产/真实个人数据。
- 不得宣称支持 MySQL CLI、断点恢复、exactly-once、高吞吐并发、生产级 PII 识别准确率或 macOS 支持。
- 不得宣称匿名化、无碰撞或参照完整性保证；不得宣称 `--strict` 提供额外严格检查。
- 不得宣称 Dry Run 在任何情况下都不新增文件（F1）；不得宣称 WAL 目标已验证。
- 不得宣称审批等于 RBAC 或抗内部人；`ReviewerID` 只是自填标签（R-01、R-02）。
- 不得宣称审批绑定目标**内容**（R-10），或历史存储防篡改（R-09）。
- **不得宣称发行许可已合规**：`docs/licenses/` 不是发行工件；NOTICE 与 MPL-2.0 源码获取途径尚未落地（R-08）。

## 8. 发布决定

**ACCEPT WITH RESTRICTIONS — limited M1 release permitted**

依据：P0=0；产品代码未变且全部闸门 green；四条幸存 P2 中 3 条已修复；唯一 P1 属**发行/文档**层面，且在“不发布二进制”的受限范围内不构成当前运行风险。

限制（每条都可独立核查）：

1. **不发布二进制工件**。任何 `maskriver` 可执行文件、压缩包、容器镜像或类似发行物**均不得**在当前树状态下发布，直到 R-08 关闭（§5.3 的 NOTICE、完整适用文本、MPL-2.0 源码获取途径、逐项留证）。
2. 发布范围仅限**合成 SQLite 快照**上的受限 CLI 流程；不得用于生产或真实个人数据。
3. 使用前必须保证审核主机与执行主机时钟同步（R-03）；否则刚签发的审批会被 Runner 拒绝。
4. 源与目标必须是**隔离副本**且为 rollback-journal 模式；不得声称 Dry Run 不新增任何文件（R-04）。
5. 不得把 `--strict`、`ReviewerID`、trust file 或审批记录描述为超出 §7 的能力（R-01、R-02、R-05）。
6. 读者可见的发布说明必须与本文件 §6/§7 的 VERIFIED/UNVERIFIED 清单一致；`README.md:10` 的状态横幅在修正前（R-17）不得被引用。
7. 本决定**不替代**后续 Issue 对上一轮 6 条 #11 P2（其中 2 条仍为 UNVERIFIED）与 #9 P2-1…P2-8 的逐条关闭。

## 9. Issue #11 是否可关闭

**暂不可关闭**。理由：

1. 存在 1 条未关闭的 **P1（R-08）**：二进制发行所需的第三方声明集合不完整，且 `docs/m1-acceptance-issue11.md:§8` 的发布前核查清单**未列出** `modernc.org/sqlite` 的 74323 字节声明、`LICENSE-SQLITE`、`modernc.org/memory` 的附加文本与 `x/sys` 的 `PATENTS` —— 按该清单执行仍会产出不合规的发行包。这属于 AGENTS.md 合并门禁第 3 条（“没有未解决的 P0/P1”）与第 6 条（“无敏感/合规风险”）的范围。
2. 上一轮 2 条 P2 仍为 UNVERIFIED（R-11），限制清单可能不完整；在补齐前，不应以“P1=0”为基础关闭。
3. `README.md:10` 与 `#9 CLOSED` 的矛盾（R-17，P2）应在关闭动作前一并修正，避免关闭记录本身引用一个自相矛盾的状态源。

关闭条件（建议写入 Issue）：① 补齐 §5.3 的 NOTICE/文本/MPL 源码获取途径，并把它们加入 `docs/attribution.md` 与 §8 清单；② 该修复由**第二个独立上下文**复核；③ 修正 `README.md:10`；④ 关闭记录引用本文件与 `docs/m1-acceptance-issue11.md`，并声明 2 条丢失 P2 仍为 UNVERIFIED（或由仓库所有者重新提供/重新评审）。

## 10. 证据路径

- 本文件：`docs/reviews/issue11-rereview-2026-10-19.md`（上一轮报告的替代证据载体；**不恢复**丢失原文）
- `docs/m1-acceptance-issue11.md`（裁定、六条 P2、§8 发布前核查清单）
- `docs/attribution.md` 与 `docs/licenses/`（12 份原文）
- `docs/feature-matrix.md:119`、`docs/roadmap.md:12-14`、`:73-80`、`docs/test-plan.md:171-172`
- `docs/issue9-acceptance-report.md:§11-F1`（WAL sidecar 实测）
- 被审查提交 `c3907fe`、`39d738f`，以及工作树 HEAD 与 `origin/main` 的空 diff（§1）
- 模块缓存内的上游声明（以 `<GOMODCACHE>/<module>@<version>/…` 引用，不写入主机路径）
