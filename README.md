# MaskRiver

A streaming, reliable data masking engine written in Go.

面向 **2026 开放原子大赛 · 开源基础软件与解决方案赛道** 的独立 Go 项目。
这是参赛研发目标，不代表已报名、入选或获官方背书。

MaskRiver 是独立研发的 Go 数据脱敏与敏感信息治理工具，面向企业开发、测试与数据共享场景：自动发现敏感数据，提供安全、确定性的脱敏策略，支持 SQLite 与 MySQL，并提供数据完整性校验。

> 当前阶段：可编译 CLI、冻结的共享契约、安全测试与 SQLite 合成环境。
> **尚不能连接数据库、扫描、脱敏或验证数据。** Streaming/reliable 是设计目标，不是已验证的性能承诺。

## 快速开始

需要 Go 1.26.3 或更新版本；测试环境已锁定 SQLite/MySQL 驱动，CLI 仍为不执行数据库操作的骨架。

```sh
go run ./cmd/maskriver --help
go run ./cmd/maskriver --version
go test ./...
go vet ./...
go build -o bin/maskriver ./cmd/maskriver
```

`scan`、`mask`、`validate` 为预留命令，返回退出码 2（未实现），不会打开数据库或写入文件。
`mask` 默认 Dry Run；即使传入 `--apply`，当前版本仍拒绝执行。
直接运行编译后的二进制可观察原始退出码；`go run` 会包装子进程退出状态。

## 产品路线图

- M1：敏感字段扫描、规则检测、脱敏策略引擎、确定性映射、SQLite/MySQL、默认 Dry Run、完整性校验、CLI。
- M2：中文敏感数据治理与格式约束、有界流式并发、故障恢复与断点续跑、性能基准与可视化。
- 当前状态：[Feature Matrix](docs/feature-matrix.md)。驱动已为测试环境引入；业务数据库适配器尚未实现。
- 完整阶段划分见 [Roadmap](docs/roadmap.md)。

## 文档

- [架构设计](docs/architecture.md)
- [Roadmap](docs/roadmap.md)
- [测试计划](docs/test-plan.md)
- [协作与安全边界](AGENTS.md)
- [Agent 与模型配置](docs/agents.md)
- [初始化验收记录](docs/bootstrap-validation.md)
- [M1-A 共享契约](docs/contracts.md)、[冻结报告](docs/m1a-freeze-report.md)
- [驱动决策](docs/database-drivers.md)、[测试环境](docs/test-environment.md)
- [四 Worker 任务](docs/parallel-tasks.md)、[worktree 预检](docs/worktree-preflight.md)
- [收尾报告](docs/closeout-report.md)、[M1-B 冒烟与启动方案](docs/m1b-launch-plan.md)

## 来源与许可

MaskRiver 采用 [MIT](LICENSE)。本项目独立研发：不是任何其他仓库的 Fork，不共享其 Git 历史，未复制其源码、词典、配置或测试数据。
早期设计文档曾把 MIT 许可的 Python 项目 `dbmask`（Copyright (c) 2026 Siyuan Feng）作为先例参考；为免误述该参考、并保留其许可声明，其完整 MIT 文本保留在
[docs/licenses/dbmask-MIT-LICENSE.txt](docs/licenses/dbmask-MIT-LICENSE.txt)。
第三方依赖保留各自许可，详见 [来源与许可说明](docs/attribution.md)。
若将来确实改编或复用第三方代码、文档或测试，必须先在该说明中保留其版权、许可与来源标注。

## 安全提示

仅处理获授权的数据库副本；脱敏不等于匿名化。确定性映射可能遭受猜测攻击，也不自动保证唯一性或所有关联完整性。
不要向仓库、日志或 Agent 会话提交真实个人数据、连接凭证、种子及映射库。
