# MaskRiver

A streaming, reliable data masking engine written in Go.

面向 **2026 开放原子大赛 · 开源基础软件与解决方案赛道** 的独立 Go 项目。
这是参赛研发目标，不代表已报名、入选或获官方背书。

> 当前为 M1-A 接口与环境准备：可编译 CLI、共享契约、安全测试、SQLite 合成环境与迁移规划。
> **尚不能连接数据库、扫描、脱敏或验证数据。** Streaming/reliable 是设计目标，非已验证的性能承诺。

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

## 路线图

- 第一阶段：敏感字段扫描、规则检测、脱敏策略、确定性持久映射、SQLite/MySQL、默认 Dry Run、完整性验证、CLI。
- 第二阶段：中国本土敏感数据与格式保持、高性能有界流式并发、故障恢复/断点续跑、性能基准及可视化。
- 当前状态：[Feature Matrix](docs/feature-matrix.md)。两驱动已引入供测试准备；业务数据库适配器尚未实现。

## 文档

- [架构与模块契约](docs/architecture.md)
- [迁移计划](docs/migration-plan.md)
- [测试计划](docs/test-plan.md)
- [协作与安全边界](AGENTS.md)
- [Agent 与模型配置](docs/agents.md)
- [初始化验收记录](docs/bootstrap-validation.md)
- [M1-A 共享契约](docs/contracts.md)、[冻结报告](docs/m1a-freeze-report.md)
- [驱动决策](docs/database-drivers.md)、[测试环境](docs/test-environment.md)
- [四 Worker 任务](docs/parallel-tasks.md)、[worktree 预检](docs/worktree-preflight.md)
- [收尾报告](docs/closeout-report.md)、[M1-B 冒烟与启动方案](docs/m1b-launch-plan.md)

## 技术来源与许可

参考 [sealandseacat/dbmask](https://github.com/sealandseacat/dbmask) 的扫描 → 脱敏 → 验证工作流、规则策略与安全回归用例。
分析基线：`7d8789ef4883a423ddba2f8934b95997d1aaf099`。
MaskRiver 是独立初始化的仓库，不是 GitHub Fork，不共享上游 Git 历史；不逐行翻译 Python 实现。
当前 Go 骨架为新写代码，未复制上游配置、字典数据、密钥或测试数据。

MaskRiver 采用 [MIT](LICENSE)。上游 Copyright (c) 2026 Siyuan Feng，完整 MIT 文本保留在
[docs/upstream-LICENSE](docs/upstream-LICENSE)。以后改编代码/文档/测试时须保留来源、署名与许可；独立仓库不免除这些义务。

## 安全提示

仅处理获授权的数据库副本；脱敏不等于匿名化。确定性映射可能遭受猜测攻击，也不自动保证唯一性或所有关联完整性。
不要向仓库、日志或 Agent 会话提交真实个人数据、连接凭证、种子及映射库。
