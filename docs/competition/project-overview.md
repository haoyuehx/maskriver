# MaskRiver 项目概览

MaskRiver 是独立开发的 Go 数据脱敏与敏感信息治理工具，面向开发、测试与数据共享场景。它把一次脱敏组织成可验证的工程流程：

`Scan → Preview → Approve → Apply → Validate`

- **Scan**：在授权的隔离副本上抽样识别敏感字段，证据不足时保留 `UNKNOWN`。
- **Preview**：只读生成确定性计划，不写入目标副本。
- **Approve**：由独立进程签发 Ed25519 审批记录，绑定计划摘要。
- **Apply**：仅在显式 `--apply` 与有效审批同时存在时写入计划字段。
- **Validate**：全量核对 Schema、行数、双向键集、字段变化和数据库完整性。

支持 SQLite 完整流程；MySQL 8.0.46 的 CLI 流程已在隔离 CI 中真实验证。Linux amd64 上支持 SQLite-only 的有界 Streaming、Outbox、Checkpoint 与 Resume。

发布：`v1.0.0`（Linux amd64）。官网：https://haoyuehx.github.io/maskriver/ 。

## 开发方式

项目通过 GitHub Issue、PR 与 CI 推进开发和验证。

**支持范围：** SQLite 核心 CLI 与本地 Streaming/Recovery；MySQL 8.0.46 核心 CLI。具体环境和功能要求见使用指南。
