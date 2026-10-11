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
| MySQL 隔离环境 | 部分实现 | 私有 socket fixture 已通过 CI；#5 提供 MySQL 适配器与可选业务测试入口，但业务测试仍可能 SKIP/UNVERIFIED，真实验收由 #10 负责 |
| 可选工作树工具 | 已实现（历史单机） | `scripts/check-native-worktree.mjs` 的旧预检，不是跨平台开发前置条件 |
| 四人模块任务定义 | 已实现（文档） | parallel-tasks.md；独立克隆/分支/PR，业务组件仍待实现 |
| 跨平台 CI | 部分实现 | Ubuntu/Windows 测试、格式、vet；Ubuntu race/MySQL fixture。配置不等于运行通过，以 PR checks 为准 |

## 数据库接入

| 功能 | 说明 / 范围 | 状态 | 目标 |
|---|---|---|---|
| 独立 Go module、可编译 CLI help/version | 无框架依赖，标准库 CLI | 已实现 | M0 |
| `scan` / `mask` / `validate` CLI | 当前为占位命令，返回退出码 2 且不接触数据库 | 部分实现 | M1 |
| 默认 Dry Run，`--apply` 单一写意图 | 零值即只读；配置不得激活写入 | 部分实现 | 数据库级零副作用待测；M1 |
| 配置文件、环境变量展开、参数校验 | 连接/检测/脱敏/验证分段校验 | 计划中 | M1 |
| 连接生命周期、schema/table/column/type/PK 元数据 | SQLite/MySQL Reader/Writer，元数据与 SchemaCoverage 保守声明；不支持或可能丢精度的映射显式拒绝 | 部分实现（#5 组件） | #5 / PR #21；MySQL 业务待 #10 验证 |
| distinct 非空抽样、表/列过滤 | typed、非 NULL distinct 有界抽样；按 DataSetID/Schema/TableRef 限定访问范围 | 部分实现（#5 组件） | #5 / PR #21 |
| SQLite 连接与真实集成 | SQLite 真实驱动执行 Reader/Writer、只读、类型、复合键分页和安全拒绝测试；以 PR #21 最终 CI 为证据 | 部分实现（#5 组件） | #5 / PR #21 |
| MySQL 连接与真实集成 | 驱动适配和 `TestMySQLAdapter` 已实现；需要隔离 socket，否则 SKIP；MySQL fixture 绿灯不等于适配器业务用例验证 | 部分实现（业务 UNVERIFIED） | #10 最终 M1 验收 |
| MariaDB / PostgreSQL / SQL Server / Oracle | 需独立适配与独立验证 | 未实现 | 后续评估 |
| 参数化更新、批次事务、复合键 keyset 分页 | #5 提供按真实复合主键游标、参数绑定、乐观受影响行数检查与回滚；跨方言独立验证持续进行 | 部分实现（#5 组件） | #5 / PR #21，MySQL 待 #10 |
| 无主键 apply 拒绝、敏感主键明确未处理 | 数据库 Writer 已对无主键及试图改主键的计划拒绝；CLI 端到端授权与拒绝仍待 Main 集成 | 部分实现（组件） | #5 / #9 |

## 敏感信息检测

China-first 的 13 种检测规则由 PR #20（Issue #6）实现；其规则是有界、上下文敏感的组件能力，**并不代表全国 PII 全覆盖或 CLI/数据库端到端扫描已经交付**。以下“已实现”仅表示存在对应 Go 代码和合成测试，不代表生产环境准确率或安全批准。#12 的格式约束脱敏仍未实现。

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
| 扫描错误 fail-closed 与非零退出 | 依赖 Main 的 CLI/runner 集成，不能以组件测试代替 | 计划中 | M1/Main |
| 可选外部/本地 LLM 辅助分类 | 默认关闭；不得外发样本 | 未实现 | 不在 M1/M2 承诺内 |

**评估证据与局限**：`internal/detect/eval_test.go` 使用 74 条自编合成 `(value, column)` 样本（38 正、36 负；8 条超出既定覆盖范围），以 `MinSamples=1` 独立检验规则/列上下文，**并非生产默认 20 样本的整列检测效果，也不是独立留出测试**。本次测得 `cn_person_name` TP=3、FP=3、FN=1，Precision=0.500、Recall=0.750、F1=0.600；包含普通词误报。部署前必须扩展独立标注数据、开展人工审核，不得默认用它批准 Apply。身份证/信用代码格式校验不是身份或登记真实性验证。规则列表不意味着覆盖一切行政区划、号码段、姓名和地址。原北美 SSN/US ZIP/NANP 等规则不属于本次 China-first 组件交付。

## 脱敏策略

以下为 MaskRiver 规划的策略目录；当前 **没有任何算法实现**。M1 只交付首批有界切片。

| 策略 | 说明 | 状态 | 目标 |
|---|---|---|---|
| null | SQL NULL | 计划中 | M1/go-mask |
| blank | 空串 | 计划中 | M1/go-mask |
| redact | 遮蔽内容、保留分隔符 | 计划中 | M1/go-mask |
| format_random | 字符类别/长度保持，typed 值专用分支 | 计划中 | M1/go-mask |
| fake_email | 保留邮箱结构、使用不可投递域 | 计划中 | M1/go-mask |
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

M1 首批切片（`null` / `blank` / `redact` / `format_random` / `fake_email`）之外的策略必须在各自验收通过后才可升级状态。

## 确定性、映射与安全

| 功能 | 说明 / 边界 | 状态 | 目标 |
|---|---|---|---|
| 策略优先级与未知策略拒绝 | 审核结论 > 列级 > 规则级 > 默认；未知即失败 | 计划中 | M1/go-mask |
| typed 输出、未变化结果拒绝 | 输出必须符合目标类型与约束 | 计划中 | M1/go-mask |
| 显式列级 NULL 标记 | 仅精确列生效，先于映射，不写映射 | 计划中 | M1/go-mask |
| 策略/词典注册与来源审核 | 新词典必须单独审核来源与许可 | 计划中 | M1/go-mask |
| 确定性：同输入同 scope 同输出 | 不承诺与任何第三方实现逐字节一致 | 计划中 | M1/go-mask |
| 持久映射：加盐指纹、不存原值、可复用 | 长度前缀编码，禁止拼接碰撞 | 计划中 | M1/go-mask |
| Dry Run 零持久副作用 | 需文件快照 + 连接 spy 证据 | 计划中 | M1/go-mask |
| 默认隐藏预览值 | 原值显示开关须单独评审，不默认实现 | 计划中 | M1/Main |
| 审核历史：pending/approved、审核人、过期、审计 | 最小格式优先，CSV 次之 | 计划中 | M1/Main |
| 历史导入/导出、回写与冲突检测 | 回写须备份与冲突检查 | 未实现 | 后续增量 |
| 结构化报告与退出码 | 扫描/验证/历史报告 | 计划中 | M1/Main |

## 完整性校验

| 功能 | 说明 / 边界 | 状态 | 目标 |
|---|---|---|---|
| 行数校验 | 不得增删行 | 计划中 | M1/go-verify |
| schema 校验：列类型/PK/索引/FK/unique/check | 能力缺失必须显式声明 | 计划中 | M1/go-verify |
| triggers/grants 校验 | 需驱动能力支持，不在首版承诺 | 未实现 | 后续评估 |
| 主键对齐逐值完整性校验 | 必须包含双向键差异 | 计划中 | M1/go-verify |
| 无键/限量覆盖/strict 模式 | 未完整覆盖不得 PASS | 计划中 | M1/go-verify |
| 测试数据忽略、采样上限、形状遮蔽 | 忽略策略须显式审核 | 计划中 | M1/go-verify |
| 稳定公共库 API | `pkg/contracts` 仅内部协作冻结，非稳定外部 SDK | 未实现 | 待契约稳定后评估 |

## M2 研发目标

| 功能 | 状态 | 目标 |
|---|---|---|
| 中国 PII 检测与格式约束脱敏 | 部分实现（仅检测） | 13 类 China-first 有界检测见 PR #20/#6；格式约束替换仍属 #12，不能因检测通过而关闭 #12 |
| 有界流式并发、背压、取消 | 计划中 | M2；M1 仅正确有界分页，不冒充高性能并发 |
| 故障恢复、版本化 checkpoint、幂等重放 | 计划中 | M2；先解决事务提交与映射一致性 |
| precision/recall 与吞吐/内存基准、可视化 | 部分实现 | 仅 #6 的 74 条开发时合成检测评估已产生数值；独立评测、吞吐/内存基准与可视化仍未实现 |
| 数据目录/治理平台导出 | 未实现 | 未纳入当前承诺 |

任何状态升级必须附可复现测试与文档更新。只创建包、类型、配置或让空包通过编译，都不能算业务功能已实现。
