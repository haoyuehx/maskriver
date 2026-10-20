# Issue #13 Owner 决策包 — 严格内存 AC、审计保证与实施授权

**文件定位：** 供 Issue Owner 做决策，不是实现方案，也不是批准记录。ADR 全文与证据在
[`issue13-architecture-decision-record.md`](issue13-architecture-decision-record.md)（下称 ADR）和
[`issue13-bounded-receipts-design.md`](issue13-bounded-receipts-design.md)（下称 Design）。

**当前状态：** `DECIDED — DESIGN DIRECTION RECORDED`（2026-10-20）。ADR-01..07/09/10/11 已按
[`issue13-owner-direction-2026-10-20.md`](issue13-owner-direction-2026-10-20.md) 记录决定；ADR-08 仍为
`PENDING MEASUREMENT AND OWNER APPROVAL`。仅 WP1（新增版本化契约类型与测试）获得有限授权；生产代码、
冻结契约语义、事务行为、CLI 写入授权、BatchSize 默认值、M1 Release 均未改变。Issue #13 保持 OPEN，
#14 未启动。决策来源是仓库所有者对本轮 AI 技术负责人的委托记录，**不是 GitHub 签名**。

**证据边界：** 本包中的所有架构描述都是设计，不是已实现或已验证的能力。真实代码事实只有两类：
(a) 现有实现的复杂度/行为推导（见 ADR "Current memory and receipt facts"），(b) Phase-5 的 42 次真实
SQLite Runner 试验（[`docs/perf/issue13-phase5-20261019-final/summary.md`](../perf/issue13-phase5-20261019-final/summary.md)）。
Outbox、Anchor、Seal、Verifier、平台 durability 均 **未实现、未实测**。

---

## 1. 一页执行摘要

**#13 为什么还不能关闭？**
原始 AC 要求 "memory growth is bounded by batch × workers"。当前 `Apply` 返回
`ApplyReport.Committed []BatchReceipt`，包含每个成功批次一条回执；Phase-5 实测其 backing capacity 随
批次数从 0.019 MiB（10k 行）增至 1.102 MiB（1M 行）。该返回值的具体表示需要 Ω(K) 个元素，
K 在稠密可写数据上为 Θ(N/B)。因此"全部 receipts 一次性返回"与"内存只受 batch × workers 约束"
在保持现有结果语义时不可兼得。此外审批目录扫描为 O(H)，Plan/列/元数据、行 payload、driver/native
cache、Go runtime 也未纳入固定上界。**所以严格 AC 未通过，不是测量不足，而是契约级冲突 + 尚未实现的
有界架构。**

**当前真正的功能阻断是什么？**
不是 worker pool（PR #36 已交付有界、确定性、可取消的页内并发）。阻断点是**返回路径**：
(1) 完整 receipt 切片 Θ(K)；(2) 缺少一个有界内存的流式交付接口。
**崩溃一致性与篡改检测不是原始内存 AC 的组成部分**：若 Owner 额外要求“提交数据与持久回执同成败”，
才需要 A3 outbox；若还要求“事后可检测篡改”，才需要 Anchor/Seal。这三层必须分别决策。

**为什么提出新流式 API？**
新 API 可以在每批提交时同步写出一条回执（Q=0，不累积），只返回小摘要 + 可分页读取的审计句柄，
从而让应用 heap 中的回执保留与 K 无关。旧 API 保持不变，避免破坏 `m1a-v1` 调用方。

**为什么旧 API 不能直接获得严格内存保证？**
`ApplyReport.Committed` 是已冻结的公共返回结构。调用者在返回时可观察到全部 K 条回执；任何"压缩/预分配/
移到别的 heap"都只是常数优化，不能改变 Ω(K) 表示下界。要满足 S1 必须改变返回语义（cursor/分页/退场），
这是契约修改，不是实现优化。

**Outbox 与 Anchor 是否都是必须的？**
不是同一层需求：
- **同库事务 Outbox（A3）** 解决"业务已提交但回执丢失"的崩溃窗口，是"每批 receipt 与数据同成败"这一
  保证的最小机制；如果不要求崩溃一致性，可用更弱的 A1/A2，但必须明确撤回"完整审计"声明。
- **独立 Anchor/Seal** 解决"主动篡改/整段删除/回滚"的检测。它对**数据正确性**不是必需的；只有当你要求
  "对已登记 run 的事后篡改可检测"时才必需。它引入新的外部基础设施、密钥托管和可用性代价。
- 因此：**A3 是 receipt 完整性所需；Anchor 是篡改证据所需。** 两者都不是当前已批准的需求，且 Anchor
  的威胁边界必须由 Owner 明确（ADR-11）。

**哪些风险已由设计解决？**
在设计层面：批次 receipt 与业务写入的原子性（A3 同库同事务）、CommitUnknown 不重试且保持 UNKNOWN、
缺 anchor ack / 缺 seal 只能是不完整、Verifier 结果互斥优先级、`COMPLETE_VALID` 必须要求 `COMPLETE`
seal、拒绝任意 Consumer 回调、不引入恢复/重放语义、保留全部既有审批与写入门禁。

**哪些还需要实现验证？**
SQLite/MySQL 同事务 outbox 是否真实成立、journal/synchronous 与断电 durability、Windows/Linux 路径
身份与 TOCTOU、短写/fsync/ENOSPC、CommitUnknown 的真实分类、anchor 不可用行为、篡改检测的实际覆盖、
以及所有资源上限的实测。以上全部为 **IMPLEMENTATION UNVERIFIED / TEST REQUIRED**。

---

## 2. 原始 #13 范围 vs 解决 AC 所需工作 vs 新增产品能力

原始 Issue 文本以仓库外 GitHub Issue 为准（Main 于 2026-10-20 读取；ADR 逐字引用）。本包不修改原始 AC。

### B1 — 原始必须满足（核心 AC）

| 能力 | Issue 依据 | 当前状态 | 核心 AC | 安全前置 | 可否独立延期 | 延期后允许的声明 / 必须撤回的声明 |
|---|---|---|---|---|---|---|
| 有界并发、确定性分区、取消传播 | Scope 第 1 条；PR #36 已交付 | 已实现并有测试/CI | 是 | 是 | 否（已具备） | 可声明 worker pool 有界；不可外推到整体 Apply 内存 |
| 内存受 batch × workers 约束 | Acceptance "功能" 第一条 | **未满足**：receipt Θ(K)，另有 O(H) 等项 | 是 | 是 | 否 | 只能声明"页内工作集"有界；不得声明 Apply 总内存有界 |
| race / fuzz / 泄漏 / 取消故障注入 | Acceptance "测试" | 部分已有（PR #36）；新路径未覆盖 | 是 | 是 | 否 | 可声明既有组件证据；不得声明新架构已验证 |
| 真实规模性能基准（命令/环境/重复/分布） | Acceptance "文档" + "最终交付证据" | Phase-5 有 42 次真实 SQLite Runner 试验；1M 仅单次、非 CLI | 是 | 否 | 否 | 可引用 Phase-5 描述性数字；不得称生产吞吐、p95 或统计显著 |
| 并发正确性证据（含跨 worker 一致） | Acceptance "最终交付证据" | Phase-5 已验证跨 worker 摘要一致 | 是 | 是 | 否 | 可声明该 fixture 下的正确性 |
| 无原值/密钥泄露 | Acceptance "安全性" | 现有证据仅合成 `.invalid` 数据 | 是 | 是 | 否 | 可声明证据不含原值；不可外推 PII 结论 |
| Feature Matrix 只按实测更新 | Acceptance "文档" | 已记录 phase-5 且明确未关闭 #13 | 是 | 否 | 否 | — |

### B2 — 解决原始 AC 所必需的设计工作

| 工作 | 是否核心 AC | 是否安全前置 | 可否独立延期 | 说明 |
|---|---|---|---|---|
| 有界 receipt 返回路径（streaming + 分页读取） | 是（唯一能同时保留完整审计与有界 heap 的方向） | 是 | 否，但**是否适用于旧 API 需 Owner 决定** | 旧 API 保持 O(K)，见 S1/S2/S3 |
| 审批历史扫描 O(H) 的固定 chunk 枚举与 H/F 上限 | 是（否则 N 无关仍不成立） | 是 | 否 | 属实现细节，但数值上限需批准 |
| 资源预算（Lrow/B/W/C/T/H 数值） | 是（否则无法宣称字节上界） | 是 | 数值可在实施前批准 | 当前只有结构上界与 PROPOSED 数值 |
| 新旧 API / Plan / Approval 兼容策略 | 是（否则无法在不破坏 m1a 的前提下交付新路径） | 是 | 否 | 见 ADR-09 |
| 持久化 receipt 的错误与崩溃语义（CommitUnknown 等） | **否**（原始 AC 只要求内存增长有界；仅当 Owner 额外选择 A3/崩溃一致 receipt 时才成为前置） | 是（条件性） | 是 | 不引入恢复/重放；延期即撤回“提交与回执同成败”声明 |

### B3 — 可能是超出原始 #13 的新产品能力

| 能力 | 性质 | 可否独立延期 | 延期后必须撤回的声明 |
|---|---|---|---|
| 独立审计服务 / 可枚举 catalog | A3+Anchor 方案引入的新基础设施 | 是（若只要求 receipt 完整性） | 撤回"整 run 删除可发现" |
| 外部 Trust Anchor / 签名 Seal | 篡改检测所需，非 #13 原文要求 | 是 | 撤回"主动篡改可检测" |
| 长期审计保留 / 保留策略 | 运维能力 | 是 | 撤回"长期可审计" |
| 通用跨数据库事务 Outbox | A3 的泛化版本 | 是（先支持已测后端即可） | 撤回"任意后端可用" |
| 自动故障恢复 / Checkpoint / Resume / Exactly-once | 属 #14 | 是（且当前未授权） | 撤回任何"可恢复/幂等"表述 |

**结论：** A3 是"完整 receipt"承诺的依赖；Anchor/Seal/catalog 是"篡改证据"承诺的依赖，不是 #13 原文
必需。若 Owner 选择不要求篡改证据，则必须同时接受并写明"无法检测主动改写/删除"，而不能默认拥有该项保证。

---

## 3. 内存 AC 的三个选项（S1/S2/S3）

| 维度 | S1 所有 API 严格有界 | S2 保留旧 API + 新 Streaming API | S3 保留旧 API + 最大批次数 Kmax |
|---|---|---|---|
| 与原始 AC 文字 | 字面满足 | 仅新路径满足；**改变 AC 适用范围，需 Owner 明确批准** | 不满足（有界上限 ≠ batch×workers 复杂度） |
| 旧 API 兼容 | 破坏性（返回语义改变） | 完全保留 `m1a-v1` | 保留，但超限任务被拒 |
| 既有调用方迁移 | 必须迁移 | 无需迁移 | 大任务需改写或分片 |
| 旧 Plan/审批 | 需重算/重签（revision 参与 Plan.ID） | 旧 Plan 继续有效；新路径需独立 approval binding | 旧 Plan 有效 |
| 冻结契约修改成本 | 高（破坏性） | 中（新增并行 surface，不改全局 revision） | 低（仅资源门禁） |
| M1 稳定性风险 | 高 | 低-中 | 低 |
| 大表可扩展性 | 可（但需新语义） | 可（新路径） | **牺牲** >Kmax·B 行 |
| 能否单独关闭 #13 内存项 | 是（若实现并验证） | 仅当 Owner 将 AC 限定到新路径 | 否 |
| 当前是否可实施 | 否（违反冻结契约，无授权） | 否（需契约+实现授权） | 否（需资源门禁授权） |
| 相对复杂度 | VERY HIGH | HIGH | LOW |

**决策依据（2026-10-20 已选 S2；不是新批准）：** 若 Owner 接受"原始 AC 覆盖所有 Apply API"的字面解释，则唯一诚实结论是
**#13 内存项在 `m1a-v1` 下不可满足**，只能通过契约修订（S1）解决；若 Owner 明确把 #13 的内存 AC 限定为
"新的有界端到端路径"，则 S2 是兼容代价最低的方案，但**必须在 Issue 与文档中显式记录范围收窄**。
S3 只能作为兼容性资源护栏，不能作为 AC 通过依据。

> **决策（2026-10-20，委托方向）：** 选择 **S2** —— 新流式路径承担严格上界，旧 API 保留 O(K) 并在
> Issue #13 中显式记录范围修订。此处保留 S1/S2/S3 比较作为决策依据，不再表示待决。

---

## 4. 审计保证 A1/A2/A3 与 Anchor 必要性

| 维度 | A1 Best-effort | A2 Durable Reconciliation | A3 Same-transaction Outbox |
|---|---|---|---|
| 每个成功 Commit 对应同事务 receipt（durability 需另行验证） | 否 | 可最终判定，但存在中间态 | 是（同库同事务；不等于断电 durability） |
| CommitUnknown | 已提交但可能无回执 | 可靠记录 attempt，事后按权威库判定 | 数据与 outbox 同成败；仍可能 UNKNOWN |
| 崩溃后可验证内容 | 仅已落盘部分 | attempt + 权威库状态 | 已提交批次的回执与数据一致 |
| 必须保持 UNKNOWN 的内容 | 未知批次 | 权威库不可用/证据不一致时 | 同左 |
| 允许部分提交 | 是（且无完整审计） | 是（可识别） | 是（可识别且每批 receipt 一致） |
| 安全重试 | 否 | 否（reconciliation ≠ replay） | 否 |
| 需要新表 | 否（外部 sink） | 可能需要 | 是（目标库预置 outbox） |
| 需要独立审计服务 | 否 | 否 | 否（Anchor 才是独立服务） |
| 相对实现/维护成本 | LOW | MEDIUM | HIGH |
| 能否支持"完整审计"声明 | 否 | 有条件（需定义 reconciliation 语义） | 是（针对本地 DB 完整性） |

### 保证矩阵（必须分开理解，不可混为一谈）

| 保证 | A1 | A2 | A3 | A3+Anchor/Seal |
|---|---|---|---|---|
| 已确认提交的正确报告 | 部分（可能丢回执） | 是（事后） | 是 | 是 |
| 持久化审计完整性（本机 DB） | 否 | 有条件 | 是（事务原子） | 是 |
| 意外崩溃后的可检查性 | 低 | 中 | 高（每批一致） | 高 |
| 对主动篡改的检测 | 否 | 否 | 否（本机 DB 可被改写） | 是（登记后、目录内） |
| 整个 Run 被删除后的发现能力 | 否 | 否 | 否 | 是（独立 catalog 对比） |
| Host/Root 被攻破后的保护 | 否 | 否 | 否 | **否**（ADR-11 明确排除） |
| 外部 Anchor 不可用时 | 不适用 | 不适用 | 不适用 | `AUDIT_INCOMPLETE`，绝不成功 |

**是否真正需要独立 Anchor？** 若 Owner 只要求"数据正确 + 本机 receipt 与数据同成败"，A3 足够，不需要
Anchor。若 Owner 要求"事后能发现有人改写/删除审计记录"，Anchor + 独立 catalog + Seal 是必要条件，
且必须接受其威胁边界（不覆盖预登记改写、Host/Runner 被控、Anchor 自身回滚）。**不能"为了简单"删掉
Anchor 却继续声明篡改检测。**

**决策（2026-10-20）：** A3 作为 receipt 完整性基线；Anchor 作为可独立延期的增强项（WP4，DEFERRED），
与 #13 核心 AC 解耦。

---

## 5. ADR-01..ADR-11 Owner 决策表

分组依据：**第一类**决定会改变 #13 的验收含义；**第二类**依赖第一类；**第三类**属实施前批准。
除 ADR-08 外，"Owner 决议"栏记录 2026-10-20 的委托方向（[决策记录](issue13-owner-direction-2026-10-20.md)），
不是 GitHub 签名，也不构成实现批准。

### 第一类 — 必须优先决定（决定 #13 能否关闭及以何种含义关闭）

| ID | 决策问题 | 推荐（工程建议，未批准） | 备选 | 安全/兼容影响 | 前置依赖 | Owner 决议 | 决议依据 | 状态 |
|---|---|---|---|---|---|---|---|---|
| ADR-01 | 严格内存 AC 是否覆盖所有 Apply API？S1/S2/S3 | S2（显式范围收窄）或明确"S1 需要契约修订" | S1 / S3 / 保持 OPEN | 决定 #13 是否能关闭；S2 需公开承认旧 API 仍 O(K) | 无 | S2：新流式路径承担严格上界 | 2026-10-20 所有者委托记录 | **DECIDED — S2** |
| ADR-02 | 旧 API 是否允许保留 O(K)？ | 保留并在文档/CLI 标注 O(K) | 上限/退役 | 破坏 vs 诚实标注 | ADR-01 | 保留旧 API，标注 O(K) | 2026-10-20 所有者委托记录 | **DECIDED — legacy retained** |
| ADR-03 | 审计保证级别 A1/A2/A3（+Anchor 与否） | A3 为 receipt 基线；Anchor 独立可选 | A1 / A2 / A3+Anchor | 决定"完整审计/篡改检测"能否声明 | ADR-01 | 新路径 A3（同库同事务 outbox） | 2026-10-20 所有者委托记录 | **DECIDED — A3** |
| ADR-11 | 审计威胁模型（预登记改写、Host/Runner 被控、Anchor 回滚） | 仅承诺"登记后、目录内、可信 Runner/Anchor 边界" | 更强独立准入/attestation | 决定是否必须建设新基础设施 | ADR-03 | 可信本地边界；不承诺主动攻击防护 | 2026-10-20 所有者委托记录 | **DECIDED — trusted local boundary** |

### 第二类 — 依赖第一类决定

| ID | 决策问题 | 推荐 | 备选 | 安全/兼容影响 | 前置依赖 | Owner 决议 | 决议依据 | 状态 |
|---|---|---|---|---|---|---|---|---|
| ADR-04 | 目标 DB 是否可有私有 outbox 表？ | 预置、版本化、无运行时 DDL | 无 schema 变更 / 独立库 | 目标 schema 变更；需迁移/权限/保留策略 | ADR-03=A3/A2 | 允许预置专用 outbox schema | 2026-10-20 所有者委托记录 | **DECIDED — pre-provisioned outbox** |
| ADR-05 | 是否要求独立持久审计 Anchor？ | 仅当要求篡改证据时要求 | 仅本地 | 引入外部服务、密钥托管、可用性代价 | ADR-03, ADR-11 | 延期，不属于 #13 必需 | 2026-10-20 所有者委托记录 | **DECIDED — deferred** |
| ADR-06 | 是否允许第三方/任意 Consumer？ | 不允许；仅内部同步 outbox | 隔离进程/可信 sink | 任意回调可阻塞/重入/泄露 | ADR-03 | 仅受控内部 sink，无任意回调 | 2026-10-20 所有者委托记录 | **DECIDED — internal sink only** |
| ADR-09 | Plan/Approval/API revision 兼容策略 | 保留 `m1a-v1`，新增并行版本化 surface | 全局 revision bump / 破坏旧审批 | 全局 bump 会使旧 Plan/审批失效 | ADR-01, ADR-02 | 并行版本化契约，m1a-v1 不变 | 2026-10-20 所有者委托记录 | **DECIDED — parallel versioning** |

### 第三类 — 实施前批准（不应与基础决策捆绑）

| ID | 决策问题 | 推荐 | 备选 | 安全/兼容影响 | 前置依赖 | Owner 决议 | 决议依据 | 状态 |
|---|---|---|---|---|---|---|---|---|
| ADR-07 | 哪些平台/存储必须提供 durable 语义？ | 先仅支持已测本地 SQLite（rollback-journal + 已验证 sync）；MySQL 验证后加入 | Linux only / +Windows / MySQL too | 未测组合必须 fail closed | ADR-03, ADR-04 | Linux 本地 SQLite 优先；Windows streaming UNSUPPORTED；MySQL→WP2b | 2026-10-20 所有者委托记录 | **DECIDED — scoped** |
| ADR-08 | 数值资源上限（B/W/Lrow/C/T/H/F/元数据） | 先定义符号预算，数值在测量后批准 | 直接设固定上限 / 不设上限 | 无上限则无法宣称字节界 | ADR-01 | 数值未批准；WP1 仅定义字段与校验 | 2026-10-20 所有者委托记录 | **PENDING MEASUREMENT** |
| ADR-10 | 下一步允许修改哪些生产模块？ | 当前无；批准后按 WP 分文件授权 | 仅 contracts / runner+DB / 跨模块 | 越界修改会破坏安全不变量 | ADR-01..09 | 仅 WP1 有限授权；WP2/2b/3/4 未授权 | 2026-10-20 所有者委托记录 | **DECIDED — WP1 only** |

> **注意：** ADR-01/03/11 已于 2026-10-20 记录方向；ADR-07/10 已随之确定，ADR-08 仍待测量批准。
> 本表保留比较依据，不表示这些项仍在待决。

---

## 6. 最小安全实施路径比较

| 路径 | 满足的保证 | 无法满足的保证 | 涉及模块 | 冻结契约 | 额外基础设施 | 依赖 #14 | 能否在原 #13 范围内完成 | 相对复杂度 |
|---|---|---|---|---|---|---|---|---|
| P1 只解决 receipt 内存（新 API 边提交边丢弃/分页） | 应用 heap 回执不随 K 增长；页内工作集有界 | 崩溃后 receipt 与数据一致性；篡改检测；完整字节上界 | contracts（新 surface）、runner、cli、verify | 是（新增并行 surface） | 无 | 否 | 仅当 Owner 接受 S2 | HIGH |
| P2 Streaming API + 保留旧 API（P1 + 资源门禁/审批目录界限） | P1 + 可声称的限定工作集上界 | 崩溃一致性；篡改检测 | 同上 + history 读取、config 上限 | 是 | 无 | 否 | 仅当 Owner 接受 S2 | HIGH |
| P3 P2 + 目标库同事务 Outbox（A3） | P1/P2 + 每批 receipt 与数据同成败；可识别部分提交 | 主动篡改/整段删除检测 | 同上 + db writer/schema/migration | 是 | 目标 schema 变更 | 否 | 是（A3 属 receipt 完整性） | VERY HIGH |
| P4 P3 + 独立 Anchor/Seal/Verifier | P3 + 登记后篡改证据、Run 删除发现 | Host/Root 被控、预登记改写、Anchor 回滚 | 同上 + 新审计服务/密钥/运维 | 是 | **是**（外部服务） | 否 | 技术上可，但属新增产品能力 | VERY HIGH |

**路径决定（2026-10-20）：** 已选择 S2 + A3 且不要求篡改证据，因此 **P3 是本轮方向下的最小诚实路径**；
P4 作为独立后续能力（WP4，DEFERRED），不并入 #13。P1/P2 不足以提供 receipt 与数据同成败，P4 超出当前威胁模型。
本表是决策依据记录，不是实施授权：WP2/WP3 仍为 `NOT AUTHORIZED`。

---

## 7. Security P2 的实现门禁（未来实施必须满足）

当前剩余 Security P2：目标存储路径、平台支持与 durability 未通过真实实现验证。以下不是待办清单的
"已解决项"，而是**实施前必须满足或显式声明 UNSUPPORTED 的门禁**。

| # | 门禁 | 属性 |
|---|---|---|
| G1 | 同库同事务：业务 UPDATE 与 outbox INSERT 在同一 `sql.Tx`、同一目标数据库 | DESIGN SPECIFIED / IMPLEMENTATION UNVERIFIED / TEST REQUIRED |
| G2 | SQLite journal 模式边界：仅支持已验证的 rollback-journal（DELETE/TRUNCATE/PERSIST）；WAL/memory/off/未知模式 | DESIGN SPECIFIED / OWNER APPROVAL REQUIRED / TEST REQUIRED |
| G3 | SQLite synchronous 与 durability 设置必须显式验证，不得由 `Commit()==nil` 推断持久 | DESIGN SPECIFIED / IMPLEMENTATION UNVERIFIED / TEST REQUIRED |
| G4 | Windows 与 Linux 的文件身份、reparse point/symlink、ACL、锁与 fsync 语义差异 | DESIGN SPECIFIED / UNSUPPORTED UNTIL TESTED |
| G5 | 目标路径别名、符号链接、硬链接、TOCTOU：当前 stat-then-open 未固定句柄 | DESIGN SPECIFIED / IMPLEMENTATION UNVERIFIED / TEST REQUIRED |
| G6 | 文件权限、私有目录、no-clobber、source/target 身份不同 | DESIGN SPECIFIED / TEST REQUIRED |
| G7 | 短写、fsync 失败、ENOSPC/配额耗尽的行为（回滚或 CommitUnknown，绝不部分成功） | DESIGN SPECIFIED / TEST REQUIRED |
| G8 | CommitUnknown 分类：仅在底层 Commit 尝试后产生；不重试、不猜测 | DESIGN SPECIFIED / IMPLEMENTATION UNVERIFIED / TEST REQUIRED |
| G9 | 崩溃/重启后的状态分类（UNKNOWN / INCOMPLETE / TRUNCATED / TAMPERED） | DESIGN SPECIFIED / TEST REQUIRED |
| G10 | 独立 Anchor 不可用：`ANCHOR_PENDING/AUDIT_INCOMPLETE`，停止后续写入 | DESIGN SPECIFIED / OWNER APPROVAL REQUIRED / IMPLEMENTATION UNVERIFIED |
| G11 | 篡改检测范围：仅登记后、catalog 内、可信 Runner/Anchor 边界；不含 Host/Root | DESIGN SPECIFIED / OWNER APPROVAL REQUIRED |
| G12 | MySQL：同库、事务引擎（InnoDB）、primary-only、durability 配置，adapter CI ≠ CLI/outbox 证据 | DESIGN SPECIFIED / UNSUPPORTED UNTIL TESTED |

**本轮不实现、不运行真实 outbox、不声称 durability 已证明。** 清除该 P2 的唯一方式是实际实现 + 故障注入
证据 + 独立复核，而不是文档措辞。

---

## 8. 实施工作包（仅 WP1 获得有限授权）

| WP | 内容 | 授权前置 | 代码所有者 | 修改范围 | 安全门禁 | QA 要求 | 验收证据 | 独立 PR |
|---|---|---|---|---|---|---|---|---|
| WP1 | 版本化流式契约：协议版本、request/summary、分页 receipt、状态/错误分类、受控 sink 能力、Plan/Approval 绑定、ResourceBudget 纯校验、新旧兼容测试 | ADR-01/02/09 方向 + 本轮 contracts 有限授权 | Main | **仅新增** `pkg/contracts/**` 版本化类型与测试 | 不改 m1a 语义/常量；旧审批仍有效；跨协议混用拒绝 | `go test -race`、golden vector、正反例绑定、序列化 | 契约评审 + QA + Security 记录 | 是（独立门禁） |
| WP2 | DB 事务与审计能力（**初始仅 SQLite**）：预置 schema/migration、同事务 outbox、capability/durability 探针、故障注入 | **NOT AUTHORIZED**；需 WP2 专项授权 + DB 所有权 | go-db | `internal/db/**`、迁移脚本 | G1–G9（**含 G4–G5 平台/路径**）、G6；无运行时 DDL；未测平台/路径 fail closed | 真实 SQLite 事务、路径替换/符号链接、ENOSPC、崩溃、CommitUnknown | 故障矩阵结果（含平台/路径） | 是 |
| WP2b | MySQL outbox 扩展：独立验证同库/InnoDB/primary-only/durability 后才纳入 | **NOT AUTHORIZED**；WP2b 专项授权 | go-db | 同一 db 模块的 MySQL 适配 | G12；未测组合 fail closed | 真实 MySQL 事务与 durability | 跨后端证据 | 是（可独立延期） |
| WP3 | Runner/CLI 流式集成：Q=0 同步写入、有界工作集、审批目录 chunk 扫描、显式 CLI 选择、资源上界验证、跨 worker 正确性 | **NOT AUTHORIZED**；需 ADR-08 数值 + 模块授权 + WP1/WP2 完成 | Main / go-mask / go-verify | `internal/runner/**`、`cmd/maskriver/**`、`internal/history/**` | 保留 `--apply`、审批、preflight、dry-run；无自动回退 | race/fuzz/泄漏/取消、CLI 子进程规模矩阵、跨 worker 摘要一致 | 有界性证明 + 实测分布 + SKIP 清单 | 是 |
| WP4 | 额外审计安全能力（**仅 Anchor/Seal 相关**）：独立 Anchor、Seal、Verifier、攻击/故障测试 | **DEFERRED**（ADR-05：不属于 #13 必需） | Security / Main | 新服务 + verifier + Anchor 故障测试 | G10–G11；Anchor 不可用时 AUDIT_INCOMPLETE | 篡改/删除/回滚/Anchor 宕机测试 | 攻击矩阵、密钥托管审查 | 是（可独立延期） |

**平台/路径前置：** G4–G5（Windows/Linux 文件身份、符号链接/硬链接/TOCTOU）属于 **WP2 必需门禁**，不是可延期项；未验证的平台/路径必须 fail closed，因而 WP2 交付时只能声明已验证组合。WP4 只承担 Anchor/Seal 专属门禁（G10–G11）；G12 属 WP2b 的 MySQL 门禁。

**WP4 是否必须先于 WP3？** 不必须。WP3 的 A3 路径不依赖外部 Anchor；若 Owner 不要求篡改证据，WP4 可
独立延期。但若 Owner 要求“完整审计含篡改检测”，则 WP4 的 Seal/Anchor 验证必须在宣称该保证前完成——
此时 WP3 可先交付“同事务 receipt 一致性”，但**不得**对外声明“审计防篡改”。

**术语边界：** 本包中“receipt 完整/完整审计”仅指**同事务 receipt 一致性**（业务写入与该批 receipt 同成败），
不包含主动篡改检测、不包含长期保留、也不证明 durability。只有 WP4 完成且通过独立审查后，才可声称
“登记后、catalog 内、可信 Runner/Anchor 边界”的篡改证据；否则必须使用“本地一致性”表述。

**当前：WP1 有限授权（仅新增契约类型与测试）；WP2 / WP2b / WP3 为 `NOT AUTHORIZED`；WP4 为 `DEFERRED`。**

---

## 9. 决策后的授权状态与下一次最小授权

2026-10-20 已记录的委托方向解决了 ADR-01/02/03/04/05/06/07/09/11 与 ADR-10 的 WP1 范围；
**ADR-08 数值仍未批准**。

下一次最小授权建议：**WP2（SQLite 同事务 outbox）**，前提是 WP1 契约通过独立门禁并合并。WP2 授权必须
按文件集合（`internal/db/**` + 迁移）与安全门禁 G1–G9 明确记录，不能顺带授权 Runner/CLI 或 MySQL。

---

## 10. 状态与裁决

- Issue #13：**OPEN**。原始严格内存 AC 未通过；本包不改变该事实。
- Issue #14：**OPEN，未授权**。本包不涉及 checkpoint/resume/exactly-once。
- PR #71：文档/设计专用；按文档门禁合并后即关闭该设计 PR。
- 生产修改权限：**仅 WP1 新增契约类型与测试**；WP2/2b/3/4 无。
- ADR-01..07/09/10/11：**已记录决定**（委托方向）；ADR-08：**PENDING MEASUREMENT**。

**本包裁决：`DESIGN DIRECTION DECIDED — WP1 AUTHORIZED, IMPLEMENTATION UNVERIFIED`** —— 决策记录、原始 AC
溯源、完整 diff 审查（见 `docs/reviews/issue13-pr71-full-diff-review.md` 及其附录）与独立复核已完成；这不
等于生产实现验收，也不等于 #13 关闭。
