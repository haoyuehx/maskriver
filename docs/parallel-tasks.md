# M1-B 四个独立任务（仅定义，尚未授权启动）

拓扑：multi-seam，四个独立可测试边界；完成组件后 **Main 串行集成**，不是四个 Agent 同时改 runner。
共同基线：`pkg/contracts` 的 `APIRevision=m1a-v1` 与 Main 收尾提交后的干净 `refs/heads/main`。启动前记录其完整 SHA，分配后逐个检查 baseCommit 相同，禁止过程中随 main 移动而混用基线。
源仓库 `/home/haoyue/Project/maskriver`；所有上游路径使用绝对路径 `/home/haoyue/Project/dbmask/...`，禁止从 worktree 推导 `../dbmask`。
上游 commit `7d8789ef4883a423ddba2f8934b95997d1aaf099`，只读，不运行 Python、不复制环境/字典/数据/密钥、不做上游 Git 写操作。

## 共同启动/交付条件

1. 按 [worktree-preflight.md](worktree-preflight.md) 使用根 `/home/haoyue/Project/worktrees`、provider=native，reload 配置后先通过 go-db 小范围模型冒烟，再取得用户正式 M1-B 授权。
2. Main 记录 `lane | repo | cwd(绝对路径) | branch/baseRef | 文件范围 | 测试门禁 | handoff`。cwd 必须是 Native 实际返回的 `/home/haoyue/Project/worktrees/maskriver/pi-worktree-<runId>-<index>`，不同 writer 不得相同。四份正式工作树仍未创建。
3. 每个 writer 只写自己的独立工作树。共享代码 `pkg/**`、go.mod/go.sum、internal/config/runner/history/testenv、tools、scripts、docs、.pi 均只读。
4. 依赖已统一锁定；Worker 不 go get、不安装扩展、不改全局设置。不得递归委派、commit/push/GitHub 发布；Main 收集 diff/测试报告后提交。
5. 所有任务必须 `gofmt` 自己的 Go 文件、`go test ./...`、`go vet ./...`；报告 MySQL SKIP 不算通过。并发结构 `go test -race`。输出改动文件、接口断言、实际命令/退出码、风险、阻塞。
6. 各 owner 只接受 frozen contracts；遇签名不足/所有权冲突/类型不可无损表示/权限不足/基础设施失败，停止该任务并交 Main 决策，不自行扩大类型或换模型。
7. 首轮 SQLite 真实集成是硬门禁；MySQL 缺实例允许记录 UNVERIFIED/SKIP，不阻塞其他组件开发，但保留 go-db MySQL 实现/真实集成任务。Mock 不能替代真实 MySQL，最终 M1 验收仍被该项阻塞。

## Lane go-db — adapters / metadata / keyset / transactions

模型：`openai-codex/gpt-5.6-sol`。

**精确只读参考路径**：
- `/home/haoyue/Project/dbmask/src/dbmask/connectors/base.py`
- `/home/haoyue/Project/dbmask/src/dbmask/connectors/sql.py`
- `/home/haoyue/Project/dbmask/src/dbmask/masking/engine.py`（仅分页/事务语义）
- `/home/haoyue/Project/dbmask/tests/test_mask_apply_paging.py`
- `/home/haoyue/Project/dbmask/tests/test_masking_safety.py`

**允许修改**：`internal/db/**/*.go`（包含本包单元/集成测试）；不能改其他文件。
**依赖**：DatabaseOptions、TableRef/ColumnRef/Type/Value、TableSchema/SchemaCoverage、Reader、Writer/WriteTx、Page/Sample、TablePlan/RowChange、安全错误。
**入口冻结**：`db.OpenReader(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Reader,error)`、`db.OpenWriter(ctx context.Context, opts contracts.DatabaseOptions) (contracts.Writer,error)`。

**范围**：SQLite/MySQL 两方言明确实现；只读开库、元数据能力、不生成数据库；distinct 抽样；按真实有序复合 PK 的有界 keyset；参数化 optimistic UPDATE、逐批事务、取消/资源关闭与安全错误。
**非目标**：检测/策略、schema 迁移/重命名、PK/FK 变更、跨库复制、断点恢复、并发调度、生产 DSN 配置、MariaDB/其他引擎。
**必须测试**：
- compile-time Reader/Writer/WriteTx 断言；非法 limits/options/列名拒绝。
- SQLite 只读缺文件不创建、不能 UPDATE；MySQL 真实隔离实例只读能力。
- 25行/page10、复合主键、不足尾页、空表、游标前进、无键拒绝、任意恶意标识符安全引用。
- NULL/bytes/decimal/date/unsigned、unknown schema coverage、collation 不假设 Go 字符串排序。
- Expected 冲突零提交、no key/sensitive PK 拒绝、rollback、context cancel、commit 不确定不自动重试。
**完成标准**：首轮 SQLite contract tests 全部真实执行；Reader 与 Writer 分离，不 import detect/mask/verify；有失败路径证据。MySQL 实现、测试入口和用例仍必须交付，可按未验证状态完成本轮组件交接，但不得声称完整 MySQL 支持；最终 M1 必须实际执行 MySQL contract/integration tests。
**停止条件**：Native 隔离失败、SQLite 真实测试失败、需要扩展 DTO/驱动、不能确定 metadata/type loss、超出文件范围。MySQL 环境缺失只阻塞 MySQL 实测，不自动阻断其他已授权组件工作；无 sudo/权限修改/生产库替代。

## Lane go-detect — rule-only classifier

模型：`openai-codex/gpt-5.6-sol`。

**精确只读参考路径**：
- `/home/haoyue/Project/dbmask/src/dbmask/detection/patterns.py`
- `/home/haoyue/Project/dbmask/src/dbmask/detection/pipeline.py`
- `/home/haoyue/Project/dbmask/src/dbmask/detection/overrides.py`
- `/home/haoyue/Project/dbmask/src/dbmask/dates.py`
- `/home/haoyue/Project/dbmask/src/dbmask/ssn.py`
- `/home/haoyue/Project/dbmask/tests/test_contextual_patterns.py`
- `/home/haoyue/Project/dbmask/tests/test_pattern_precedence.py`
- `/home/haoyue/Project/dbmask/tests/test_unknown_detection.py`
- `/home/haoyue/Project/dbmask/tests/test_date_detection.py`

**允许修改**：`internal/detect/**/*.go` 及其中测试。
**依赖**：Detector、DetectionRequest、Decision、Sensitivity/Source、Sample/Evidence/IssueCode、Value/Type。
**入口冻结**：`detect.New() contracts.Detector`；规则 immutable、无 package 全局可变数据。
**范围**：12类 M1 规则（email/url/IPv4/UUID/SSN/card/US ZIP/北美 phone/address/city/full_name/date），明确适用范围；人工/已审核结果/skip 优先级、distinct 非空样本阈值、冲突 UNKNOWN、无秘密输出。
city 若需字典，先用最小自写合成测试字典并向 Main 提真实词典来源建议；不能复制上游字典或自行增加数据依赖。
**非目标**：LLM、中国格式、历史文件处理、DB 抽样、持久化/并发引擎、全球号码或完整姓名识别承诺。
**必须测试**：接口断言；每规则正/负例，19/20 样本、90% 边界、纯数字无上下文、空/NULL、互相冲突、MDY/DMY、metadata 与值冲突、wrong-column overrides、取消、输入 DTO 未修改。
**完成标准**：规则覆盖与残余误报明示；UNKNOWN 从不变为 safe；所有策略与报告只含受控证据；测试纯内存，不等待 db 实现。
**停止条件**：需新字段/依赖/字典授权、上下文规则冲突无合同依据、要求外发样本。

## Lane go-mask — first strategy slice / deterministic memory mapping

模型：`openai-codex/gpt-6.1-sol`。

**精确只读参考路径**：
- `/home/haoyue/Project/dbmask/src/dbmask/masking/rules.py`
- `/home/haoyue/Project/dbmask/src/dbmask/masking/format.py`
- `/home/haoyue/Project/dbmask/src/dbmask/masking/engine.py`
- `/home/haoyue/Project/dbmask/src/dbmask/masking/seed_store.py`
- `/home/haoyue/Project/dbmask/tests/test_strategy_formats.py`
- `/home/haoyue/Project/dbmask/tests/test_masking_contracts.py`
- `/home/haoyue/Project/dbmask/tests/test_seed_map.py`

**允许修改**：`internal/mask/**/*.go` 及其中测试。
**依赖**：Strategy/StrategyRef、MaskContext、ColumnPlan/Type/Value、MappingKey/MappingReader/MappingWriter、安全错误。
**入口冻结**：`mask.NewStrategy(ref contracts.StrategyRef) (contracts.Strategy,error)`；`mask.NewMemoryMapping() *mask.MemoryMapping` 同时实现 MappingReader/MappingWriter；本地 MemoryMapping 是具体实现，不是新 DTO。
**范围**：首个有界切片为 `null/blank/redact/format_random/fake_email`，版本 v1；HMAC 派生确定性随机流、typed-lexical-v1、scope/keyID/version 隔离；并发安全内存 get-or-create。format_random 首轮只支持 Text/Bytes，其他 Kind 明确 ErrUnsupported，不伪装覆盖 typed 数值。
**非目标**：其他12策略、复制字典、持久 seed map/数据库 schema、事务引擎、跨库 exactly-once、密钥轮换迁移、中国数据/FPE、Python 输出逐字节兼容。
**必须测试**：Strategy 与两 mapping interface 断言、固定输入/上下文一致、改变 scope/version/key 不串用、NULL 与空串、Unicode/bytes、非空输入不得 unchanged、Type.Nullable/长度约束、未知策略/短 key 错误、上下文取消、并发唯一 winner；`go test -race ./internal/mask`。
**完成标准**：策略零 I/O，Lookup miss 不写入；无原值/secret 泄露；依赖不含 db；用自建合成值 golden tests，文档报告仅这5策略覆盖。
**停止条件**：需要 schema/DB/公共类型变更、要求所有17策略一次完成、无法满足输出约束而需产品决策。

## Lane go-verify — verifier against interfaces

模型：`openai-codex/gpt-5.6-sol`。

**精确只读参考路径**：
- `/home/haoyue/Project/dbmask/src/dbmask/validation/validator.py`
- `/home/haoyue/Project/dbmask/src/dbmask/validation/row_count.py`
- `/home/haoyue/Project/dbmask/src/dbmask/validation/schema_elements.py`
- `/home/haoyue/Project/dbmask/src/dbmask/validation/masking_completeness.py`
- `/home/haoyue/Project/dbmask/src/dbmask/validation/result.py`
- `/home/haoyue/Project/dbmask/tests/test_validation.py`
- `/home/haoyue/Project/dbmask/tests/test_validation_pk_aligned.py`
- `/home/haoyue/Project/dbmask/tests/test_scan_error_guard.py`

**允许修改**：`internal/verify/**/*.go`、`tests/**/*.go`，新自写纯合成 `tests/testdata/**`；不得修改 Main-owned internal/testenv/tools/scripts/pkg。
**依赖**：Validator、Reader、Plan、ValidateRequest、ValidationReport/Coverage/Issue、安全错误。
**入口冻结**：`verify.New() contracts.Validator`。
**范围**：用只读 Reader 验行数/schema/双向键集合/逐敏感值差异、明确覆盖范围、strict 失败规则；分页内存有界。使用 fake Reader 独立推进，不依赖尚未合并的 db.New/OpenReader。
**非目标**：读取配置、重新扫描目标推断应验证列、自动修复/写目标、无键 heuristic PASS、假设不同 DB collation 等价、性能可视化。
**必须测试**：接口断言；单字段漏改、源缺键/目标多键/行数相同键不同、缺表/能力未知、恰好 limit 与超过 limit、空表、无 PK、输入错误、context 取消、Reader 错误、不 mutation Plan、不泄露键和值；不同 collation 无法安全对齐时明确 ErrUnsupported，不用 Go lexical sort 误判。
**完成标准**：无完整检查不 PASS、未验证与失败区别明确；独立 fake tests 全覆盖；Main 合并后另跑 SQLite/MySQL 端到端，不把 fake tests 当实际适配验证。
**停止条件**：需要公共 DTO 改动、读能力不足、跨方言比较策略未定、需要改其他模块修复。

## 最终交接门禁

Main 等四个终态交接后做集成；不让任一 Worker独占所有成果。独立只读 go-reviewer 审核需下轮授权，当前不启动。
M1-B 组件完成不等于 M1 完成：runner/history/CLI、其他策略、持久映射和端到端安全门属于后续阶段。
