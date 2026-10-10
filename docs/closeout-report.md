# M1-A 收尾与 M1-B 启动准备报告

> 历史设备记录，不是当前协作规范。下文的 Native 路径、模型门禁及待处理清单已退出项目开发前置条件；当前以 [AGENTS.md](../AGENTS.md) 和 [跨平台 PR 流程](m1b-launch-plan.md) 为准。旧资源由设备持有人处理，此次改造未删除工作树，也未把旧 smoke 标记为通过。

## 已完成的准备

- 共享接口 `pkg/contracts` APIRevision=m1a-v1 与 docs/contracts.md 对照一致；未更改冻结接口语义，四 Worker 的入口/依赖/独占文件范围仍一致。
- SQLite modernc.org/sqlite v1.60.1（BSD-3-Clause），MySQL github.com/go-sql-driver/mysql v1.10.1（MPL-2.0），go.mod/go.sum 固定；直接依赖许可保留原文，Go module 校验通过。
- Native 唯一根 `/home/haoyue/Project/worktrees`，实际项目子目录 maskriver；Main 在已安装 pi-subagents 0.76.1 的用户配置设 provider=native/baseDir。
- 实际配置加载/优先级/非法路径拒绝预检通过；这不把旧 Pi 会话视作已热更新，执行模型 smoke 前需 reload/重启。
- 五个 Agent 被 list 正确发现；models 的项目映射与指定值一致，doctor 显示 async 可用、active runs=0。
- 单 go-db 冒烟脚本 `.pi/workflows/go-db-smoke.js` 仅准备，静态 validate 返回 ok=true。静态工具提示 dynamic-spawn-count，未来 runtime 必须限制 maxSubagentSpawnsPerRun=1；校验不证明实际模型调用。
- 四正式 Worker 未启动，没有业务代码开发；Native 预检由 Main 直接调用现有 allocator，不使用外部 Agent/CLI fallback。

## 验证与边界

`gofmt`、`go test -count=1 ./...`、`go vet ./...`、`go mod verify`、diff whitespace 检查通过。
真实 SQLite 合成 fixture 测试通过；MySQL opt-in 测试 SKIP，未把默认 test 通过当 MySQL 通过。
MySQL 阻塞明确发生于新 datadir 的 `mysqld --initialize-insecure`：OS errno 13 Permission denied，服务尚未启动，尚未进入驱动连接/测试。无 sudo/系统权限修改/生产库访问。
首轮允许 MySQL UNVERIFIED，但实现和真实测试任务保留；最终 M1 必须补齐。非特权候选环境见 test-environment.md。

## Native 实际分配验收

已在冻结提交 `816252addf30d818fd50c879c9dfedf7f0e637f7` 的 clean main 上实际运行 `node scripts/check-native-worktree.mjs --allocate`，结果 PASS。

- provider：native；实际路径 `/home/haoyue/Project/worktrees/maskriver/pi-worktree-main-native-probe-1791546842314-0`。
- 临时 branch：`pi-subagents/main-preflight-main-na-9126-s0-t0`；共同 Git 根确认为 MaskRiver `.git`，baseCommit 等于冻结提交。
- Main 仅在新工作树生成 `internal/db/native_probe_test.go`，执行 gofmt、go test -count=1 ./...，包含真实 SQLite 合成测试，全部通过；MySQL 仍跳过。
- Native 捕获并验证一个文件、12 行新增的补丁；SHA-256 `ca4fa3429d0b204a93f1d55a879a23de0cb40432781ebe5a7f0b5d42b6a5bbf6`。
- 本地证据 `.local/main-native-probe-1791546842314/`：setup.json、tests.log、diffs.json、patches/task-0-main-preflight.patch、cleanup.json。证据不公开上传。
- 保存补丁后只移除 Main 自己新增的诊断文件，再由 Native 回收：cleanup.state=complete，worktreeRemoved=true、branchRemoved=true；git worktree list 仅剩主工作树，pi-subagents 分支列表为空。
- Main HEAD 未变、无自动 apply/merge/push 诊断补丁；仓库未引入外部仓库文件。

该预检 agentLaunched=false、modelCalled=false、merged=false，证明的是 allocator/隔离/补丁/回收链，不是模型调用。

## 单 go-db 模型冒烟：已尝试，被模型配额阻断

用户在批准“设计并执行单 go-db 冒烟”后，Main 于 `d295c664f9fa8e37aab7b13fe5fd11578d8f889b` 上实际执行了一次 workflow（runId `9195e6c5-1516-456d-8b1a-155a3c298729`）。

已证实的部分：

- Worktree 分配成功且隔离正确：`/home/haoyue/Project/worktrees/maskriver/pi-worktree-9195e6c5-1516-456d-8b1a-155a3c298729-s0-0`，branch `pi-subagents/go-db-smoke-9195e6c-a37e-s0-t0`，baseCommit 正是冻结提交。
- 子会话在正确 cwd 启动，并加载了该 worktree 内的 AGENTS.md 项目上下文，说明隔离与上下文注入生效。
- 子 Agent 收到正确任务与绑定输出路径。

未完成的部分与其原因：

- 模型调用被 provider 拒绝：`provider=openai-codex, model=gpt-5.6-sol, stopReason=error, errorMessage="Codex error: The usage limit has been reached"`，`usage.totalTokens=0`，耗时约 2.5s。属于账号/模型配额限制，不是隔离、配置或任务问题，也不是代码缺陷。
- 因此没有产生测试、补丁或子会话输出：handoff 记录 `changed=false`、`filesChanged=0`，输出文件未生成。
- 框架把 worktree 保留下来（`cleanup.state=partial`，原因 `retained child resume requires managed worktree cwd`），`resumeDisposition=resumable`。这是既有的保留行为，不是泄漏；worktree 内容为 clean（与基线一致，无改动）。

结论：Native 隔离、基线对齐、上下文注入、补丁通道已获真实证据；**“模型在隔离 worktree 内完成测试、产出补丁并回收”仍未被端到端证明**，当前唯一阻塞是该模型的用量配额。Main 按规则不自动更换模型。

## Git 与启动门禁

共享基线提交 `816252a` 已推送 origin/main；后续仅补记本报告与许可证空白豁免，最终 local/remote HEAD 以交付命令对照为准（避免在提交内引用自身 SHA）。
首次暂存 whitespace 检查发现 MPL 许可原文第38行尾随空格；因 shell 命令链边界错误，第一次 commit/push 先于自动内容筛查执行。已立即对该提交全部35个文件补查，没有凭证/数据/会话文件；两个驱动 LICENSE 与 module cache 原文逐字节一致。后续使用 `set -e`，`.gitattributes` 仅对这一份未改写的第三方许可禁用 whitespace 检查，不放宽源码检查、不重写公开 Git 历史。
最终补记提交前再次检查完整 index 的路径/内容：不提交 .local 数据库/诊断记录、认证/环境配置、Agent 会话、worktree/patch artifacts。模式扫描只是防误提交辅助，不宣称完整安全审计。
M1-B 组件计划与环境门禁已准备；完整四 Agent 正式启动仍等待单 go-db 模型冒烟通过及用户正式授权，不能把 registry/config 存在当成模型调用验证。

## 待处理清单（M1-B 前）

1. **模型配额**：`openai-codex/gpt-5.6-sol` 返回用量上限；需配额恢复，或由用户决定是否批准替代模型映射。Main 不静默换模型。
2. **单 go-db 冒烟重试**：配额恢复后重跑，取得模型调用、测试、补丁、回收四项端到端证据（用户已要求暂缓）。
3. **遗留 worktree/分支**：`pi-worktree-9195e6c5-…-s0-0` 与分支 `pi-subagents/go-db-smoke-9195e6c-a37e-s0-t0` 按框架保留以待 resume；若不再 resume，需用户确认后走受控 discard，不强制清理。
4. **MySQL 真实集成**：M1-B 首轮可不阻塞，但仍是最终 M1 验收门禁。
5. **正式 M1-B 授权与 lane board**：四条 cwd/ref/SHA/门禁需在冒烟通过后由 Main 记录。
6. **只读独立审查**：go-reviewer 已配置但未授权运行，属可选的第五个角色，不属于四个 writer。
