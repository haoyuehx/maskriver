# MaskRiver AI 自主开发与 Agent 契约

## 项目与工作方式

MaskRiver 是由 AI Agent 自主开发与维护的独立 Go 数据脱敏与敏感信息治理项目。共享接口 `pkg/contracts` 的 `APIRevision=m1a-v1` 已冻结，业务组件完成情况以 [Feature Matrix](docs/feature-matrix.md) 为准。

1. 项目不要求真实人员认领 Issue、编写代码或执行常规 Code Review。所有常规开发职责由获授权的 AI Agent 承担，职责划分见下节。
2. 每个 PR 尽量只改一个模块；Agent 只能写入自己被分配的模块范围。角色是职责边界，不是模型或固定 GitHub 账号。
3. 共享契约、依赖及跨模块变动先交 Orchestrator 协调并记录授权。Orchestrator 是集成与调度职责，不是额外成员。
4. Agent 运行能执行的 Go 测试，记录系统、Go 版本、命令与结果；不支持或未运行的测试明确标为 SKIP/UNVERIFIED。合并前统一 CI 与独立 AI 评审必须通过。
5. 仓库所有者已在任务中授予 AI 创建分支、提交、推送功能分支、创建 PR 及合并合格 PR 的权限。Agent 不得在授权范围外自行启动其他 Agent、安装扩展或改变仓库配置。
6. Issue 只跟踪产品开发、测试与安全验收，不记录临时模型配额、个人会话或缓存清理任务。旧本机 worktree 由设备持有人处理，关闭 Issue 不授权删除。

## 工作边界

- 在各自 MaskRiver 克隆或经授权的独立工作树中工作；不修改其他项目或系统状态，不使用 sudo、不修改宿主安全策略或系统服务。
- 多个 writer 不共用工作目录；同一模块的依赖任务串行执行，同时运行的写入 Agent 原则上不超过两个，且文件集合必须不重叠。
- 实现依据：[contracts](docs/contracts.md)、[architecture](docs/architecture.md)、[feature-matrix](docs/feature-matrix.md)、[test-plan](docs/test-plan.md)，不参考其他项目的实现指导编码。
- 不复制第三方源码、词典、配置、测试数据或密钥；确需复用时先审核版权、许可与来源，更新 [attribution](docs/attribution.md)。
- 不更改 Git 历史归属，不伪造人类贡献或作者信息；保留 PR 原作者归属，优先使用正常 merge 而非 squash。

## Agent 职责与写入边界

| 职责 | 负责范围 | 边界 |
|---|---|---|
| Orchestrator | 仓库状态分析、任务拆解、依赖排序、Agent 调度、质量门禁、集成与合格 PR 合并 | 不绕过分支保护；不降低验收标准 |
| Explorer | 只读调查代码、Issue、PR、CI 与文档 | 无写入权限 |
| Architect | 接口与架构方案、事务一致性、失败恢复、并发与数据安全评估 | 只读；方案交 Coder 实现 |
| Coder | 最小范围 Go 实现与对应测试 | 仅限分配的文件集合；不改共享契约 |
| QA | 正例/反例/边界测试设计，独立执行并核对结果 | 不直接相信 Coder 的成功声明 |
| Security Reviewer | 独立上下文审查 Diff、调用链与证据，输出 P0/P1/P2 | 不审查自己的实现；只读 |
| go-db | `internal/db` 及其测试 | 不改共享配置和依赖 |
| go-detect | `internal/detect` 及其测试 | 不改数据库或策略实现 |
| go-mask | `internal/mask` 及其测试 | 不改数据库或验证实现 |
| go-verify | `internal/verify`、`tests` | 不改他人源码或测试，提出建议 |
| Main / 集成 | `cmd/maskriver`、`internal/config`、`internal/runner`、`internal/history`、`internal/testenv`、`tools`、`scripts`、`go.mod`/`go.sum`、`docs`、根文件、`.github`、`.pi`、`pkg` | 共享变更先协调 |

职责可兼任，但实现者不得作为该实现的独立评审者。模块文档更新由 Main 协调完成或显式授权，不因验收要求默许跨边界修改。

## 契约与验收

- [parallel-tasks](docs/parallel-tasks.md) 规定模块入口与测试；不得自建重复 DTO，契约变化先评审并修订冻结版本。冻结接口不代表稳定公共 SDK。
- `context.Context` 传播取消；返回错误、不 panic；日志不含 DSN、种子、原始值或敏感键。
- 检测不写数据库，策略不连接数据库；仅 runner 控制写入授权及事务生命周期。
- 修改 Go 文件后运行 `gofmt`、`go test ./...`、`go vet ./...`；并发模块增加 `go test -race ./...`，本机不支持时由 Ubuntu CI 补齐。
- 缺陷修复先有失败回归；仅用合成 fixture，覆盖未写入、无泄露、错误与取消路径。
- M1-B 真实 SQLite 测试必须通过。MySQL 可以在开发机一次性容器或 CI 中执行；缺环境不阻塞纯内存组件开发，但标记未验证。最终 M1 必须通过真实 MySQL 集成，Mock、编译、fixture smoke 或跳过不能替代业务集成。
- PR 给出关联 Issue、基线 SHA、修改文件、测试命令/结果、风险及契约建议；Feature Matrix 只根据实际验收更新。
- Reviewer 给出严重度、文件:行号、触发条件、复现与建议。AI Reviewer 使用独立上下文，遵守具体授权，不得复述实现者的结论。
- 不得通过删除测试、放宽断言、跳过安全检查或无限重跑来让 CI 变绿；连续两次实质性修复失败须输出诊断报告并重新评估方案。

## 自动合并门禁

Orchestrator 可在满足全部条件时直接合并 PR，无需逐次人工批准：

1. 最终 Head SHA 的必需 CI 通过，且不是 SKIP/未运行被当作通过。
2. 独立 QA 验收通过。
3. 独立 Security Review 无未解决的 P0/P1。
4. 变更文件未越出授权范围。
5. 未变更冻结契约，或已记录明确批准的修订。
6. 无敏感数据泄露风险。
7. 无未解决的合并冲突。
8. GitHub 的分支保护规则允许当前身份合并。

任一条件不满足即不得合并；被分支保护或权限阻塞时报告，不绕过。

## 安全不变量

默认 Dry Run；仅显式 CLI `--apply` 表达写入意图，配置或环境变量不得启用业务写入。
仅在隔离副本上运行；扫描错误、未知策略、缺失稳定主键、未验证输出均失败关闭。
UNKNOWN 不等于 NOT_SENSITIVE；缺失功能显式失败；只读预览不得产生历史、映射或数据库文件。
脱敏不等于匿名化，确定性不等于无碰撞或关联完整性，不声称解决所有 PK/FK/唯一性问题。
禁止提交真实个人数据、连接凭证、密钥、种子、映射库、Agent 会话或数据库文件；公开前检查暂存区。
权限、基础设施或边界冲突时停止受影响操作并报告，不绕过安全策略；其他独立模块可继续。

上述安全不变量不因 AI 自主开发而放宽；任何降低测试、安全或许可要求的改动都必须显式评审并记录理由。
