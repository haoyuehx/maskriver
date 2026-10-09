# M1-A 共享契约冻结：m1a-v1

规范实现：`pkg/contracts/{value,contracts,plan}.go`；Owner **Main**。
这是四个 Worker 的开发接口冻结，不是稳定公共 SDK/生产能力承诺。变更需 Main 修订本文件、APIRevision、共享测试与任务定义；Worker 禁止定义同义 DTO 或改签名。

## 1. 包依赖与注入

```text
cmd/maskriver → internal/config + internal/runner
internal/runner → contracts + db/detect/mask/verify/history（后续组装）
internal/{db,detect,mask,verify,history} → pkg/contracts → 标准库
internal/db → database/sql + 两个锁定驱动
internal/mask → contracts（M1-B 先内存映射；持久适配另行分配）
internal/verify → contracts.Reader（不 import internal/db）
internal/detect → contracts.Detector（不 import db/history/mask）
internal/testenv + tools/fixture → database/sql + 驱动（Main 独占环境准备）
```

禁止组件横向导入彼此的具体实现；依赖以接口传入。历史批准/过期/类型校验由 Main/history 完成，将有效 Reviewed Decision 注入检测请求；检测不得自己查询历史。
`pkg/contracts` 不导入 `internal`、数据库驱动或第三方模块。所有 DTO 仅一份；外部项目即使能 import，当前不享有跨版本稳定保证。

## 2. 定位、类型、值

- `TableRef{Database, Schema, Table}`：Database 为逻辑数据集 ID，不是 DSN；Schema 对 SQLite 固定 `main`，MySQL 是 schema 名。标识符保留大小写，不拼接后再 split。
- `ColumnRef{TableRef, Column}`：精确四元组。各 Reader 把同一逻辑引用绑定到自身的源/目标连接；M1-B 不做跨方言重命名映射。
- `Type{Kind, Native, Nullable, Length, Precision, Scale, Collation}`：Native 是声明类型。没有驱动能力时不能伪造精度/排序规则；未知长度等为 0。Nullable 必须从 metadata 获取。
- `Value`：私有 kind/raw、不可变且可比较；`NewValue(Kind, string)` 是唯一非 NULL 构造器。零值是 SQL NULL；空文本、空二进制、0、false 与 NULL 全部不同。

| Kind | Payload 契约 |
|---|---|
| Null | 必须空 payload；不由字符串 "NULL" 推断 |
| Text | 合法 UTF-8，不 trim、不 Unicode 归一化 |
| Bytes | 任意二进制 string；驱动 []byte 必须复制后构造，不保留借用 buffer |
| Int / Uint | 十进制 64 位、有符号/无符号独立；无前导 + 或多余 0 |
| Float | 有限 float64；构造时标准化为最短 round-trip 格式，拒绝 NaN/Inf |
| Decimal | 正则 `^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`，不经过 float64；保留 scale，无指数 |
| Bool | true / false |
| Date | 合法 YYYY-MM-DD，年份 >=1，无时区 |
| LocalDateTime | YYYY-MM-DDTHH:MM:SS[.fraction]，最多纳秒，无时区；fraction 不留尾部 0 |
| Instant | RFC3339Nano 输入，标准化为 UTC；不能把 MySQL DATETIME 强加 UTC |

UUID/IP/phone 等是 Text 的语义规则，不创建与 SQL 无关的重复 Value 类型。MySQL TIME、JSON 内嵌 PII、零日期等首轮不能无损映射时返回 ErrUnsupported，而不是伪装正常日期。
`typed-lexical-v1` 的身份是 Kind + 长度前缀 Payload；Decimal 1.0 与 1.00 保留区别。关联列需同 Kind/scale/策略/scope/key；跨类型数值归一化不在首轮范围。
`Payload()` 仅算法/适配器可用；禁止日志输出。Value 的所有 fmt 动词与 JSON 都遮蔽，JSON 反序列化拒绝；这些是防误用措施，不防止主动调用 Payload 泄露。不要直接用报告 JSON 做恢复数据。

## 3. schema 与 DB Reader/Writer

`DatabaseOptions{Dialect, DSN, DatasetID, Schemas, MaxOpenConns, ConnectTimeout}` 由 Main/config 构造；Dialect 仅 SQLite/MySQL，DSN 为 Text Value、不得输出。DatasetID 绑定 TableRef.Database，Schemas 是显式允许列表（非空），连接数/超时必须正数；M1-B MaxOpenConns=1 起步。源与目标使用相同逻辑 dataset/schema/table，但不同只读快照/目标连接；首轮不做 schema 重命名（必要时不同实例使用相同 schema 名）。

`TableSchema` 含列、**按原顺序**的 PrimaryKey、索引、外键动作、唯一约束和 CHECK；`SchemaCoverage` 每项 bool 标识是否真的取到。false 不等于空集合，不支持不能返回“空但已知”。

```go
type Reader interface {
 Tables(context.Context, []string) ([]TableRef, error)
 Describe(context.Context, TableRef) (TableSchema, error)
 Sample(context.Context, SampleRequest) (Sample, error)
 Page(context.Context, PageRequest) (Page, error)
 Count(context.Context, TableRef) (int64, error)
 Close() error
}
type Writer interface {
 Begin(context.Context, TablePlan) (WriteTx, error)
 Close() error
}
type WriteTx interface {
 Update(context.Context, []RowChange) (int64, error)
 Commit(context.Context) error
 Rollback(context.Context) error
}
```

- 所有输入在 SQL 前验证；limit 必须 1..10000，schema 列表/引用必须在连接允许范围。配置/registry 在构造组件时注入，不用全局状态。
- `Sample` 是最多 Limit 个 DISTINCT 非 NULL typed 值；不删除空串，Detector 再计算非空证据；`Basis=DistinctNonNull`，用 limit+1 判断 Truncated。比例绝不叫全表行比例。
- `PageRequest` 明确 Columns、KeyColumns、After、Limit。KeyColumns 必须完全等于真实有序主键；After 为空代表起点，否则数量/类型匹配且无 NULL。Columns 必须包含 keys，不隐式漏读键。
- keyset 按 DB 原生列排序/比较语义；不能用 Go 字符串排序替代 collation。M1-B 无键分页 ErrUnsupported（包含无键 preview/validate）；禁用 OFFSET 冒充可恢复游标。
- `Page` 持有完整且有界 Rows，SQL rows/游标在返回前关闭。Next 为最后一行原始键，Done 用额外一行或后续空页判定；Done=false 必须 Rows 非空且游标前进。不得在 error 时消费半页。
- `Row` 缺字段不同于字段 NULL；缺列是错误。原始 Row/After/Next/RowChange 不进入日志/报告；Value 遮蔽仍不代表可以散播 row metadata。
- Reader 打开已存在库，禁止建库/建表/写映射/写审计；SQLite mode=ro，MySQL 使用实际 SELECT/metadata 只读能力。依靠接口分离不是数据库权限隔离，应另用最小权限账号。
- Writer 只在 runner Apply 注入。`Begin` 接收固定 TablePlan，拒绝无主键、主键变更、未知列/策略类型；所有 value 参数化，标识符由 metadata 允许列表与方言引用生成。
- `RowChange.Key` 为有序原始键；Expected 与 Replacement 键集合必须完全等于本批计划的所有修改列。UPDATE 带键和所有 Expected 的 NULL-safe 乐观比较；每项必须精确影响一行，否则整个事务废弃并回滚，不允许部分继续。
- `Update` 返回事务内受影响行数，不等于 committed。只允许一次 Commit；取消/错误后禁止 Commit。Rollback 幂等；结束后的 Update/Commit 返回 ErrClosed。
- Commit 无法确定是否持久化返回 ErrCommitUnknown，不能标记为已回滚或自动重试。批次已提交不假装整库原子性。Close 幂等，adapter 不保留活跃游标。

M1-B 在静止隔离副本上测试，禁止同时由外部进程修改输入；Reader 不保证跨连接快照事务。源快照由 Main 在 scan 前准备并固定。

## 4. Detection Decision

`Sensitivity` 的零值是 Unknown，另有 NotSensitive/Sensitive；`DecisionSource` 零值 SourceUnknown 不可作为已判定结果。
`Decision` 包含 Column、Type、Sensitivity、Source、RuleID、可选 StrategyRef、SampleBasis、Evidence；每个 Evidence 含规则 ID、命中/非空样本数、阈值、Eligible、受控 Reasons。
`DetectionRequest` 明确列/sample、MinSamples、MinRatio、DateOrder、Override、Reviewed、Skip。
优先级：精确合法 Override > Main 已审核 Reviewed > 显式 Skip > 规则。传入位置/类型不一致的 override/history 是 ErrInvalid，不能静默忽略。
规则不访问 DB，不写历史，不外发样本。冲突/不足/空样本返回 Unknown + evidence、通常 nil error；错误表示检测过程未完成，两者都不能批准 Apply。
M1-B 不提供自动接受 UNKNOWN 或 allow-partial 逃生选项。

## 5. Masking Plan / Strategy / mapping

`Plan` 包含 Revision、ID、SourceSnapshotID、TargetID、SchemaDigest、ConfigDigest、ScanComplete、Unresolved、Tables。
每个 `TablePlan` 固定 Table/PrimaryKey/Columns；每个 `ColumnPlan` 固定 Column/Type、StrategyRef(ID+Version)、Scope、NormalizationVersion、KeyID。
- Plan 发布后 immutable **为契约要求**，Go 的 slice/map 并非物理不可变；构建器/Main 必须深拷贝，Worker 不修改入参，也不得并发修改 DTO。
- ID 是 Main 将版本化结构按明确顺序/字段 canonical encoding 后的摘要；本轮仅冻结字段，canonical plan builder 在 Main 后续实现，不允许用随意字符串授权。
- `ValidateShape()` 已实现基础字段/重复/归属/扫描完成/无键/敏感 PK 检查；它不验证摘要、不验证 schema/FK、不解析算法 registry，不是写入授权。
- 密钥只在 `MaskContext.Key` 内存中传入，Kind=Bytes、至少32字节（Worker 检查）；KeyID 不是密钥。Map/Plan/报告不能携带 secret。

`Strategy.Ref()` + `Mask(ctx, Value, MaskContext)`：无 I/O、同 typed 输入与同 context 必须同输出；NULL 保留（显式 null 策略可生成 NULL），非 NULL 必须输出合法类型/格式。null/blank 必须满足目标约束；未变化的非空敏感值拒绝。不吞掉类型错误或 fallback 成伪合法日期。
`MappingReader.Lookup` 与 `MappingWriter.GetOrCreate` 分离；key 是 HMAC 指纹 + strategy/version/scope/normalization/keyID；不能普通 hash 原值。
映射内部编码必须长度前缀/明确类型，禁止简单拼接碰撞。原值不存储，候选值并发 get-or-create 只保留一个 winner。Dry Run 只 Lookup，miss 做纯计算，不 GetOrCreate。
M1-B 先验证纯策略与内存原子映射；持久映射、key 轮换迁移、目标/映射跨库提交一致性后续 Main 单独批准，不作恢复承诺。

## 6. scan / preview / apply / validate

签名由 `Workflow` 定义；底层 opened Reader、Detector、策略 registry、映射及可选 Writer 由 Main 构造器注入，组件不自行管理上游或配置文件。

| 操作 | 输入 | 输出/安全不变量 |
|---|---|---|
| Scan | ScanRequest：显式表、sample limit、阈值、日期语义 | ScanReport：Decisions、Coverage、Issues；零持久写；列错误设置 Complete=false 并返回 ErrIncomplete/原有安全类别 |
| Preview | PreviewRequest：固定 Plan + LimitPerTable(1..10000) | PreviewReport：PlanID、RowsRead、Coverage、Issues；仅计数/证据，无原始或变换后行；截断 coverage=false；不打开 Writer、不创建 store |
| Apply | ApplyRequest：固定 Plan + ExplicitApply + BatchSize(1..10000) | ApplyReport：已提交 BatchReceipt、未知提交标记、Complete、Issues；false 必须 ErrUnsafe，无持久写入 |
| Validate | ValidateRequest：固定原始 Plan、RowLimit>=0、Strict | ValidationReport：PlanID、Coverage、Issues；0 表示有界分页全量，不是无限内存；正数是上限，到达但仍有数据则不完整 |

CLI 仍为骨架，这些 workflow 签名没有实现。将来 `mask` 默认路由 Preview；只有显式 CLI --apply 才设置 ExplicitApply=true，YAML/环境变量不得激活写入。`ExplicitApply` 是意图位，不是鉴权令牌；程序库的授权边界由调用方负责。
Apply 前 Main 必须重新校验完整扫描、所有 UNKNOWN 已经人工重新决策、源快照不变、目标身份/schema/plan/config 摘要匹配、策略 registry、列约束及权限。首轮禁止改 PK/FK 关系列；独立副本仍需备份。没有写入能力不能输出 success。
Preview 截断不声称全表验证；ApplyReport 只有 Commit 成功后才追加 receipt，失败返回 partial report + error；不能忽略 error。

## 7. Validation Report / 安全错误

`Validator.Validate(ctx, source Reader, target Reader, request)` 不依赖具体 DB 实现。
报告必须覆盖行数、schema、完整双向键集合、按键逐敏感字段变化与计划约束。只比较 intersection 不可 PASS。
`Coverage.Complete` 默认 false，Tables/Columns/Rows 是实际完成范围计数；不一致为 Fail，没检查到是 Warning/Error，禁止把 UNKNOWN/零值 Status 当 Pass。
`Issue` 只允许受控 Status/IssueCode、定位和计数，不提供 arbitrary message、sample、明文主键。
`Passed(strict)` 要求 PlanID 非空、Complete=true、至少一张表/一个列已检查、Rows>=0；任何 Fail/Error/未知状态失败，strict 还拒绝 Warning。报告零值失败。完整空表可 PASS（Rows=0），缺失表不行。

`errors.Is` 类别：ErrInvalid、ErrUnsupported、ErrIncomplete、ErrUnsafe、ErrConflict、ErrNotFound、ErrClosed、ErrDatabase、ErrCommitUnknown；ctx.Err() 保持 context.Canceled/DeadlineExceeded 可识别。
不把驱动原始 Error() 拼进公开输出（可能含 SQL/DSN/值）；适配器返回安全类别，Main 将其映射为受控 IssueCode。允许安全类别间包装，不允许原始秘密 cause 被日志透传。
所有长操作使用非 nil Context，进入/批次/循环检查取消；nil ctx 是调用错误，禁止后台 goroutine 越过调用返回继续写。清理 rollback/close 使用独立有界 cleanup context；取消不证明 commit 没发生，提交状态不明须优先报告 ErrCommitUnknown。

## 8. 冻结边界与未实现项

本轮实现 DTO、Value 安全构造/输出、Plan 结构检查、Report gate；不实现 Detector/Strategy/DB adapters/Validator/Workflow。
M1-B 各组件独立依赖 contracts，可用 fake Reader 开发 verifier，不等待 go-db 合并。
schema digest/plan builder、SQL 权限、数据快照/持久 mapping、恢复/并发引擎仍非本轮功能；冻结不替代安全审查。
接口测试在 `pkg/contracts/contracts_test.go`。驱动与环境另见 [database-drivers.md](database-drivers.md)、[test-environment.md](test-environment.md)。
