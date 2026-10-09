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

实际分配必须在这批文件提交后的 clean main 上运行 `node scripts/check-native-worktree.mjs --allocate`。
初始提交暂不声称实际分配成功；后续验收证据将在同一报告补记。预检只在新 Native worktree 增加一个合成测试，捕获并验证补丁后回收，不将补丁应用到 main。

## Git 与启动门禁

Main 将先提交并推送本轮共享基线，再补记 Native 实测结果；精确提交及最后 remote/local HEAD 对照以交付时命令结果为准（避免在提交内引用自身 SHA）。
发布前检查 staged 内容，不提交 .local 数据库/诊断记录、认证/环境配置、Agent 会话、worktree/patch artifacts。
M1-B 组件计划与环境门禁已准备；完整四 Agent 正式启动仍等待单 go-db 模型冒烟通过及用户正式授权，不能把 registry/config 存在当成模型调用验证。
