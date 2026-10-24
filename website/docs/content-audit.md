# 内容事实核对

网站文案以仓库 `README.md`、`docs/feature-matrix.md`、`docs/m3/*`、`docs/guides/*` 与 `docs/releases/v1.0.1.md` 为依据，未把计划中或未验证的能力写成已交付。

| 网站表述 | 依据 |
|---|---|
| SQLite 完整 M1 流程，MySQL 8.0.46 CLI 已通过隔离 CI | `docs/feature-matrix.md`、PR #93、`TestMySQLCLIWorkflow` |
| 默认只读预览；写入需要 `--apply`、`--approved` 与计划绑定的 Ed25519 审批 | `README.md`、`docs/guides/approval.md`、`internal/runner/approval.go` |
| SQLite-only streaming/outbox/checkpoint/recovery | `docs/guides/streaming-recovery.md`、`docs/m3/platform-matrix.md` |
| 恢复仅 Linux amd64、rollback journal、`synchronous=FULL`、确定性策略 | `docs/design/issue14-recovery-protocol.md`、验收报告 |
| MySQL recovery、Windows/macOS streaming/recovery、WAL recovery 不支持 | `docs/m3/platform-matrix.md`、CLI 运行时拒绝 |
| 脱敏不等于匿名化；不保证无碰撞/不可关联 | `docs/guides/security-model.md`、`README.md` |
| 检测评测为合成/生成式，不证明生产 PII 准确率 | `docs/m3/detection-evaluation.md`、PR #95 |
| Strict 验证：SQLite quick/FK；MySQL CHECK TABLE + 可见 FK | `internal/verify/verify.go`、`internal/db/integrity.go`、M3-1/M3-2 文档 |
| 下载与文档链接 | `v1.0.1` Release、`docs/guides/*` |
| 架构图组件 | `internal/db`、`internal/detect`、`internal/mask`、`internal/verify`、`internal/runner`、`internal/history` |

## 明确不写入网站的内容

- 不写 Kubernetes、Kafka、微服务、云审批、分布式事务或“生产已验证”。
- 不写性能提升倍数、固定内存上界或 exactly-once。
- 不写客户、用户数量、认证、合规结论或团队开发日志。
- 不把概念 UI 写成现有 Web 后台；流程演示均标注“示意”。
- 视频评审指出无配音与未覆盖 MySQL，未在网站中隐去。

## 已知文案边界

- “4 分 20 秒”来自 260 秒交付时长。
- 站点只承诺 Linux amd64 发行包；macOS/Windows 仅记录非流式 M1 CI，不提供对应产物。
- 页面不包含分析、追踪或真实脱敏 API。
