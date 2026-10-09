# M1-A 接口冻结检查报告（历史记录）

> 本文保留 M1-A 原始检查时的状态，不代表当前启动门禁。后续用户已批准外部 Native 根并放宽首轮 MySQL 门禁；当前冻结提交与验证以 [closeout-report.md](closeout-report.md) 为准。以下“本轮未提交”“BLOCKED”均指收尾前的历史状态。

结论：**共享接口 m1a-v1 已在本地工作区冻结；M1-B 整体启动条件不满足（BLOCKED）**。
Main 单独完成准备，没有启动任何 Worker/reviewer，没有创建新分支/worktree，没有自动开始迁移。
本轮未提交/推送；当前 main 基线仍为 `0d40165bbb01f20771fe7191ea7514c3250b7a37`。下轮启动前 Main 需要先验收并提交共享变更，记录干净的命名 baseRef；不得让 Worker 基于旧提交缺少接口启动。

## 1. 冻结清单

| 检查 | 结果 | 证据 |
|---|---|---|
| 单一 Main-owned 公共类型 | PASS | pkg/contracts，APIRevision=m1a-v1 |
| ColumnRef / TableRef / 类型与 NULL | PASS | 私有 immutable Value、强区分 NULL/empty、decimal 不走浮点、时间语义独立 |
| Detection Decision | PASS | 三态、来源、distinct evidence、人工/审核注入接口 |
| Masking Plan / Strategy | PASS | version/scope/keyID、固定计划 DTO、ValidateShape、纯 Strategy 接口 |
| Reader / Writer / WriteTx | PASS | 分离能力、keyset/metadata/抽样、optimistic batch 与 commit-unknown 语义 |
| 验证报告与 workflow I/O | PASS | coverage 默认不完整、strict gate、scan/preview/apply/validate DTO 与接口 |
| 错误/取消/秘密输出 | PASS（基础契约） | 安全 sentinel errors；Value fmt/JSON 遮蔽测试；不等于未来实现已满足全部安全条件 |
| 包依赖单向 | PASS | contracts 仅标准库；Worker 通过 interface 注入，无横向具体实现依赖 |
| 共享文件所有权 | PASS | AGENTS、四任务与 Agent 定义均更新；Main owns contracts/dependencies/testenv/tools/scripts |
| 上游绝对路径 | PASS | 五个 Agent 定义与四个任务要求 /home/haoyue/Project/dbmask，不用 ../dbmask |
| 两驱动锁定 | PASS | go.mod/go.sum；直接许可证已阅读/保存，go mod verify 通过 |
| SQLite 合成环境 | PASS | 实际 driver smoke、静态 fixture、只读/rollback/FK/取消测试 |
| MySQL 真实隔离环境 | BLOCKED | 禁网私有实例初始化 Permission denied；Docker daemon 不可用 |
| native worktree 与政策相容 | BLOCKED | native 禁止仓库内部路径；现有 AGENTS 要求 .local/worktrees |
| 共享冻结 Git 基线 | PENDING | 本轮未 commit/push，工作区变更待提交 |
| M1-B 用户授权 | NOT GRANTED | 当前授权仅 M1-A，禁止自动启动 |

## 2. 实际命令与结果

环境：Go 1.26.3 linux/amd64，项目 `/home/haoyue/Project/maskriver`。

| 命令 | 实际结果 |
|---|---|
| gofmt -w pkg/contracts internal/testenv tools/fixture | 完成；最后 gofmt -l 应无输出 |
| go mod tidy | 成功；固定 SQLite v1.60.1 / MySQL v1.10.1，生成 go.sum |
| go test -count=1 ./... | PASS；cmd/config/contracts/testenv 有测试，其余业务模块仍占位；MySQL 明确 SKIP |
| go vet ./... | PASS，无诊断 |
| go test -race ./pkg/contracts ./internal/testenv | PASS；MySQL 未启动仍 SKIP，非完整引擎并发验证 |
| CGO_ENABLED=0 go test ./internal/testenv -run '^TestSQLiteFixture$' -count=1 | PASS，证实当前环境 SQLite 测试不要求 CGO |
| go mod verify | all modules verified |
| go test -count=1 -v ./internal/testenv ./pkg/contracts | SQLite/contracts PASS；TestMySQLFixture 明确 SKIP |
| go run ./tools/fixture | 成功，.local/m1a-synthetic.db，0600 |
| 再次 go run ./tools/fixture | 按预期拒绝覆盖；执行前后数据库 SHA-256 相同 |
| python3 scripts/test_mysql.py | FAIL/BLOCKED，mysqld 初始化 OS errno 13 Permission denied；没有测试通过宣称 |
| git diff --check | PASS |
| git worktree list --porcelain | 仅原 MaskRiver 主工作树/main；未创建 Worker worktree |

contracts 测试覆盖整数边界、二进制/中文、decimal scale、闰日/时区、纳秒超精度拒绝、零值/NULL、fmt 动词/JSON 遮蔽、错误 plan、strict 与未知 status。
SQLite 测试只是环境/驱动 smoke，不是 go-db adapter、流水线或 MySQL 支持。没有运行生产库、上游 pytest、外部 LLM、恢复/吞吐测试或 CI。

## 3. 完整性与退出状态

- 上游 `/home/haoyue/Project/dbmask` 143 文件（含 .git）SHA-256 与初始基线逐项一致，Git status 干净。
- 本轮修改仅在 MaskRiver 源码/文档/环境路径；依赖下载为 Go 标准 module cache 行为。
- `.local` 的 fixture/诊断文件不入 Git，不包含真实个人数据或凭证。没有存活的本轮 mysqld 进程，未建立后台服务。
- 四业务 owner 目录没有实现变更；CLI 数据库命令仍 fail-closed 骨架。

## 4. M1-B 前必须完成

1. 用户决定 worktree：批准仓库外的 native managed MaskRiver 工作树路径，或批准 Main 手动仓库内 Git worktree + 明确 cwd 的调度方式。详情 worktree-preflight.md；不能偷偷改变政策或绕过扩展校验。
2. 操作者提供可用隔离 MySQL 容器/实例权限，Main 重跑真实驱动 smoke；不连接已有生产/系统数据库。
3. Main 验收并提交这批共享文件，形成四 Worker 一致的干净 baseRef；按 parallel-tasks 填入每个 cwd/ref/交付证据位置。
4. 用户明确批准 M1-B 后再启动。当前停止，不先行启动“能做的”三条 lane。

接口设计没有已知待定的 Worker DTO 冲突；未实现的 plan builder/权限检查/持久映射/完整算法属于明确后续范围，不得用 ValidateShape 当生产授权。
