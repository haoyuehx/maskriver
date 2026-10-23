# Test plan

MaskRiver 的测试目标：证明组件在真实数据库上按其契约工作，并证明安全不变量不会被绕过。
本计划只依据 MaskRiver 自身的接口（[contracts.md](contracts.md)）与范围（[feature-matrix.md](feature-matrix.md)）。

## 已有测试（当前真实状态）

- `pkg/contracts/contracts_test.go`：typed Value / NULL / 精度 / 时间语义、fmt 与 JSON 遮蔽、无效 Plan 拒绝、报告 strict gate。
- `internal/testenv/fixture_test.go`：SQLite 真实驱动与合成数据、约束、事务、只读、取消；MySQL 为 opt-in，未启动时明确 `SKIP`。
- `cmd/maskriver/main_test.go`：help/version/非法参数、无授权拒绝，以及真实 SQLite 合成源与目标库 Scan、默认 Preview、显式 `--apply --approved`、strict Validate、源与目标文件哈希/唯一列和重复 Apply 拒绝；另含多表 CLI、scan 故障退出码、`null` 策略拒绝与 `--strict` 接线回归。
- `cmd/maskriver/approval_e2e_test.go`：以子进程执行 scan/preview/approve/新进程 apply/validate；覆盖两表整计划审批成功、仅批一张表被拒、换序/换策略/换 schema/换源快照/换目标绑定被拒，以及只读命令不创建历史目录。
- `internal/runner/*_test.go`：真实 SQLite 多表 Plan/Preview/Apply/Validate、驱动级 SQL Spy 零写、静态安全门禁、NULL 保持、批次故障/取消/提交未知和静态拒绝顺序。
- `internal/config/config_test.go`：零值只读与显式 Apply 意图。
- `tools/fixture`：生成 `.local/m1a-synthetic.db`，拒绝覆盖已有文件。
- `scripts/test_mysql.py`：可选 POSIX 本机专用禁网 MySQL；历史宿主机权限失败不是通用开发阻塞。
- `scripts/test_mysql_container.py`：Linux 或 Ubuntu CI 一次性禁网 MySQL 8.0.46 容器，复用 marker + 私有 socket guard；现执行 TestMySQLFixture、TestMySQLAdapter 与真实 TestMySQLCLIWorkflow（PR #30、#93）。

`internal/detect`、`internal/db`（SQLite/MySQL Reader/Writer、keyset 分页与事务）、`internal/mask`（五种 M1 策略、内存映射与九种中国格式替换）和 `internal/verify` 均已在 `main` 交付，测试证据见各自包内测试。`internal/db/mysql_test.go` 的 `TestMySQLAdapter` 只有在传入隔离 MySQL socket 时执行；Ubuntu CI 的一次性 MySQL 8.0.46 容器会执行这一适配器业务测试，本地普通测试无 socket 时仍会 SKIP。

当前真实 SQLite CLI 证据覆盖单表与多表：`cmd/maskriver/main_test.go` 的 `TestCLIRealSQLiteWorkflow`、`TestCLIMultiTableWorkflow`、`TestCLINullStrategyRefusedBeforeWrite`、`TestCLIStrictFlagWiring`、`TestCLIScanFailureExitsNonZeroWithoutWrite`，以及 `cmd/maskriver/approval_e2e_test.go` 的子进程审批/新进程 apply 用例。MySQL 8.0.46 CLI 与 strict 语义后由 M3-2/PR #93 补齐；本节早期 SQLite 证据不代表 #9/#11 的全部验收，也不代表 M1 的最终发布结论。

```sh
gofmt -l cmd internal pkg tools
go test -count=1 ./...
go vet ./...
go build -o bin/maskriver ./cmd/maskriver
```

## 跨平台与统一 CI

贡献者通过 fork/分支/PR 参与；角色名称不预设固定人数。纯内存模块可在 Windows/Linux/macOS 独立测试；不依赖本地 Agent smoke。共享测试环境变动由 Main 协调。

`.github/workflows/ci.yml` 在 Ubuntu/Windows 执行格式检查、`go test -count=1 -v ./...` 和 `go vet ./...`，Ubuntu 另跑 race。MySQL 隔离业务测试单独在 Ubuntu 禁网容器执行 fixture + adapter，失败即 CI 失败；普通测试无 MySQL socket 时明确 SKIP。macOS 可以本地测试，macOS 非流式 M1 已由 CI 覆盖；macOS/Windows streaming/recovery 仍不支持。

PR 记录 OS、Go 版本、命令、退出码、跳过原因和剩余风险，统一 CI 与评审完成后才能合并。本机不支持 race 可由 Ubuntu CI 补齐，不能记成本机通过。#10 的当前适配器用例已在 PR #30 隔离 MySQL 8.0.46 实跑；完整 MySQL CLI workflow 后续由 M3-2/PR #93 验证，其他 MySQL 版本与生产环境仍未验证。

## 分阶段门禁

- M1-B 首轮：真实 SQLite 必须通过；MySQL 可 `UNVERIFIED/SKIP`，但实现与真实集成用例必须交付。
- 最终 M1 验收：真实 MySQL 集成必须通过。缺此项不得宣称 M1 完成。
- Mock、编译通过、跳过都不能替代真实数据库验证。
- 工作树/模型诊断是可选工具检查，不是开发启动或业务验收门禁。

## 第一阶段验收矩阵（M1 验收目标；已覆盖项见各专项证据）

| 范围 | 关键用例 |
|---|---|
| 扫描 | 单列失败阻断 apply；skip 与 override 次序；UNKNOWN 不写入历史；错误分类与退出码 |
| 检测（#6 / PR #20） | 13 类 China-first 有界规则的正反例；身份证/USCC/Luhn 校验；城市中文与拼音；中文姓名普通词误报；默认 20 非空 distinct 与 19/20、18/20 vs 17/20、90% 阈值；NULL/空串；冲突 UNKNOWN；位置/类型不匹配的人工覆盖 ErrInvalid；DateOrder 参数验证（MDY/DMY 文本解析不在本次覆盖）；取消、输入不变性和无敏感值日志 |
| 策略 | 首批 5 种策略的格式与类型保持；Unicode；NULL 与空串；typed 数值/日期；校验位有效性；未变化结果拒绝 |
| 确定性 | 固定 key/scope/version 重复一致；跨表同 scope 一致；映射复用；词典变化不漂移；碰撞处理 |
| Dry Run | 配置无法绕过 CLI 写意图；连接 spy 证明无写事务；文件树无新增或修改；历史与映射只读 |
| 分页/写入 | 超过两页；复合键；最后不足一页；无键或敏感主键拒绝；事务回滚；受影响行数；提交结果不明 |
| NULL 标记 | 仅精确列生效；先于映射；不写映射；目标列 NOT NULL 时报错 |
| 验证 | 行数与 schema；索引与约束；双向键差异；单字段未变化；无键、截断、缺表在 strict 下失败 |
| 历史 | pending/approved；审核人；类型与过期；修订冲突；导入失败不污染；回写备份 |
| 端到端 | 合成源快照 → scan → preview → apply → strict validate；SQLite 与真实 MySQL 分别独立运行 |

## China-first 检测专项测试与评估说明

- 代码证据：`internal/detect/detect_test.go`、`cn_test.go`、`cn_geo_test.go`、`eval_test.go`；不依赖数据库，应用 `go test -count=1 ./internal/detect/...`、`go test -race ./internal/detect/...`、全仓 `go test ./...` 和 `go vet ./...`。
- 合成评估：74 条自行构造的 `(value,column)` 样本（38 正、36 负；8 条超出本轮规则覆盖）；`MinSamples=1` 用于测试匹配器与列上下文，而非默认聚合阈值。报告每条规则的 TP/FP/FN、Precision、Recall、F1 和明确的覆盖界限。样本同时用于开发与评估，**不宣称独立验证或生产准确率**。
- 姓名误报：开发时数据 `cn_person_name` 的 TP=3、FP=3、FN=1（P=0.500、R=0.750、F1=0.600）。在独立负例扩充、评审和业务级不确定处理完成前，姓名检测只作实验性提示，不能自动推导写入批准。
- 测试与范围分离：组件测试不能替代真实 SQLite/MySQL 端到端扫库、策略映射、Dry Run、Apply 和严格验证。中国身份证/手机号/银行卡/USCC 的**格式约束替换**仍是 #12 与 go-mask 的待完成范围。
- GitHub CI 的 Ubuntu/Windows 格式、测试与 vet 以及 Ubuntu race 需针对**最终 PR Head**重新执行；MySQL fixture 绿灯不是业务 Adapter 验收。所有 SKIP/UNVERIFIED 需明确记录。

## 数据库适配器专项验收（#5 / PR #21）

- 使用真实 SQLite 驱动运行 `go test -count=1 ./internal/db/...`，确认只读且缺失文件不创建、表元数据与保守 `SchemaCoverage`、有界 DISTINCT 非 NULL 抽样、NULL/空串/Bytes/Decimal/Date 与安全类型处理、25 行三页、真实有序复合主键 keyset、乐观锁冲突零提交、Rollback、取消、非法列名与跨 DatasetID/Schema 白名单拒绝。特别覆盖新增 `internal/db/count_scope_test.go` 的 Count 越界检查。
- Go CI 应在 Ubuntu/Windows 运行全仓测试和 vet，并在 Ubuntu 运行 race；以合并目标分支对应最终 PR Head 的通过记录为准，不把 queued、SKIP 或 fixture 环境通过写成适配器验收通过。
- MySQL 业务测试入口包括 `internal/db/mysql_test.go:TestMySQLAdapter` 与 `cmd/maskriver/mysql_e2e_test.go:TestMySQLCLIWorkflow`，都需要隔离容器私有 socket 和 `MASKRIVER_TEST_MYSQL_SOCKET`；本地无权限时记录 `SKIP/UNVERIFIED`。M3-2 起 CI 作业实际执行真实 CLI 的 Scan→Preview→Approve→Apply→Strict Validate，并覆盖 source 不变、target 变化、重复 apply 拒绝、同库拒绝和 streaming 拒绝。MySQL recovery 仍未实现。
- 本轮数据库组件不等于 CLI/runner 端到端已交付；全链路 Dry Run/Apply/严格 Validate 仍由 #9/#11 验收。

## 脱敏策略组件专项验收（#7 / 原 PR #18 / PR #24）

- 五种策略 `null`、`blank`、`redact`、`format_random`、`fake_email` 仅实现纯内存有界切片，统一入口 `mask.NewStrategy`、`mask.NewMemoryMapping`，并验证冻结 `m1a-v1` 签名；不实现中国身份证/银行卡/信用代码校验位保持替换或密码学 FPE（#12 的九种格式替换已由后续 PR #29 交付，见下节）。
- 测试覆盖：合法 `Bytes` 密钥且 >=32 字节，Text 密钥拒绝；ctx nil/取消；NULL 与空串；非 Nullable 列 null 拒绝；非空敏感输入不得保持不变；未知策略及不支持的 Kind 拒绝；HMAC keyed domain 隔离；仅支持部分 ASCII/汉字类的 Text 形状保持、任意 Bytes 等长；中文与二进制短输入边界；`fake_email` 只输出 keyed HMAC 和固定域名，不附原始值 CRC32。
- 内存映射采用键控指纹与原子 get-or-create 的单赢家语义；lookup 不写入；并发与 `go test -race ./internal/mask/...` 在最终 PR Head 的 Ubuntu CI 验证。暂不承诺持久映射、跨库 exactly-once、唯一性/碰撞或参照完整性。
- 最终 CI 需执行 `gofmt -l`、`go test -count=1 ./...`、`go vet ./...`、Ubuntu race，记录真实结果。仅 CI 配置就绪或 MySQL fixture 通过不能替代运行态组件验收。

## #12 中国格式约束脱敏组件（PR #29）

- **代码边界**：`internal/mask/cn_format.go`、`cn_numeric.go`、`cn_text.go`、`cn_format_test.go`；仅新增 9 个 `cn_*` StrategyRef，版本 `1`。共享 `m1a-v1`、数据库适配器和 CLI 工作流接口均不变。
- **明确约束**：身份证合法日期/有限地区码/MOD11-2；手机已知号段及可选国家码；银行卡 16–19 位/Luhn；USCC 有限类别/地区/MOD31；邮编六位；城市有界中英词表；姓名 2–4 汉字和限定姓氏；地址窄格式街道门牌；合法中国年月日及 typed Date。超范围输入必须返回 `ErrInvalid`/`ErrUnsupported`，而非伪造成功。
- **安全测试**：密钥 Bytes 且至少 32B，类型及策略/Scope/KeyID 绑定，取消与 nil context，NULL 保持、非 NULL 不得不变、目标长度不匹配拒绝，明确校验位错误/非闰日/错误号段/无效地址负例；合成输入不进入错误与日志，策略不访问数据库。
- **逐规则评估**：`TestCNFormatSyntheticEvaluation` 的开发时手工构造集合含 21 个正例、28 个负例（九条规则），测试格式接受结果并校验替换输出能再次通过对应格式规则。初轮 PR #29 CI 实测九条规则总 TP=21、FP=0、FN=0；逐规则 P/R 均为 1.000。这个小规模自编集合不是独立留出评估，更不是生产 PII 检测准确率或匿名化证明；检测规则的独立指标仍见 `internal/detect/eval_test.go`。
- **可复现命令**：`go test -count=1 -v ./internal/mask/...`、`go test -count=1 ./...`、`go vet ./...`、`go test -race ./internal/mask/...`、`gofmt -l internal/mask`；最终 PR Head 的 Ubuntu/Windows GitHub CI 应全部成功。先前通过的旧 SHA 不算最新提交通过。
- **真实 SQL 组件集成**：PR #30 在 SQLite 真实快照做日期策略的只读文件哈希对比、optimistic UPDATE/COMMIT、独立 SQL 回读和源快照不变性校验；另对全部九种策略在实际 SQLite 和 MySQL 8.0.46 表上做 typed Page → mask → WriteTx → 持久化读回及行数/主键检查。
- **仍未验证**：九种 CN 策略经 CLI 全链路的 Scan/Preview/Apply/strict Validate、针对 CN 策略的 SQL Spy 断言、跨列唯一/PK-FK、持久映射及所有目标列的字节长度限制，属于 #9/#11；通用 SQL Spy 已由 `internal/db` 与 `internal/runner` 的预览回归覆盖，但不是 CN 策略专项证据。生成格式有效的身份证、银行卡、手机号等号码并不保证无人持有、不会被投递/联系，也不保证匿名性；只能处理授权的合成/隔离测试数据。

更多覆盖范围与限制见 [#12 中国格式组件说明](issue12-cn-replacement.md)。

## #8 验证器组件（PR #32 原作者，PR #33 安全整合）

- **接口**：`verify.New() contracts.Validator`，仅传入两个 `contracts.Reader` 和冻结 `ValidateRequest.Plan`；不读取配置、不写源库/目标库、不扫描目标内容推断敏感列、不输出行或主键。
- **有界内存**：两边各保持至多 256 行的 `Page`，按真实 Keyset 顺序逐键合并；聚合同类受控 `Issue`，不会为每条行差异生成无界报告。对于未支持的键排序/Collation 显式 `ErrUnsupported`。
- **契约**：`RowLimit=0` 执行全量有界分页；正数表示总行预算，超过时 `PartialCoverage` + `Complete=false`；负数或 nil ctx/reader 拒绝。缺失 schema/PK 能力、类型不符、计划列缺失、重复键/倒退游标/空页面等必须失败关闭；完整空表可通过。
- **回归**：`internal/verify/verify_test.go` 保留原始 PR #32 的 fake-Reader 用例；`verify_safety_test.go` 新增 520 行分页、Issue 聚合、非法游标、计划/schema 错配、nil Context、缺少字段、刚好达到总行预算和截断拒绝。仅冻结 `Plan` 中的敏感列查验非 NULL 值变化，NULL 语义异常不通过。
- **门禁**：`gofmt -l internal/verify`、`go test -count=1 -v ./internal/verify`、`go test -count=1 ./...`、`go vet ./...`、`go test -race ./...`（Linux CI）；记录最终 PR Head 三项 CI 真实结论，不把初期 green 当作末次 green。
- **MySQL 状态**：M3-2 已实现真实 8.0.46 CLI 严格 Validate、触发器探测与 FK orphan 检查；恢复仍不支持。verify 包自身仍以 fake Reader 为主，真实 SQLite/MySQL 路径由 `cmd/maskriver` 与 `internal/runner` 覆盖。

## 特别安全门禁

- 标准输出、错误、日志、报告、Agent 会话均不得出现原始值、敏感主键、DSN、seed/salt。
- Dry Run 用文件哈希快照与连接 spy 证明零持久写入，不能只断言 `rows_written=0`。
- 验证基于脱敏前的快照与固定 Plan；覆盖不完整不得 PASS。字段值发生变化不能证明其中没有 PII。
- 注入网络中断、`context` 取消、写入冲突、磁盘或权限错误，并明确已提交批次与未完成范围。
- 映射存储与目标不共享事务时，必须测试提交间隙，且不得提前承诺恢复或幂等。

## 第二阶段质量与性能

`go test -race ./...`；对规则、编码与格式函数做 Go fuzz；用合成序列做故障恢复测试；检查 goroutine 与连接泄漏及有界内存。
`go test -bench . -benchmem` 必须固定 Go/驱动/数据库版本、CPU、行数与分布、batch/workers，多次重复并报告统计分布，而不是最佳值。
对比正确性与吞吐/延迟/RSS/CPU/分配；检测能力以标注集按规则报告 precision/recall/F1。可视化只使用合成数据与汇总指标。

不设置虚构覆盖率或吞吐成绩。不使用生产数据；临时文件使用 `t.TempDir`，MySQL 使用隔离测试实例。

## #9 Runner 授权与故障回归（2026-10-18）

`internal/runner/workflow_acceptance_test.go` 使用真实 SQLite source/target，包装现有 Writer/WriteTx 接口作实例级能力与事务 Spy：

- 未 Approved、无 ExplicitApply、篡改 Plan、Preview 目标不一致：不打开 Writer。
- Preview：读取全部计划行，fixture 目录（source/target/key/sentinel）文件集合及 SHA256 不变，Writer/Begin/Update/Commit 为零；驱动级 SQL Spy 回归（`TestPreviewIssuesNoWriteSQLThroughSQLSpy`、`TestMultiTablePreviewIssuesNoWriteSQLThroughSQLSpy`、`TestSQLSpyReadOnlyOperationsIssueNoWrites`）另行确认实际 SQL 无写/事务语句。
- 混合 NULL 跨批次只更新非 NULL，NULL 保持；130 行全 NULL 跨页不打开 Writer。Preview 计读取行，receipt 只计实际更新行。
- Begin/Update/受影响行数不符/取消：错误返回，SQL 回读无未提交变化，Writer 关闭；Rollback 使用未取消、有 deadline 的独立清理 context。驱动可能不遵守该 deadline，不承诺硬超时。
- 第二批失败保留首批 receipt，SQL 回读确认已提交前缀；包装 ErrCommitUnknown 可识别，不自动重试或声称回滚。真实 SQLite 提交后模拟确认丢失，证明实际写入与保守报告的区别。

命令：`go test -count=1 -v ./internal/runner`、`go test -count=1 ./...`、`go vet ./...`、`go test -race ./...`。本机 Linux/Go 1.26.3 已执行；MySQL 无隔离 socket 时仍 SKIP，不能以本包替代 MySQL CLI 或完整 #11 验收。2026-10-18 的早期记录见 [Runner 收尾证据](m1-runner-acceptance.md)；其中若干边界已被后续提交改变（CHECK 元数据、多表、SQL Spy、审批记录），以本文件和当前代码为准。

## #9 真实 SQLite CLI 与签名审批（当前证据）

以下路径已在真实 SQLite 合成快照上端到端执行；证据以测试名为准，不复制旧文档结论。

- **工作流**：`TestCLIRealSQLiteWorkflow`（单表 25 行、串行/并行结果一致、重复 Apply 拒绝）、`TestCLIMultiTableWorkflow`（两表两个 scope 的 scan→preview→apply→strict validate）、`TestE2EPlanIssueNewProcessApply`（preview → 独立子进程 approve → 新进程 preview/apply → 新进程 strict validate）、`TestE2ETwoTableDecisionCoversWholePlan`。
- **审批绑定**：`approve` 用 Ed25519 私钥签发记录，记录绑定 plan_id、source_snapshot_id、target_id、schema_digest、config_digest、strategy/version、key_id 与有序 scope 列表（`newRecord`、`checkBinding`）。`--approved` 只是声明，`--apply` 才是写意图，两者都必需且都不充分；还需要 `history.Store.Lookup` 验证签名、绑定、有效期与撤销后返回记录（`TestE2EInvalidDecisionsRefused`）。
- **整计划语义**：两表计划必须整体审批；仅批一张表不能授权整计划（`TestE2EPartialDecisionDoesNotAuthorizeWholePlan`）。换序、换策略、改 schema、改源快照、改目标绑定都会生成不同 Plan，原记录因此失效（`TestE2EDecisionBoundFieldChangesRefused`；存储层对应 `TestLookupRejectsScopeReorder`、`TestLookupRejectsChangedBoundField`、`TestLookupRejectsPartialMultiTableApproval`）。
- **信任边界**：`apply` 只加载部署预置 trust file 中的公钥（`history.LoadTrustFile`），路径上不接收审核私钥；`TestApplyPathTakesNoReviewerPrivateKey` 固定这一接口面，`TestOpenRejectsPrivateKeyThatDoesNotMatchTrustedKey` 拒绝私钥与信任项不匹配。同一操作者同时控制 trust file 与审核私钥时工具不阻止，`ReviewerID` 只是自填标签；这是本地签名边界，不是 RBAC。
- **`null` 策略**：`Engine.maskColumn` 在 dryRun 中拒绝非 NULL → NULL，Preview 与 Apply 都在打开 Writer 前失败（`TestCLINullStrategyRefusedBeforeWrite`、`TestApplyRefusesNullReplacementBeforeWriter`）。`internal/db` 写入层仍接受该组合（P2-1）。
- **静态门禁**：UNKNOWN 扫描在 Plan 阶段 `ErrUnsafe`；无稳定主键或覆盖不全 `ErrIncomplete`；被脱敏列位于 UNIQUE `ErrUnsafe`、CHECK/FK/被引用 FK/任意触发器 `ErrUnsupported`；非 rollback-journal 源 `ErrIncomplete`（WAL 已由 `TestWALSourceIsRefusedBySnapshotIdentity` 覆盖；MEMORY/OFF 按同一代码路径拒绝，无专项 CLI 用例）；未知策略 `ErrInvalid`；调用方提供的 Plan 与重新计算的 Plan 不一致（快照/schema/策略）`ErrConflict`；截断扫描 `ErrIncomplete`；严格验证覆盖不足（`SchemaCoverage` 缺失、`PartialCoverage`）以 `Warning` + `Coverage.Complete=false` 退出 1。证据：`TestPlanRefusesNonSensitiveColumnEvenWhenApproved`、`TestPreflightRefusesTriggerOnPlannedTable`、`TestPreflightRefusesUniqueConstraintOnModifiedColumn`、`TestPreflightRefusesCheckOnModifiedColumn`、`TestPreflightRefusesForeignKeyOnModifiedColumn`、`TestPreflightRefusesIncomingReferenceFromUnplannedTable`、`TestE2EApprovalDoesNotBypassExistingGates`。
- **批次语义**：`BatchReceipt` 只在 Commit 成功后追加；失败保留已提交前缀但 `Complete=false`；`ErrCommitUnknown` 标记 `CommitOutcomeUnknown`，不重试也不声称回滚；没有跨批全局原子性；表在运行中被清空使前后 Count 不一致 → `ErrConflict`。证据：`TestApplyRetainsReceiptForEarlierCommittedBatch`、`TestApplyWrappedCommitUnknownIsNotReportedAsRolledBack`、`TestApplyRefusesTableEmptiedBeforeItsTurn`。
- **`--strict` 现状**：`verify` 忽略 `ValidateRequest.Strict`；所有 Warning 路径已把 `Coverage.Complete` 置 false，因此 `contracts.Passed` 无论 strict 都不接受这些报告。CLI 测试只证明 flag 解析、转发（`validationRequest`）与退出码敏感性（`TestCLIStrictFlagWiring`），以及 `Passed` 对 Warning 的判定（`TestValidationPassedHonorsStrictWarning`）；没有数据库 fixture 能区分 strict 与非 strict。不得把它描述为已具备独立严格语义门禁。

## 真实支持与仅组件级证据

- **真实 SQLite CLI 端到端**：上节列出的 `cmd/maskriver` 用例，以及 `internal/runner` 中真实 SQLite + Writer/WriteTx 包装 spy 的多表/故障用例。
- **真实 SQLite 组件级**：`internal/db` 的 Reader/Writer、分页、事务与驱动级 SQL Spy；`internal/history` 的真实文件存储（不连接数据库）；`internal/mask`、`internal/detect` 为纯内存组件；同一表多列的 Plan/Preview/Apply 由 `TestPreviewCoversEveryColumnOfOneTable`、`TestApplyWritesEveryPlannedColumnOfOneTable` 覆盖，不是 CLI 级证据。
- **fake Reader / 仅适配器**：`internal/verify` 的 schema/行数/keyset 回归使用 fake Reader；MySQL 只有 `TestMySQLAdapter` 适配器组件测试，没有 MySQL CLI。
- 组件测试通过不等于用户 CLI 路径或生产可用；MySQL CLI 与完整 #11 独立验收不在已测范围。

## Issue #14 本地事务恢复（m2-recovery-v1）

- **协议与边界**：[恢复协议](design/issue14-recovery-protocol.md)。仅 Linux amd64 本地 SQLite、rollback journal、session `synchronous=FULL`；manifest + append-only progress + 同事务 checkpoint；显式 `init-recovery`/`--resume`/`status`。
- **DB 证据**（`internal/db/recovery_test.go`）：schema 安装/重复安装/版本不符、同事务 UPDATE+receipt+progress+CAS、checkpoint 失败回滚业务、no-op progress、重复/跳号、CAS 过期、terminal state 不可变、真实 Commit 后响应丢失、tampered progress 可见不修复、legacy outbox 不可恢复、shape 篡改拒绝、read-only reader 文件哈希不变。
- **Runner 证据**（`internal/runner/recovery_test.go`）：1/多批中断、跨表、全 NULL 页、Commit 前崩溃回滚、真实 Commit 后进程死亡、commit_unknown 重核、重复 resume 幂等、目标外部修改、manifest 领先日志、source/settings/approval 变化拒绝、错误 key 拒绝、verification 后写入前外部修改拒绝、Validate 中断后重验、validation_failed 不得 complete。
- **真实 CLI 子进程证据**（`cmd/maskriver/recovery_e2e_test.go`）：第 1/10/100/500 批后进程退出 → 新进程 `status`（只读）→ `--resume` → strict `validate` → 再次 resume；与不中断基线的排序脱敏邮箱列表一致；`before_commit` 与 `after_validate` 中断恢复；无 `--apply`/无审批/撤销审批/坏 run id/不存在 run 的退出码。
- **规模证据**（opt-in，`cmd/maskriver/recovery_scale_test.go`）：10k/100k/1M 真实 CLI，正常执行与“半程中断 + resume”对比；记录 apply/resume/validate 耗时、Peak RSS、rows/batches/checkpoint/receipt、目标大小与 A/B 摘要一致。
- **命令**：`gofmt -l cmd internal pkg tools`、`go vet ./...`、`go test -count=1 ./...`、`go test -race -count=1 ./...`、`GOOS=windows/darwin go build ./...`，以及 `MASKRIVER_ISSUE14_SCALE=1 MASKRIVER_ISSUE14_SCALE_DIR=<abs> MASKRIVER_ISSUE14_ROWS=10000,100000,1000000 go test -run TestCLIRecoveryScaleEvidence -v ./cmd/maskriver`。
- **明确不支持**：MySQL/Windows/WAL 恢复、跨库/持久映射的跨存储原子性、通用分布式 exactly-once、真实断电 durability、TOCTOU-proof 文件绑定、无 `--force` 冲突修复、approval 运行期持续复核（仅在 resume 开始时检查）。

## UNVERIFIED 清单

- MySQL CLI 端到端：已由 M3-2/PR #93 在隔离 MySQL 8.0.46 完成；其他 MySQL 版本未验证。
- MySQL/Windows/WAL 恢复、跨存储 mapping commit gap 的恢复、通用 exactly-once：不支持；不得从本地 SQLite 恢复能力推断。
- 生产级检测准确率：只有 74 条自编样本（38 正、36 负，8 条超出覆盖），姓名 P=0.500/R=0.750；不是独立留出评估。
- 持久映射、全局唯一性/匿名化、参照完整性：未实现/未验证。
- macOS CI：仓库 CI 跑 Ubuntu/Windows/macOS 非流式 M1；macOS/Windows streaming/recovery 仍不支持。
- `--strict` 的 MySQL 严格完整性路径：M3-2 已实现 `CHECK TABLE ... QUICK` + 可见 FK orphan 检查；其他 MySQL 版本与全服务器一致性仍未验证。

## 已知残余缺陷（已接受 P2）

以下问题已记录并接受为 P2，不阻塞 #9 的 AC 文档收口；均需后续 Issue 关闭（编号待登记，预计在 #9 收尾后另立）。

| 编号 | 位置/现象 | 影响 | 限制/现状 | 后续 |
|---|---|---|---|---|
| P2-1 | `internal/db` 写入层仍接受非 NULL→NULL | 绕过 CLI 直接调用 Writer 的调用者可能写入永远无法通过严格验证的行；CLI 路径已在上游拒绝 | `TestApplyRefusesNullReplacementBeforeWriter` 只固定 CLI/Runner 行为 | 后续 Issue（编号待登记） |
| P2-2 | 部分扫描拒绝测试的 `spy.opens != 0` 断言无法失败 | `TestPlanRefusesNonSensitiveColumnEvenWhenApproved`、`TestPlanRefusesTruncatedSampleEvenWhenApproved` 在 Plan 失败后仍用空 Plan 调 Apply，Apply 先做形状校验就返回，Writer 计数与扫描闸门无关，不能捕获闸门回归 | 扫描本身结构上不打开 Writer，产品行为未受影响 | 后续 Issue（编号待登记） |
| P2-3 | 取消测试未覆盖 `gather` 的 ctx 检查 | `TestScanFailsClosedOnMidScanCancellation` 在第二次 `Sample` 内触发并由 Reader 返回 `context.Canceled`；`gather` 循环顶部的 `ctx.Err()` 短路没有单独用例 | 已测取消路径仍有效；取消传播证据不完整 | 后续 Issue（编号待登记） |
| P2-4 | `main.go` 为测试 seam 引入两个可变包级函数变量 | `TestCLIStrictFlagWiring` 依赖替换 `validationRequest`/`validationPassed`，测试修改全局状态，不能并行 | 生产行为不受影响，只影响测试隔离 | 后续 Issue（编号待登记） |
| P2-5 | CLI e2e 的 `targetDigest` 忽略 sidecar | 只哈希目标主文件；WAL 目标可能把提交写入 `-wal` 而字节断言不可见 | 当前 fixture 使用 rollback journal；源端 WAL 被拒绝，目标端 journal mode 未校验 | 后续 Issue（编号待登记） |
| P2-6 | 历史存储不检测整体回滚/撤销回滚 | 能写 store 目录者可恢复旧的有效审批或撤销前文件；过期仍限制窗口 | 本地文件存储不是透明日志；部署需外发记录摘要/过期信息 | 后续 Issue（编号待登记） |
| P2-7 | typed-nil seam 防护不完整 | runner 的 `ApprovalQuery` 已用 `nilApprovalQuery` 防护；`internal/db` 的注入式连接 seam 只有 `nil` 判断，没有等价的 typed-nil 回归 | 影响限于测试/自定义注入路径；生产使用 `nil` 工厂 | 后续 Issue（编号待登记） |
| P2-8 | Runner 侧审批时间窗口没有时钟偏移容差 | `internal/runner/approval.go` 要求 `IssuedAt <= now`，而 `internal/history/record.go` 允许 5 分钟未来签发容差；若审核方时钟略快，刚签发的记录在 Runner 复核处被拒绝 | fail-closed，不会误授权写入；仅在时钟轻微漂移时降低可用性（需要等待或重签） | 后续 Issue（编号待登记）；届时统一两处容差语义 |
