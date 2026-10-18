# Test plan

MaskRiver 的测试目标：证明组件在真实数据库上按其契约工作，并证明安全不变量不会被绕过。
本计划只依据 MaskRiver 自身的接口（[contracts.md](contracts.md)）与范围（[feature-matrix.md](feature-matrix.md)）。

## 已有测试（当前真实状态）

- `pkg/contracts/contracts_test.go`：typed Value / NULL / 精度 / 时间语义、fmt 与 JSON 遮蔽、无效 Plan 拒绝、报告 strict gate。
- `internal/testenv/fixture_test.go`：SQLite 真实驱动与合成数据、约束、事务、只读、取消；MySQL 为 opt-in，未启动时明确 `SKIP`。
- `cmd/maskriver/main_test.go`：help/version/非法参数、无授权拒绝，以及真实 SQLite 合成源与目标库 Scan、默认 Preview、显式 --apply --approved、strict Validate、源与目标文件哈希/唯一列和重复 Apply 拒绝（PR #35）。
- `internal/config/config_test.go`：零值只读与显式 Apply 意图。
- `tools/fixture`：生成 `.local/m1a-synthetic.db`，拒绝覆盖已有文件。
- `scripts/test_mysql.py`：可选 POSIX 本机专用禁网 MySQL；历史宿主机权限失败不是通用开发阻塞。
- `scripts/test_mysql_container.py`：Linux 或 Ubuntu CI 一次性禁网 MySQL 8.0.46 容器，复用 marker + 私有 socket guard；现执行 TestMySQLFixture + TestMySQLAdapter 业务测试（PR #30）。

`internal/detect` 已由 PR #20 合并有界 China-first 检测实现；`internal/db` 的 SQLite/MySQL Reader/Writer、keyset 分页与事务由 #5 的 PR #21 交付，是否合并和测试通过以 GitHub 最终记录为准。`internal/db/mysql_test.go` 的 `TestMySQLAdapter` 只有在传入隔离 MySQL socket 时执行；PR #30 的独立隔离 MySQL CI 现已实际执行这一业务测试并成功；本地普通测试无 socket 时仍会 SKIP。go-mask 的五种 M1 策略及内存映射由 PR #24 从 #18 原始提交整合，测试证据见 `internal/mask/mask_test.go`、`security_test.go`；是否合并以 GitHub 实际记录为准。其他尚未交付的模块 PR 不得被视为已在 `main` 交付。PR #35 的 SQLite **单表单列**真实 CLI 流程已覆盖；尚未实现多表、持久审批/审计、SQL Spy、异常提交注入和 MySQL CLI，因此不代表全部 #9/#11 验收。

```sh
gofmt -l cmd internal pkg tools
go test -count=1 ./...
go vet ./...
go build -o bin/maskriver ./cmd/maskriver
```

## 跨平台与统一 CI

四位成员各自 clone、认领 Issue、开分支并提交 PR。纯内存模块可在 Windows/Linux/macOS 独立测试；不依赖本地 Agent smoke。共享测试环境变动由 Main 协调。

`.github/workflows/ci.yml` 在 Ubuntu/Windows 执行格式检查、`go test -count=1 -v ./...` 和 `go vet ./...`，Ubuntu 另跑 race。MySQL 隔离业务测试单独在 Ubuntu 禁网容器执行 fixture + adapter，失败即 CI 失败；普通测试无 MySQL socket 时明确 SKIP。macOS 可以本地测试，当前 CI 不宣称覆盖 macOS。

PR 记录 OS、Go 版本、命令、退出码、跳过原因和剩余风险，统一 CI 与评审完成后才能合并。本机不支持 race 可由 Ubuntu CI 补齐，不能记成本机通过。#10 的当前适配器用例已在 PR #30 隔离 MySQL 8.0.46 实跑；尚未验证完整用户 CLI workflow、其他版本及生产环境。

## 分阶段门禁

- M1-B 首轮：真实 SQLite 必须通过；MySQL 可 `UNVERIFIED/SKIP`，但实现与真实集成用例必须交付。
- 最终 M1 验收：真实 MySQL 集成必须通过。缺此项不得宣称 M1 完成。
- Mock、编译通过、跳过都不能替代真实数据库验证。
- 工作树/模型诊断是可选工具检查，不是开发启动或业务验收门禁。

## 第一阶段验收矩阵（尚未实现）

| 范围 | 关键用例 |
|---|---|
| 扫描 | 单列失败阻断 apply；skip 与 override 次序；UNKNOWN 不写入历史；错误分类与退出码 |
| 检测（#6 / PR #20） | 13 类 China-first 有界规则的正反例；身份证/USCC/Luhn 校验；城市中文与拼音；中文姓名普通词误报；默认 20 非空 distinct 与 19/20、18/20 vs 17/20、90% 阈值；NULL/空串；冲突 UNKNOWN；位置/类型不匹配的人工覆盖 ErrInvalid；DateOrder 参数验证（MDY/DMY 文本解析不在本次覆盖）；取消、输入不变性和无敏感值日志 |
| 策略 | 首批 5 种策略的格式与类型保持；Unicode；NULL 与空串；typed 数值/日期；校验位有效性；未变化结果拒绝 |
| 确定性 | 固定 key/scope/version 重复一致；跨表同 scope 一致；映射复用；词典变化不漂移；碰撞处理 |
| Dry Run | 配置无法绕过 CLI 写意图；连接 spy 证明无写事务；文件树无新增或修改；历史与映射只读 |
| 分页/写入 | 超过两页；复合键；最后不足一页；无键或敏感主键拒绝；事务回滚；受影响行数；提交结果不明 |
| NULL 标记 | 仅精确列生效；先于映射；不写映射；目标列 NOT NULL 时报错 |
| 验证 | 行数与 schema；索引与约束；双向键差异；单字段未变化；无键、截断、缺表在 strict 下失败 |
| 历史 | pending/approved；审核人；类型与过期；修订冲突；导入失败不污染；回写备份 |
| 端到端 | 合成源快照 → scan → preview → apply → strict validate；SQLite 与真实 MySQL 分别独立运行 |

## China-first 检测专项测试与评估说明

- 代码证据：`internal/detect/detect_test.go`、`cn_test.go`、`cn_geo_test.go`、`eval_test.go`；不依赖数据库，应用 `go test -count=1 ./internal/detect/...`、`go test -race ./internal/detect/...`、全仓 `go test ./...` 和 `go vet ./...`。
- 合成评估：74 条自行构造的 `(value,column)` 样本（38 正、36 负；8 条超出本轮规则覆盖）；`MinSamples=1` 用于测试匹配器与列上下文，而非默认聚合阈值。报告每条规则的 TP/FP/FN、Precision、Recall、F1 和明确的覆盖界限。样本同时用于开发与评估，**不宣称独立验证或生产准确率**。
- 姓名误报：开发时数据 `cn_person_name` 的 TP=3、FP=3、FN=1（P=0.500、R=0.750、F1=0.600）。在独立负例扩充、评审和业务级不确定处理完成前，姓名检测只作实验性提示，不能自动推导写入批准。
- 测试与范围分离：组件测试不能替代真实 SQLite/MySQL 端到端扫库、策略映射、Dry Run、Apply 和严格验证。中国身份证/手机号/银行卡/USCC 的**格式约束替换**仍是 #12 与 go-mask 的待完成范围。
- GitHub CI 的 Ubuntu/Windows 格式、测试与 vet 以及 Ubuntu race 需针对**最终 PR Head**重新执行；MySQL fixture 绿灯不是业务 Adapter 验收。所有 SKIP/UNVERIFIED 需明确记录。

## 数据库适配器专项验收（#5 / PR #21）

- 使用真实 SQLite 驱动运行 `go test -count=1 ./internal/db/...`，确认只读且缺失文件不创建、表元数据与保守 `SchemaCoverage`、有界 DISTINCT 非 NULL 抽样、NULL/空串/Bytes/Decimal/Date 与安全类型处理、25 行三页、真实有序复合主键 keyset、乐观锁冲突零提交、Rollback、取消、非法列名与跨 DatasetID/Schema 白名单拒绝。特别覆盖新增 `internal/db/count_scope_test.go` 的 Count 越界检查。
- Go CI 应在 Ubuntu/Windows 运行全仓测试和 vet，并在 Ubuntu 运行 race；以合并目标分支对应最终 PR Head 的通过记录为准，不把 queued、SKIP 或 fixture 环境通过写成适配器验收通过。
- MySQL 业务测试入口是 `internal/db/mysql_test.go:TestMySQLAdapter`，需要隔离容器私有 socket 和 `MASKRIVER_TEST_MYSQL_SOCKET`；未提供时将 `SKIP/UNVERIFIED` 原样记录。#10 的 MySQL 8.0.46 适配器场景已由 PR #30 的测试入口实跑；尚不能宣称用户 CLI 的最终 M1 端到端验收。
- 本轮数据库组件不等于 CLI/runner 端到端已交付；全链路 Dry Run/Apply/严格 Validate 仍由 #9/#11 验收。

## 脱敏策略组件专项验收（#7 / 原 PR #18 / PR #24）

- 五种策略 `null`、`blank`、`redact`、`format_random`、`fake_email` 仅实现纯内存有界切片，统一入口 `mask.NewStrategy`、`mask.NewMemoryMapping`，并验证冻结 `m1a-v1` 签名；不实现中国身份证/银行卡/信用代码校验位保持替换或密码学 FPE（仍为 #12）。
- 测试覆盖：合法 `Bytes` 密钥且 >=32 字节，Text 密钥拒绝；ctx nil/取消；NULL 与空串；非 Nullable 列 null 拒绝；非空敏感输入不得保持不变；未知策略及不支持的 Kind 拒绝；HMAC keyed domain 隔离；仅支持部分 ASCII/汉字类的 Text 形状保持、任意 Bytes 等长；中文与二进制短输入边界；`fake_email` 只输出 keyed HMAC 和固定域名，不附原始值 CRC32。
- 内存映射采用键控指纹与原子 get-or-create 的单赢家语义；lookup 不写入；并发与 `go test -race ./internal/mask/...` 在最终 PR Head 的 Ubuntu CI 验证。暂不承诺持久映射、跨库 exactly-once、唯一性/碰撞或参照完整性。
- 最终 CI 需执行 `gofmt -l`、`go test -count=1 ./...`、`go vet ./...`、Ubuntu race，记录真实结果。仅 CI 配置就绪或 MySQL fixture 通过不能替代运行态组件验收。

## #12 中国格式约束脱敏组件（PR #29）

- **代码边界**：`internal/mask/cn_format.go`、`cn_numeric.go`、`cn_text.go`、`cn_format_test.go`；仅新增 9 个 `cn_*` StrategyRef，版本 `1`。共享 `m1a-v1`、数据库适配器和 CLI 工作流接口均不变。
- **明确约束**：身份证合法日期/有限地区码/MOD11-2；手机已知号段及可选国家码；银行卡 16–19 位/Luhn；USCC 有限类别/地区/MOD31；邮编六位；城市有界中英词表；姓名 2–4 汉字和限定姓氏；地址窄格式街道门牌；合法中国年月日及 typed Date。超范围输入必须返回 `ErrInvalid`/`ErrUnsupported`，而非伪造成功。
- **安全测试**：密钥 Bytes 且至少 32B，类型及策略/Scope/KeyID 绑定，取消与 nil context，NULL 保持、非 NULL 不得不变、目标长度不匹配拒绝，明确校验位错误/非闰日/错误号段/无效地址负例；合成输入不进入错误与日志，策略不访问数据库。
- **逐规则评估**：`TestCNFormatSyntheticEvaluation` 的开发时手工构造集合含 21 个正例、28 个负例（九条规则），测试格式接受结果并校验替换输出能再次通过对应格式规则。初轮 PR #29 CI 实测九条规则总 TP=21、FP=0、FN=0；逐规则 P/R 均为 1.000。这个小规模自编集合不是独立留出评估，更不是生产 PII 检测准确率或匿名化证明；检测规则的独立指标仍见 `internal/detect/eval_test.go`。
- **可复现命令**：`go test -count=1 -v ./internal/mask/...`、`go test -count=1 ./...`、`go vet ./...`、`go test -race ./internal/mask/...`、`gofmt -l internal/mask`；最终 PR Head 的 Ubuntu/Windows GitHub CI 应全部成功。先前通过的旧 SHA 不算最新提交通过。
- **真实 SQL 组件集成**：PR #30 在 SQLite 真实快照做日期策略的只读文件哈希对比、optimistic UPDATE/COMMIT、独立 SQL 回读和源快照不变性校验；另对全部九种策略在实际 SQLite 和 MySQL 8.0.46 表上做 typed Page → mask → WriteTx → 持久化读回及行数/主键检查。- **仍未验证**：CLI 全链路 Scan/Preview/Apply/strict Validate、SQL spy、跨列唯一/PK-FK、持久映射及所有目标列的字节长度限制，属于 #9/#11。生成格式有效的身份证、银行卡、手机号等号码并不保证无人持有、不会被投递/联系，也不保证匿名性；只能处理授权的合成/隔离测试数据。

更多覆盖范围与限制见 [#12 中国格式组件说明](issue12-cn-replacement.md)。

## #8 验证器组件（PR #32 原作者，PR #33 安全整合）

- **接口**：`verify.New() contracts.Validator`，仅传入两个 `contracts.Reader` 和冻结 `ValidateRequest.Plan`；不读取配置、不写源库/目标库、不扫描目标内容推断敏感列、不输出行或主键。
- **有界内存**：两边各保持至多 256 行的 `Page`，按真实 Keyset 顺序逐键合并；聚合同类受控 `Issue`，不会为每条行差异生成无界报告。对于未支持的键排序/Collation 显式 `ErrUnsupported`。
- **契约**：`RowLimit=0` 执行全量有界分页；正数表示总行预算，超过时 `PartialCoverage` + `Complete=false`；负数或 nil ctx/reader 拒绝。缺失 schema/PK 能力、类型不符、计划列缺失、重复键/倒退游标/空页面等必须失败关闭；完整空表可通过。
- **回归**：`internal/verify/verify_test.go` 保留原始 PR #32 的 fake-Reader 用例；`verify_safety_test.go` 新增 520 行分页、Issue 聚合、非法游标、计划/schema 错配、nil Context、缺少字段、刚好达到总行预算和截断拒绝。仅冻结 `Plan` 中的敏感列查验非 NULL 值变化，NULL 语义异常不通过。
- **门禁**：`gofmt -l internal/verify`、`go test -count=1 -v ./internal/verify`、`go test -count=1 ./...`、`go vet ./...`、`go test -race ./...`（Linux CI）；记录最终 PR Head 三项 CI 真实结论，不把初期 green 当作末次 green。
- **剩余未验收**：全量 DB schema 的 Index/FK/Unique/CHECK/Trigger 安全不变量以及真实 CLI/runner 的独立源快照严格 Validate 仍属 #9/#11。以上测试是 fake Reader **不是实际数据库全流程验证**，不能将 #8 组件通过描述成 M1 完成。

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

## #9 Runner 授权与故障回归（2026-10-18）

`internal/runner/workflow_acceptance_test.go` 使用真实 SQLite source/target，包装现有 Writer/WriteTx 接口作实例级能力与事务 Spy：

- 未 Approved、无 ExplicitApply、篡改 Plan、Preview 目标不一致：不打开 Writer。
- Preview：读取全部计划行，fixture 目录（source/target/key/sentinel）文件集合及 SHA256 不变，Writer/Begin/Update/Commit 为零；这**不是 SQL Driver Spy**。
- 混合 NULL 跨批次只更新非 NULL，NULL 保持；130 行全 NULL 跨页不打开 Writer。Preview 计读取行，receipt 只计实际更新行。
- Begin/Update/受影响行数不符/取消：错误返回，SQL 回读无未提交变化，Writer 关闭；Rollback 使用未取消、有 deadline 的独立清理 context。驱动可能不遵守该 deadline，不承诺硬超时。
- 第二批失败保留首批 receipt，SQL 回读确认已提交前缀；包装 ErrCommitUnknown 可识别，不自动重试或声称回滚。真实 SQLite 提交后模拟确认丢失，证明实际写入与保守报告的区别。

命令：`go test -count=1 -v ./internal/runner`、`go test -count=1 ./...`、`go vet ./...`、`go test -race ./...`。本机 Linux/Go 1.26.3 已执行；MySQL 无隔离 socket 时仍 SKIP，不能以本包替代 MySQL CLI 或完整 #11 验收。完整边界及未解决发现见 [Runner 收尾证据](m1-runner-acceptance.md)。
