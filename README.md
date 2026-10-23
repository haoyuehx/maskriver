# MaskRiver

**安全、可验证、可恢复的数据脱敏工具。**

MaskRiver 是独立开发的 Go 数据脱敏与敏感信息治理项目，面向开发、测试与数据共享场景。它把敏感字段识别、只读预览、Ed25519 审批、执行脱敏、严格验证与本地故障恢复组织成一条可追踪的 CLI 工作流。

- **官网与演示：** https://haoyuehx.github.io/maskriver/
- **稳定版：** [v1.0.0](https://github.com/haoyuehx/maskriver/releases/tag/v1.0.0)（Linux amd64）
- **许可：** MIT
- **状态：** 支持范围内稳定；不是生产级 PII 准确率或法律匿名化认证

## 官网与演示视频

产品展示网站提供可交互的产品介绍、五步流程、Streaming 与 Recovery 演示：

**https://haoyuehx.github.io/maskriver/**

网站集成了 4 分 20 秒的产品概念演示视频（1920×1080、H.264、中文字幕、当前无配音）。视频与网站均为产品说明材料，不构成生产环境承诺。

## 核心功能

- **敏感信息识别** —— 上下文与格式规则结合，覆盖 Email、URL、IP、UUID、中国身份证/手机号/银行卡/USCC/日期/城市/姓名/地址等 13 类规则；证据不足时保留 `UNKNOWN`，不当作安全结论。
- **安全审批** —— Ed25519 签名记录与精确 Plan 绑定；`--apply` 还需要 `--approved`、部署 trust file、历史目录和有效审批记录。
- **确定性脱敏** —— 支持格式约束替换、确定性 HMAC 策略与多种类型；脱敏不等于匿名化，不承诺无碰撞。
- **严格验证** —— 全量无截断扫描、双向键集、行数、Schema、计划字段与适配器完整性检查；失败即 fail-closed。
- **有界流式处理** —— SQLite-only 的分批读取、批次事务、资源预算与 Outbox 回执；不宣称任何情况下内存固定。
- **Checkpoint / Resume** —— Linux amd64 本地 SQLite 的中断恢复、只读 `status`、显式 `--resume` 与幂等重放。

## 工作流程

```text
Scan → Preview → Approve → Apply → Validate
```

1. **Scan** 抽取有界样本并分类字段。
2. **Preview** 只读生成计划，不写入目标副本。
3. **Approve** 由独立进程签发绑定计划的 Ed25519 审批记录。
4. **Apply** 仅在显式 `--apply` 和有效审批同时存在时写入计划字段。
5. **Validate** 重新核对 Schema、行数、键集、字段变化和完整性。

实际工具通过 CLI 执行；网站中的流程界面是产品示意，不是已实现的 Web 数据库管理后台。

## 技术亮点

- 冻结的 `pkg/contracts`（`APIRevision=m1a-v1`）与 SQLite/MySQL 适配器。
- SQLite 完整 M1 流程；MySQL 8.0.46 的 Scan/Preview/Approve/Apply/strict Validate 已在隔离 CI 中真实执行。
- MySQL 同一物理库拒绝基于 `@@server_uuid` + `DATABASE()`，而非仅比较 DSN 文本。
- SQLite `quick_check` + 数据库级 FK 检查；MySQL `CHECK TABLE ... QUICK` + 可见 FK orphan 检查。
- `m2-stream-v1` Outbox 与 `m2-recovery-v1` Checkpoint/Resume；批事务不等于整库原子性。
- 真实 SQLite/MySQL E2E、隔离 MySQL 8.0.46 CI、Ubuntu race、Windows/macOS 非流式 M1 测试。

## 支持范围与限制

| 能力 | 状态 |
| --- | --- |
| SQLite M1 CLI（多表、审批、验证） | 支持 |
| MySQL 8.0.46 M1 CLI | 支持（隔离 CI 验证） |
| SQLite Streaming / Outbox / Checkpoint / Resume | 支持（Linux amd64、rollback journal、`synchronous=FULL`） |
| MySQL Streaming / Recovery | 不支持 |
| Windows/macOS Streaming / Recovery | 不支持 |
| WAL Recovery | 不支持 |
| 持久映射 / 跨存储事务 | 不支持 |
| 通用分布式 Exactly-once | 不宣称 |
| 生产级 PII 检测准确率 / 法律匿名化 | 未证明 / 不宣称 |

- 仅处理获授权、隔离、可丢弃的数据库副本；不要直接对生产数据库运行。
- 本地 trust file + reviewer key 不是 RBAC；同时控制两者的操作者可自行批准。
- 历史目录整体回滚或撤销回滚不可检测。
- MySQL strict 不校验 CHECK 表达式、唯一重复或未触及表，也不是整库/复制一致性检查。
- 其他 MySQL 版本、Windows/macOS MySQL 运行时未验证。

## 安装与 Quick Start

从 [v1.0.0 Release](https://github.com/haoyuehx/maskriver/releases/tag/v1.0.0) 下载 Linux amd64 包，先验证归档 SHA-256：

```sh
curl -LO https://github.com/haoyuehx/maskriver/releases/download/v1.0.0/maskriver-v1.0.0-linux-amd64.tar.gz
curl -LO https://github.com/haoyuehx/maskriver/releases/download/v1.0.0/SHA256SUMS
sha256sum -c SHA256SUMS
tar -xzf maskriver-v1.0.0-linux-amd64.tar.gz
cd maskriver-v1.0.0-linux-amd64
./maskriver --version
./maskriver --help
```

从源码构建：

```sh
go build -o bin/maskriver ./cmd/maskriver
go test ./...
go vet ./...
```

SQLite 快速开始见 [Quick Start](docs/guides/quickstart.md)；MySQL 见 [MySQL 教程](docs/guides/mysql.md)。

## 验证与测试证据

- 最终 `main` CI：Ubuntu/Windows/macOS 格式、测试、vet；Ubuntu race；隔离 MySQL 8.0.46 fixture/adapter/CLI；确定性发行包；bench 与 phase3-stress。
- M3-1：strict 完整性、Writer NULL 语义、审批时钟偏差；独立 Architecture/Security/QA PASS。
- M3-2：真实 MySQL CLI E2E；独立 Architecture/Security/QA PASS。
- M3-4：345 个生成式检测 case（210 正/135 负）与 5×100 脱敏不变量；仅合成评测，不是生产准确率。
- v1.0.0 最终验收：独立 Review PASS，无未解决 P0/P1；发行资产在公开下载后复核 SHA-256。

证据入口：[Feature Matrix](docs/feature-matrix.md)、[测试计划](docs/test-plan.md)、[M3 验收](docs/m3/acceptance-matrix.md)、[v1.0.0 验收](docs/releases/v1.0.0-acceptance.md)。

## Contributors / 项目贡献者

以下账号通过 GitHub Contributors API、Git 历史、PR 作者和文件变更交叉核对。Git commit author 名称、GitHub 登录账号和真实个人身份不一定一一对应；不据此推断未经验证的个人职责。

| Contributor | Contribution Areas | Evidence |
| --- | --- | --- |
| [haoyuehx](https://github.com/haoyuehx) | 核心 CLI 与 Runner、数据库适配器集成、检测/脱敏集成、验证、Streaming/Recovery、文档与发布、展示网站 | 317 个 Git 提交；PR #35–#99 的主要作者；`cmd/`、`internal/`、`docs/`、`website/` 主要变更 |
| [emgps](https://github.com/emgps) | 脱敏策略提案、验证器提案（PR #18、#32 已合并；#17/#19/#31 未合并） | GitHub PR #18/#32 作者及文件列表；PR #17/#19/#31 提案 |
| [whh-true](https://github.com/whh-true) | 确定性脱敏策略 | PR #26（已合并）；commit `498dd7f` |
| [HerrscherofSentience555](https://github.com/HerrscherofSentience555) | SQLite/MySQL 数据库适配器 | PR #16（已合并）；commit `b8c9c5f`（Git author `szh`） |
| [PyGuy2](https://github.com/PyGuy2) | 脱敏策略实现相关 GitHub 贡献 | commit `515d628`（mask strategies）；该 commit 关联的 PR #18 由 `emgps` 发起，账号与个人职责无法进一步可靠拆分 |

AI Agent 提交身份（如 `AI M3 Orchestrator`、`AI Release Orchestrator`、`AI Orchestrator`）是自动化开发/发布身份，**不是未经核实的人类贡献者**。Git 历史未被改写，原始 PR 作者与合并提交保留。

## Development & Contribution

- 项目通过 GitHub Issue、分支、PR 和 CI 管理；合并保留真实 Git 历史，不使用 squash 覆盖作者。
- 开发与审查使用独立 AI Agent 上下文；实现者不能作为自身实现的独立评审者。
- 合并门禁包括必需 CI、独立 QA、独立安全审查、无未解决 P0/P1、文件范围检查和真实测试证据。
- 欢迎通过 Issue/PR 参与；请只使用合成或获授权的隔离数据，不要提交真实个人数据、密钥、DSN 或数据库文件。

详细的角色边界、契约和合并规则见 [AGENTS.md](AGENTS.md) 与 [Roadmap](docs/roadmap.md)。早期四人分工计划仅作为历史快照保存在 [docs/history/four-person-collaboration-plan.md](docs/history/four-person-collaboration-plan.md)。

## 文档

- [产品展示网站](https://haoyuehx.github.io/maskriver/) / [网站源码](website/)
- [操作指南与教程](docs/guides/README.md)
- [架构设计](docs/architecture.md) / [共享契约](docs/contracts.md)
- [Feature Matrix](docs/feature-matrix.md) / [测试计划](docs/test-plan.md)
- [v1.0.0 发行说明](docs/releases/v1.0.0.md) / [验收记录](docs/releases/v1.0.0-acceptance.md)
- [M3 进度](docs/m3/progress.md) / [验收矩阵](docs/m3/acceptance-matrix.md)
- [竞赛交付材料](docs/competition/submission-checklist.md)
- [来源与许可](docs/attribution.md) / [安全边界](docs/guides/security-model.md)

## License 与安全边界

MaskRiver 采用 [MIT](LICENSE)。第三方依赖保留各自许可，详见 [来源与许可说明](docs/attribution.md)。

仅处理获授权的数据库副本；脱敏不等于匿名化，确定性映射不保证唯一性或所有关联完整性。不要向仓库、日志或 Agent 会话提交真实个人数据、连接凭证、种子及映射库。完整信任边界与不宣称事项见 [安全模型](docs/guides/security-model.md)。
