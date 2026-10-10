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
| SQLite 合成环境 | 已实现 | `internal/testenv`、`tools/fixture`；业务适配器未实现 |
| MySQL 隔离环境 | 部分实现 | MySQL 8.0.46 私有 socket fixture 在 PR #15 CI 已实际 PASS 并清理容器；业务集成仍待 #5/#10 |
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
| 连接生命周期、schema/table/column/type/PK 元数据 | 读能力与能力标记，不伪造精度/排序规则 | 计划中 | M1/go-db |
| distinct 非空抽样、表/列过滤 | 抽样口径必须可追溯 | 计划中 | M1/go-db |
| SQLite 连接与真实集成 | 首轮硬门禁 | 计划中 | M1/go-db |
| MySQL 连接与真实集成 | 实现与用例必须交付；真实运行是最终 M1 门禁 | 计划中 | M1/go-db |
| MariaDB / PostgreSQL / SQL Server / Oracle | 需独立适配与独立验证 | 未实现 | 后续评估 |
| 参数化更新、批次事务、复合键 keyset 分页 | 无 OFFSET 伪装游标；无键拒绝 | 计划中 | M1/go-db |
| 无主键 apply 拒绝、敏感主键明确未处理 | 不允许静默跳过并报告成功 | 计划中 | M1/Main + go-db |

## 敏感信息检测

| 功能 | 说明 / 范围 | 状态 | 目标 |
|---|---|---|---|
| 人工覆盖、优先级与 skip | 精确合法覆盖 > 已审核结论 > 显式 skip > 规则 | 计划中 | M1/go-detect |
| 已审核结论复用与类型/过期校验 | pending 不自动批准；UNKNOWN 不缓存为安全 | 计划中 | M1/Main |
| 规则证据、阈值、上下文、冲突判定 | 非加权赢家；证据不足或冲突输出 UNKNOWN | 计划中 | M1/go-detect |
| email | 格式与长度约束 | 计划中 | M1/go-detect |
| url | http(s)/www 形式 | 计划中 | M1/go-detect |
| ip_address | 仅 IPv4，不宣称 IPv6 检测 | 计划中 | M1/go-detect |
| uuid | UUID 字符串形状 | 计划中 | M1/go-detect |
| ssn | 受约束编号格式与上下文 | 计划中 | M1/go-detect |
| credit_card | Luhn + 受支持品牌/长度 + 上下文 | 计划中 | M1/go-detect |
| zip_code | US ZIP / ZIP+4 + 上下文 | 计划中 | M1/go-detect |
| phone | 北美号码格式，非全球识别 | 计划中 | M1/go-detect |
| address | 英文街道模式 | 计划中 | M1/go-detect |
| city | 城市词典成员 + 列上下文 | 计划中 | M1/go-detect |
| full_name | 姓名特征 + 列上下文 | 计划中 | M1/go-detect |
| date | 日历有效性、MDY/DMY、多种格式 | 计划中 | M1/go-detect |
| UNKNOWN 不等于非敏感、不持久化 | 空样本/冲突/不确定一律 UNKNOWN | 计划中 | M1/go-detect |
| 扫描错误 fail-closed 与非零退出 | 逃生选项须单独安全评审 | 计划中 | M1/Main |
| 可选外部/本地 LLM 辅助分类 | 默认关闭；不得外发样本 | 未实现 | 不在 M1/M2 承诺内 |

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
| 中国身份证/手机号/银行卡/统一社会信用代码/姓名地址检测与格式约束 | 计划中 | M2；合成样本评估，报告覆盖率与误报/漏报 |
| 有界流式并发、背压、取消 | 计划中 | M2；M1 仅正确有界分页，不冒充高性能并发 |
| 故障恢复、版本化 checkpoint、幂等重放 | 计划中 | M2；先解决事务提交与映射一致性 |
| precision/recall 与吞吐/内存基准、可视化 | 计划中 | M2；尚无成绩 |
| 数据目录/治理平台导出 | 未实现 | 未纳入当前承诺 |

任何状态升级必须附可复现测试与文档更新。只创建包、类型、配置或让空包通过编译，都不能算业务功能已实现。
