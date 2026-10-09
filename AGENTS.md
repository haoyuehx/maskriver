# MaskRiver Agent 协作契约

## 当前授权范围

M1-A 已完成设计。本轮仅 Main 做 M1-A 收尾与 M1-B 启动准备、Native Worktree 预检、冻结提交/推送与单 go-db 冒烟方案设计；不启动四个正式 Worker，不进行大规模迁移。单 Agent 模型冒烟须按独立方案明确批准后执行，不因文件存在自动启动。
后续由用户授权 Main 拆分有界任务；子 Agent 不得递归委派、发布、推送或操作 GitHub。

## 源码参考边界

- 工作仓库为 `/home/haoyue/Project/maskriver`；上游必须用绝对路径 `/home/haoyue/Project/dbmask`，只能读取源码、测试、文档及许可。所有子 Agent 禁止从 worktree 使用 `../dbmask` 推导路径。
- 上游基线 `7d8789ef4883a423ddba2f8934b95997d1aaf099`；不得修改任何上游文件，包括 `.git`。
- 禁止在上游执行 init/checkout/switch/commit、安装依赖、运行测试或生成缓存；只读 Git 查询使用 `GIT_OPTIONAL_LOCKS=0`。
- 每次任务先确认 cwd/Git 根目录为 MaskRiver，或 Native 分配的 `/home/haoyue/Project/worktrees/maskriver/` 下独立 worktree；`git --git-common-dir` 必须指向 `/home/haoyue/Project/maskriver/.git`，不得用上游做 worktree。
- 项目文件与 Git 写操作仅限 MaskRiver 主仓库及上述已授权 worktree；Main 可维护已安装 pi-subagents 的已知 Native 配置。不得复制密钥、环境配置、生产数据或映射库。
- 参考行为和测试意图，不机械翻译。改编部分注明源文件/提交，保留 MIT 署名及 `docs/upstream-LICENSE`。

## 文件所有权

| Owner | 独占写入范围 | 不可擅自修改 |
|---|---|---|
| Main | cmd/maskriver、internal/config、internal/runner、internal/history、internal/testenv、tools、scripts、go.mod/go.sum、docs、根文件、.pi、pkg（含 contracts） | 上游及其他仓库 |
| go-db | internal/db 及其单元测试 | 共享配置、模块依赖清单 |
| go-detect | internal/detect 及其单元测试 | DB/策略实现 |
| go-mask | internal/mask 及其单元测试 | DB/验证实现 |
| go-verify | internal/verify、tests | 他人模块中的测试或源码；需提修改建议 |
| go-reviewer | 无，只读审查并在回复中给出发现 | 所有文件；不执行 shell、不运行测试 |

跨边界改动先交 Main 协调，不能顺手修复。新依赖由 Main 审核许可证、必要性、版本后统一加入。
每个并发写 Agent 使用独立 worktree；一份工作树只允许一个 writer；Main 在子任务完成前不改其文件。
Worktree 根固定 `/home/haoyue/Project/worktrees`；仅使用 pi-subagents Native，实际叶目录为 `maskriver/pi-worktree-<runId>-<index>`。禁止仓库内 `.local/worktrees/`、手动共享 cwd 或分配失败后降级运行。
Main 在干净冻结 ref 上申请 worktree，记录路径/branch/baseCommit，先保存并验证补丁，再按证据回收；禁止子 Agent git add/commit/push/merge，框架收尾捕获补丁可暂存，其行为不等于子 Agent 提交。
提示词与文件所有权不是 OS 沙箱：迁移前应限制凭证/网络，并将上游以只读方式提供。

## 模块契约

以 `docs/contracts.md` 和 `pkg/contracts` 的 `APIRevision=m1a-v1` 为 M1-B 组件冻结基线；architecture 是背景设计。
Main 独占共享类型、接口、go.mod/go.sum 与合成环境；Worker 不得修改或自建重复 DTO，变更必须交 Main 评审并修订冻结版本。
四条任务范围及入口签名见 `docs/parallel-tasks.md`；接口冻结不等于运行授权或稳定公共 SDK。
`context.Context` 传播取消；错误返回，不 panic；日志不含 DSN、种子、原始值或敏感键。
检测不写 DB，策略不连接 DB；只有 runner 控制写入授权和事务生命周期。
缺失功能显式失败；UNKNOWN 与 NOT_SENSITIVE 不可混同；只读预览不产生历史/映射/数据库文件。

## 验收标准

- 每个任务给出目标、cwd/ref、文件所有权、输入/输出契约、验收用例、停问条件。
- 修改后 gofmt、go test ./...、go vet ./...；并发模块增加 go test -race ./...。
- 漏洞修复先有失败回归；采用合成 fixture，证实未写入、无泄露、错误路径和取消路径。
- M1-B 首轮 SQLite 真实集成必须通过；MySQL 保留实现与集成测试要求，暂可标记 UNVERIFIED/SKIP 继续组件开发，但不得把 Mock/编译/跳过当真实集成通过。最终 M1 验收前 MySQL 真实集成必须通过。
- 返回修改文件、测试命令/结果、风险、契约变更建议；不得把配置就绪说成模型执行成功。
- reviewer 输出严重度、文件/行号、复现条件和建议；只读工具 allowlist 为 read/grep/find/ls。
- Main 独占最终整合、验收、提交/推送权限。Feature Matrix 随每项验收更新。

## 安全不变量

默认 Dry Run；仅显式 CLI --apply 表达写入意图，配置文件不能静默启用写入。
在隔离副本上运行；扫描错误、未知策略、无稳定主键、未验证输出应失败关闭。
脱敏不等于匿名化；确定性不等于无碰撞；禁止对外声称已解决所有 PK/FK/唯一性问题。
禁止上传会话记录、真实 PII、DSN、密钥、种子及映射库；公开前检查暂存区。
出现权限不足、模型不可用、边界冲突或基础设施失败，停止并报告，不自动换模型/安装扩展。
