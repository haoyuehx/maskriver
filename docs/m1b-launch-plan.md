# M1-B 跨平台开发与 PR 集成

本文件取代旧的本机 Agent 启动方案。#2、#3、#4 属旧设备执行管理事项，不再作为产品开发依赖；不要求恢复任何旧 worktree、模型会话或补丁缓存，也不授权删除它们。

## 开发入口

1. 四位成员各自 clone，安装 `go.mod` 要求的 Go 版本；Windows、Linux、macOS 均可。
2. 认领 #5～#8 中对应模块，记录基线 SHA，创建个人功能分支。无需四人同时开始或使用相同目录。
3. 阅读 [contracts.md](contracts.md) 与 [parallel-tasks.md](parallel-tasks.md)，保持 `m1a-v1`；共享契约、依赖及跨模块调整先协调。
4. 用合成数据实现和测试，提交功能分支，创建关联 Issue 的 PR。尽量一 PR 一模块。

## 开发任务及真实依赖

| Issue | 职责 | 可独立执行的测试 |
|---|---|---|
| #5 | go-db | 临时 SQLite；MySQL 在一次性容器/CI 执行 |
| #6 | go-detect | 纯内存规则、边界、冲突和取消 |
| #7 | go-mask | 纯内存确定性、隔离、并发与取消 |
| #8 | go-verify | fake Reader、coverage、strict gate；非 DB 集成证明 |
| #9 | 集成 | 依赖 #5～#8，真实 SQLite 端到端 |
| #10 | MySQL 测试 | 环境可并行准备；适配器验收依赖 #5 |
| #11 | 安全评审 | 依赖 #9/#10 的真实执行证据 |
| #12～#14 | M2 backlog | 仍依赖 M1 验收，不因协作改造而完成或启动 |

## PR 门禁

- 提供目标、代码范围、基线 SHA、改动文件、测试命令/结果和未验证项。文档/Feature Matrix 更新由 Main 协调或授权。
- `gofmt`、`go test ./...`、`go vet ./...`；并发模块要求 race。Windows 本机无法执行的工具链测试交统一 CI，不能冒称本机通过。
- Ubuntu/Windows CI 检查格式、Go 测试与 vet；Ubuntu 跑 race 和一次性 MySQL fixture。fixture 仅证明环境，不证明业务适配器实现。
- SQLite 真实测试不得跳过；无 MySQL 环境时明确 SKIP/UNVERIFIED，最终 M1 不豁免真实 MySQL 业务集成。
- 人类成员或经授权的 AI Reviewer 审查安全、代码、许可证和测试证据；修复后重测，由维护者人工合并，不自动合并。

此次协作规则改造不启动业务开发或 Subagent，不修改冻结接口，不回收任何旧机器工作树。CI 是否成功以 PR 的实际运行结果为准。
