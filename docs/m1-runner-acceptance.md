# M1 Runner 授权与事务安全收尾证据

日期：2026-10-18。关联 #9、#11。基线 `cd7ad9a340840807ab009a01903df6ed0f18f5c3`，冻结契约 `APIRevision=m1a-v1` 未改。本文是有界工作包证据，不是完整 M1、安全批准或发行结论。

## 当前仓库与依赖

GitHub main 基线已有四个核心组件、真实 SQLite 单表单列 CLI 和 PR #36 的有界 Worker；本轮读取的 [main CI](https://github.com/haoyuehx/maskriver/actions/runs/38379436864) 中 Ubuntu/Windows 测试/vet、Ubuntu race、隔离 MySQL adapter 均成功。它们不代表本次新增提交的 CI。

Open Issues 为 #9、#11、#13、#14；唯一 Open PR 为 [#37](https://github.com/haoyuehx/maskriver/pull/37)，head `0e006f66392f3fb1cbc0478398e4daa9112a803c`，范围为性能基准/连接生命周期/CI。该 head 的 [CI 运行](https://github.com/haoyuehx/maskriver/actions/runs/38532168654) 和 [benchmark 运行](https://github.com/haoyuehx/maskriver/actions/runs/38532167032) 实际结论为 failure；未见 job 证据，不能断定具体业务测试失败，也不能沿用较早 SHA 绿灯。本包没有修改其文件。

```text
现有组件/SQLite CLI
  → #9 Runner 授权与事务故障证据（本包）
  → #9 完整 Schema/SQL Spy/审核记录/多表
  → #11 独立安全验收（另需 MySQL 全流程范围证据）
  → #13 完整性能验收 / #14 恢复协议
```

## 已交付的行为

- Runner Apply 同时要求 Approved 和 ExplicitApply，不能只依赖 CLI 检查；Plan/Preview 预检失败前不获取 Writer。
- 保持 NULL 的行不生成无意义 UPDATE；全 NULL 页继续推进游标，无需 Writer/事务。receipt.Rows 是实际 UPDATE 行数，Ordinal 是实际成功提交的事务序号，不是原始页号或恢复游标。
- 包装的 ErrCommitUnknown 使用 errors.Is 识别，报告 Complete=false/CommitOutcomeUnknown=true；不自动重试，不确认未知批次成功，也不声称已回滚。
- 已知失败用独立的 2 秒清理 context 请求回滚。底层 sql.Tx.Rollback 不接受 context，因此这不是驱动调用硬 2 秒上界或连接无泄漏的压力证明。

## 回归与实际结果

环境：Linux amd64，Go 1.26.3；SQLite `modernc.org/sqlite v1.60.1`，全部数据为本项目自编合成 fixture。构建缓存放在可写临时目录。

修复前回归：`go test -count=1 ./internal/runner` 退出 1，分别证明未 Approved 仍成功 Apply、NULL 行触发 ErrUnsafe。修复后：

| 检查 | 结果 | 证据边界 |
| --- | --- | --- |
| gofmt / git diff --check | PASS | 仅格式/补丁检查 |
| go test -count=1 ./... | PASS | 真实 SQLite、组件与回归；MySQL opt-in 跳过 |
| go vet ./... | PASS | 静态检查 |
| go test -race ./... | PASS | 已执行路径的 race 检查；没有新 fuzz campaign |
| MySQL TestMySQLAdapter | SKIP / UNVERIFIED | 本机未提供隔离 socket；不是成功的 MySQL 业务执行 |
| 最终 PR head 统一 CI | UNVERIFIED | 以 PR 上该 SHA 的实际运行结论为准 |
| 独立 AI Reviewer | Scoped PASS | 最终 Runner focused/race 实跑通过，无本包阻塞发现；不是完整 #11 或维护者合并批准 |

独立 Reviewer 另行执行全仓 test/vet/race 均退出 0；最终测试补证后，独立重跑 `go test -count=1 -v ./internal/runner` 与 `go test -race -count=1 ./internal/runner` 均退出 0。实现与评审由不同执行者完成。

新增真实 SQLite 用例覆盖：授权双门禁与预检失败零 Writer；Preview 的目录文件集合/SHA256 不变；混合 NULL、130 行全 NULL 跨页；Begin/Update/影响行数错误和取消；目标 SQL 回读确认回滚与已提交前缀；Writer Close 和独立清理 context；真实第二批 Commit 成功后模拟确认丢失，回读两批均持久化但仅确认首批 receipt。

Spy 观测的是 Runner 对 Writer/WriteTx 的能力调用，**不是 SQL Driver Spy**，不覆盖底层全部 SQL 或连接初始化行为。文件快照限于静止合成目录；不能证明 WAL 或并发修改条件下的不可变快照。未知提交注入模拟确认丢失，不代表真实网络故障已测。

## 独立基线审查发现及剩余门禁

以下发现是在本包基线真实复现或明确标记的静态风险，不得因本包测试通过而标记已修复。

| 严重度/状态 | 位置、触发与复现 | 建议/依赖 |
| --- | --- | --- |
| P1 FAIL | `internal/verify/verify.go:59` 只要求 Columns/PK coverage；`internal/db/metadata.go:264` 的 SQLite Checks=false。构造相同列/PK的源目标，源含 CHECK、目标删除，敏感值已变化；真实 CLI validate --approved --strict 仍 pass=true | db 提供可验证的有界 CHECK 元数据，verify 比较已声明约束并拒绝覆盖缺失；需 db/verify 协同 |
| P2 FAIL | `cmd/maskriver/main.go:48` flag 解析错误直达 stderr；`mask --workers SYNTHETIC-SECRET` exit 2 原样回显输入 | 独立 CLI 输出清洗包，保留显式 help |
| P2 FAIL | `internal/verify/verify.go` compareRows 将合法 null 策略的非 NULL→NULL 当类型冲突；Apply 成功而 validate 失败 | 仅允许固定计划明确指定且 Nullable 的 null 转换，其他 NULL 异常继续拒绝 |
| UNVERIFIED | Runner SourceSnapshotID 仅主文件哈希，未绑定活跃 WAL/并发源变更；TargetID 仅路径 | 固定独立快照或明确拒绝不支持的活动状态；补专项测试 |
| 未实现/未验收 | 多表/多列、安全配置加载、审核身份/过期/存储、真正 SQL Driver Spy、MySQL CLI、checkpoint | 继续 #9，随后 #11；不启动恢复承诺 |

本包未新增依赖、未复制第三方源码/fixture；既有 MIT 及两个直接驱动许可文本存在。完整传递依赖 notices/SBOM/漏洞与发行合规仍未验收。不得将脱敏称为匿名化、确定性称为无碰撞，或将批次事务称为整库原子与 exactly-once。
