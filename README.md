# MaskRiver

An independently developed Go data masking and sensitive-data governance project. `v1.0.0` is stable within its supported scope: Linux amd64 SQLite and MySQL 8.0.46 M1 workflows, plus SQLite-only bounded streaming and local transactional recovery. It is not a claim of production PII accuracy, legal anonymisation, MySQL recovery, or universal platform support.

面向 **2026 开放原子大赛 · 开源基础软件与解决方案赛道** 的独立 Go 项目。
这是参赛研发目标，不代表已报名、入选或获官方背书。

MaskRiver 是独立研发的 Go 数据脱敏与敏感信息治理工具，面向企业开发、测试与数据共享场景：自动发现敏感数据，提供安全、确定性的脱敏策略，数据库适配器与 CLI 支持 SQLite 与 MySQL 8.0.46，并提供数据完整性校验。

> **`v1.0.0` 是支持范围内的稳定版：Linux amd64、SQLite 与 MySQL 8.0.46 M1 工作流，SQLite-only streaming/recovery。** 详见 [v1.0.0 发行说明](docs/releases/v1.0.0.md) 与 [验收记录](docs/releases/v1.0.0-acceptance.md)。
>
> 支持范围仅限授权、隔离、可弃置的副本。MySQL Recovery、Windows/macOS Streaming/Recovery、WAL Recovery、跨存储 Mapping 原子性、通用 exactly-once、真实断电持久性、生产级检测准确率和法律意义上的匿名化均未验证。M3-2 的 MySQL CLI 证据仅限 pinned 8.0.46 CI；历史 `v0.2.1-preview` / `v0.2.0-preview.1` 保持可访问，旧资产的能力边界不因新版本扩大。

## 快速开始

请使用 `go.mod` 指定的 Go 工具链；以下构建和单元测试无需连接生产数据库。

```sh
go run ./cmd/maskriver --help
go run ./cmd/maskriver --version
go test ./...
go vet ./...
go build -o bin/maskriver ./cmd/maskriver
```

`scan`、`mask`、`validate`、`approve`、`revoke` 支持明确限定的 **SQLite 多表** 与 **MySQL 8.0.46** 合成测试工作流。SQLite 必须提供两个各自存在且不同的绝对路径文件；MySQL 使用 `--dialect mysql` 和两个指向不同数据库的 DSN（连接后按 `@@server_uuid`+`DATABASE()` 拒绝同一物理库，包括 unix/tcp 或地址别名），默认只读。MySQL streaming/recovery 未实现并被明确拒绝；非法配置或未获批准的写入一律拒绝。
`mask` 默认 Dry Run。写入需要三者同时成立：`--approved`（操作者声明）、`--apply`（写意图）和一条由 `approve` 独立签发并绑定当前 Plan 的 Ed25519 审批记录；前两者单独或组合都不充分。`apply` 只从部署预置 trust file 加载可信公钥，不接收审核私钥；`revoke` 撤销已有记录。Preview 由驱动级 SQL Spy 与文件树 SHA256 证明零写入。

严格的 `validate --strict` 现在要求全量扫描和适配器完整性检查：SQLite 使用 `quick_check` + 数据库级 FK 检查，MySQL 8.0.46 使用全行解析的 `CHECK TABLE ... QUICK` + 触及计划表的 FK orphan 检查，任一失败即 fail closed。MySQL strict 不校验 CHECK 约束表达式、唯一重复或未触及表，也不是整库/复制一致性检查。

可选的 M2 路径使用 `--streaming`、独立 `--budget-file` 和同事务 Outbox 回执。恢复需要先显式 `init-outbox`、`init-recovery`，然后以 `--streaming --apply --resume --run-id <32-hex>` 继续；`status` 只读。已提交批次通过确定性重算校验后跳过，未提交后缀必须仍等于源值；目标外部修改、错误 Key、快照漂移和 `commit_unknown` 均 fail closed。恢复不改变“批次事务不等于整库原子性”的既有边界。
可复现路径见 `cmd/maskriver/approval_e2e_test.go` 的 `TestE2EPlanIssueNewProcessApply`（子进程签发与加载），以及 `cmd/maskriver/main_test.go` 的 `TestCLIMultiTableWorkflow` 与 `TestCLIRealSQLiteWorkflow`。
直接运行编译后的二进制可观察原始退出码；`go run` 会包装子进程退出状态。

### 发行包快速开始（linux/amd64）

```sh
sha256sum -c SHA256SUMS
tar -xzf maskriver-v1.0.0-linux-amd64.tar.gz
cd maskriver-v1.0.0-linux-amd64
./maskriver --version
./maskriver --help
```

v0.2.1-preview 的资产与 SHA-256 见 [Release 页](https://github.com/haoyuehx/maskriver/releases/tag/v0.2.1-preview)；v0.2.0-preview.1 的历史验收见 [验收记录](docs/releases/v0.2.0-preview.1-acceptance.md)；最终 Tag、资产 SHA-256 与发布后下载复核记录在 GitHub Release 验收评论中。只对授权、隔离、可丢弃的 SQLite/MySQL 副本运行。仅构建和发布 linux/amd64；其他 Linux 架构未构建、未测试。

## 四人跨平台协作

团队成员各自 clone 仓库、认领 Issue、通过分支和 PR 交付；不要求固定设备、AI 工具或共用工作树。建议按当前 M1 收尾任务而不是按已经完成的组件再次分工：

| 角色（待认领） | 当前主责 | 修改边界 |
| --- | --- | --- |
| A · Main/集成协调（建议组长） | #9：Config、Plan、Runner、CLI、合并与发布协调 | `cmd/maskriver/**`、`internal/config/**`、`internal/runner/**`、`internal/history/**` |
| B · 数据库/事务 | SQLite 快照、SQL Spy、只读/写入/回滚和 MySQL 适配器回归 | `internal/db/**` |
| C · 检测/脱敏 | UNKNOWN 与策略门禁、格式约束、映射一致性和合成负例 | `internal/detect/**`、`internal/mask/**` |
| D · 验证/QA | 验证器、真实端到端回归、#11 独立验收报告 | `internal/verify/**`、`tests/**`；审查时只读 |

公共契约 `pkg/contracts`、CI、根文档及跨模块修改由 A 统一协调。非作者交叉审核：D 审 A/B，B 审 C，C 审 D；不得自审自批。具体 PR 依赖与验收标准见 [Roadmap](docs/roadmap.md) 和 [AGENTS.md](AGENTS.md)。

CI 包括 Ubuntu/Windows 的测试与 vet、Ubuntu race，以及隔离 MySQL 8.0.46 的实际 `TestMySQLAdapter` 和 `TestMySQLCLIWorkflow`（真实 CLI Scan→Preview→Approve→Apply→Validate）。MySQL 证据仅限该 pinned 镜像；未运行或 SKIP 的测试仍需如实记录。
## 产品路线图

- **M1-B 已交付：** 数据库适配器（#5、#10）、检测（#6）、脱敏（#7、#12）和验证器（#8），有真实数据库或有界 fake Reader 测试证据；各组件不等于完整产品。
- **M1-C 已关闭：** [#9](https://github.com/haoyuehx/maskriver/issues/9) 的 AC 已由独立上下文签署（[验收报告](docs/issue9-acceptance-report.md)）。SQLite 多表 CLI、签名审批记录、SQL Spy、静态门禁和批次故障回归已有真实测试；`--strict` 与 MySQL 8.0.46 CLI 已由 M3-1/M3-2 补齐，MySQL recovery 仍未实现。
- **M1-D 已裁定 ACCEPT WITH RESTRICTIONS：** [#11](https://github.com/haoyuehx/maskriver/issues/11) 独立安全验收已完成，P0=0、P1=0——原 P1（第三方许可/署名）由 PR #58 修复并经第二独立上下文复核；六条 P2 仍开放。`v0.2.0-preview.1` 的发行包已按实际链接集合生成 NOTICE、随包完整许可文本、MPL-2.0 源码获取说明与 SBOM；历史 M1 报告中的限制不因新版本而消失。
- **M2 已交付（本地 SQLite 预览）：** [#13](https://github.com/haoyuehx/maskriver/issues/13) 提供有界 Streaming、事务 Outbox 与资源预算；[#14](https://github.com/haoyuehx/maskriver/issues/14) 提供 `m2-recovery-v1` Checkpoint、只读 Status、显式 Resume 与幂等故障恢复。MySQL/Windows/WAL 恢复、跨库映射与通用 exactly-once 仍不支持。
- 完整阶段划分见 [Roadmap](docs/roadmap.md)。

## 文档

- [操作指南与教程](docs/guides/README.md)
- [M3 进度与验收](docs/m3/progress.md)
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
- **`null` 策略端到端拒绝**：Preview 与 Apply 都在打开 Writer 前拒绝“非 NULL → NULL”，因为验证器把该变化判为冲突证据，此类写入永远无法通过严格验证。M3-1 起 `internal/db` 写入层也拒绝该组合（`ErrUnsafe`）。
- **`--strict`（M3-1 起）**：严格验证强制全量无截断扫描（正数 `RowLimit` 被拒绝），并要求源/目标适配器提供并通过对每张计划表的数据库完整性检查（SQLite `quick_check`/数据库级 `foreign_key_check`；MySQL 严格完整性尚未实现并 fail closed）；缺少该能力 fail closed。普通验证保留有界语义。
