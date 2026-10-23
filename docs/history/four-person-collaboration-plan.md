# 历史快照：四人跨平台协作计划

> **历史文档。** 这是项目早期（M1）用于组织 Issue/PR 的计划表格，不代表当前团队、贡献者或最终分工。当前贡献者与证据见根目录 [README](../../README.md#contributors--项目贡献者)。

早期计划把工作划分为：

| 角色（历史占位） | 当时计划的主责 | 修改边界 |
| --- | --- | --- |
| A · Main/集成协调 | Config、Plan、Runner、CLI、合并与发布协调 | `cmd/maskriver/**`、`internal/config/**`、`internal/runner/**`、`internal/history/**` |
| B · 数据库/事务 | SQLite 快照、SQL Spy、只读/写入/回滚和 MySQL 适配器回归 | `internal/db/**` |
| C · 检测/脱敏 | UNKNOWN 与策略门禁、格式约束、映射一致性和合成负例 | `internal/detect/**`、`internal/mask/**` |
| D · 验证/QA | 验证器、真实端到端回归与独立验收 | `internal/verify/**`、`tests/**` |

这些字母从未对应到四个可核实的 GitHub 账号；后续实际贡献来自不同 PR 作者、维护者与 AI Agent。保留此文件仅用于历史追溯，不据此推断任何个人职责。
