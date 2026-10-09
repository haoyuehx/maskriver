# Migration plan

## M0 — 已完成的初始化边界

创建独立 Git/Go/GitHub 项目、可编译且拒绝数据库操作的 CLI、规划文档、五个 Agent 定义。
不安装上游依赖、不运行上游测试、不迁移完整引擎，不启动 Agent 工作流。
上游基线：sealandseacat/dbmask @ `7d8789ef4883a423ddba2f8934b95997d1aaf099`，MIT / Siyuan Feng。
仅保留许可文本；新 Go 代码独立编写，无 Python 运行时依赖。

## M1-A — 已批准接口与环境准备

Main 实现 `pkg/contracts`，冻结 m1a-v1；固定两驱动版本、准备 SQLite 合成数据/MySQL 私有实例脚本，四任务只定义不启动。
详见 contracts.md、parallel-tasks.md、m1a-freeze-report.md。收尾阶段已授权外部 Native 根 `/home/haoyue/Project/worktrees`，并允许 M1-B 首轮 MySQL 未验证；SQLite 真实环境必须具备。正式四 Worker 启动仍须用户批准，单 go-db 冒烟方案见 m1b-launch-plan.md。

## M1 — 第一阶段核心兼容（M1-B 及以后待用户批准启动）

1. **契约门禁/Main**：确定 Go DTO、包依赖、字段命名、三态检测、错误与 CLI 退出码；冻结第一轮范围。选择并锁定 SQLite/MySQL 驱动，审查许可证。
2. **可并行组件**：go-db 连接/metadata/keyset/事务；go-detect 规则证据与优先级；go-mask typed strategy/确定性版本化/映射；go-verify 验证器及独立合成 fixtures。共享类型及依赖文件仅 Main 修改。
3. **串行集成/Main**：config/runner/history，scan→plan→preview→apply→validate，完成默认 Dry Run 与 fail-closed 写入安全门。
4. **历史与 CLI/Main**：优先实现人工覆盖/审核决策最小格式；CSV、审计、持久映射管理分步验收；XLSX/Markdown 双向回写另立后续增量。
5. **门禁**：SQLite/MySQL 真实集成、原始源快照对照、失败注入、无秘密/无原值输出检查、go-reviewer 独立只读审查。由 Main 执行测试并提供可审阅证据。

不将“有 database/sql 接口”算作数据库支持；未测试的引擎保持未实现。
Go 输出以语义、约束、稳定性为兼容目标，不保证等于 Python RNG 或字典映射结果。
历史/seed map 文件迁移工具尚未实现，禁止直接把旧库当新格式读取。

## M2 — 第二阶段创新

- 中国本土格式识别及约束有效输出；标注覆盖范围、误报/漏报及模糊样本。
- 有界流式并发、背压、取消、资源配额；先 race/正确性再压测。
- 幂等事务批次、崩溃恢复、版本/源快照绑定 checkpoint；不要将简单 last-key 文件当成 exactly-once。
- 固定合成数据集与配置，记录吞吐/延迟/RSS/CPU/分配/正确性；输出机器可读报告与静态可视化。
- 竞赛材料：来源与许可清单、可复现演示、架构、安全边界、性能证据；报名条件/时间以官方通知核实。

## 后续评估，非首期承诺

LLM 外部/本地分类、XLSX/Markdown 审核回写、PostgreSQL/MariaDB/SQL Server/Oracle 适配、稳定公共 Go API、数据目录集成。
外部 LLM 默认关闭，真实数据不得作为 Agent 提示词或未获批准的外发样本。

## 并行启动前清单

- 用户明确批准迁移；Main 约定每个任务的文件范围、接口、测试与停问条件。
- 在 MaskRiver 项目内打开 Pi、审阅/信任项目配置，确认五个 Agent 与模型映射；注册表存在不代表实际调用或配额验证。
- 仅 Native 在 `/home/haoyue/Project/worktrees/maskriver/` 分配独立工作树；分配失败即停止，禁止共享 cwd 降级。上游统一绝对路径 `/home/haoyue/Project/dbmask`，只读隔离、移除生产凭证、限制联网。
- 首轮准备真实 SQLite 合成环境；MySQL 实现/测试继续保留，缺少真实实例标记 UNVERIFIED。最终 M1 验收前补齐一次性 MySQL 实例、合成数据、源快照及清理机制。
- 接口评审后提交共享契约与驱动版本，再启动组件并行；不得并发改 go.mod 或同一工作树。
- 准备 CI、依赖漏洞/许可检查、race/fuzz 与集成门禁；本轮只在本机执行骨架测试，未配置或宣称 CI 通过。

每一里程碑验收后更新 Feature Matrix，记录真实命令/环境/结果，保留未验证事项。
