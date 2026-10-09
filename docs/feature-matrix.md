# Feature Matrix

基于 dbmask `7d8789ef4883a423ddba2f8934b95997d1aaf099` 的源码、README 与测试静态分析，未执行上游测试。
下表“状态”只指 **MaskRiver 当前实现**，不是上游状态。

- **已实现**：有实际骨架代码及相应测试/构建证据。
- **部分实现**：有最小行为，但不具备完整业务能力。
- **计划中**：已排入 M1/M2，但没有对应业务实现。
- **未实现**：未纳入确定里程碑，需另行决策。

上游路径相对 `src/dbmask/`（测试路径另标）；M1 为第一阶段，M2 为第二阶段。

## M1-A 准备状态（不是业务功能）

| 项目 | 状态 | 证据/边界 |
|---|---|---|
| 共享 DTO/接口 m1a-v1 | 已实现 | pkg/contracts + 契约测试；不是完整引擎 |
| 两个 database/sql 驱动版本锁定 | 已实现 | go.mod/go.sum、database-drivers.md |
| SQLite 合成环境 | 已实现 | internal/testenv、tools/fixture；业务 adapter 未实现 |
| MySQL 隔离环境 | 部分实现 | 脚本与 opt-in 测试已准备；初始化 Permission denied，未通过 |
| 四 Worker 任务/隔离预检 | 部分实现 | 外部 Native 根已批准；实际 Main 预检证据见 closeout-report.md，单 go-db 模型冒烟仅设计，四 Worker 未启动 |

## 基础与连接

| 功能 | 上游依据 / 实际范围 | 状态 | 目标/Owner |
|---|---|---|---|
| 独立 Go module、可编译 CLI help/version | CLI 工作流参考 cli.py；Go 骨架为新写 | 已实现 | M0/Main |
| scan/mask/validate CLI | cli.py：scan、mask、validate | 部分实现 | 仅命令占位、退出 2；M1/Main |
| 默认 Dry Run，--apply 单一写意图 | cli.py；tests/test_cli_mask_safety.py | 部分实现 | 当前零值 Dry Run、所有 DB 操作拒绝；DB 级零副作用待测；M1/Main |
| YAML 配置、环境变量展开、参数校验 | config.py | 计划中 | M1/Main |
| 连接生命周期、schema/table/column/type/PK 元数据 | connectors/base.py, sql.py | 计划中 | M1/go-db |
| distinct 非空抽样、表/列过滤 | connectors/sql.py, detection/pipeline.py | 计划中 | M1/go-db + go-detect |
| SQLite 连接与真实集成 | SQLAlchemy 路径，上游主要 pytest 覆盖 | 计划中 | M1/go-db |
| MySQL 连接与真实集成 | SQLAlchemy 方言路径；上游 README 不承诺完整集成覆盖 | 计划中 | M1/go-db |
| MariaDB/PostgreSQL/SQL Server/Oracle | SQLAlchemy 方言可接入，不等于每种已完整验证 | 未实现 | 后续单独适配/测试 |
| 参数化更新、批次事务、复合键 keyset 分页 | connectors/sql.py, masking/engine.py | 计划中 | M1/go-db |
| 无主键 apply 拒绝、敏感 PK 明确未处理 | masking/engine.py, connectors/sql.py | 计划中 | M1/Main + go-db |

## 检测

| 功能 | 上游依据 / 细节 | 状态 | 目标/Owner |
|---|---|---|---|
| 人工字段覆盖、优先级和 skip | detection/overrides.py, pipeline.py | 计划中 | M1/go-detect |
| 审核历史优先、类型/过期检查 | history/records.py, detection/pipeline.py | 计划中 | M1/Main |
| 规则证据、阈值、元数据上下文、冲突 UNKNOWN | detection/patterns.py；默认 20 样本、90%，非加权赢家 | 计划中 | M1/go-detect |
| email | detection/patterns.py：格式/长度检查 | 计划中 | M1/go-detect |
| url | 同上：http(s)/www 形式 | 计划中 | M1/go-detect |
| ip_address | 同上：检测实际为 IPv4，不能宣称已有 IPv6 检测 | 计划中 | M1/go-detect |
| uuid | 同上：UUID 字符串形状 | 计划中 | M1/go-detect |
| ssn | 同上 + ssn.py：受约束 SSN/部分遮蔽形式及上下文 | 计划中 | M1/go-detect |
| credit_card | 同上：Luhn + 支持的品牌/长度 + 上下文 | 计划中 | M1/go-detect |
| zip_code | 同上：US ZIP/ZIP+4 + 上下文 | 计划中 | M1/go-detect |
| phone | 同上：北美格式/扩展号，不是全球号码识别 | 计划中 | M1/go-detect |
| address | 同上：英文街道模式 | 计划中 | M1/go-detect |
| city | 同上：US 字典成员与列上下文 | 计划中 | M1/go-detect |
| full_name | 同上：姓名字符与列上下文，含 first/last name hints | 计划中 | M1/go-detect |
| date | 同上 + dates.py：日历有效性、MDY/DMY、多种格式 | 计划中 | M1/go-detect |
| UNKNOWN 不当作非敏感、不持久化为安全 | detection/pipeline.py；空样本/冲突/不确定 | 计划中 | M1/go-detect |
| scan 错误 fail-closed / --allow-partial | runner.py, cli.py | 计划中 | M1/Main；逃生选项须单独安全评审 |
| 可选 OpenAI/兼容端点、本地 LLM | llm/*；默认关闭、metadata-only、token budget、外发警告 | 未实现 | 不在 M1/M2 承诺中 |

## 脱敏策略（上游完整注册表：17 项）

所有策略依据 `masking/rules.py`；当前无任何算法实现。

| 策略 | 上游语义 | 状态 | 目标/Owner |
|---|---|---|---|
| null | SQL NULL | 计划中 | M1/go-mask |
| blank | 空串 | 计划中 | M1/go-mask |
| redact | 遮蔽内容、保留分隔符 | 计划中 | M1/go-mask |
| format_random | 字符类别/长度保持，typed 值专用分支 | 计划中 | M1/go-mask |
| shuffle | 字符重排，typed 值专用分支 | 计划中 | M1/go-mask |
| fake_name | 姓名词典 | 计划中 | M1/go-mask |
| fake_first_name | 名词典 | 计划中 | M1/go-mask |
| fake_last_name | 姓词典 | 计划中 | M1/go-mask |
| fake_city | US 城市词典 | 计划中 | M1/go-mask |
| fake_email | example.invalid 域名 | 计划中 | M1/go-mask |
| fake_email_keep_domain | 原域保留，可能可识别，须显式选择 | 计划中 | M1/go-mask |
| fake_uuid | 确定性 v4 UUID | 计划中 | M1/go-mask |
| fake_ip | IPv4/IPv6 输出分支；不等于 IPv6 检测 | 计划中 | M1/go-mask |
| fake_credit_card | 品牌/长度/分隔符与 Luhn 有效 | 计划中 | M1/go-mask |
| fake_date | 合法日历日期偏移，MDY/DMY 语义 | 计划中 | M1/go-mask |
| fake_phone | 受约束号码格式 | 计划中 | M1/go-mask |
| fake_ssn | 受约束 SSN 格式 | 计划中 | M1/go-mask |

## 映射、安全、历史、验证与接口

| 功能 | 上游依据 / 实际边界 | 状态 | 目标/Owner |
|---|---|---|---|
| 策略优先级及未知策略拒绝 | masking/engine.py：审核策略 > 列策略 > rule mapping > 默认 | 计划中 | M1/go-mask |
| typed 输出、大小写/分隔符、未变化输出拒绝 | masking/format.py, rules.py, engine.py | 计划中 | M1/go-mask |
| 显式列级 NULL placeholder | masking/engine.py, config.py | 计划中 | M1/go-mask |
| 可注册策略/字典、词典加载 | masking/rules.py, dictionaries/__init__.py | 计划中 | M1/go-mask；词典来源另审 |
| seeded 确定性、跨表同 scope 一致 | masking/format.py | 计划中 | M1/go-mask；不承诺 Python 位级兼容 |
| 持久 seed map、加盐指纹、不存原值、缓存与重用 | masking/seed_store.py | 计划中 | M1/go-mask |
| dry run 不记录 pair，复用已有映射 | masking/engine.py；lazy connect 仍需额外审查 | 计划中 | M1/go-mask；Go 要求零持久副作用 |
| 默认隐藏预览值、显式 show-values | cli.py | 计划中 | M1/Main；原值显示开关另审，不默认实现 |
| history：pending/approved、身份、过期、版本、审计 | history/records.py, store.py | 计划中 | M1/Main |
| history-import/export CSV | history/files.py, cli.py | 计划中 | M1/Main |
| XLSX/Markdown 导入导出、source_file 权威源 | history/files.py, file_store.py | 未实现 | 后续增量 |
| history-writeback、备份、baseline 冲突检测 | history/writeback.py, cli.py | 未实现 | 后续增量 |
| history/seeds/strategies 查询 CLI | cli.py | 计划中 | M1/Main |
| JSON 扫描/验证/历史报告与退出码 | cli.py | 计划中 | M1/Main |
| 行数验证 | validation/row_count.py | 计划中 | M1/go-verify |
| schema：列类型/PK/索引/FK/unique/check | validation/schema_elements.py | 计划中 | M1/go-verify |
| triggers/grants 验证 | SQL connector 未采集，validator 实际比较列表未包含 | 未实现 | 后续能力评估，不宣称兼容 |
| PK 对齐逐值完整性验证 | validation/masking_completeness.py | 计划中 | M1/go-verify；增加双向键差异 |
| 无键 heuristic、限量覆盖、strict 模式 | 同上 + result.py；部分路径仍 PASS，见 architecture | 计划中 | M1/go-verify；Go 严格门禁更保守 |
| 测试数据忽略、采样上限、形状遮蔽 | validation/testdata.py, config.py | 计划中 | M1/go-verify；忽略策略须显式审核 |
| 稳定公共库 API | runner.py 可调用；上游稳定 API 仍属路线图 | 未实现 | pkg/contracts 仅内部协作开发冻结，非稳定外部 SDK |
| Python 包/PyPI/Colab demo | 上游发布与演示路径 | 未实现 | 不移植语言专用渠道，后续 Go release/demo |

## 新增研发目标（非上游已交付能力）

| 功能 | 状态 | 目标 |
|---|---|---|
| 中国身份证/手机号/银行卡/统一社会信用代码/姓名地址 | 计划中 | M2：检测与格式约束，合成样本评估 |
| 高性能有界流式并发、背压、取消 | 计划中 | M2；M1 仅正确有界分页，不冒充高性能并发 |
| 故障恢复、版本化 checkpoint、幂等重放 | 计划中 | M2；先解决事务提交/映射一致性 |
| precision/recall 与吞吐/内存基准、可视化 | 计划中 | M2；尚无成绩 |
| OpenMetadata/DataHub 治理导出 | 未实现 | 上游路线图，不在当前承诺内 |

任何状态升级必须附可复现测试与文档更新。只创建包、类型、配置或通过空包编译不能算业务功能已实现。
