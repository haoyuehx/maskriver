# Test plan

MaskRiver 的测试目标：证明组件在真实数据库上按其契约工作，并证明安全不变量不会被绕过。
本计划只依据 MaskRiver 自身的接口（[contracts.md](contracts.md)）与范围（[feature-matrix.md](feature-matrix.md)）。

## 已有测试（当前真实状态）

- `pkg/contracts/contracts_test.go`：typed Value / NULL / 精度 / 时间语义、fmt 与 JSON 遮蔽、无效 Plan 拒绝、报告 strict gate。
- `internal/testenv/fixture_test.go`：SQLite 真实驱动与合成数据、约束、事务、只读、取消；MySQL 为 opt-in，未启动时明确 `SKIP`。
- `cmd/maskriver/main_test.go`：help/version、占位命令返回非零、默认 Dry Run、`--apply` 仍拒绝、未知参数与多余位置参数。
- `internal/config/config_test.go`：零值只读与显式 Apply 意图。
- `tools/fixture`：生成 `.local/m1a-synthetic.db`，拒绝覆盖已有文件。
- `scripts/test_mysql.py`：尝试启动专用禁网 MySQL；当前因宿主机权限失败，**不是通过**。

`internal/{db,detect,mask,verify,history}` 目前仍为占位包。上述 CLI/config/testenv 用例**不是**业务功能测试。

```sh
gofmt -l cmd internal pkg tools
go test -count=1 ./...
go vet ./...
go build -o bin/maskriver ./cmd/maskriver
```

## 分阶段门禁

- M1-B 首轮：真实 SQLite 必须通过；MySQL 可 `UNVERIFIED/SKIP`，但实现与真实集成用例必须交付。
- 最终 M1 验收：真实 MySQL 集成必须通过。缺此项不得宣称 M1 完成。
- Mock、编译通过、跳过都不能替代真实数据库验证。
- 隔离工作树冒烟只证明工作树生命周期；它不是模型调用或业务功能的证据。

## 第一阶段验收矩阵（尚未实现）

| 范围 | 关键用例 |
|---|---|
| 扫描 | 单列失败阻断 apply；skip 与 override 次序；UNKNOWN 不写入历史；错误分类与退出码 |
| 检测 | 12 类规则正反例；19/20 样本边界；90% 阈值；冲突判定；无上下文纯数字；MDY/DMY 日期 |
| 策略 | 首批 5 种策略的格式与类型保持；Unicode；NULL 与空串；typed 数值/日期；校验位有效性；未变化结果拒绝 |
| 确定性 | 固定 key/scope/version 重复一致；跨表同 scope 一致；映射复用；词典变化不漂移；碰撞处理 |
| Dry Run | 配置无法绕过 CLI 写意图；连接 spy 证明无写事务；文件树无新增或修改；历史与映射只读 |
| 分页/写入 | 超过两页；复合键；最后不足一页；无键或敏感主键拒绝；事务回滚；受影响行数；提交结果不明 |
| NULL 标记 | 仅精确列生效；先于映射；不写映射；目标列 NOT NULL 时报错 |
| 验证 | 行数与 schema；索引与约束；双向键差异；单字段未变化；无键、截断、缺表在 strict 下失败 |
| 历史 | pending/approved；审核人；类型与过期；修订冲突；导入失败不污染；回写备份 |
| 端到端 | 合成源快照 → scan → preview → apply → strict validate；SQLite 与真实 MySQL 分别独立运行 |

## 特别安全门禁

- 标准输出、错误、日志、报告、Agent 会话均不得出现原始值、敏感主键、DSN、seed/salt。
- Dry Run 用文件哈希快照与连接 spy 证明零持久写入，不能只断言 `rows_written=0`。
- 验证基于脱敏前的快照与固定 Plan；覆盖不完整不得 PASS。字段值发生变化不能证明其中没有 PII。
- 注入网络中断、`context` 取消、写入冲突、磁盘或权限错误，并明确已提交批次与未完成范围。
- 映射存储与目标不共享事务时，必须测试提交间隙，且不得提前承诺恢复或幂等。

## 第二阶段质量与性能

`go test -race ./...`；对规则、编码与格式函数做 Go fuzz；用合成序列做故障恢复测试；检查 goroutine 与连接泄漏及有界内存。
`go test -bench . -benchmem` 必须固定 Go/驱动/数据库版本、CPU、行数与分布、batch/workers，多次重复并报告统计分布，而不是最佳值。
对比正确性与吞吐/延迟/RSS/CPU/分配；检测能力以标注集按规则报告 precision/recall/F1。可视化只使用合成数据与汇总指标。

不设置虚构覆盖率或吞吐成绩。不使用生产数据；临时文件使用 `t.TempDir`，MySQL 使用隔离测试实例。
