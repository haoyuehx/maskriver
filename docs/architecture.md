# Architecture — M1-A 契约基线与后续设计

## 状态与原则

当前已有 CLI 骨架、Main-owned `pkg/contracts` 与合成驱动测试环境；业务适配器/引擎仍待实现。正式冻结接口以 [contracts.md](contracts.md) 为准，不可据此宣称已支持数据库业务或流式处理。
MaskRiver 是原生 Go 设计：不沿用其他语言生态的对象模型或 ORM 抽象，直接基于 `database/sql` 与显式接口组合。
第一阶段优先可验证的串行有界分页；第二阶段才引入高性能并发与恢复。

```text
cmd/maskriver → config → runner
                         ├─ db: metadata / distinct sample / keyset pages / transaction
                         ├─ detect: overrides → reviewed history → rules → UNKNOWN
                         ├─ mask: immutable plan → typed strategy → durable mapping
                         ├─ verify: source snapshot ↔ target, explicit coverage
                         └─ history: reviewed decisions / audit (no raw values)
```

`pkg/contracts` 是 Main 独占的开发协作 API，不承诺稳定外部 SDK；已锁定两个数据库驱动供合成环境及后续 adapter，未引入 CLI 框架。

## 模块设计意图（具体冻结签名/依赖以 contracts.md 为准）

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

共享概念已放入 Main 独占 `pkg/contracts`：ColumnRef/Value/Decision/Plan/Report、数据库/算法/验证接口；详见 contracts.md。
依赖保持单向：runner 组合各模块；各组件只依赖 contracts，不横向导入具体实现。detect 接收 Main 注入的 metadata/sample/reviewed Decision，不自己查 db/history。

## 第一阶段执行模型

1. 只读发现元数据与 distinct 样本；默认至少 20 个非空样本、命中率阈值 90%，两者均可配置。
2. 人工覆盖 > 有效已审核历史 > 规则；冲突或证据不足为 UNKNOWN，不能作为安全字段自动放行。
3. 构建不可变 Plan；扫描错误阻断写入。UNKNOWN 必须人工处置或明确批准排除并报告，不能宣称全库安全。
4. Dry Run 仅生成有界、默认隐藏值的预览；不创建数据库、历史、映射或审计文件。读取已有映射使用真正只读连接。
5. Apply 在非生产副本按稳定不可变主键做 keyset pagination；先结束读游标再写，避免 SQLite 读写锁冲突。无键表及敏感 PK 不允许默默跳过并报告成功。
6. 批次事务保证该批原子性，非整库原子性；返回已提交批次范围，失败不能称全局回滚。暂不自动重试已部分提交的全表，防止二次脱敏。
7. 验证使用脱敏前源快照及固定 Plan，不重新扫描脱敏结果推断应验证哪些列。

SQLite/MySQL 计划各自实现 `database/sql` 适配。驱动已固定 modernc.org/sqlite v1.60.1（BSD-3-Clause）与 github.com/go-sql-driver/mysql v1.10.1（MPL-2.0），选择与约束见 database-drivers.md。仅 SQLite 合成 driver smoke 实际通过；MySQL 隔离实例权限受阻，业务 adapters 均未实现。

## 确定性与完整性

拟采用带类型规范化和版本化 scope 的 HMAC 指纹、稳定算法与持久映射。不承诺与任何第三方实现逐字节一致。
同输入只有在相同规范化、策略、scope/key 约束下才同输出；变更需版本化，不能静默重映射。
映射一致性不等于一一映射：唯一约束碰撞应显式失败；关联列必须共用 scope，PK/FK 变更不属于首版写入范围。
映射与目标若跨库，不假设分布式原子提交；第一阶段必须明确提交顺序及失败报告，不能声称已支持恢复。

验证覆盖行数、列/类型、PK、索引、FK、unique/check、主键集合双向差异、逐键逐敏感值变化、NULL/类型及计划规定的约束。
无键、抽样截断、缺失表/能力不足为 WARNING/ERROR，严格模式非零。所有键值默认遮蔽，测试不能只检查普通列泄露。

## 严格性约定（设计决策）

以下为 MaskRiver 自身确定的设计约束，不是对任何外部实现的对照：

- 验证报告不得包含原始主键或原始值；报告只携带受控状态、位置与计数。
- Dry Run 必须对文件系统与数据库都零持久写入，并用文件哈希快照加连接 spy 证明，而不是只统计写入行数。
- 校验目标由脱敏前固定的 Plan 决定，不通过重扫已脱敏数据来推断应校验哪些列。
- 覆盖不完整（无键、抽样截断、缺表、能力不足）一律不得 `PASS`；`strict` 模式对 `Warning` 也判失败。
- 这些约定当前仍属设计约束，尚未全部实现。

## 第二阶段

有界 channel、背压、确定性 worker pool、分区顺序与取消传播；内存随 batch×workers 增长而非总行数。
恢复清单包含源快照标识、schema/plan/策略版本、提交批次与 keyset checkpoint；先解决跨库映射提交一致性、幂等标记和重放，再宣称断点续跑。保留失败注入测试。
中国身份证/手机号/银行卡/统一社会信用代码/姓名地址识别与格式约束采用合成数据验证；不把格式保持称为密码学 FPE。
