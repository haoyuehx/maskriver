# 技术亮点

1. **审批绑定执行计划**：`--approved` 是声明，`--apply` 是写入意图，二者仍需一条绑定 Plan 的 Ed25519 记录；审批不替代检测证据。
2. **身份与快照**：SQLite 源由主文件摘要与 journal-mode 门禁绑定；MySQL 使用计划表全内容摘要，并在连接后以 `@@server_uuid` + `DATABASE()` 拒绝同一物理库。
3. **严格验证 fail-closed**：SQLite `quick_check` + 数据库级 `foreign_key_check`；MySQL 每行 `CHECK TABLE ... QUICK` 必须是 `status/OK`，并对计划表相关的可见 FK 做 orphan 查询。
4. **有界执行**：分批读取、显式资源预算、批次事务和 Outbox 回执。
5. **本地恢复**：业务更新、Outbox Receipt、Progress 与 Manifest CAS 在同一本地 SQLite 事务中提交；恢复核对已提交前缀并跳过正确部分，冲突拒绝覆盖。
6. **真实测试**：SQLite 多表 CLI、真实 MySQL 8.0.46 隔离容器、Ubuntu race、Windows/macOS 非流式 M1、确定性发行包与 Playwright 网站测试。

## 开发方式

项目采用 GitHub Issue、PR 与 CI 管理研发及验收。

## 架构

`CLI/Config → Detection → Plan → Approval → Runner → Masking → DB Adapter → Validation`，Streaming/Outbox/Recovery 为受支持的本地 SQLite 增量路径。
