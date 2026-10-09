# M1-B 四个独立组件任务

拓扑：multi-seam，四个无重叠的可测试边界；组件完成后由 **Main 串行集成**，不是四个 Agent 同时改 runner。

共同基线：`pkg/contracts` 的 `APIRevision=m1a-v1` 与 Main 冻结提交后的干净 `refs/heads/main`。启动前记录其完整 SHA，分配后逐个检查 baseCommit 相同，禁止过程中随 main 移动而混用基线。
规范依据：本仓库的 [contracts.md](contracts.md)（接口与安全不变量）、[architecture.md](architecture.md)（执行模型）、[test-plan.md](test-plan.md)（验收用例）、[roadmap.md](roadmap.md)（里程碑范围）。
本项目不再以复现或翻译其他项目的内部实现为验收目标；所有实现与验收以本仓库的接口、需求与测试为依据。

## 共同启动 / 交付条件

1. 按 [worktree-preflight.md](worktree-preflight.md) 使用根 `/home/haoyue/Project/worktrees`、provider=native，配置 reload 后先通过一次小范围隔离冒烟，再取得用户正式 M1-B 授权。
2. Main 记录 `lane | repo | cwd(绝对路径) | branch/baseRef | 文件范围 | 测试门禁 | handoff`。cwd 必须是 Native 实际返回的 `/home/haoyue/Project/worktrees/maskriver/pi-worktree-<runId>-<index>`，不同 writer 不得相同。四份正式工作树仍未创建。
3. 每个 writer 只写自己的独立工作树。共享代码 `pkg/**`、`go.mod/go.sum`、`internal/config|runner|history|testenv`、`tools`、`scripts`、`docs`、`.pi` 均只读。
4. 依赖已统一锁定；Worker 不 `go get`、不安装扩展、不改全局设置。不得递归委派、不得 `git add/commit/push`，也不得操作 GitHub；Main 收集 diff 与测试报告后统一提交。
5. 所有任务必须对改动文件执行 `gofmt`，并运行 `go test ./...` 与 `go vet ./...`；是否新增 `-race` 见各 lane。报告 MySQL `SKIP` **不算**通过。输出改动文件、接口断言、实际命令/退出码、风险与阻塞。
6. 各 owner 只接受冻结契约；遇签名不足、所有权冲突、类型不可无损表示、权限不足或基础设施失败，停止并交 Main 决策，不自行扩大类型、换模型或降级执行方式。
7. 首轮真实 SQLite 集成是硬门禁；MySQL 缺实例允许记录 `UNVERIFIED/SKIP`，不阻塞其他组件，但保留其实现与真实集成用例。Mock 不能替代真实 MySQL，最终 M1 验收仍被该项阻塞。

## Lane go-db — 数据库访问组件

模型：`openai-codex/gpt-5.6-sol`。

**规范依据**：contracts.md §3（`DatabaseOptions`、`TableSchema`、`Reader`/`Writer`/`WriteTx`、分页与事务语义）；architecture.md 第一阶段执行模型第 5–6 条；roadmap.md M1 数据库接入范围。

**允许修改**：`internal/db/**/*.go`（含本包单元/集成测试）。其余文件一律只读。
**依赖**：`contracts.DatabaseOptions`、`TableRef`/`ColumnRef`/`Type`/`Value`、`TableSchema`/`SchemaCoverage`、`Reader`、`Writer`/`WriteTx`、`Page`/`Sample`、`TablePlan`/`RowChange`、安全错误类。
**入口冻结**：
- `db.OpenReader(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Reader, error)`
- `db.OpenWriter(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer, error)`

**范围**：SQLite 与 MySQL 两方言明确实现；只读开库且不创建数据库；元数据与能力标记；distinct 有界抽样；按真实有序复合主键的 keyset 分页；参数化 NULL-safe 乐观批量 `UPDATE`；逐批事务；取消与资源关闭；安全错误分类。MySQL 实现与测试入口必须交付。

**非目标**：检测/策略/校验逻辑；schema 迁移或重命名；PK/FK 变更；跨库复制；断点恢复；并发调度；生产 DSN 管理；MariaDB 及其他引擎。

**必须测试**：
- 编译期断言满足 `Reader`/`Writer`/`WriteTx`；非法 limit/options/列名在 SQL 之前被拒绝。
- SQLite：只读打开缺失文件不创建；只读连接 `UPDATE` 失败；25 行按每页 10 行得到三页且终止正确；复合主键游标推进；空表；无键拒绝。
- 类型往返：NULL / 空串 / bytes / decimal / date / unsigned；`SchemaCoverage` 未知不得等同空集合；不假设 Go 字符串排序等于数据库排序。
- 写入：`Expected` 冲突零提交；无键或敏感主键拒绝；rollback；`context` 取消；提交结果不明不得自动重试。
- MySQL：在可用隔离实例上执行同等用例；实例缺失时明确 `SKIP` 并标注未验证。
- 存在并发结构时 `go test -race ./internal/db`。

**完成标准**：首轮 SQLite 契约测试全部真实执行；Reader 与 Writer 分离；不 import `internal/detect|mask|verify`；有失败路径证据。MySQL 可按未验证状态交接本轮，但不得声称完整支持。

**停止条件**：Native 隔离失败；SQLite 真实测试失败；需要扩展共享 DTO 或驱动；无法确定类型/元数据是否无损；超出文件范围。

## Lane go-detect — 敏感信息检测组件

模型：`openai-codex/gpt-5.6-sol`。

**规范依据**：contracts.md §4（`Detector`、`DetectionRequest`、`Decision`、三态、`Evidence`、`IssueCode`）；feature-matrix.md 检测范围（12 类 M1 规则及其明确局限）；test-plan.md 检测用例。

**允许修改**：`internal/detect/**/*.go`（含本包测试）。其余文件一律只读。
**依赖**：`Detector`、`DetectionRequest`、`Decision`、`Sensitivity`/`DecisionSource`、`Sample`/`Evidence`/`IssueCode`、`Value`/`Type`。
**入口冻结**：`detect.New() contracts.Detector`；规则不可变，无包级可变全局状态。

**范围**：`email`、`url`、`ip_address`（仅 IPv4）、`uuid`、`ssn`、`credit_card`、`zip_code`、`phone`（北美格式）、`address`（英文街道）、`city`、`full_name`、`date`；人工覆盖 / 已审核结论 / 显式 skip 的优先级；distinct 非空样本阈值；证据不足或冲突输出 UNKNOWN；所有输出只含受控证据。
如需城市词典，先用最小自写合成测试词典，并向 Main 提出正式词典来源与许可建议，不得自行引入数据依赖。

**非目标**：LLM 分类；中文格式（属 M2）；历史文件处理；数据库抽样；持久化与并发引擎；不承诺全球号码识别或完整姓名识别。

**必须测试**：编译期 `Detector` 断言；每条规则正例与负例；19/20 样本边界与 90% 阈值；无上下文纯数字；NULL 与空串；多个规则同时合格时的冲突；列名/类型上下文与取值冲突；位置或类型不匹配的 override 返回 `ErrInvalid`；MDY/DMY 日期语义；`context` 取消；断言输入 DTO 未被修改。

**完成标准**：明确列出已实现规则与不支持的形态；UNKNOWN 不会被转换为安全结论；测试为纯内存，不依赖 `internal/db`。

**停止条件**：需要新增契约字段或第三方依赖；词典来源与许可未决；规则冲突无法在现有契约内表达；要求外发样本。

## Lane go-mask — 脱敏策略引擎

模型：`openai-codex/gpt-6.1-sol`。

**规范依据**：contracts.md §5（`Strategy`、`StrategyRef`、`MaskContext`、`MappingKey`、`MappingReader`/`MappingWriter`）；feature-matrix.md 策略目录与「首批切片」约定；test-plan.md 策略与确定性用例。

**允许修改**：`internal/mask/**/*.go`（含本包测试）。其余文件一律只读。
**依赖**：`Strategy`/`StrategyRef`、`MaskContext`、`ColumnPlan`/`Type`/`Value`、`MappingKey`/`MappingReader`/`MappingWriter`、安全错误类。
**入口冻结**：
- `mask.NewStrategy(ref contracts.StrategyRef) (contracts.Strategy, error)`
- `mask.NewMemoryMapping()`，返回同时实现 `MappingReader` 与 `MappingWriter` 的具体并发安全类型。

**范围**：首批有界切片 `null` / `blank` / `redact` / `format_random` / `fake_email`，版本 `v1`；确定性随机流由 `MaskContext.Key` 派生（keyed fingerprint），不使用进程级全局 RNG；输入身份按 `typed-lexical-v1`（Kind + 长度前缀载荷）；映射键包含策略 id/version、scope、规范化版本与 key id；并发安全的内存 get-or-create 只保留一个 winner。`format_random` 本轮只支持 `Text`/`Bytes`，其他 Kind 返回 `ErrUnsupported`。

**非目标**：其余 12 种策略；复制任何第三方词典；持久映射存储或数据库 schema；事务引擎；跨库 exactly-once；密钥轮换迁移；中文数据与 FPE；与任何第三方实现的逐字节兼容。

**必须测试**：编译期断言满足 `Strategy`、`MappingReader`、`MappingWriter`；固定输入与上下文重复执行结果一致；改变 scope/version/key 不复用他人映射；NULL 与空串区分；Unicode 与二进制载荷；非空敏感值不得返回未变化结果；未知策略报错；`MaskContext.Key` 过短报错；`Lookup` 未命中不产生写入；并发的 get-or-create 只有一个 winner；取消路径；`go test -race ./internal/mask`。

**完成标准**：策略零 I/O；依赖不包含 `internal/db`；无原值、密钥或盐出现在日志与错误中；用自建合成值做 golden 测试；文档明确只覆盖这 5 种策略。

**停止条件**：需要 schema/数据库/公共类型变更；要求一次性实现全部策略；输出约束无法满足且需产品决策。

## Lane go-verify — 验证系统

模型：`openai-codex/gpt-5.6-sol`。

**规范依据**：contracts.md §7（`Validator`、`ValidationReport`、`Coverage`、`Issue`、`Passed(strict)`）；architecture.md「确定性与完整性」；test-plan.md 验证用例。

**允许修改**：`internal/verify/**/*.go`、`tests/**/*.go`，以及新自写的纯合成 `tests/testdata/**`。不得修改 Main 所有权的 `internal/testenv`、`tools`、`scripts`、`pkg`。
**依赖**：`Validator`、`Reader`、`Plan`、`ValidateRequest`、`ValidationReport`/`Coverage`/`Issue`、安全错误类。
**入口冻结**：`verify.New() contracts.Validator`。

**范围**：仅通过 `contracts.Reader` 读取；行数比对、schema 元素比对、**双向**键集合比对、按固定 `Plan` 的逐键逐敏感值变化校验；显式 `Coverage` 统计，`Complete` 默认 false；缺表、能力不足、无法安全对齐时输出 `Warning`/`Error`，绝不静默 `PASS`；分页读取保证内存有界。组件测试必须使用 fake `Reader`，不依赖 `internal/db`。

**非目标**：读取配置；重扫目标来推断校验列；自动修复或写入目标；无键启发式 `PASS`；假设不同排序规则等价；性能可视化。

**必须测试**：编译期 `Validator` 断言；单个敏感字段未变化可被检出；源缺失键、目标多键、行数相同但键不同均判失败；`Coverage.Complete=false` 时 `Passed(strict)` 必须为 false（`strict` 另拒 `Warning`）；空表可 PASS 但缺表不可；恰为 `RowLimit` 与超过 `RowLimit`；无主键；非法输入；`context` 取消；Reader 报错传播；断言输入 `Plan` 未被修改；断言语义上无法安全对齐时返回 `ErrUnsupported`，不用 Go 词法排序误判。

**完成标准**：未完整覆盖不得 PASS；「未验证」与「失败」在报告中可区分；fake `Reader` 测试全覆盖；明确声明 fake 测试**不是**数据库集成证据。

**停止条件**：需要修改公共 DTO；读能力不足；跨方言比对策略未定；需要改其他模块才能修复。

## 最终交接门禁

Main 收齐四个终态交接后执行串行集成：先审文件所有权、接口变动与敏感内容，再用 `git apply --check` 按基线应用补丁，随后执行完整 `go test ./...`、`go vet ./...` 与 SQLite 端到端。
独立只读 `go-reviewer` 审查需单独授权，当前不启动。
M1-B 组件完成不等于 M1 完成：runner/config/history/CLI 集成、持久映射、其余策略与端到端安全门属于后续阶段。
