# MaskRiver Agent 协作契约

## 项目定位与当前授权范围

MaskRiver 是独立研发的 Go 数据脱敏与敏感信息治理工具：敏感数据自动检测、安全且确定性的脱敏策略、SQLite/MySQL 支持、数据完整性校验；后续阶段引入有界流式处理、失败恢复与中文敏感数据治理。

当前处于 **M1-B 启动准备**：接口 `m1a-v1` 已冻结，测试环境与隔离工作树已验证，四个正式 Worker **未启动**。
本轮只允许 Main 做仓库内文档、配置与协调工作；不启动多 Agent、不执行正式业务开发、不新建分支。正式 M1-B 启动需用户另行批准。
子 Agent 不得递归委派、不得提交或推送、不得操作 GitHub。

## 工作边界

- 项目根固定为 `/home/haoyue/Project/maskriver`；只在本仓库及其经批准的隔离工作树内读写。
- 隔离工作树仅通过 Native 机制在 `/home/haoyue/Project/worktrees/maskriver/` 分配；`git --git-common-dir` 必须指向 `/home/haoyue/Project/maskriver/.git`。分配失败即停止，不得共享 cwd 降级。
- 实现与验收以本仓库的 [docs/contracts.md](docs/contracts.md)、[docs/architecture.md](docs/architecture.md)、[docs/feature-matrix.md](docs/feature-matrix.md)、[docs/test-plan.md](docs/test-plan.md) 为依据；不参考其他项目的实现来指导编码。
- 不得复制第三方源码、词典、配置、测试数据或密钥。若将来确实需要改编或复用第三方内容，先保留其版权、许可与来源标注，并更新 [docs/attribution.md](docs/attribution.md)。
- 不得修改本仓库以外的任何项目或系统状态；不得使用 `sudo`，不得修改宿主机安全策略或系统服务。

## 文件所有权

| Owner | 独占写入范围 | 不可擅自修改 |
|---|---|---|
| Main | `cmd/maskriver`、`internal/config`、`internal/runner`、`internal/history`、`internal/testenv`、`tools`、`scripts`、`go.mod`/`go.sum`、`docs`、根文件、`.pi`、`pkg`（含 contracts） | 本仓库之外的一切 |
| go-db | `internal/db` 及其单元测试 | 共享配置、模块依赖清单 |
| go-detect | `internal/detect` 及其单元测试 | 数据库或策略实现 |
| go-mask | `internal/mask` 及其单元测试 | 数据库或验证实现 |
| go-verify | `internal/verify`、`tests` | 他人模块中的源码与测试；需提修改建议 |
| go-reviewer | 无：只读审查，结论写入回复 | 所有文件；不执行 shell、不运行测试 |

跨边界改动先交 Main 协调，不能顺手修复。新依赖由 Main 审核许可证、必要性、版本后统一加入。
每个并发写 Agent 使用独立工作树；一份工作树只允许一个 writer；Main 在子任务完成前不改其文件。
Main 在干净冻结 ref 上申请工作树，记录路径 / branch / baseCommit，先保存并验证补丁，再按证据回收；禁止子 Agent `git add`/`commit`/`push`/`merge`。
提示词与文件所有权不是操作系统沙箱：运行前应隔离生产凭证与网络。

## 模块契约

以 `pkg/contracts` 的 `APIRevision=m1a-v1` 与 [docs/contracts.md](docs/contracts.md) 为 M1-B 组件冻结基线；[docs/architecture.md](docs/architecture.md) 为设计背景。
Main 独占共享类型、接口、`go.mod`/`go.sum` 与合成环境；Worker 不得修改或自建重复 DTO，变更必须交 Main 评审并修订冻结版本。
四条任务的文件范围、入口签名与验收用例见 [docs/parallel-tasks.md](docs/parallel-tasks.md)。接口冻结不等于运行授权，也不等于稳定公共 SDK。
`context.Context` 传播取消；错误返回，不 panic；日志不得包含 DSN、种子、原始值或敏感键。
检测不写数据库，策略不连接数据库；只有 runner 控制写入授权与事务生命周期。
缺失功能显式失败；UNKNOWN 与 NOT_SENSITIVE 不可混同；只读预览不得产生历史、映射或数据库文件。

## 验收标准

- 每个任务给出目标、cwd/ref、文件所有权、输入/输出契约、验收用例与停问条件。
- 修改后运行 `gofmt`、`go test ./...`、`go vet ./...`；并发模块增加 `go test -race ./...`。
- 缺陷修复先有失败回归；使用合成 fixture，并覆盖未写入、无泄露、错误路径与取消路径。
- M1-B 首轮真实 SQLite 集成必须通过；MySQL 保留实现与集成测试要求，可标记 `UNVERIFIED`/`SKIP` 继续组件开发，但不得把 Mock、编译或跳过当作真实集成通过。最终 M1 验收前 MySQL 真实集成必须通过。
- 返回修改文件、测试命令与结果、风险与契约变更建议；不得把配置就绪说成模型执行成功。
- reviewer 输出严重度、文件:行号、复现条件与建议；只读工具 allowlist 为 `read, grep, find, ls`。
- Main 独占最终整合、验收、提交与推送权限。Feature Matrix 随每项验收更新。

## 安全不变量

默认 Dry Run；只有显式 CLI `--apply` 表达写入意图，配置文件或环境变量不得启用写入。
仅在隔离副本上运行；扫描错误、未知策略、缺失稳定主键、未验证输出一律失败关闭。
脱敏不等于匿名化；确定性映射不等于无碰撞，也不保证唯一性或关联完整性；不得对外声称已解决全部 PK/FK/唯一性问题。
禁止提交或上传真实个人数据、连接凭证、密钥、种子、映射库、Agent 会话记录或数据库文件；公开前检查暂存区。
出现权限不足、模型不可用、边界冲突或基础设施失败，停止并报告，不自动更换模型或安装扩展。
