# M1-A 收尾与 M1-B 启动准备报告

## 已完成的准备

- 共享接口 `pkg/contracts` APIRevision=m1a-v1 与 docs/contracts.md 对照一致；未更改冻结接口语义，四 Worker 的入口/依赖/独占文件范围仍一致。
- SQLite modernc.org/sqlite v1.60.1（BSD-3-Clause），MySQL github.com/go-sql-driver/mysql v1.10.1（MPL-2.0），go.mod/go.sum 固定；直接依赖许可保留原文，Go module 校验通过。
- Native 唯一根 `/home/haoyue/Project/worktrees`，实际项目子目录 maskriver；Main 在已安装 pi-subagents 0.76.1 的用户配置设 provider=native/baseDir。
- 实际配置加载/优先级/非法路径拒绝预检通过；这不把旧 Pi 会话视作已热更新，执行模型 smoke 前需 reload/重启。
- 五个 Agent 被 list 正确发现；models 的项目映射与指定值一致，doctor 显示 async 可用、active runs=0。
- 单 go-db 冒烟脚本 `.pi/workflows/go-db-smoke.js` 仅准备，静态 validate 返回 ok=true。静态工具提示 dynamic-spawn-count，未来 runtime 必须限制 maxSubagentSpawnsPerRun=1；校验不证明实际模型调用。
- 四正式 Worker 未启动，没有完整代码迁移；Native 预检由 Main 直接调用现有 allocator，不使用外部 Agent/CLI fallback。

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
- Main HEAD 未变、无自动 apply/merge/push 诊断补丁，上游143个文件（含 .git）哈希不变。

该预检 agentLaunched=false、modelCalled=false、merged=false。单 go-db 模型调用与子会话/端到端收尾尚未实测，不与 Main allocator 预检混淆。

## Git 与启动门禁

共享基线提交 `816252a` 已推送 origin/main；后续仅补记本报告与许可证空白豁免，最终 local/remote HEAD 以交付命令对照为准（避免在提交内引用自身 SHA）。
首次暂存 whitespace 检查发现原版 MPL 第38行尾随空格；因 shell 命令链边界错误，第一次 commit/push 先于自动内容筛查执行。已立即对该提交全部35个文件补查，没有凭证/数据/会话文件；两个驱动 LICENSE 与 module cache 原文逐字节一致。后续使用 set -e，.gitattributes 仅对这一份未改写的上游许可禁用 whitespace 检查，不放宽源码检查、不重写公开 Git 历史。
最终补记提交前再次检查完整 index 的路径/内容：不提交 .local 数据库/诊断记录、认证/环境配置、Agent 会话、worktree/patch artifacts。模式扫描只是防误提交辅助，不宣称完整安全审计。
M1-B 组件计划与环境门禁已准备；完整四 Agent 正式启动仍等待单 go-db 模型冒烟通过及用户正式授权，不能把 registry/config 存在当成模型调用验证。
