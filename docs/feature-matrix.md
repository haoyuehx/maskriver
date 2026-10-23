# Feature Matrix

MaskRiver 是一个独立研发的 Go 数据脱敏与敏感信息治理工具，面向企业开发、测试与数据共享场景。
本矩阵只描述 **MaskRiver 自身** 的需求范围与实现状态。

状态定义：

- **已实现**：有实际代码并有可复现的测试或构建证据。
- **部分实现**：有最小行为，但不具备完整业务能力。
- **计划中**：已排入 M1/M2 里程碑，但尚无业务实现。
- **未实现**：未纳入确定里程碑，需另行决策。

里程碑：**M1** = 可用核心（检测 → 脱敏 → 校验 + SQLite/MySQL）；**M2** = 流式、恢复与中文敏感数据治理。

## 当前准备状态（不是业务功能）

| 项目 | 状态 | 证据 / 边界 |
|---|---|---|
| 共享 DTO/接口 `m1a-v1` | 已实现 | `pkg/contracts` + 契约测试；不是完整引擎 |
| `database/sql` 驱动版本锁定 | 已实现 | `go.mod`/`go.sum`、database-drivers.md |
| SQLite 合成环境 | 已实现 | `internal/testenv`、`tools/fixture`；业务 Reader/Writer、分页和事务测试由 #5/PR #21 交付，是否通过以最终 CI 为准 |
| MySQL 隔离环境 | 已实现（隔离测试 + 真实 CLI） | 固定 MySQL 8.0.46 私有 socket 容器执行 TestMySQLFixture、TestMySQLAdapter 与真实 TestMySQLCLIWorkflow；其他 MySQL 版本仍未验证 |
| 可选工作树工具 | 已实现（历史单机） | `scripts/check-native-worktree.mjs` 的旧预检，不是跨平台开发前置条件 |
| 四人模块任务定义 | 已实现（文档） | parallel-tasks.md；独立克隆/分支/PR，业务组件仍待实现 |
| 跨平台 CI | 已实现（非流式 M1） | Ubuntu/Windows/macOS 测试、格式、vet；Ubuntu race/MySQL fixture；Linux 交叉构建 darwin/windows/linux-arm64。streaming/recovery 仍仅 Linux |

## 数据库接入

| 功能 | 说明 / 范围 | 状态 | 目标 |
|---|---|---|---|
| 独立 Go module、可编译 CLI help/version | 无框架依赖，标准库 CLI | 已实现 | M0 |
| `scan` / `mask` / `validate` / `approve` / `revoke` CLI | SQLite 多表真实路径（同一表多列由 runner 级用例覆盖）：Detect→Preview→独立进程签发审批记录→新进程 Apply→Validate；证据见 `TestCLIMultiTableWorkflow`、`TestE2EPlanIssueNewProcessApply`。M3-2 增加 MySQL 8.0.46 真实 CLI 路径：两个 DSN、共享逻辑 schema、内容摘要计划绑定，证据见 `TestMySQLCLIWorkflow`；MySQL streaming/recovery 明确拒绝 | 已实现（SQLite + MySQL 8.0.46 CLI；MySQL recovery 不支持） | #9 / #86 |
| 默认 Dry Run，`--apply` 为写意图（需与声明、记录三者同时成立） | 写入需 `--approved` 声明 + `--apply` 意图 + 有效签名记录，三者缺一即拒绝；Preview 由 Writer 能力 Spy（opens/begins/updates/commits 全为 0）、驱动级 SQL Spy（`internal/db/sqlspy.go`，无写/事务语句）与文件树 SHA256 共同证明零写入；不证明并发快照 | 部分实现（SQLite 路径） | #9/#11 |
| 配置文件、环境变量展开、参数校验 | 连接/检测/脱敏/验证分段校验 | 计划中 | M1 |
| 连接生命周期、schema/table/column/type/PK 元数据 | SQLite/MySQL Reader/Writer，元数据与 SchemaCoverage 保守声明；不支持或可能丢精度的映射显式拒绝 | 部分实现（#5 组件） | #5 / PR #21；MySQL 适配器见 #10 / PR #30 |
| distinct 非空抽样、表/列过滤 | typed、非 NULL distinct 有界抽样；按 DataSetID/Schema/TableRef 限定访问范围 | 部分实现（#5 组件） | #5 / PR #21 |
| SQLite 连接与真实集成 | SQLite 真实驱动执行 Reader/Writer、只读、类型、复合键分页和安全拒绝测试；以 PR #21 最终 CI 为证据 | 部分实现（#5 组件） | #5 / PR #21 |
| MySQL 连接与真实集成 | TestMySQLAdapter 与真实 TestMySQLCLIWorkflow 在一次性 MySQL 8.0.46 实例通过；包含 CN 策略的 typed SQL 写入/读回；其他 MySQL 版本未验收 | 已实现（适配器 + MySQL 8.0.46 CLI） | #10 / #86 |
| MariaDB / PostgreSQL / SQL Server / Oracle | 需独立适配与独立验证 | 未实现 | 后续评估 |
| SQLite 流式回执 Outbox（WP2，DB 层能力） | `internal/db` 显式 opt-in：预置版本化 outbox schema、同库同事务业务 UPDATE + receipt INSERT、有界 keyset 分页读取、CommitUnknown 保守分类、Linux 本地 SQLite（journal=DELETE，会话 synchronous=FULL）；已接入 Runner/CLI（#13）并被 #14 recovery 复用；未验证断电 durability/MySQL/Windows streaming | 已实现（Linux amd64 本地 SQLite） | #13 / #14 |
| 参数化更新、批次事务、复合键 keyset 分页 | #5 提供按真实复合主键游标、参数绑定、乐观受影响行数检查与回滚；跨方言独立验证持续进行 | 部分实现（#5 组件） | #5 / PR #21；MySQL 适配器见 #10 / PR #30 |
| SQLite 本地事务恢复 schema（manifest/progress/KCV） | 显式 `init-recovery` 安装 `maskriver-recovery-v1`；manifest 与 append-only progress 只含 ID、摘要、计数、状态与时间；业务 UPDATE + receipt + progress + manifest CAS 同事务；无-op 页面只推进 progress；旧 outbox v1 数据/形状不变；部分或错误版本 fail closed | 已实现（Linux amd64 本地 SQLite） | #14 |
| CLI `status` 与显式 `--resume` | `status --target --run-id` 以 SQLite `mode=ro` 打开，显示 state/checkpoint/已处理/已提交/批次数/表位置/resume_eligible/commit_unknown/conflict，零写入；`mask --streaming --apply --resume --run-id` 复用真实 Ed25519 审批与静态 preflight，已提交批次不重写；退出码区分 complete/recoverable/conflict/commit_unknown/unsupported/invalid/unauthorized/validation_failed | 已实现（Linux amd64 本地 SQLite） | #14 |
| 无主键 apply 拒绝、敏感主键明确未处理 | 数据库 Writer 已对无主键及试图改主键的计划拒绝；CLI 端到端拒绝路径已由 `TestE2EApprovalDoesNotBypassExistingGates` 覆盖；MySQL CLI 复用同一 runner 门禁 | 已实现（组件 + SQLite/MySQL 8.0.46 CLI 拒绝） | #5 / #9 / #86 |

## 敏感信息检测

China-first 的 13 种检测规则由 PR #20（Issue #6）实现；其规则是有界、上下文敏感的组件能力，**并不代表全国 PII 全覆盖或 CLI/数据库端到端扫描已经交付**。以下“已实现”仅表示存在对应 Go 代码和合成测试，不代表生产环境准确率或安全批准。#12 的九种**纯内存有界格式约束替换**见 PR #29；这些 CN 策略的 CLI 端到端 Apply、schema/碰撞处理和独立验证尚未完成。

| 功能 | 说明 / 范围 | 状态 | 目标 |
|---|---|---|---|
| 人工覆盖、已审核结果与显式 Skip | 精确合法 Override > Reviewed > Skip > Rules；Skip 不代表验证为非敏感 | 已实现（组件） | #6 / PR #20 |
| 规则证据与三态判断 | DISTINCT 非空样本、默认最小 20 样本与 90% 阈值、冲突/不足返回 UNKNOWN | 已实现（组件） | #6 / PR #20 |
| email | ASCII dot-atom 子集，域名/IP 字面值结构校验 | 已实现（有界） | #6 / PR #20 |
| url | http(s) 绝对 URL；有效 host/IP 和端口 | 已实现（有界） | #6 / PR #20 |
| ip_address | IPv4 和 IPv6，暂不覆盖带 zone 的形式 | 已实现（有界） | #6 / PR #20 |
| uuid | RFC variant 的 UUID v1–v5；不覆盖新版本 | 已实现（有界） | #6 / PR #20 |
| cn_id_card | 中国大陆 18 位身份证；生日、有限行政区划代码和 MOD11-2 校验 | 已实现（有限区划） | #6 / PR #20 |
| cn_mobile | 有限版本号段；中国大陆手机格式，需字段上下文 | 已实现（有界） | #6 / PR #20 |
| cn_bank_card | 16–19 位、Luhn、字段上下文；不识别真实发卡行 | 已实现（有界） | #6 / PR #20 |
| cn_postal_code | 六位格式及邮编列上下文；不验证真实投递区域 | 已实现（有界） | #6 / PR #20 |
| cn_uscc | 18 位统一社会信用代码，有限登记类别/区划与 MOD31 校验 | 已实现（有限区划） | #6 / PR #20 |
| cn_city | 自建有限城市/省份别名映射，中文与拼音，需字段上下文 | 已实现（有限词表） | #6 / PR #20 |
| cn_person_name | 常见单姓、复姓及汉字形状，需姓名字段上下文；误报显著 | 部分实现（实验性） | #6 / PR #20 |
| cn_address | 有序中文地址片段、道路/门牌标记，需字段上下文 | 已实现（有界） | #6 / PR #20 |
| cn_date | 合法年-月-日、年/月/日、中文年月日和 typed Date；不识别自由文本及 MDY/DMY 字符串 | 已实现（有界） | #6 / PR #20 |
| UNKNOWN 不等于非敏感 | 样本不足、覆盖外、冲突或不确定不得自动授权写入 | 已实现（检测组件） | #6 / PR #20 |
| 扫描错误 fail-closed 与非零退出 | `runner.Scan` 在 Describe/Sample 故障与中途取消时整体失败；CLI 扫描不完整退出 1 并输出脱敏拒绝信息；证据见 `TestScanFailsClosedOnEvidenceSeamError`、`TestCLIScanFailureExitsNonZeroWithoutWrite`；MySQL CLI 复用同一 fail-closed 路径，真实 8.0.46 作业执行 | 已实现（SQLite + MySQL 8.0.46 CLI） | #9 / #86 |
| 可选外部/本地 LLM 辅助分类 | 默认关闭；不得外发样本 | 未实现 | 不在 M1/M2 承诺内 |

**评估证据与局限**：`internal/detect/eval_test.go` 使用 74 条自编合成 `(value, column)` 样本（38 正、36 负；8 条超出既定覆盖范围），以 `MinSamples=1` 独立检验规则/列上下文，**并非生产默认 20 样本的整列检测效果，也不是独立留出测试**。本次测得 `cn_person_name` TP=3、FP=3、FN=1，Precision=0.500、Recall=0.750、F1=0.600；包含普通词误报。部署前必须扩展独立标注数据、开展人工审核，不得默认用它批准 Apply。身份证/信用代码格式校验不是身份或登记真实性验证。规则列表不意味着覆盖一切行政区划、号码段、姓名和地址。原北美 SSN/US ZIP/NANP 等规则不属于本次 China-first 组件交付。

## 脱敏策略

以下为 MaskRiver 的策略目录。首批五种 M1 策略和并发安全内存映射由 PR #24 交付；#12 的九种中国格式替换由 PR #29 提供，全部属于**纯内存组件能力**，不意味着 CLI Apply、持久映射或身份真实性已被验证。

| 策略 | 说明 | 状态 | 目标 |
|---|---|---|---|
| null | 显式生成 SQL NULL；CLI 端到端刻意拒绝：dryRun 在 openWriter 前拒绝“非 NULL → NULL”，Preview 与 Apply 均拒绝，因该变化会被验证器判为冲突证据（`TestCLINullStrategyRefusedBeforeWrite`、`TestApplyRefusesNullReplacementBeforeWriter`）；M3-1 起 `internal/db` 写入层也拒绝该组合（`ErrUnsafe`） | 部分实现（组件；CLI 拒绝） | #7 / PR #24 |
| blank | Text/Bytes 清空；拒绝不支持的数值/日期类型 | 部分实现（组件） | #7 / PR #24 |
| redact | 支持文本、字节和有限基本数值类型；结果未变化时拒绝 | 部分实现（组件） | #7 / PR #24 |
| format_random | HMAC 派生的 Text ASCII/汉字有界类别替换与 Bytes 等长变换；未支持脚本拒绝。非密码学 FPE | 部分实现（有界组件） | #7 / PR #24 |
| fake_email | HMAC 派生本地部分与固定 `.local` 域；不暴露原始值 CRC32，不声称真实邮箱可投递 | 部分实现（组件） | #7 / PR #24 |
| cn_id_card | 限定区划+日期+序号+MOD11-2 校验位 | 部分实现（有界组件） | #12 / PR #29 |
| cn_mobile | 已列明号段，保持可选 +86/86 表示方式；不保证未分配 | 部分实现（有界组件） | #12 / PR #29 |
| cn_bank_card | 16–19 位、Luhn 有效；不保证 BIN/真实卡号安全 | 部分实现（有界组件） | #12 / PR #29 |
| cn_uscc | 限定类别/地区 + MOD31 校验；不保证真实登记 | 部分实现（有界组件） | #12 / PR #29 |
| cn_postal_code | 6 位 ASCII 数字；不验证邮政投递地区 | 部分实现（有界组件） | #12 / PR #29 |
| cn_city | 7 个城市的有界汉字/拼音词表，保留部分后缀 | 部分实现（有界组件） | #12 / PR #29 |
| cn_person_name | 2–4 字、有界姓氏与复姓；不保证姓名真实语义 | 部分实现（实验性组件） | #12 / PR #29 |
| cn_address | 汉字街道/门牌片段，替换为明确合成地址；不保证投递有效 | 部分实现（有界组件） | #12 / PR #29 |
| cn_date | 年优先合法公历文字或 typed Date，保留格式 | 部分实现（有界组件） | #12 / PR #29 |
| shuffle | 字符重排，typed 值专用分支 | 计划中 | 后续/go-mask |
| fake_name | 姓名合成 | 计划中 | 后续/go-mask |
| fake_first_name | 名合成 | 计划中 | 后续/go-mask |
| fake_last_name | 姓合成 | 计划中 | 后续/go-mask |
| fake_city | 城市合成 | 计划中 | 后续/go-mask |
| fake_email_keep_domain | 保留原域（可能可识别，须显式选择） | 计划中 | 后续/go-mask |
| fake_uuid | 确定性 UUID | 计划中 | 后续/go-mask |
| fake_ip | 合法 IP 输出 | 计划中 | 后续/go-mask |
| fake_credit_card | 品牌/长度/分隔符与校验位有效 | 计划中 | 后续/go-mask |
| fake_date | 合法日历日期偏移 | 计划中 | 后续/go-mask |
| fake_phone | 受约束号码格式 | 计划中 | 后续/go-mask |
| fake_ssn | 受约束编号格式 | 计划中 | 后续/go-mask |

M1 首批切片（`null` / `blank` / `redact` / `format_random` / `fake_email`）之外的策略必须在各自验收通过后才可升级状态。PR #24 对应原作者 PR #18；真实 CI/验收以最终合并记录为准。


**#12 合成评估（仅针对策略输入的格式接受/拒绝）**：`internal/mask/cn_format_test.go` 的开发时手工合成数据共 21 个正例、28 个负例；在 PR #29 初轮 CI 上九条规则 TP 分别为 2/3/2/1/2/3/3/2/3，全部 FP=0、FN=0。每条在这个**非独立、很小且人为构造**的数据集上 Precision=Recall=1.000。不可推断生产识别准确率、匿名化强度、未分配号码或真实数据库约束兼容。详细范围、算法、负例与剩余工作见 [#12 组件说明](issue12-cn-replacement.md)。

## 确定性、映射与安全

| 功能 | 说明 / 边界 | 状态 | 目标 |
|---|---|---|---|
| 策略优先级与未知策略拒绝 | 审核结论 > 列级 > 规则级 > 默认；未知即失败 | 计划中 | M1/go-mask |
| typed 输出、未变化结果拒绝 | 首批纯策略针对合法 Kind 输出或 fail-closed，非空未变化拒绝；目标 Schema 约束由 runner preflight 拒绝 UNIQUE/CHECK/FK/触发器等冲突，并由 verify 复核 | 部分实现（组件 + runner 门禁） | #7 / PR #24、#9 |
| 显式列级 NULL 标记 | 仅精确列生效，先于映射，不写映射 | 计划中 | M1/go-mask |
| 策略/词典注册与来源审核 | 新词典必须单独审核来源与许可 | 计划中 | M1/go-mask |
| 确定性：同输入同 scope 同输出 | HMAC 绑定 typed 输入/策略 ID+Version/scope/normalization/keyID；不承诺无碰撞或第三方兼容 | 部分实现（组件） | #7 / PR #24 |
| 并发安全内存映射 | Lookup 和 atomic GetOrCreate；映射键为加密指纹，测试含并发/取消；不持久化 | 部分实现（内存组件） | #7 / PR #24 |
| 持久映射：加盐指纹、不存原值、可复用 | 持久加盐指纹映射、加密存储及恢复仍无实现 | 计划中 | 后续 M1/Main |
| Dry Run 零持久副作用 | 拆成三项分别成立的事实，不合并成一句承诺：① 驱动级 SQL Spy 观察到无写/事务语句（`TestPreviewIssuesNoWriteSQLThroughSQLSpy`、`TestSQLSpyReadOnlyOperationsIssueNoWrites`）；② 主数据库文件执行前后字节不变（`source.db`/`target.db` 的 SHA-256 相同）；③ 当次 fixture（rollback-journal）执行前后文件树未新增文件。**F1**：被拒绝的 WAL 源仍可能留下 sidecar——对 `journal_mode=WAL` 的源执行 `mask`，在 Plan 阶段被快照身份闸门拒绝（退出 1）后 `source.db` 未变，但仍新增 `-shm`（32768 字节）与空的 `-wal`（0 字节），目标端同样。因此不对 WAL 源作“零文件变化”的无条件承诺，③ 只对当次实测文件集成立；三项都不等于并发快照保证 | 已实现（SQLite 证据） | #9/#11 |
| 默认隐藏预览值 | 原值显示开关须单独评审，不默认实现 | 计划中 | M1/Main |
| 审核历史：签名审批、审核人标签、过期、撤销 | Ed25519 记录绑定 Plan、digest、strategy/key_id 与有序 scope；过期、未来签发、撤销、同 plan 多记录均拒绝（`internal/history/*_test.go`、`TestE2EDecisionBoundFieldChangesRefused`）；`ReviewerID` 是自填标签；无 CSV、导入导出与防回滚 | 部分实现（本地签名存储） | #9/#11 |
| 历史导入/导出、回写与冲突检测 | 回写须备份与冲突检查 | 未实现 | 后续增量 |
| 结构化报告与退出码 | CLI 退出码 0/1/2 与脱敏计数摘要已测（`TestCLIArguments`、`TestCLIApplySummaryIsSanitized`）；无 JSON/CSV 结构化报告 | 部分实现（退出码） | M1/Main |

## 完整性校验

**#8 验证组件边界**：`verify.New()` 只读取两个 `contracts.Reader`，不读取配置或原始密钥、不自动写入，报告不包含明文主键和值。PR #32 的原始实现经 PR #33 补强为每边最多 256 行的有界内存双向 keyset 校验。verify 包自身的回归仍以 fake Reader 为主；真实 SQLite 多表路径由 `cmd/maskriver` 与 `internal/runner` 覆盖；MySQL 8.0.46 CLI 已由 #86 验收。对排序语义无法安全对齐的键显式拒绝。


| 功能 | 说明 / 边界 | 状态 | 目标 |
|---|---|---|---|
| 行数校验 | Count 对比，双向键集逐项核对，数量不一致失败；fake Reader 组件与真实 SQLite 多表 CLI 已测，M3-2 在真实 MySQL 8.0.46 CLI 中通过 | 已实现（组件 + SQLite/MySQL CLI） | #8 / #86 |
| schema 校验：列类型/PK/索引/FK/unique/check | `schemasMatch` 比较列名/类型/NULL/长度/精度/比例、PK、索引、unique、FK、CHECK，并要求对应 `SchemaCoverage`；`schema_coverage_test.go` 覆盖，真实 SQLite CLI 已验证含 CHECK/索引的表；触发器不在 verify 内，由 runner preflight 拒绝 | 已实现（组件 + SQLite CLI） | #8 / PR #33 |
| triggers/grants 校验 | runner preflight 拒绝任何触发器与入向 FK；verify 不做触发器校验，grants 需驱动能力支持 | 部分实现（触发器拒绝） | 后续评估 |
| 主键对齐逐值完整性校验 | 基于冻结 Plan，双向 keyset 流式合并，已选敏感列未变化失败，字段缺失/类型/NULL 转换失败 | 已实现（fake Reader 组件） | #8 / PR #32、#33 |
| 无键/限量覆盖/strict 模式 | RowLimit=0 有界分页全量；正数为总行预算，截断/无主键证据/分页异常不得 PASS。M3-1 起 strict 要求 `RowLimit=0`、拒绝 Warning，并要求每张计划表在源/目标通过适配器 `IntegrityCheck`；SQLite 已实现 `quick_check`+数据库级 `foreign_key_check`；M3-2 的 MySQL 实现要求 `CHECK TABLE` 每行均为 `status/OK`，并对触及计划表的每个 FK 执行 orphan 查询，任一失败即 fail closed。证据：`TestStrictRequiresFullScanAndIntegrityCapability`、`TestStrictIntegrityFailureIsNotPass`、`TestSQLiteIntegrityCheckDetectsForeignKeyViolation`、`TestMySQLCLIWorkflow` | 已实现（SQLite + MySQL 8.0.46 严格语义） | #8 / #33；#86 |
| 测试数据忽略、采样上限、形状遮蔽 | 忽略策略须显式审核 | 计划中 | M1/go-verify |
| 稳定公共库 API | `pkg/contracts` 仅内部协作冻结，非稳定外部 SDK | 未实现 | 待契约稳定后评估 |

## M2 研发目标

| 功能 | 状态 | 目标 |
|---|---|---|
| 中国 PII 检测与格式约束脱敏 | 部分实现（检测 + 有界替换 + SQL 集成测试） | 检测见 #6/PR #20，九种策略见 PR #29，真实 SQLite/MySQL 8.0.46 组件级读/写/回读与只读快照测试见 PR #30；用户 CLI 工作流与独立安全评审仍属 #9/#11 |
| SQLite 同事务流式 Apply + M2 审批 + CLI | 已实现并验收（Linux amd64 SQLite；DB 层物化前字节预检、Runner dry-run/apply/validate 统一受 MaxRowBytes 约束；ADR-08 已按 delegated technical direction 批准，生产默认仍显式 opt-in） | 显式 `--streaming --apply` + 独立 `--streaming` Ed25519 审批 + canonical plan 绑定 + WP2 outbox 同事务回执 + 有界分页 + 独立 Validate；Issue #14 增加可选 recovery schema 与 `--resume`，不改变无 recovery schema 时的 #13 事务。证据见 [WP3 设计与证据](design/issue13-wp3-streaming-apply.md)、[规模数据](perf/issue13-wp3-a4a2e41/summary.md)、[Issue #14 验收](issue14-acceptance-report.md)。G5/G7/G9 残余风险、MySQL/Windows streaming 与 1M 单次样本仍未验证 |
| 有界流式并发、背压、取消 | 部分实现（PR #36） | SQLite：每页有界 1–16 Worker、固定队列、顺序输出、取消与故障注入回归；SQL 读写仍顺序执行；多表 + 多 Worker 的 CLI 证据与 DB 性能评估未交付 |
| 故障恢复、版本化 checkpoint、幂等重放 | 已实现（Linux amd64 本地 SQLite） | `m2-recovery-v1`：manifest + append-only progress + 同事务 checkpoint/receipt/业务 UPDATE + 显式 `--resume` + 只读 `status`；纯确定性策略 allowlist、KCV 绑定、冲突/commit_unknown fail closed；真实 CLI 子进程在 1/10/100/500 批、跨表、Validate 前后中断均恢复且 A/B 结果一致。MySQL/Windows/WAL、跨库映射与通用 exactly-once 不支持 |
| precision/recall 与吞吐/内存基准、可视化 | 部分实现（有实测合成值） | #6 的 74 条检测合成评估；PR #36 的 128 条 HMAC 合成批次 3 次重复基准；#13 phase 5 在 Linux 对真实 SQLite Runner 执行 10k/100k/1M 合成矩阵（42 次、BatchSize=64），含分阶段耗时、rows/s、采样 RSS/CPU/连接、收据容量与正确性核验，证据见 [phase-5 报告](perf/issue13-phase5-20261019-final/summary.md)。仅 1M 为单次，矩阵非 CLI、无 MySQL CLI、非未插桩生产吞吐；结果不证明严格的 batch×workers 内存上界，也未支持并行优化或关闭 #13。详见 [性能证据](performance.md) |
| 数据目录/治理平台导出 | 未实现 | 未纳入当前承诺 |

任何状态升级必须附可复现测试与文档更新。只创建包、类型、配置或让空包通过编译，都不能算业务功能已实现。
