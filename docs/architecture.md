# Architecture — 设计草案

## 状态与原则

当前仅 CLI/安全选项骨架，以下架构均待实现；不能据此宣称支持数据库或流式处理。
Go 独立实现参考 dbmask 的行为契约，不复刻 SQLAlchemy/Click/Python 对象模型。
第一阶段优先可验证的串行有界分页；第二阶段才引入高性能并发与恢复。

```text
cmd/maskriver → config → runner
                         ├─ db: metadata / distinct sample / keyset pages / transaction
                         ├─ detect: overrides → reviewed history → rules → UNKNOWN
                         ├─ mask: immutable plan → typed strategy → durable mapping
                         ├─ verify: source snapshot ↔ target, explicit coverage
                         └─ history: reviewed decisions / audit (no raw values)
```

`pkg/` 暂不发布 API；不提前引入框架或数据库驱动。

## 预定模块契约（语义草案，不是已存在的接口）

| 模块/Owner | 输入 → 输出 | 边界与错误语义 |
|---|---|---|
| config/Main | CLI + 后续配置 → validated options | 零值 Dry Run；Apply 只能来自 CLI；拒绝未知配置；秘密从环境/外部凭证注入 |
| db/go-db | context、连接选项、ColumnRef → metadata/sample/page | 读接口与事务写接口分离；参数化值，按方言引用标识符；返回能力标记及 errors |
| detect/go-detect | ColumnRef、类型、非空 distinct 样本、人工策略 → Decision | SENSITIVE/NOT_SENSITIVE/UNKNOWN；规则、来源、命中/样本数、阈值、冲突理由；不可持久化原值 |
| mask/go-mask | typed Value、StrategyID/version、scope/key → replacement | 保留 NULL/数值精度/时间语义；拒绝无效与未变化结果；纯策略不依赖连接 |
| mapping（mask 内） | scope/version + keyed fingerprint → typed replacement | 不存原值；明确 key 轮换、冲突、并发 get-or-create；存储适配可调用 db，不反向依赖 mask |
| verify/go-verify | 不变源快照、目标、原始 Plan → Report | PASS/FAIL/WARNING/ERROR、覆盖范围、计数、脱敏证据；严格模式对未覆盖也失败 |
| history/Main | 位置/类型/版本、审核身份 → 可复用 Decision | pending 不自动批准；过期/类型变化失效；UNKNOWN 不缓存为安全 |
| runner/Main | options、上述依赖 → RunReport | 唯一写入协调者；关闭资源、取消、错误传播；不得把部分完成汇总为成功 |

共享概念由 Main 在迁移前审定：ColumnRef = database/schema/table/column；Value 必须区分 NULL、空串、bytes、decimal 与时间；Decision 必须携带抽样口径；Plan 固定配置/策略版本、扫描覆盖和允许修改列；Report 不保存明文键。
依赖保持单向：runner 组合各模块，detect 可消费 db metadata 和 history；history 不反向依赖 detect 实现；基础 DTO 的归属在接口评审时确定，避免循环导入。

## 第一阶段执行模型

1. 只读发现元数据与 distinct 样本；默认至少 20 非空样本、90% 命中率作为上游兼容起点，可配置。
2. 人工覆盖 > 有效已审核历史 > 规则；冲突或证据不足为 UNKNOWN，不能作为安全字段自动放行。
3. 构建不可变 Plan；扫描错误阻断写入。UNKNOWN 必须人工处置或明确批准排除并报告，不能宣称全库安全。
4. Dry Run 仅生成有界、默认隐藏值的预览；不创建数据库、历史、映射或审计文件。读取已有映射使用真正只读连接。
5. Apply 在非生产副本按稳定不可变主键做 keyset pagination；先结束读游标再写，避免 SQLite 读写锁冲突。无键表及敏感 PK 不允许默默跳过并报告成功。
6. 批次事务保证该批原子性，非整库原子性；返回已提交批次范围，失败不能称全局回滚。暂不自动重试已部分提交的全表，防止二次脱敏。
7. 验证使用脱敏前源快照及固定 Plan，不重新扫描脱敏结果推断应验证哪些列。

SQLite/MySQL 各自实现 `database/sql` 适配。候选驱动为 modernc.org/sqlite 与 github.com/go-sql-driver/mysql，**尚未引入或定版**；评估纯 Go/CGO、许可、decimal、时区、collation、标识符及事务语义后决定。

## 确定性与完整性

拟采用带类型规范化和版本化 scope 的 HMAC 指纹、稳定算法与持久映射。不承诺与 Python 输出逐字节相同。
同输入只有在相同规范化、策略、scope/key 约束下才同输出；变更需版本化，不能静默重映射。
映射一致性不等于一一映射：唯一约束碰撞应显式失败；关联列必须共用 scope，PK/FK 变更不属于首版写入范围。
映射与目标若跨库，不假设分布式原子提交；第一阶段必须明确提交顺序及失败报告，不能声称已支持恢复。

验证覆盖行数、列/类型、PK、索引、FK、unique/check、主键集合双向差异、逐键逐敏感值变化、NULL/类型及计划规定的约束。
无键、抽样截断、缺失表/能力不足为 WARNING/ERROR，严格模式非零。所有键值默认遮蔽，测试不能只检查普通列泄露。

## 与上游实际代码的差异

基线见 README。`validation/masking_completeness.py` 的无键且无共同值分支返回 PASS，且 PK 有限覆盖也可 PASS；不能直接把 README 的严格性描述视作所有代码路径的保证。
上游报告可能含 `repr(row_key)`；Go 版禁止原始键出现在日志。
上游 `masking/engine.py` Dry Run 不记录新 pair，但 lazy seed store 会连接；Go 版要用文件系统/SQL spy 测试更强的“零持久写入”约束。
上游 `runner.py` 可根据目标重扫决定验证列，Go 版改用预先固定 Plan。
这些是设计修正点，非本轮已实现修复。

## 第二阶段

有界 channel、背压、确定性 worker pool、分区顺序与取消传播；内存随 batch×workers 增长而非总行数。
恢复清单包含源快照标识、schema/plan/策略版本、提交批次与 keyset checkpoint；先解决跨库映射提交一致性、幂等标记和重放，再宣称断点续跑。保留失败注入测试。
中国身份证/手机号/银行卡/统一社会信用代码/姓名地址识别与格式约束采用合成数据验证；不把格式保持称为密码学 FPE。
