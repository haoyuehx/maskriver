# Issue #13：严格内存上界与批次回执架构评审

**状态：架构调查已形成候选方案；生产实现未授权。裁决：DESIGN BLOCKED（AC范围、审计原子性及资源界契约尚未决；独立Security Review未完成）。**

- 仓库基线：`9d6e03627c15fa31df051b16370f79e895e96bcc`（main，与 origin/main 一致；调查时工作树干净）。
- Issue #13：OPEN；PR #70：MERGED，head `b82bb6d65fce0d4ec7ca61787d7ac19435b15bdb`，merge `9d6e03627c15fa31df051b16370f79e895e96bcc`。
- `pkg/contracts.APIRevision=m1a-v1`；release `v0.1.0-dev` 不变。#14 未授权、未启动。
- 本文只读调查与设计；没有修改生产 API、事务策略、默认值或运行代码。结论中的未测量内容均明确标为设计/待验证。

## 1. 范围、证据等级与裁决依据

阅读范围包括 `AGENTS.md`、GitHub Issue #13 全文/更新、`docs/{architecture,test-plan,performance,feature-matrix}.md`、PR #70 完整归档目录、`pkg/contracts`、`internal/runner`、`internal/history`、`internal/db`、`internal/verify`、`cmd/maskriver` 及批次提交/取消/CommitUnknown/审批相关测试。

标记：**[代码]** 表示当前实现直接可见；**[测试]** 表示当前测试明确覆盖；**[推导]** 表示由数据结构/控制流得到的复杂度结论；**[设计]** 表示候选设计，不等于实现证明；**[未验证]** 表示没有运行或现有证据不足。

关键现状：[代码] `ApplyReport.Committed []BatchReceipt` 必须在 `Apply` 返回时携带所有已成功提交批次；`BatchReceipt` 含 TableRef、批次序号、行数。调用者可以读取/遍历这组完整历史。Issue 原始 AC 明确要求内存由 BatchSize×Workers 限制，而非总数据/批次数。

## 2. Apply 回执生命周期与调用链

```text
CLI mask --apply --approved
  └─ runner.Engine.Apply(ctx, ApplyRequest) (ApplyReport, error)
      ├─ plan 形状 → 显式写意图/审批声明 → 签名审批 Lookup
      ├─ preflight（元数据/约束）→ 全表 dryRun（无 Writer）
      ├─ applyRun：一 Writer；逐表、逐 keyset 页；每批一个事务
      │   └─ writeBatch：过滤全 NULL 行 → Begin → Update → Commit
      │       ├─ Commit 返回 nil：append BatchReceipt{Table, Ordinal, Rows}
      │       ├─ 普通失败：rollback（适用时）；此前 Committed 前缀保留
      │       └─ ErrCommitUnknown：标记 CommitOutcomeUnknown，不回滚/重试/追加该批回执
      ├─ 所有表成功后 Complete=true
      └─ 返回 report + error
  └─ CLI applySummary 遍历全部 Committed，聚合批次数/行数/最后成功批次
```

- Receipt 在批次 `Commit(ctx)` **成功返回之后**创建/append；其三个字段是可确认的数据库提交记账（表、批次局部 ordinal、受影响行数），不含行键、原值、替换值或摘要。[代码：`internal/runner/execute.go:650-735`，`pkg/contracts/contracts.go:338-347`]
- 底层 SQL `Commit` **调用已尝试后**返回错误时，db writer 将结果归类为 `ErrCommitUnknown`；调用方不进行 rollback 或 retry，报告仅标示 unknown，不能确定该批已写或未写。Commit调用之前的 nil/context/closed 状态校验失败属于确定的前置拒绝，不是 CommitUnknown。真实 SQLite 的“提交后确认丢失”测试证明目标可能已变更但该批不出现在 Committed。[代码/测试：`internal/db/write.go:246-261`、`internal/runner/workflow_acceptance_test.go` 中 `TestApplyWrappedCommitUnknownAfterDurableCommitReportsUnknown`]
- 后续批次失败/取消时，前序已确认 receipts 保留，`Complete=false`；未提交本批回滚。测试覆盖首批成功、第二批失败及取消；不代表整表/全局事务回滚。[测试：`TestApplyRetainsReceiptForEarlierCommittedBatch`、`TestApplyRollsBackBeginBatchFailuresAndCancellation`、`multi_apply_test.go`]
- `ApplyReport` 由 `Engine.Apply` 初始化，`applyRun` 通过指针持有并 append；返回后由调用者持有。CLI 的 `applySummary` 逐项读取并临时按表聚合。CLI 当前没有将详细 receipts 持久化：history Store 仅保存签名审批决定；审批数据不是 Apply 审计记录。[代码：`cmd/maskriver/main.go:applySummary`、`internal/history/record.go`]
- Validate 不接受 ApplyReport/Receipt；它独立扫描 source/target 和固定 Plan。它每侧保留一个最多 256 行的 keyset 页面并聚合有限类别/列的 Issues，不需要完整 receipts。[代码：`internal/verify/verify.go`；测试计划与安全测试]
- Writer/事务关闭后，`Engine.Close` 关闭 source/target SQL handles，但 Go 返回值仍由调用者引用；关闭 Engine 不会释放 `ApplyReport.Committed`。Runner 不保留历史 receipts 的额外全局副本。[代码]
- 审批存储记录随计划 scope 数量增长，单个 Record 的 Scope 列表为 O(计划列数)，与数据行/批次数无直接关系；历史文件持久化审批，不是批次审计。Apply 的 approval Lookup 调用 `requireUnambiguous`，其 `os.ReadDir` 将目录全部 entry/name 一次性保留并扫描，故内存另有 O(H + 所有文件名字节) 项，H 为审批目录 entry 数；即使当前 plan 固定，历史目录累积也可能扩大该次 Apply 内存。不能删减歧义检查来规避，未来严格界需在不弱化安全的前提下以有界索引/存储或强制目录上限解决。Store 的单记录读上限 1 MiB 不等价于整个历史目录磁盘或 ReadDir 内存有界。[代码：`internal/history/store.go:Lookup/load/requireUnambiguous`]

## 3. 完整内存生命周期与复杂度

符号：`Nᵢ`=第i张计划表的行数，`B`=Apply请求的分页/批次上限，`W`=mask workers，`C`=计划列数，`T`=计划表数，`D`=数据库中所有表元数据规模，`H`=审批目录entry数，`L`=最大单值/单行投影字节数，`K`=本次成功Commit且过滤后至少有一行可写的页面数。分页在过滤全NULL行之前进行，因此 `K ≤ Σᵢ ceil(Nᵢ/B)`；全NULL页不产receipt，混合NULL会使K不等于基于可写行数的简单公式。固定表数且稠密可写数据时最坏情形 `K=Θ(N/B)`；若T可增长，表边界本身可能使K接近T。失败/取消只留下成功前缀。复杂度须把字符串/byte payload纳入；只按对象个数计算不是字节上界。

| 子系统/生命周期 | 当前保留对象 | 规模关系与说明 |
|---|---|---|
| Worker 临时工作集 | `maskBatch` 的 result、jobs channel、最多 W goroutine；worker 仅改结果槽 | [代码] 每次最多 1000 输入、W≤16；O(B·L + W·L)，确定性索引输出。不是全 Apply 上界的充分证明。 |
| Reader/Writer 分页 | Apply source/target Page、`changes`、按列 originals/rows/masked、WriteBatch 的 pending；SQL statement args | [代码/推导] 单批 O(B·C·L) 多份短时副本，行 payload 不限时没有固定字节上界；请求页≤B≤1000，但 Reader 为 lookahead 分配 B+1 条并在截断后仍保留 backing array 中的额外元素。Preview/dryRun 返回页≤128、底层最多129；Reader 返回关闭游标后的 materialized page。 |
| Plan/元数据 | gather 的全部 scope evidence/sample、schema map、Plan Tables/Columns；Apply preflight 的所有计划表 schema；incoming-FK 检查对 source/target 调 `Tables` | [代码/推导] O(T+C+S·(sampleLimit+1)+ schema metadata D)；样本返回≤sampleLimit，但底层最多保留 sampleLimit+1 lookahead value；`sampleLimit≤10000`，scope数/T/D当前没有API级硬上限。Plan不是随N单调增长的row collection，但可随数据库表数增长。 |
| 去重/映射/索引 | detector 在样本上 distinct/map；mask 策略纯函数 | [代码] Detector 的临时集合由 bounded sample 驱动；Apply 未调用 mapping store，未见按每行累计的内存映射/索引。未来引入持久映射不得计作“免费”。 |
| 审批/历史 | 一个审批 Record 与 scopes；history 目录文件 | [代码] Record内存随scope数；审批目录ReadDir带来O(H+文件名字节)额外内存，磁盘文件数随审批操作增长。它不含receipt。 |
| Commit Receipts | `ApplyReport.Committed` slice | [代码/推导] Θ(K)；最坏情况下固定B时K=Θ(N/B)，但NULL-only页可不产生receipt且失败只保留成功前缀；切片扩容可能使容量大于长度。Phase 5稠密 fixture测得0.019 / 0.133 / 1.102 MiB（10k / 100k / 1M，B=64），是样本而非渐近证明。按 amd64 结构体推导，每项 BatchReceipt 含 3 个 string header 的 TableRef（48B）及两个 int64（16B），约 64B/receipt，不含共享字符串存储。 |
| Validate | 每侧一个 page、当前 key、比较状态、Issue 聚合 | [代码] 返回行数≤256/side，但Reader lookahead backing array最多257行；不保存所有行；Issue上限与不同状态/代码/列位置数有关，可随C增长，不随N每行增加。 |
| CLI 输出 | `applySummary` positions/batches/rows 每表临时数组，逐 receipt 聚合；fmt 写 io.Writer | [代码/推导] 临时 O(T+K) CPU 时间、O(T) 数组；stdout/stderr 是 OS 流，不由 CLI 主动整体缓存。嵌入 API 的调用者提供任意缓冲 Writer 则由调用者决定额外内存。 |
| Go runtime/GC | heap arenas、栈、数据库驱动/SQLite native heap/cache、GC 尚未回收或保留的页 | [推导] 不是由 B×W 单独限定。RSS 包含 runtime/native/cache；采样无法证明瞬时峰值或严格渐近界。 |

Phase-5 sampler不是产品路径，但每100ms将资源记录 append进`rows`，stop时又复制全slice；其测量进程内存为O(run duration / 100ms)，并会污染更长运行的内存曲线。[代码：`internal/runner/phase5_scale_test.go:newPhase5Sampler/record/stop`]

因此，当前一次 Apply 整体至少为 **O(K + B·C·L + T + C·(sampleLimit+1) + D + H + filenameBytes + runtime/cache)**；K项在稠密可写场景Θ(N/B)，审批目录entry扫描还可能随历史操作数增长。Phase5 sampler另为测试仪器O(duration/interval)，不得归入产品工作集或复用为长跑有界测量器。严格字节界还需定义L、表/列/metadata/approval-dir上限、数据库native cache、Go runtime保留等纳入范围。即便将 Receipt Slice 预分配、压缩字段、复用内存，Θ(K) 不变。

## 4. 原始严格 AC 的可满足性

**否：若当前公共 API 必须在 Apply 返回时提供全部完整 `Committed []BatchReceipt`，不可能在不改变结果语义的前提下让当前切片表示的内存与总批次数无关。** 直接依据是 API 返回包含K个非零尺寸 `BatchReceipt` 元素的 slice，元素 backing array 至少按K线性增长；这是当前具体表示的 Ω(K) 下界。无需假设每个字段携带独立信息（TableRef可能重复，ordinal可顺序生成）。把它放到另一个 heap、全局 map、隐藏 goroutine 或进程外的未计量缓冲都没有改变对象/语义，并且不符合要求。

必须区分：

- **内部流式工作集**：可设计成 O(B·C·L + W·L + 固定队列)，由 B/W 上限约束；当前页处理部分已如此。[代码+推导]
- **ApplyReport 返回值**：当前是 Θ(K)；不能与总批次数无关。[代码+推导]
- **CLI 峰值内存**：当前 `Engine.Apply` 先保留 Θ(K) report，CLI 再遍历；CLI 仍 Θ(K)。替换 CLI 路径为 streaming 后可降低到工作集界，但旧 API 的调用者仍无界。[推导]
- **持久化存储**：完整逐批审计通常需要 Θ(K) 磁盘空间和 I/O；内存界不代表磁盘总资源有界。[设计]
- **对外完整审计**：可通过持久化记录/消费式接口完整提供，但其存储和失败语义必须明确定义；单纯计数摘要不能替代“每批完整 Receipt”。[设计]
- **累计数据结构**：全量 Slice、全量 map、无界 channel、把回执缓存到 CLI 日志 buffer，仍然 Θ(K)。[推导]

如果原始 AC 是针对**每种 API 调用**都要求 Apply 返回全 receipts，则它与现有契约不可兼得；不能宣称全项目 AC PASS。若授权把 #13 的被测完整工作流限定为一个新的 receipt-streaming API/CLI 路径，则该路径可以满足内存部分 AC，但旧 M1 API 仍无界，需 Issue owner 对 AC 适用范围作明确契约决策。**不能单方面把“完整工作流”缩窄。**

## 5. 架构方案比较

| 维度 | A 保持 ApplyReport | B 直接 Receipt Consumer | C 持久 Receipt Store | D 双层 API（推荐方向） |
|---|---|---|---|---|
| 内存峰值 | Θ(K)+临时工作集 | 消费者可同步时 O(工作集+有限队列)；消费者自己可攒成 Θ(K) | 应用 heap O(工作集+摘要/游标)，但 OS page cache/native cache另计 | 新 API/CLI可 O(工作集+有界sink buffer)；旧 API 仍 Θ(K) |
| 总批次数相关内存 | 是，线性 | 仅消费者违反流式契约时；无界队列也线性 | 记录不在应用 heap；磁盘 Θ(K) | 按各 API 明确不同 |
| 持久化开销 | 无 receipt store | 取决消费者；stdout不是可靠审计 | Θ(K) 文件/SQLite行 + index/WAL/空间 | 新 API 可由持久 Sink 承担，旧 API内存 |
| API 兼容性 | 完全兼容 | 新增回调/接口签名；改变失败可见性 | 需要句柄/store契约 | M1保留；新增 APIRevision/接口或能力 |
| 事务一致性 | 成功提交后报告，当前语义明晰 | commit成功后调用 Sink；Sink失败时已提交却无法完整交付 | 外置 store 与 DB 无原子事务；同 DB receipt 表可能同事务但只适用于明确 DB/架构 | 需定义 sink durability + CommitUnknown 协议；仍不能默认原子 |
| CommitUnknown | 当前 batch 未列入 Committed，标记 unknown | 必须发 unknown 事件；不能发“已提交” receipt | intent/final-state协议复杂；未知不能伪装成提交/回滚 | 新 API 必须暴露 confirmed prefix + unknown + sink failure；绝不自动重试 |
| 取消/背压 | 无消费者背压 | 同步回调自然背压；回调阻塞会阻塞 Apply；取消必须有界退出 | 写盘同步背压；空间/IO故障处理复杂 | bounded queue=0/小定长；取消先停止新事务，已提交记录不可撤销 |
| 完整审计 | 进程内返回，调用者需持久化 | 若消费方不持久/进程崩溃，事件丢失 | 可持久化，但跨存储原子性/崩溃窗口待定义 | 可提供 durable sink 句柄，但此语义须作为新契约显式设计 |
| 故障恢复 | 不承诺跨进程恢复 | 不自动恢复 | 可能诱发 checkpoint/replay/恢复承诺，需严格防止扩展 | 保持无 resume/exactly-once；只记录观察到的状态 |
| 测试/性能 | 简单；小常数优化无效于阶 | 回调死锁/错误/取消测试；吞吐受消费速度限制 | crash/fsync/disk-full/tamper/permissions测试多 | 最大复杂度；双路径兼容、sink状态机、故障矩阵 |
| 安全风险 | 丢失进程就丢报告；无持久审计 | Sink 可能泄密或误用；任意callback可重入/阻塞 | 文件路径/权限/篡改、敏感元数据、空间耗尽 | 同时有 API 和审计语义复杂度，需安全审查 |
| 冻结契约 | 否 | 需新增签名/接口，不改旧字段也属于契约扩展 | 需定义存储/句柄 DTO | 必须评审共享契约；建议 APIRevision 新修订 |
| 原始 AC | **不满足** | 可满足新 streaming 调用的内存项，不满足旧完整列表 API | 进程 heap 可满足，但磁盘增长；现契约报告语义仍需改 | 可使明确指定的流式端到端路径满足；不能宣称旧路径满足 |

**A：不推荐**。减少结构体、准确预分配只减常数，完整结果仍 Θ(K)。容量收缩只会丢数据或另行复制，后者仍需同时驻留/额外峰值。

**B：有用但不独立充分**。直接同步回调比 channel 更容易施加背压；channel若采用，容量必须固定且小（推荐 0 或 1），生产者只能在 commit 后逐条发送并等待接收。消费者错误在 DB 已提交之后发生，不能回滚已经提交的批次。同步 callback 也可能重入 Apply/阻塞；需禁止重入并通过接口约束/测试防死锁。调用者若保留全部事件则仍 Θ(K)。

**C：可作 B/D 的 sink 实现，不宜单独作为 M2 恢复协议。** 本地 append-only 文件或 SQLite audit 表将完整 receipts 外置，heap可有界，磁盘仍 Θ(K)。外部 DB+target DB 无分布式事务；先提交数据再写 receipt 有“已写无审计”窗口，先写 intent 再提交则 crash/CommitUnknown留下待判定 intent。只有同一个目标 DB 的 receipts 表与 data UPDATE 在一个 tx 中原子提交才消除这类窗口，但需改变 Writer、目标 schema/权限、Verify/CLI、保留策略，且对现有 SQLite/MySQL 适配、目标只读门禁有跨模块影响；不等于支持 checkpoint/resume/exactly-once。磁盘满必须在 Commit 前保证审计容量，或事先记录 intent 并把不确定状态作为合法结果；不能在 commit 成功后因写 receipt 失败仍宣称完整审计。

**D：推荐作为产品演进形态，但不是无条件最终实现选择。** 保留 `Apply` 旧行为用于兼容；提供新 streaming/durable API，返回小型摘要（plan id、完整标志、confirmed count/rows、unknown flag、sink outcome）并逐批消费完整 receipt。CLI需显式选用新路径，不能静默丢 receipts。若要“完整审计”，新路径必须要求可靠 sink；sink输出存储 O(K) 磁盘，或规定调用者消费责任。必须通过冻结契约审查明确新旧语义、错误返回与 CommitUnknown。这个 recommendation 确需合同决策，故此文没有实施。

## 6. 推荐数据流与事务时序（拟议，不是现状）

```mermaid
sequenceDiagram
  participant C as CLI/caller
  participant R as Runner
  participant W as Writer/DB
  participant S as Receipt Sink
  C->>R: StreamingApply(plan, explicit intent, bounded batch)
  R->>R: validate shape + signed approval + preflight + full dry-run
  loop each batch
    R->>W: Begin + Update
    R->>W: Commit
    alt Commit confirmed
      W-->>R: success
      R->>S: append confirmed BatchReceipt (sync; bounded)
      S-->>R: durable/accepted or error
    else Commit outcome unknown
      W-->>R: ErrCommitUnknown
      R->>S: append OutcomeUnknown(batch position, no success claim)
      S-->>R: durable/accepted or error
      R-->>C: stop; unknown; no retry
    else definite pre-commit failure
      W-->>R: failure + rollback where valid
      R-->>C: stop; prior confirmed prefix remains
    end
  end
  R-->>C: bounded summary + sink handle/status
```

若 sink 失败发生在已确认 Commit 后，行已提交而完整 receipt 外送失败。新 API 必须返回**非成功**、`Complete=false`、confirmed totals 仍如实反映，但不能提供已丢的全部 receipt；是否用同库 transaction/outbox 消除此差异是需另行授权的关键决策。Consumer取消/失败不能撤销先前 commit。Unknown事件本身若无法持久化，也不能假称审计完整；过程只能明确返回 unknown + audit incomplete，并停止。

## 7. 审批与提交未知安全模型

- 保持现有门禁顺序：合法 Plan、`ExplicitApply=true`、CLI审批声明、对同 Plan/源快照/目标/策略/scopes 的签名决策校验、静态 preflight、完整 dry-run，以上都成功才打开 Writer。[代码/测试]
- Receipt 消费不是审批；不能新增基于 sink 的写入授权，也不能让消费者错误降级安全闸门。
- Commit 成功返回是当前唯一已确认批次证据；Commit error 统一可能 unknown，不能据错误文本猜定失败。Unknown批次不得自动 retry、rollback声称成功、发 confirmed receipt 或声明全局 rollback。[代码/测试]
- 流式协议需区分 ConfirmedReceipt、CommitOutcomeUnknown、SinkFailure/Incomplete；事件序列与摘要不得把 Unknown 合并为 0 行成功。
- 新设计不宣称恢复、幂等重放或 exactly-once。若未来有 reconciliation/checkpoint，必须独立授权为 #14/后续范围，设计源快照/plan/目标状态绑定、重复脱敏风险与审批有效期，不能从本次审计 Sink 推出。

## 8. API、CLI、能力边界

- `m1a-v1` 不得原地改变 `ApplyReport.Committed` 类型或删除其全量语义；否则调用者可静默丢回执，是破坏性契约变更。
- 备选：新 APIRevision（例如 `m1b-v1`，名称仅示意）定义新的 `StreamingApply`/sink/summary DTO；兼容旧 `m1a-v1`。共享契约变更要先提交提案、跨模块评审和明确授权，本轮没有此授权。
- CLI是旧 API消费者；要满足流式内存目标必须经显式迁移到新路径，并把详细回执写入用户选择且权限受控的 sink，或要求同步消费者。不能只把 `applySummary` 改为打印计数、丢弃逐批回执。
- 只有新 streaming 端到端路径在有界 sink 队列下可以主张“应用 heap 的批次维度不随 K 增长”；旧 API仍 O(K)，故若原 AC针对 MaskRiver 全部 Apply API，则全局 AC仍不通过。应由 Issue owner正式判定原AC的作用域，不得通过文案规避。
- 对外审计完整性与严格内存上界可以同时成立（流式写入 durable sink），但磁盘 O(K)，且跨数据库提交/审计原子性尚未设计证明。仅调用回调并不自动获得持久审计。

## 9. 资源预算与复杂度验收定义

当前可证明的结构上界是返回逻辑长度，而非完整backing存储或固定字节值：Apply页≤B≤1000但Reader最多保留B+1 lookahead；workers≤16；Verify页≤256/side但最多保留257；sample≤10000/scope但最多保留10001；审批目录`ReadDir`为O(H)；Row.Value payload、scope/table count、schema/D容量、SQLite cache、Go runtime均未纳入固定上限。[代码]

候选新 API 目标（设计假设）：

`HeapLive <= H0 + α·B·C·Lmax + β·W·Lmax + γ·Q·R + M(T,C,D) + A(H,filenameBytes) + G(runtime)`, `Q≤1`（同步消费/容量1），不含完整 receipts。该式只有在审批目录项被有界索引/明确上限约束时才是对N无关的预算；目前 `ReadDir` 的O(H)并未解决。若要声称绝对bytes bound，必须另外设并验证Lmax、T/C/D限额及driver/native cache预算；原AC若仅指batch×workers的数据工作集，需定义这些固定元数据项允许存在。

持久 store容量约 `K*(record header + receipt fields + integrity metadata) + index/WAL`，随批次数增长。不得宣称总资源有界；应提供可预检容量、保留/轮替政策或磁盘满失败计划，但不能在缺少可靠审计时静默回退到内存数组。

## 10. 验证方案（尚未执行）

1. **测量维度**：固定 B/W，N=10k/100k/1M；同环境重复至少5次（1M资源允许时），新旧 API分别测。再固定 N 变化 B（例如16/64/256/1000）和 W（1/2/4/8/16），每格按预算重复。真实 Runner完整 Scan→Plan→Preview→Approve→Apply→Validate，并单独测 Apply 与全过程。
2. **采集**：应用 heap `HeapAlloc/HeapInuse/TotalAlloc/NumGC/PauseTotalNs`、进程峰值/持续 RSS、GC周期时点、receipt backing bytes（legacy）、sink buffer bytes、goroutine、SQL pool Open/InUse/WaitCount、SQLite native/cache估算分开记录；执行固定采样并另用进程级peak工具，明确采样会漏尖峰。测量结果全部独立归档，不把估算混入实测。
3. **拆分来源**：legacy 返回值基准保留 report 引用到测量终点；streaming 测量同步写入临时 sink 后不保留事件；同一程序用 heap profile/allocation trace定位 slice；运行后强制 GC并分别记录 live heap 与 RSS，解释 GC/runtime保留、OS page cache/native SQLite cache。不能以强制 GC 后 HeapAlloc下降证明峰值有界。
4. **数学证明**：用 API 类型/对象流证明旧接口 Ω(K)；新实现审查无累计容器，队列容量固定，消费函数同步阻塞；在固定 B/W/C/L条件下，live retained receipt memory对N斜率为零。三个规模的实验只能佐证，不替代代码不变量/复杂度证明。
5. **正确性**：独立逐批比较 sink records 与事务成功记录、总行数/批次数；确认无 receipt被删、重排或重复；Apply结束后sink可重读并验证封存完整性；任何序列断号则失败关闭。

所有上述实验均为计划，**本轮未执行原型或新增测量**。历史10k/100k/1M Phase5数据不被改写/重算成新架构结果。

## 11. 失败注入矩阵（拟议）

| 点 | 可能已提交 | 确定/未知 | 可安全重试？ | 审计/成功报告要求 |
|---|---|---|---|---|
| Receipt对象构造前、Begin/Update前失败 | 无当前批；之前前缀已提交 | 当前批确定未提交（按Writer契约/可回滚错误） | 不自动重试；人工新运行还需新检查 | 记录此前前缀；不声称全局回滚 |
| Commit调用前故障 | 当前事务未提交，Rollback尝试 | 未提交仅当驱动/Writer明确保证；否则分类未知 | 不自动 | 不生成 confirmed receipt |
| Commit成功后、通知Consumer前崩溃 | 当前批已提交 | 数据确定已提交，但进程外部未获通知 | 不安全重跑；无自动恢复承诺 | 消费式 API存在不可消除 crash gap；需要持久 outbox/同库原子策略才能审计完整 |
| Sink写入失败（Commit已成功） | 当前批已提交 | 数据确认，receipt交付失败 | 不自动 | Run失败且 audit incomplete；不得 Complete=true。只能用事先设计的同库原子receipt机制消除此窗口 |
| Receipt已 durable，调用者未收到 ack | 当前批已提交（若写入序在commit后） | sink有receipt；调用者认知缺失 | 不自动重放 | 用唯一批次键/序号可识别重复通知，但这不是数据 exactly-once |
| CommitUnknown | 可能已提交也可能未提交 | 未知 | 禁止自动 retry | 发 unknown 事件若可行；无证据时不能称完整审计/成功 |
| Consumer取消/背压 | 已提交前缀存在；当前批取决于时点 | stop前的确认前缀确定 | 不自动 | 同步阻塞限制吞吐；ctx必须传播，退出等待必须测试；不丢历史 |
| 持久盘满/权限拒绝 | 若先commit则当前已提交；若预写intent策略则可能未开始 | 取决写入顺序 | 不自动 | 必须规定容量/预留和失败状态；不得把文件错误吞掉后继续 |
| 进程异常终止 | 可能有已提交但未通知批次 | 无外部原子证据则未知 | 不安全 | 只允许描述已持久事件；不要承诺恢复、checkpoint或exactly-once |

避免二次脱敏依赖不重试/旧计划源快照绑定/目标当前值不再匹配 full dry-run；不得新增基于估算的“自动跳过已完成批次”。

## 12. 兼容、安全与事务验收

新路径必须复用而非替换现有保障，并新增回归证明：

- 既有 M1 CLI默认 dry-run；`--apply`仍是唯一业务写意图，配置/env/stream sink不可授权写入。
- Preview仍不创建任何审计/映射/receipt文件，不写数据库；sink不能在 Preview开启。
- 签名审批仍绑定完整 Plan；stream接口不接收 reviewer私钥、不绕过 trust/history Lookup。
- 独立 Validate继续按固定 Plan扫描；不能由 receipts代替验证，也不能因sink错误跳过验证后报告完整。
- 现有 m1a调用方继续拿到全部 Committed receipts；不得静默迁移/丢失。旧路径明确记录 O(K) 限制。
- source只读、target事务门禁和 per-batch原子性维持；不扩大自动重试/恢复语义。
- Sink只允许由调用者显式提供；敏感路径/权限、DSN/密钥/原值不写事件；receipt只含已公开的TableRef/ordinal/rows或明确审查后的更少字段。表名本身可能是敏感元数据，外发/文件权限要按威胁模型处理。

## 13. 生产变更授权清单与工作包

无授权前不可修改：`pkg/contracts/**`、`APIRevision`、`internal/runner/execute.go` 的事务/报告行为、`internal/db/write.go` 的Commit/Writer语义、`internal/runner/parallel.go` worker行为、`cmd/maskriver` Apply授权路径、`internal/history`审计语义、M1 Release/Tag/默认BatchSize。当前任务只授权设计文档，不授权这些文件写入。

若批准推荐方向，至少拆为 **3个顺序工作包**（这是设计估算，非事实）：
1. Main/Architect提交冻结契约提案：双API DTO、sink背压/生命周期、summary与错误状态、CommitUnknown、durability/atomicity边界及APIRevision决定；独立评审通过后获显式授权。
2. Runner/DB owner串行实现：bounded streaming apply及同步Sink，先做明确限定的非持久/持久实验；如要求强审计原子性，另设数据库/存储owner的事务outbox工作包，不与第一版混入。不得并行修改同一模块。
3. CLI/History/DB跨模块集成与验收：显式选择流式接口、受控持久sink和错误展示，最后完成独立QA/Security与规模/故障测试。模块边界先协调，必要时将outbox拆为第四个包。

仅文档/实验原型无需改生产代码；若在当前授权下准备变更冻结契约或生产文件，应停止并请求明确审批。

## 14. #13 其他验收缺口复核

| 缺口 | 对原始 #13 的判断 | 后续证据/改进 |
|---|---|---|
| 1M重复样本只有1次 | 原始要求固定环境、多次分布；属于真实验收缺口，但不是receipt架构本身的逻辑阻断 | 新架构确认后增加至少5次或明确资源限制为UNVERIFIED |
| SQLite CLI子进程规模性能 | Issue要求真实 DB规模/完整workflow；PR #70只有Runner API矩阵，属于主要验收缺口 | CLI子进程E2E做独立隔离测量；不把Runner矩阵冒充CLI |
| Windows/macOS原生性能 | Issue要求跨平台开发/CI但AC核心写性能与资源，平台原生perf证据未完成；Windows CI测试不是性能运行 | GitHub windows-latest原生执行合成SQLite规模测试；macOS后续可用原生runner，交叉编译/Wine不算 |
| 无完整 B/op、allocs/op | Issue明确要求分配率/bytes等指标，诊断缺口；是否验收硬门需按原始Issue文本解释，不能用heap/RSS替代 | `-benchmem`微基准 + 全流程alloc profile/TotalAlloc，区分每行/每批分摊 |
| preflight未覆盖TMPDIR/cgroup | PR70 Security P2，诊断工具正确性/稳定性问题；不是内存AC本身但可能导致运行在资源不足环境被误判安全 | 检查fixture实际文件系统及cgroup limit，预检失败关闭；不影响结论当前AC不满足 |
| 子进程环境继承/输出净化 | PR70 Security P2，测试工具泄露风险，不是业务AC核心但须在扩大CLI规模测试前修正 | allowlist child env、全面清理临时路径/DSN错误输出、失败时清理/忽略产物 |
| 环境元数据/summary一致性 | PR70 QA/Security P2/P3；当前关键数据经独立QA重算一致，但方法文本仍有未验证字段 | 从runtime/build info/PRAGMA导出并做自动交叉校验 |
| 100ms采样漏短峰 | 对峰值资源证据是方法学限制；RSS峰值报告不能证明硬界 | OS high-water mark/连续采样+heap profile；结构复杂度证明仍必要 |
| 证据路径/source SHA耦合 | P2/P3可维护性和可复现性缺陷，不是业务AC阻断 | 明确immutable harness snapshot/clean checkout构建及证据manifest，不依赖测试运行目录相对路径 |
| 缺MySQL CLI E2E | 项目无MySQL CLI；MySQL Adapter不等于CLI E2E。Issue #13的范围同时提SQLite/MySQL端到端，则属其相应数据库端到端未验收；不应临时实现CLI扩成范围 | 先确定CLI/数据库原始验收解释；适配器测试不得记为E2E |

真正硬阻断包括严格内存 AC 与当前全量回执语义冲突、及未有新有界架构证据；其余按原Issue中性能矩阵/平台/分配指标要求区分验收缺口与工具P2，不能扩大成无限后续功能清单，也不能擅自忽略明确AC。

## 15. 最终架构裁决

**DESIGN BLOCKED。** 这不是否定严格内存目标，而是现证据不足以把候选 D 认定为完整可验收方案。独立架构审查发现并确认了目录扫描 O(H) 与资源/审计语义遗漏；这些已在当前文档修正。独立 Security Review 未完成（go-reviewer任务因provider Insufficient Balance失败，不能算通过）。

推荐 D 仍是可讨论方向：维持旧 `m1a-v1 Apply` 全量回执兼容语义，另立版本化streaming API。它可将**新API的进程heap receipt保留**降至与K无关，但前提是sink同步/队列有界；旧API仍O(K)，故不能让全局Apply AC PASS。若要求完整durable audit，普通Commit后sink存在崩溃窗口；同库outbox、事务内receipt或明确允许审计不完整三者的选择尚未决，不能以“合同变更授权”替代这个安全/事务决策。审批目录O(H)也必须在新工作流预算里受控，且L、表/列/metadata/driver/runtime范围需由AC owner定义。Issue owner须先决定原AC作用范围及审计保证、资源界定义；之后再进行独立Security Review和契约提案。本文不是实施授权，也不声称原始AC已PASS。

最终问题答复：
1. 当前 Apply 内存复杂度：`O(K + B·C·L + T + C·(sampleLimit+1) + D + H + filenameBytes + runtime/cache)`；K在稠密可写数据时Θ(N/B)，另有审批目录O(H)扫描等规模项。
2. 否，不止 Receipt：Plan/evidence随scope/table/sample上限增长，preflight需数据库元数据，Apply页/副本随B·C·L，CLI摘要随表数，Verify issues随列数，runtime/native cache不受B×W严格限定。明确没有发现按每个数据行累计的持久映射或Validate全行集合。
3. 是；当前具体的`[]BatchReceipt`返回表示需要K个非零尺寸slice元素，其backing array随K线性增长；不能通过缩小元素常数消除这一表示下界。
4. 不改变冻结公共API、不改变结果语义时，没有真正解决所有调用方的方案。内部可增加仅服务新路径的stream实现，但原API仍Θ(K)，不等于完成全局AC。
5. 候选推荐 D（新流式API + 有界sink），但仅作为待决方向；durable sink是否足以审计，取决于尚未确定的原子性/崩溃协议。
6. 复用现有审批、dry-run、单批事务；仅Commit成功后才产生 confirmed事件；CommitUnknown永不自动重试并明确 unknown；Sink失败后停止，不宣称完整。跨库审计原子性未解决，必须先设计/批准同库事务outbox或明确“不保证完整审计”。
7. 按当前共享接口演进规则，需契约修订审查；若新增public DTO/Workflow方法，应升级APIRevision（精确新revision由提案决定），旧m1a保留。
8. 当前不能判定#13原始内存AC PASS。即使限定新API，新路径的有界heap receipts只解决其中一项；审批目录、资源范围、audit崩溃窗口及其他1M重复/CLI/platform/allocation证据仍待定。
9. 基础版本化契约提案、Runner/DB流式实现、CLI与持久sink集成至少3个顺序工作包；若同库outbox跨数据库模块需第4包。估算，不代表授权。
10. 未获授权前，绝不能改 `pkg/contracts/**`、APIRevision、`internal/runner/execute.go`事务/回执语义、`internal/db/write.go` commit策略、`internal/runner/parallel.go` Worker Pool、`cmd/maskriver`授权路径、`internal/history`持久审计、release/tag/BatchSize默认值。

### 独立评审状态

独立架构首轮审查记录：`reviewer`在`ee3b445daba30d03bfcdedb8e7d8807a2b945793`审阅文档与来源代码，发现P1 readiness/遗漏O(H)及四项P2；作者在后续HEAD修正。对`ea89ecc8d45cf0e2f32a02da3f9f350b89b2f585`的独立follow-up确认P0=0/P1=0，提出四项P2（K需按表求和、预算公式加O(H)、移除独立信息论表述、区分Commit尝试前后错误）；这些修正正在当前版本落实。follow-up报告在`/home/haoyue/.pi/agent/sessions/--home-haoyue-Project-maskriver--/subagent-artifacts/4890d6ce-6a15-4957-8c43-44c114555639_reviewer_output.md`（本机审查证据路径，不是仓库文件）。当前修改后的最终Head尚需一次独立架构复核。Security Review尝试在首版因go-reviewer provider `Insufficient Balance`失败，没有结果；不得记录为PASS。Security Review仍未完成，PR不可合并。审查记录必须区分每个实际SHA与范围；设计作者不作为独立评审者，P0/P1需修正并由另一个独立上下文复核；不得将PR70 review冒充本设计review。
