# AI Agent 协作模型

MaskRiver 采用 AI Agent 自主开发、独立评审与统一 CI 的协作模式。完整边界与安全不变量见 [AGENTS.md](../AGENTS.md)。

## 职责不是底层模型

同一底层模型可以承担多个职责，但每个职责使用独立上下文，且实现者不得作为该实现的独立评审者。Orchestrator 负责调度与合并，不代表存在额外成员。

| 职责 | 范围 | 写入权限 |
|---|---|---|
| Orchestrator | 状态分析、任务拆解、调度、门禁、合并 | 集成与共享范围 |
| Explorer | 代码、Issue、PR、CI 调查 | 无 |
| Architect | 接口与架构方案、事务/并发/安全评估 | 无 |
| Coder | 最小范围实现与测试 | 分配的文件集合 |
| QA | 正例/反例/边界测试与独立执行 | 测试范围 |
| Security Reviewer | 独立审查、P0/P1/P2 结论 | 无 |

模块写入边界：

| 模块 | Issue | 范围 |
|---|---|---|
| go-db | #5 | internal/db |
| go-detect | #6 | internal/detect |
| go-mask | #7 | internal/mask |
| go-verify | #8 | internal/verify、tests |

共享类型、依赖、测试环境及文档由 Main 协调。任务下发时写明基线 SHA、允许与禁止的文件、验收测试与预期交付物，不需要记录绝对路径。

## 工具与并发

Agent 可以在获授权范围内使用仓库工具、分支和工作树。不要求特定模型、配额、个人会话、补丁缓存或 Native worktree。并发写入必须有独立目录，且文件集合不重叠；同一模块的依赖任务串行执行。

`.pi/` 中的 Agent 定义是可选配置，不是 CI、Issue 或参与项目的前置条件；旧配置不能覆盖当前协作规则。

## 交付与评审

通过 PR 交付代码与合成测试，提交实际测试结果与未验证项。统一 CI 通过后，由独立上下文的 Security Reviewer 审查；实现者不得批准自己的实现，也不得单独关闭自己实现上的 P0/P1（须由第二个独立上下文复核）。Orchestrator 在 [AGENTS.md](../AGENTS.md) 自动合并门禁全部满足、且已把被评审的 Head SHA 与证据记录在 PR 中时直接合并；分支保护或权限阻塞时报告，不绕过。

评审发现写入 PR，修复由对应模块的 Coder 处理；禁止上传会话、凭证、原始个人数据或数据库文件。SKIP 与 UNVERIFIED 必须显式标注，不得当作 PASS。
