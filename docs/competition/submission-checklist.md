# 比赛提交检查清单

> 本文件是参赛研发准备材料，不代表已报名、入选或获官方背书。

- [x] v1.0.1 稳定版 Release 与 Linux amd64 资产
- [x] SQLite 多表 CLI：Scan/Preview/Approve/Apply/Validate
- [x] MySQL 8.0.46 真实隔离 CI CLI E2E
- [x] Ed25519 审批、Strict Validate、SQLite Streaming / Checkpoint / Resume
- [x] CI：Ubuntu/Windows/macOS、race、MySQL、release package
- [x] 产品展示网站与 GitHub Pages 部署
- [x] README 真实 Contributor 与历史分工迁移
- [x] 竞赛概览、核心功能、技术亮点、演示指南
- [ ] 最终配音视频（新版已制作并完成技术 QC，等待用户试听确认后上线）
- [x] 安全边界与非宣称事项
- [x] 许可证、第三方来源和发布资产保留
- [x] v1.0.1 补丁发行、公开回下载与 WAL 目标拒绝黑盒验证
- [x] [最终验收记录](final-acceptance.md)

## 当前限制

MySQL Recovery、Windows/macOS Streaming/Recovery、WAL Recovery、持久映射、跨存储原子性、通用 Exactly-once、生产级 PII 准确率与法律匿名化均不宣称。详见 [安全模型](../guides/security-model.md) 与 [v1.0.1 发行说明](../releases/v1.0.1.md)。
