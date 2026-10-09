# Test plan

## M1-A 已存在的共享契约/环境测试

- `pkg/contracts/contracts_test.go`：typed Value/NULL/精度/时间、fmt/JSON 遮蔽、无效 Plan 拒绝、报告 strict gate。
- `internal/testenv/fixture_test.go`：SQLite 真实驱动与合成数据、约束/事务/只读/取消；MySQL 单独 opt-in，未启动时明确 SKIP。
- `tools/fixture` 已生成 `.local/m1a-synthetic.db`，拒绝覆盖已有文件。
- `python3 scripts/test_mysql.py` 尝试专用禁网 MySQL，但 mysqld 初始化 Permission denied；不是通过。
- 冻结验收见 m1a-freeze-report.md；没有在上游运行测试。

## 已存在的骨架测试

`cmd/maskriver/main_test.go` 覆盖 help/version、预留命令返回非零、默认 Dry Run、--apply 仍拒绝、显式 false、未知参数及多余位置参数。
`internal/config/config_test.go` 覆盖零值只读与显式 Apply 意图。
上述 CLI/config 用例不是数据库行为测试；db/detect/mask/verify/history 仍为占位包。新增 internal/testenv 仅做合成环境 driver smoke，不表示业务适配器功能通过。

```sh
gofmt -w cmd internal pkg tools
go test ./...
go vet ./...
go build -o bin/maskriver ./cmd/maskriver
```

## 分阶段门禁

M1-B 首轮真实 SQLite 必须通过，MySQL 暂可 UNVERIFIED/SKIP；保留 MySQL 实现与真实集成用例，不把 Mock 当作 MySQL 通过。最终 M1 发布/功能验收前必须补齐真实 MySQL 集成。
Native worktree 的 Main 预检不等于模型调用成功；单 go-db 模型冒烟按 m1b-launch-plan.md 单独验收，不自动合并。

## 第一阶段验收矩阵（尚未实现）

| 范围 | 关键用例 | 上游参考 |
|---|---|---|
| 扫描 | 单列异常/预算异常阻断 apply；skip 与 override 次序；UNKNOWN 不写入历史 | test_scan_error_guard.py, test_unknown_detection.py, test_pattern_precedence.py |
| 检测 | 12 类规则正反例、19/20 样本边界、90% 阈值、冲突、纯数字语义、日期 MDY/DMY | test_contextual_patterns.py, test_date_detection.py |
| 策略 | 17 策略、Unicode、NULL/blank、typed 数值/日期/UUID、Luhn、未变化结果拒绝 | test_strategy_formats.py, test_issue36_formats.py, test_masking_contracts.py |
| 确定性 | 固定 seed/scope/version 重复一致、跨表一致、映射重用、字典变更不漂移、碰撞处理 | test_seed_map.py, test_dictionaries.py |
| Dry Run | 配置 dry_run:false 不能绕过 CLI；SQL spy 无写事务；文件树无新增/修改；历史和映射只读 | test_cli_mask_safety.py, test_masking_safety.py |
| 分页/写入 | 多于两页、复合键、最后不足一页、无键/敏感 PK 拒绝、事务回滚、受影响行数 | test_mask_apply_paging.py |
| NULL 标记 | 仅精确列生效、先于映射、不写映射、NOT NULL 冲突报错 | test_null_placeholders.py |
| 验证 | 行数/schema/索引/约束；双向键差异；单字段未变化；无键、截断、缺失表严格失败 | test_validation.py, test_validation_pk_aligned.py |
| 历史 | pending/approved、审核人、类型/过期、修订冲突、导入失败不污染；回写备份 | test_history_import.py, test_history_writeback.py |
| 端到端 | 源合成快照 → scan → preview → apply → strict verify；SQLite 与真实 MySQL 独立运行 | test_end_to_end.py |

参考名称均为上游 `tests/` 下的文件；仅迁移测试意图，重新编写 Go 用例，不运行/修改上游。

## 特别安全门禁

- 标准输出、错误、日志、Report、Agent 会话均不得含原始值、敏感主键、DSN、seed/salt。
- Dry Run 用哈希快照和连接 spy 证明零持久写入，不能只断言 rows_written=0。
- 验证基于脱敏前计划和稳定快照；缺失覆盖不可 PASS。误分类不能由“值变了”证明没有 PII。
- 注入网络断开、context 取消、写入冲突、磁盘满/权限不足；明确已提交批次与未完成范围。
- 映射 store 与目标事务不共库时测试提交间隙，不提前承诺恢复/幂等。

## 第二阶段质量与性能

`go test -race ./...`；针对规则/编码/格式函数的 Go fuzz；合成故障恢复序列；检查 goroutine/连接泄露及有界内存。
`go test -bench . -benchmem`，固定 Go/驱动/DB 版本、CPU、行数/分布、batch/workers；至少多次重复，报告统计分布，不只最佳值。
比较正确性与吞吐/RSS/CPU；检测以标注集报告 precision/recall/F1（逐规则）。可视化只用合成数据/汇总指标。

本轮不设置虚构覆盖率或吞吐成绩。不使用生产数据；临时文件使用 t.TempDir，MySQL 使用隔离测试实例。
