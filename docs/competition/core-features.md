# 核心功能

| 功能 | 用户价值 | 证据 |
| --- | --- | --- |
| 敏感信息识别 | 在开发/测试副本上发现邮箱、手机号、身份证、银行卡、USCC、日期、城市、姓名、地址等字段；UNKNOWN 不被当作安全 | 13 类规则；`internal/detect`；生成式 holdout |
| 只读预览 | 先看计划与范围，不向目标写入 | `TestCLIRealSQLiteWorkflow`、SQL Spy |
| Ed25519 审批 | 执行前需要与计划绑定的签名决策，过期、撤销、跨计划和未授权记录拒绝 | `TestE2EPlanIssueNewProcessApply`、approval tests |
| 确定性脱敏 | 格式约束替换与 HMAC 策略，输出可重放 | `internal/mask`、CN format tests |
| 严格验证 | 全量扫描、行数/键集/Schema/字段和完整性检查，失败关闭 | `TestStrictRequiresFullScanAndIntegrityCapability`、MySQL CLI E2E |
| 有界 Streaming | 分批读取、批次事务、资源预算、Outbox 回执 | `m2-stream-v1`、CI、SQLite E2E |
| Checkpoint / Resume | 中断后核对已提交部分并继续未完成部分，冲突拒绝覆盖 | `m2-recovery-v1`、真实 CLI 中断测试 |
| SQLite / MySQL 适配 | 本地 SQLite 与 MySQL 8.0.46 CLI；MySQL 同一物理库拒绝 | 隔离 MySQL CI `TestMySQLCLIWorkflow` |
| 产品展示网站 | 面向评委的流程、架构、视频、限制与快速开始 | https://haoyuehx.github.io/maskriver/ |

## 支持范围

SQLite 支持完整核心 CLI 和指定 Linux 环境中的 Streaming/Recovery；MySQL 8.0.46 支持核心 CLI 工作流。详细功能说明见项目文档。
