# MaskRiver

An independently developed Go data masking and sensitive-data governance project. Streaming performance and failure recovery are roadmap goals, not yet demonstrated guarantees.

面向 **2026 开放原子大赛 · 开源基础软件与解决方案赛道** 的独立 Go 项目。
这是参赛研发目标，不代表已报名、入选或获官方背书。

MaskRiver 是独立研发的 Go 数据脱敏与敏感信息治理工具，面向企业开发、测试与数据共享场景：自动发现敏感数据，提供安全、确定性的脱敏策略，数据库适配器支持 SQLite 与 MySQL（当前 CLI 仅 SQLite），并提供数据完整性校验。

> **M1 仍在收尾：#9 的 AC 待独立验收签署，尚未发布。** 当前真实可用范围是合成 SQLite 快照上的多表 CLI（同一表多列由 runner 级用例覆盖）：`scan → preview → approve（独立进程签发）→ 新进程 mask --apply → validate`。Ed25519 审批记录、驱动级 SQL Spy、静态安全门禁和批次故障回归已有真实 SQLite 测试；`--strict` 目前没有独立的严格语义，MySQL CLI、断点恢复/checkpoint、exactly-once、持久映射和生产级检测准确率均未验证。不能据此声称完整产品或生产就绪。

## 快速开始

请使用 `go.mod` 指定的 Go 工具链；以下构建和单元测试无需连接生产数据库。

```sh
go run ./cmd/maskriver --help
go run ./cmd/maskriver --version
go test ./...
go vet ./...
go build -o bin/maskriver ./cmd/maskriver
```

`scan`、`mask`、`validate`、`approve`、`revoke` 已支持明确限定的 **SQLite 多表**合成测试工作流（同一表多列另有 runner 级用例）；必须提供两个各自存在且不同的绝对路径数据库文件，默认只读。MySQL CLI 尚未实现；非法配置或未获批准的写入一律拒绝。
`mask` 默认 Dry Run。写入需要三者同时成立：`--approved`（操作者声明）、`--apply`（写意图）和一条由 `approve` 独立签发并绑定当前 Plan 的 Ed25519 审批记录；前两者单独或组合都不充分。`apply` 只从部署预置 trust file 加载可信公钥，不接收审核私钥；`revoke` 撤销已有记录。Preview 由驱动级 SQL Spy 与文件树 SHA256 证明零写入。写入使用批次事务，故障后不保证跨批全局原子，未实现断点恢复。
可复现路径见 `cmd/maskriver/approval_e2e_test.go` 的 `TestE2EPlanIssueNewProcessApply`、`TestCLIMultiTableWorkflow` 与 `TestCLIRealSQLiteWorkflow`。
直接运行编译后的二进制可观察原始退出码；`go run` 会包装子进程退出状态。

## 四人跨平台协作

团队成员各自 clone 仓库、认领 Issue、通过分支和 PR 交付；不要求固定设备、AI 工具或共用工作树。建议按当前 M1 收尾任务而不是按已经完成的组件再次分工：

| 角色（待认领） | 当前主责 | 修改边界 |
| --- | --- | --- |
| A · Main/集成协调（建议组长） | #9：Config、Plan、Runner、CLI、合并与发布协调 | `cmd/maskriver/**`、`internal/config/**`、`internal/runner/**`、`internal/history/**` |
| B · 数据库/事务 | SQLite 快照、SQL Spy、只读/写入/回滚和 MySQL 适配器回归 | `internal/db/**` |
| C · 检测/脱敏 | UNKNOWN 与策略门禁、格式约束、映射一致性和合成负例 | `internal/detect/**`、`internal/mask/**` |
| D · 验证/QA | 验证器、真实端到端回归、#11 独立验收报告 | `internal/verify/**`、`tests/**`；审查时只读 |

公共契约 `pkg/contracts`、CI、根文档及跨模块修改由 A 统一协调。非作者交叉审核：D 审 A/B，B 审 C，C 审 D；不得自审自批。具体 PR 依赖与验收标准见 [Roadmap](docs/roadmap.md) 和 [AGENTS.md](AGENTS.md)。

CI 包括 Ubuntu/Windows 的测试与 vet、Ubuntu race，以及隔离 MySQL 8.0.46 的实际 `TestMySQLAdapter`；这是组件业务测试，不代表 CLI 全流程已验收。未运行或 SKIP 的测试仍需如实记录。
## 产品路线图

- **M1-B 已交付：** 数据库适配器（#5、#10）、检测（#6）、脱敏（#7、#12）和验证器（#8），有真实数据库或有界 fake Reader 测试证据；各组件不等于完整产品。
- **M1 收尾中：** [#9](https://github.com/haoyuehx/maskriver/issues/9) 的 AC 待独立验收签署。SQLite 多表 CLI、签名审批记录、SQL Spy、静态门禁和批次故障回归已有真实测试；`--strict` 语义、MySQL CLI 与 [#11](https://github.com/haoyuehx/maskriver/issues/11) 独立验收仍未完成。
- **M2 待启动/部分：** [#13 有界并发/基准](https://github.com/haoyuehx/maskriver/issues/13) 已有有界 Worker 组件，完整基准与端到端吞吐未验收；[#14 幂等恢复](https://github.com/haoyuehx/maskriver/issues/14) 未启动。中文格式约束替换已有有界组件实现，不再列作从零开发。
- 完整阶段划分见 [Roadmap](docs/roadmap.md)。

## 文档

- [架构设计](docs/architecture.md)
- [Roadmap](docs/roadmap.md)
- [测试计划](docs/test-plan.md)
- [协作与安全边界](AGENTS.md)
- [成员职责与可选 AI 工具](docs/agents.md)
- [初始化验收记录](docs/bootstrap-validation.md)
- [M1-A 共享契约](docs/contracts.md)、[冻结报告](docs/m1a-freeze-report.md)
- [驱动决策](docs/database-drivers.md)、[测试环境](docs/test-environment.md)
- [四模块开发任务](docs/parallel-tasks.md)、[可选工作树](docs/worktree-preflight.md)
- [历史收尾报告](docs/closeout-report.md)、[M1-B 跨平台 PR 流程](docs/m1b-launch-plan.md)

## 来源与许可

MaskRiver 采用 [MIT](LICENSE)。本项目独立研发：不是任何其他仓库的 Fork，不共享其 Git 历史，未复制其源码、词典、配置或测试数据。
第三方依赖保留各自许可，详见 [来源与许可说明](docs/attribution.md)。
若将来确实改编或复用第三方代码、文档或测试，必须先在该说明中保留其版权、许可与来源标注。

## 安全提示

仅处理获授权的数据库副本；脱敏不等于匿名化。确定性映射可能遭受猜测攻击，也不自动保证唯一性或所有关联完整性。
不要向仓库、日志或 Agent 会话提交真实个人数据、连接凭证、种子及映射库。

- **审批信任边界**：可信公钥来自部署预置 trust file，`apply` 只加载公钥；同一操作者同时控制 trust file 与审核私钥时本工具无法阻止，这不是 RBAC。`ReviewerID` 是自填标签，不是已认证身份。
- **`null` 策略端到端拒绝**：Preview 与 Apply 都在打开 Writer 前拒绝“非 NULL → NULL”，因为验证器把该变化判为冲突证据，此类写入永远无法通过严格验证。`internal/db` 写入层本身仍接受该组合，属已接受 P2。
- **`--strict` 目前是 inert 的**：verify 忽略 `ValidateRequest.Strict`，且所有 Warning 路径已令 `Coverage.Complete=false`；现有测试只证明 flag 解析、转发和退出码敏感性，不证明独立的严格语义门禁。
