# MaskRiver 四人协作与 Agent 契约

## 项目与工作方式

MaskRiver 是四位成员共同维护的独立 Go 数据脱敏与敏感信息治理项目。共享接口 `pkg/contracts` 的 `APIRevision=m1a-v1` 已冻结，业务组件完成情况以 [Feature Matrix](docs/feature-matrix.md) 为准。

1. 四位成员各自在 Windows、Linux 或 macOS clone 本仓库，认领 Issue，创建个人功能分支，通过 PR 交付；不要求共享目录或同一台机器。
2. 每个 PR 尽量只改一个模块；仅模块负责人和经授权的协作者修改对应模块。角色是职责，不是固定 GitHub 账号或必须启动的 Agent。
3. 共享契约、依赖及跨模块变动先交 Main（集成协调职责，可由团队成员承担）协调。Main 不等于第五名成员。
4. 各自运行能执行的 Go 测试，记录系统、Go 版本、命令与结果；不支持或未运行的测试明确标为 SKIP/UNVERIFIED。合并前统一 CI 和代码评审必须通过。
5. Pi Multi-Agent 只是可选工具，不是开发前置条件。Agent 仅在明确授权的范围内工作，不自行启动其他 Agent、安装扩展、提交或推送；人类成员可正常提交、推送功能分支和创建 PR。
6. Issue 只跟踪产品开发、测试与安全验收，不记录临时模型配额、个人会话或缓存清理任务。旧本机 worktree 由设备持有人处理，关闭 Issue 不授权删除。

## 工作边界

- 在各自 MaskRiver 克隆或经授权的独立工作树中工作；不修改其他项目或系统状态，不使用 sudo、不修改宿主安全策略或系统服务。
- 多个 writer 不共用工作目录；Git worktree 和 Native 隔离均为可选方式，不强制路径、操作系统、模型或开发工具。
- 实现依据：[contracts](docs/contracts.md)、[architecture](docs/architecture.md)、[feature-matrix](docs/feature-matrix.md)、[test-plan](docs/test-plan.md)，不参考其他项目的实现指导编码。
- 不复制第三方源码、词典、配置、测试数据或密钥；确需复用时先审核版权、许可与来源，更新 [attribution](docs/attribution.md)。

## 模块责任

| 职责 | 负责范围 | 边界 |
|---|---|---|
| Main / 集成协调 | `cmd/maskriver`、`internal/config`、`internal/runner`、`internal/history`、`internal/testenv`、`tools`、`scripts`、`go.mod`/`go.sum`、`docs`、根文件、`.github`、`.pi`、`pkg` | 共享变更先协调 |
| go-db | `internal/db` 及其测试 | 不改共享配置和依赖 |
| go-detect | `internal/detect` 及其测试 | 不改数据库或策略实现 |
| go-mask | `internal/mask` 及其测试 | 不改数据库或验证实现 |
| go-verify | `internal/verify`、`tests` | 不改他人源码或测试，提出建议 |
| reviewer（人类或 AI） | 只读审查、PR 评论 | 修复另交模块负责人 |

四位成员可兼任集成或评审职责，具体认领记录在 Issue/PR。模块 PR 的文档更新由 Main 协调完成或显式授权，不因验收要求默许跨边界修改。

## 契约与验收

- [parallel-tasks](docs/parallel-tasks.md) 规定模块入口与测试；不得自建重复 DTO，契约变化先评审并修订冻结版本。冻结接口不代表稳定公共 SDK。
- `context.Context` 传播取消；返回错误、不 panic；日志不含 DSN、种子、原始值或敏感键。
- 检测不写数据库，策略不连接数据库；仅 runner 控制写入授权及事务生命周期。
- 修改 Go 文件后运行 `gofmt`、`go test ./...`、`go vet ./...`；并发模块增加 `go test -race ./...`，本机不支持时由 Ubuntu CI 补齐。
- 缺陷修复先有失败回归；仅用合成 fixture，覆盖未写入、无泄露、错误与取消路径。
- M1-B 真实 SQLite 测试必须通过。MySQL 可以在开发机一次性容器或 CI 中执行；缺环境不阻塞纯内存组件开发，但标记未验证。最终 M1 必须通过真实 MySQL 集成，Mock、编译、fixture smoke 或跳过不能替代业务集成。
- PR 给出关联 Issue、基线 SHA、修改文件、测试命令/结果、风险及契约建议；Feature Matrix 只根据实际验收更新。
- reviewer 给出严重度、文件:行号、触发条件、复现与建议。AI Reviewer 遵守具体授权；人类可运行安全的合成测试，不强制某个工具 allowlist。
- 经统一 CI 和独立评审后，由有合并权限的维护者合并；作者不自行绕过审核。未运行测试或配置就绪不等于执行成功。

## 安全不变量

默认 Dry Run；仅显式 CLI `--apply` 表达写入意图，配置或环境变量不得启用业务写入。
仅在隔离副本上运行；扫描错误、未知策略、缺失稳定主键、未验证输出均失败关闭。
UNKNOWN 不等于 NOT_SENSITIVE；缺失功能显式失败；只读预览不得产生历史、映射或数据库文件。
脱敏不等于匿名化，确定性不等于无碰撞或关联完整性，不声称解决所有 PK/FK/唯一性问题。
禁止提交真实个人数据、连接凭证、密钥、种子、映射库、Agent 会话或数据库文件；公开前检查暂存区。
权限、基础设施或边界冲突时停止受影响操作并报告，不绕过安全策略；其他独立模块可继续。
