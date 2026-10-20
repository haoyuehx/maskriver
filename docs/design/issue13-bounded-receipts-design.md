# Issue #13：严格内存上界与批次回执架构评审

**状态：候选架构与Owner决策包已形成；生产实现未授权。裁决暂为 DESIGN BLOCKED，等待本次新 Head 的独立架构/Security Review。全部产品决策仍为 `PENDING OWNER DECISION`。**

本文件 §5–§15 的早期候选方案曾讨论提交后外部Sink；该模型现由 `docs/design/issue13-architecture-decision-record.md` 中 A3（目标数据库同事务 outbox）替代为推荐候选。ADR是当前方案来源；本文件中与A3冲突的旧 sink 时序/测试表应视为历史分析，不是待实现设计。

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
| 内存峰值 | Θ(K)+临时工作集 | 消费者同步时可 O(工作集+有限队列)，消费者自己仍可累积Θ(K) | 应用heap O(工作集+摘要/游标)，但OS page cache/native cache另计 | 新API可不保留K个回执；旧API仍Θ(K)，全流程上界还取决资源限额与AC范围 |
| 总批次数相关内存 | 是，线性 | 仅消费者违反流式契约时；无界队列也线性 | 记录不在应用 heap；磁盘 Θ(K) | 按各 API 明确不同 |
| 持久化开销 | 无 receipt store | 取决消费者；stdout不是可靠审计 | Θ(K) 文件/SQLite行 + index/WAL/空间 | 新API候选为目标DB同事务outbox Θ(K)及独立anchor事件；旧API内存 |
| API 兼容性 | 完全兼容 | 新增回调/接口签名；改变失败可见性 | 需要句柄/store契约 | 保留m1a语义；并行版本化surface/approval binding需另行契约审查，不原地全局换revision |
| 事务一致性 | 成功提交后报告，当前语义明晰 | commit成功后调用Sink；Sink失败时已提交却无法完整交付 | 外置store与DB无原子事务；同DB receipt表可同事务但须明确后端 | A3要求业务写+outbox同DB tx；外部anchor逐批见证仍非原子，缺ack必须不完整 |
| CommitUnknown | 当前batch未列入Committed，标记unknown | 不能发已提交receipt | intent/final-state协议复杂，未知不能伪装 | 业务+本地outbox同成败；底层commit错误仍UNKNOWN不重试，anchor缺ack后续停止 |
| 取消/背压 | 无消费者背压 | 同步回调会阻塞，且可能忽略ctx | 写盘同步背压；空间/IO故障处理复杂 | Q=0内部outbox；每批anchor ack后才继续；driver/kernel卡死时不承诺硬退出 |
| 完整审计 | 进程内返回，调用者需持久化 | 消费方不持久或崩溃可丢事件 | 需定义跨存储原子性/崩溃窗口 | 先独立登记Run，再同库原子receipt+逐批anchor+终结seal；anchor不可用则AUDIT_INCOMPLETE |
| 故障恢复 | 不承诺跨进程恢复 | 不自动恢复 | 可能诱发 checkpoint/replay/恢复承诺，需严格防止扩展 | 保持无 resume/exactly-once；只记录观察到的状态 |
| 测试/性能 | 简单；小常数优化无效于阶 | 回调死锁/错误/取消测试；吞吐受消费速度限制 | crash/fsync/disk-full/tamper/permissions测试多 | 最大复杂度；双路径兼容、sink状态机、故障矩阵 |
| 安全风险 | 丢失进程就丢报告；无持久审计 | Sink 可能泄密或误用；任意callback可重入/阻塞 | 文件路径/权限/篡改、敏感元数据、空间耗尽 | 同时有 API 和审计语义复杂度，需安全审查 |
| 冻结契约 | 否 | 新callback/interface属于契约扩展 | 需定义存储/句柄DTO | 必须评审共享契约；旧m1a与新approval binding并行兼容策略待决 |
| 原始 AC | **不满足** | 可满足新streaming调用的部分内存项，不满足旧完整列表API | 进程heap可满足但磁盘增长；现契约报告语义仍需改 | 只有Owner明确接受S2且所有资源界/证据通过，才可能满足限定新路径；不能宣称旧路径或全局AC满足 |

**A：不推荐**。减少结构体、准确预分配只减常数，完整结果仍 Θ(K)。容量收缩只会丢数据或另行复制，后者仍需同时驻留/额外峰值。

**B：有用但不独立充分**。直接同步回调比 channel 更容易施加背压；channel若采用，容量必须固定且小（推荐 0 或 1），生产者只能在 commit 后逐条发送并等待接收。消费者错误在 DB 已提交之后发生，不能回滚已经提交的批次。同步 callback 也可能重入 Apply/阻塞；需禁止重入并通过接口约束/测试防死锁。调用者若保留全部事件则仍 Θ(K)。

**C：可作 B/D 的 sink 实现，不宜单独作为 M2 恢复协议。** 本地 append-only 文件或 SQLite audit 表将完整 receipts 外置，heap可有界，磁盘仍 Θ(K)。外部 DB+target DB 无分布式事务；先提交数据再写 receipt 有“已写无审计”窗口，先写 intent 再提交则 crash/CommitUnknown留下待判定 intent。只有同一个目标 DB 的 receipts 表与 data UPDATE 在一个 tx 中原子提交才消除这类窗口，但需改变 Writer、目标 schema/权限、Verify/CLI、保留策略，且对现有 SQLite/MySQL 适配、目标只读门禁有跨模块影响；不等于支持 checkpoint/resume/exactly-once。磁盘满必须在 Commit 前保证审计容量，或事先记录 intent 并把不确定状态作为合法结果；不能在 commit 成功后因写 receipt 失败仍宣称完整审计。

**D：兼容演进候选。** 保留 `m1a-v1 Apply` 全量回执语义（仍O(K)），只在Owner批准后增设独立 streaming surface。推荐的回执记录不是外部 callback/file sink，而是与每批业务更新处于同一目标数据库事务域的预置 outbox；具体事务状态、Seal/Trust Anchor、平台限制及兼容性见 `docs/design/issue13-architecture-decision-record.md`。新API只可返回小摘要并提供有界分页审计读取；CLI必须显式选择，不自动回退旧API。任何新接口/approval binding/API revision 仍需共享契约评审与授权。本建议不改变Issue AC作用范围，也不构成实现批准。

## 6. 推荐数据流与事务时序（拟议，不是现状）

```mermaid
sequenceDiagram
  participant C as CLI/caller
  participant R as Runner
  participant DB as Target DB
  participant A as Independent audit anchor/catalog
  C->>R: Versioned streaming Apply + explicit write intent
  R->>R: Plan + signed approval + outbox capability preflight + full dry-run
  R->>A: Register RunID/Plan/approval before any business write
  A-->>R: durable catalog enrollment or fail closed
  R->>DB: Persist RunEnvelope
  loop each non-empty batch
    R->>DB: Persist this BatchPrepare
    R->>A: Anchor this expected BatchPrepare
    A-->>R: durable prepare acknowledgment or stop
    R->>DB: BEGIN; business UPDATEs + BatchCommitted event in same tx
    R->>DB: COMMIT
    alt Commit returns success
      DB-->>R: data and receipt atomically committed
      R->>A: attest observed Commit + event hash
      A-->>R: durable per-batch acknowledgment
    else underlying Commit attempt returns error
      DB-->>R: CommitUnknown; stop, never retry data
      Note over R,A: No post-restart reconstruction from mutable DB; remain UNKNOWN/incomplete
    end
  end
  R->>DB: append terminal DB RunSeal (only if all batches acknowledged)
  R->>A: anchor terminal digest and seal
  A-->>R: durable signed anchor receipt, or ANCHOR_PENDING
  R-->>C: bounded summary + audit status; never silent fallback
```

业务数据与该批outbox记录仅在同一目标DB事务域时具有原子性；外部anchor在写前登记Run并逐批见证，但不与DB声称原子。若Commit后anchor ack缺失，Runner状态为`AUDIT_INCOMPLETE`并停止后续批次；执行停止后若本地存在未锚定事件，Verifier结果为`UNKNOWN`。重启后不得凭可变DB重建可信attestation。仅在已登记、每批都锚定且终结seal确认时判定完整；保证仅覆盖登记后的已知run及可信Runner/anchor边界。无任意Consumer callback、无进程内累计receipt队列。完整状态机和failure matrix以ADR为准。

## 7. 审批与提交未知安全模型

- 保持现有门禁顺序：合法 Plan、`ExplicitApply=true`、CLI审批声明、对同 Plan/源快照/目标/策略/scopes 的签名决策校验、静态 preflight、完整 dry-run，以上都成功才打开 Writer。[代码/测试]
- Receipt 消费不是审批；不能新增基于 sink 的写入授权，也不能让消费者错误降级安全闸门。
- Commit 成功返回是当前唯一已确认批次证据；**底层 SQL Commit 调用尝试后**产生的错误可能是 unknown，不能据错误文本猜定失败；Commit调用前的校验/cancellation拒绝是确定前置失败。Unknown批次不得自动 retry、rollback声称成功、发 confirmed receipt 或声明全局 rollback。[代码/测试]
- 流式协议需区分 ConfirmedReceipt、CommitOutcomeUnknown、AnchorFailure/AuditIncomplete；事件序列与摘要不得把 Unknown 合并为0行成功。
- 新设计不宣称恢复、幂等重放或 exactly-once。若未来有 reconciliation/checkpoint，必须独立授权为 #14/后续范围，设计源快照/plan/目标状态绑定、重复脱敏风险与审批有效期，不能从本次审计 Sink 推出。

## 8. API、CLI、能力边界

- `m1a-v1` 不得原地改变 `ApplyReport.Committed` 类型或删除其全量语义；否则调用者可静默丢回执，是破坏性契约变更。
- 备选：单纯把全局 `APIRevision` 从m1a改成新值并不自动保持兼容：`Plan.ValidateShape`只接受当前revision，且revision参与Plan.ID/审批绑定，旧Plan可能被拒绝或重算出不同审批ID。契约提案必须明确双版本共存/旧Plan及审批验证策略；优先评估单独版本化streaming接口/DTO而非破坏性替换常量。新增共享surface仍须契约评审与明确授权，本轮没有授权。
- CLI在Owner决定前保持旧API行为。只有显式批准新契约后，CLI才能显式选择streaming/outbox协议；详细回执通过有界分页读取同库outbox。不得新增任意用户路径或回调，也不能只打印计数而丢掉逐批审计。
- 只有Owner明确将AC限定到新streaming端到端路径、且历史目录/scopes/payload/runtime范围有经批准的界及证据后，才可主张其应用heap回执保留与K无关；旧API仍O(K)，因此Issue原文若覆盖全部Apply，则全局AC仍不通过。不得通过文案规避。
- 候选A3通过“业务行+outbox行同目标DB单事务”提供每批原子性，磁盘仍O(K)；跨目标DB、文件、外部anchor均不属于该事务。外部anchor是独立最终封存，缺失时Run不完整，不宣称事务原子。SQLite/MySQL、sync设置、DB引擎和文件系统支持尚未实测/证明。

## 9. 资源预算与复杂度验收定义

当前可证明的结构上界是返回逻辑长度，而非完整backing存储或固定字节值：Apply页≤B≤1000但Reader最多保留B+1 lookahead；workers≤16；Verify页≤256/side但最多保留257；sample≤10000/scope但最多保留10001；审批目录`ReadDir`为O(H)；Row.Value payload、scope/table count、schema/D容量、SQLite cache、Go runtime均未纳入固定上限。[代码]

候选新 API 目标（设计假设；所有数值/范围需Owner批准）：

`HeapLive <= H0 + α·B·C·Lrow·copies + β·W·Lvalue + M(T,C,D) + A(H,filenameBytes) + G(runtime/driver)`, 无外部Q队列（Q=0），不含完整 receipts。保留目录歧义检查但须改为固定chunk枚举并设置H/F上限，不能`ReadDir(0)`后再检查。若要声称绝对bytes bound，必须另外设并验证Lrow/T/C/D限额及driver/native cache预算；所有候选数值见ADR并标记PROPOSED。原AC如只针对应用数据working-set，必须由Owner明确；不能称进程总RSS已严格受B×W限制。

持久 store容量约 `K*(record header + receipt fields + integrity metadata) + index/WAL`，随批次数增长。不得宣称总资源有界；应提供可预检容量、保留/轮替政策或磁盘满失败计划，但不能在缺少可靠审计时静默回退到内存数组。

## 10. 验证方案（尚未执行）

1. **测量维度**：固定 B/W，N=10k/100k/1M；同环境重复至少5次（1M资源允许时），新旧 API分别测。再固定 N 变化 B（例如16/64/256/1000）和 W（1/2/4/8/16），每格按预算重复。真实 Runner完整 Scan→Plan→Preview→Approve→Apply→Validate，并单独测 Apply 与全过程。
2. **采集**：应用heap `HeapAlloc/HeapInuse/TotalAlloc/NumGC/PauseTotalNs`、进程峰值/持续RSS、GC周期时点、receipt backing bytes（legacy）、新API outbox/event临时对象、goroutine、SQL pool Open/InUse/WaitCount、SQLite native/cache估算分开记录；执行固定采样并另用进程级peak工具，明确采样会漏尖峰。测量结果全部独立归档，不把估算混入实测。
3. **拆分来源**：legacy返回值基准保留report引用到测量终点；streaming测量走候选同事务outbox并不保留事件；性能测试可用内部测试替身，不增加public callback；同一程序用heap profile/allocation trace定位slice；运行后强制GC并分别记录live heap与RSS，解释GC/runtime保留、OS page cache/native SQLite cache。不能以强制GC后HeapAlloc下降证明峰值有界。
4. **数学证明**：用 API 类型/对象流证明旧接口 Ω(K)；新实现审查无累计容器，队列容量固定，消费函数同步阻塞；在固定 B/W/C/L条件下，live retained receipt memory对N斜率为零。三个规模的实验只能佐证，不替代代码不变量/复杂度证明。
5. **正确性**：独立逐批比较目标DB outbox记录与事务成功记录、总行数/批次数；确认无receipt被删、重排或重复；Apply结束后从outbox与锚定事件有界分页重读并验证seal；任何序列断号则失败关闭。

所有上述实验均为计划，**本轮未执行原型或新增测量**。历史10k/100k/1M Phase5数据不被改写/重算成新架构结果。

## 11. 失败注入矩阵（当前A3候选）

本节的旧外部 callback 场景已由A3 outbox候选替代。完整20项故障矩阵、注入点、数据库状态、审计状态、重试规则及验证方法见 [`issue13-architecture-decision-record.md`](issue13-architecture-decision-record.md#design-level-failure-injection-plan-not-executed)。这些全部是设计计划，**未执行**。

关键不变量：同一目标事务中的业务更新与 `COMMITTED` outbox event原子提交；commit前故障不得留下业务行；底层Commit调用尝试后错误一律保守UNKNOWN且不重试；缺失终结seal/外部anchor只能标记不完整，不能推断完整成功。A3依赖同数据库事务、durability设置和受支持平台尚待实现与验证，不是当前产品行为。

## 12. 兼容、安全与事务验收

新路径必须复用而非替换现有保障，并新增回归证明：

- 既有 M1 CLI默认 dry-run；`--apply`仍是唯一业务写意图，配置/env/streaming API/outbox均不可授权写入。
- Preview仍不创建审计/映射/receipt记录或数据库文件，不写业务数据库；候选 Envelope/Prepare/outbox/anchor 操作仅能在全部显式写授权、审批、preflight与dry-run门禁通过后进入。
- 签名审批仍绑定完整 Plan；stream接口不接收 reviewer私钥、不绕过 trust/history Lookup。
- 独立 Validate继续按固定 Plan扫描；不能由 receipts代替验证，也不能因sink错误跳过验证后报告完整。
- 现有 m1a调用方继续拿到全部 Committed receipts；不得静默迁移/丢失。旧路径明确记录 O(K) 限制。
- source只读、target事务门禁和 per-batch原子性维持；不扩大自动重试/恢复语义。
- 不提供调用者自定义Consumer、回调或receipt文件路径；候选方案仅内部同事务outbox，且anchor为终端/批次信任见证，不是业务重试许可。DSN/密钥/原值不写事件；receipt仅含经审批字段。表名/稳定摘要可能是敏感元数据，需访问控制与威胁模型审查。sink失效不得回退内存或外部文件。

## 13. 生产变更授权清单与工作包

无授权前不可修改：`pkg/contracts/**`、`APIRevision`、`internal/runner/execute.go` 的事务/报告行为、`internal/db/write.go` 的Commit/Writer语义、`internal/runner/parallel.go` worker行为、`cmd/maskriver` Apply授权路径、`internal/history`审计语义、M1 Release/Tag/默认BatchSize。当前任务只授权设计文档，不授权这些文件写入。

若Owner批准 ADR-01..11（含威胁边界）并另授实现权，至少拆为 **4个顺序工作包**（这是设计估算，非事实）：
1. Main/Architect提交冻结契约与approval兼容提案：双API DTO、audit policy binding、Plan/Approval共存、summary/错误状态、CommitUnknown、durability/atomicity及APIRevision决定；独立评审通过后获显式授权。
2. DB owner提供预置schema/migration、SQLite/MySQL事务outbox与fail-closed capability/durability探针；仅同事务outbox，不实现可降级的caller Sink。
3. Runner/CLI集成 bounded streaming Apply、授权/全量验证门禁、同步outbox、anchor状态与错误展示；无自动业务重放。
4. 独立anchor/seal/verifier、平台路径/存储支持、崩溃/ENOSPC/CommitUnknown/资源限制故障验收与QA/Security。模块边界先协调，同一模块串行写。

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

**本次ADR是候选设计，不是Owner批准或实现证明。** 在`e1faa582ca5fa2d07478f081418c968101049f97`上，独立Architecture与Security Review均为P0=0/P1=0/P2=1。Architecture P2为failure table混淆Runner执行状态与Verifier单一结果；Security P2为durability/目标路径门禁尚未可执行或验证。本修订把两类状态分栏、使用精确结果token；新head尚未复核，因此暂保持 `DESIGN BLOCKED — FINAL-HEAD REVIEW PENDING`。若最终head独立评审仍P0/P1=0，可更正为 `DESIGN READY — PENDING OWNER APPROVAL`；durability/path与存储支持仍是后续实现门禁。Issue Owner须决定AC范围与审计保证；全部ADR行仍待决。Legacy API仍Θ(K)。新streaming receipts只有在历史目录/metadata/row payload有硬界且Owner接受AC范围后才可能与K无关。A3及SQLite/MySQL durability尚未实现/验证；外部anchor不与DB事务原子。本文件不授权生产实现，也不声称Issue #13 AC通过。

最终问题答复：
1. 当前 Apply 内存复杂度：`O(K + B·C·L + T + C·(sampleLimit+1) + D + H + filenameBytes + runtime/cache)`；K在稠密可写数据时Θ(N/B)，另有审批目录O(H)扫描等规模项。
2. 否，不止 Receipt：Plan/evidence随scope/table/sample上限增长，preflight需数据库元数据，Apply页/副本随B·C·L，CLI摘要随表数，Verify issues随列数，runtime/native cache不受B×W严格限定。明确没有发现按每个数据行累计的持久映射或Validate全行集合。
3. 是；当前具体的`[]BatchReceipt`返回表示需要K个非零尺寸slice元素，其backing array随K线性增长；不能通过缩小元素常数消除这一表示下界。
4. 不改变冻结公共API、不改变结果语义时，没有真正解决所有调用方的方案。内部可增加仅服务新路径的stream实现，但原API仍Θ(K)，不等于完成全局AC。
5. 候选 D 保留m1a全量回执并新增streaming surface；新路径候选采用同一目标事务内的outbox，而非任意Sink。仍须Owner批准且独立Security复核。
6. 候选协议在同一目标数据库单批事务中同时提交业务更新与COMMITTED outbox event；仅Commit返回成功或同一权威DB恢复后可证明该event存在时确认，CommitUnknown不重试。外部Seal/Anchor未持久确认前Run不可称完整审计；DB和Anchor之间不声称原子。A3、durability及Anchor依赖Owner审批与实现验证。
7. 必须进行共享契约修订审查；不得原地全局更换APIRevision或让新审核模式静默改写m1a计划/审批。候选为并行版本化streaming surface与明确的新approval binding，精确revision/DTO由后续提案决定，m1a路径保持原语义。
8. 当前不能判定#13原始内存AC PASS。A3候选若真实同库事务可解决每批数据/receipt裂缝，但批准范围、目录/metadata/row-byte界、Durability和外部Run Seal、以及其他规模/CLI/platform/allocation证据未通过实现验证。
9. 估计至少4个顺序工作包：契约/审批共存提案；DB outbox迁移+事务能力；Runner/CLI streaming集成；独立anchor/seal/verifier与平台验收。估算，不代表授权。
10. 未获授权前，绝不能改 `pkg/contracts/**`、APIRevision、`internal/runner/execute.go`事务/回执语义、`internal/db/write.go` commit策略、`internal/runner/parallel.go` Worker Pool、`cmd/maskriver`授权路径、`internal/history`持久审计、release/tag/BatchSize默认值。

### 独立评审状态与当前裁决

历史与当前审查的实际SHA、严重度、限制、四项Security P1逐条处置及原始证据关系保存在 [`docs/reviews/issue13-bounded-receipts-pr71-review-history-2026-10-20.md`](../reviews/issue13-bounded-receipts-pr71-review-history-2026-10-20.md)。`e1faa582ca5fa2d07478f081418c968101049f97`上的独立Architecture与Security Review均P0=0/P1=0/P2=1：前者指出failure table需区分Runner state和Verifier outcome；后者将durability/path capability保留为实现门禁。精确报告与限制位于 `docs/reviews/`；两者都未执行测试、也未宣称实现或AC验收。

本次文档草案已按Architecture P2重构状态表，并记录e1两项独立复核及provider失败的原始关系。但此后改动尚未接受新head复核，故状态仍 `DESIGN BLOCKED — FINAL-HEAD REVIEW PENDING`。只有最终head独立评审P0/P1=0，才可更正为 `DESIGN READY — PENDING OWNER APPROVAL`。所有Owner选项仍待决，当前产品实现风险未因设计文字而关闭。
