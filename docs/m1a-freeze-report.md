# M1-A 接口冻结记录

> 历史记录。旧设备路径和 Agent 启动前置条件不再适用；当前以 [AGENTS.md](../AGENTS.md) 和 [跨平台 PR 流程](m1b-launch-plan.md) 为准。首轮 MySQL 可标记未验证，最终 M1 仍须真实集成。
> 本文件保留当时的冻结范围与验证结果，供追溯。

结论：**共享接口 `m1a-v1` 已冻结**；业务组件尚未实现，M1-B 未启动。

## 1. 冻结清单

| 检查 | 结果 | 证据 |
|---|---|---|
| 单一 Main 所有的公共类型 | PASS | `pkg/contracts`，`APIRevision=m1a-v1` |
| 定位、类型与 NULL | PASS | 私有不可变 `Value`，严格区分 NULL 与空串，decimal 不经浮点，时间语义独立 |
| 检测结论三态 | PASS | SENSITIVE / NOT_SENSITIVE / UNKNOWN，含来源与 distinct 证据 |
| 脱敏计划与策略 | PASS | version/scope/keyID、固定计划 DTO、`ValidateShape`、纯 `Strategy` 接口 |
| 数据库读写接口 | PASS | 读能力与写事务分离，keyset/metadata/抽样，乐观批量与提交结果不明语义 |
| 验证报告与工作流 I/O | PASS | coverage 默认不完整、strict gate、scan/preview/apply/validate 签名 |
| 错误、取消与秘密输出 | PASS（基础契约） | 安全 sentinel 错误；Value 的 fmt/JSON 遮蔽测试。不等于未来实现已满足全部安全条件 |
| 包依赖单向 | PASS | `pkg/contracts` 只依赖标准库；组件经接口注入，无横向具体实现依赖 |
| 共享文件所有权 | PASS | AGENTS、四个任务与 Agent 定义一致 |
| 驱动版本锁定 | PASS | `go.mod`/`go.sum`；直接依赖许可原文已保存，`go mod verify` 通过 |
| SQLite 合成环境 | PASS | 真实驱动 smoke、fixture 工具、只读/回滚/外键/取消用例 |
| MySQL 真实隔离环境 | BLOCKED | 私有实例初始化 Permission denied；容器运行时不可用 |
| 共享冻结 Git 基线 | PENDING（当时） | 该轮未提交；后续已在 closeout-report 中完成 |

## 2. 实际命令与结果

环境：Go 1.26.3 linux/amd64，项目 `/home/haoyue/Project/maskriver`。

| 命令 | 实际结果 |
|---|---|
| `gofmt -w pkg/contracts internal/testenv tools/fixture` | 完成；`gofmt -l` 无输出 |
| `go mod tidy` | 成功；固定 SQLite v1.60.1 / MySQL v1.10.1，生成 `go.sum` |
| `go test -count=1 ./...` | PASS；cmd/config/contracts/testenv 有测试，业务模块仍为占位；MySQL 明确 SKIP |
| `go vet ./...` | PASS，无诊断 |
| `go test -race ./pkg/contracts ./internal/testenv` | PASS；MySQL 未启动仍 SKIP，非完整引擎并发验证 |
| `CGO_ENABLED=0 go test ./internal/testenv -run '^TestSQLiteFixture$' -count=1` | PASS，证实当前环境 SQLite 测试不要求 CGO |
| `go mod verify` | all modules verified |
| `go test -count=1 -v ./internal/testenv ./pkg/contracts` | SQLite/contracts PASS；`TestMySQLFixture` 明确 SKIP |
| `go run ./tools/fixture` | 成功，`.local/m1a-synthetic.db`，权限 0600 |
| 再次 `go run ./tools/fixture` | 按预期拒绝覆盖；执行前后数据库 SHA-256 相同 |
| `python3 scripts/test_mysql.py` | FAIL/BLOCKED，mysqld 初始化 `OS errno 13 Permission denied`；未宣称任何测试通过 |
| `git diff --check` | PASS |
| `git worktree list --porcelain` | 仅主工作树 `main`；未创建 Worker 工作树 |

契约测试覆盖整数边界、二进制与中文载荷、decimal scale、闰日与时区、纳秒超精度拒绝、零值与 NULL、fmt 动词与 JSON 遮蔽、非法计划、strict 与未知状态。
SQLite 测试只是环境与驱动 smoke，**不是**数据库适配器、完整流水线或 MySQL 支持。

## 3. 诚实的完成度声明

- 本轮改动只在 MaskRiver 仓库内的源码、文档与环境路径；依赖下载属于 Go module cache 的标准行为。
- `.local` 下的 fixture 与诊断文件不进入 Git，不含真实个人数据或凭证；未遗留后台服务。
- 四个业务 owner 目录没有任何实现变更；CLI 数据库命令仍是 fail-closed 骨架。

## 4. 当时列出的 M1-B 前置条件

1. 明确隔离工作树路径方案（见 [worktree-preflight.md](worktree-preflight.md)）——已完成：外部 Native 根。
2. 提供可用的非特权隔离 MySQL 环境——**仍未解决**。
3. Main 验收并提交共享文件，形成统一干净基线——已完成。
4. 用户明确批准 M1-B 后再启动——**仍未批准**。

接口设计没有已知的组件 DTO 冲突。尚未实现的 plan builder、权限检查、持久映射与完整算法属于明确的后续范围；`ValidateShape` 不是生产写入授权。
