# MaskRiver Roadmap

MaskRiver 是独立研发的 Go 数据脱敏与敏感信息治理工具。阶段目标是：在授权且隔离的测试数据库中，以可复现方式完成发现、审核计划、预览、显式脱敏写入和严格验证。**当前状态（2026-10-22）：`v1.0.0` 已发布，支持范围见 [发行说明](releases/v1.0.0.md)。** 本路线图保留 2026-10-18 起的历史里程碑记录；实现证据详见 [Feature Matrix](feature-matrix.md)、[测试计划](test-plan.md)、[契约](contracts.md) 与对应 PR/CI。

## 现状与里程碑

| 阶段 | 状态 | 验证过的边界 / 尚缺能力 |
| --- | --- | --- |
| M0 · 项目基座 | 完成（历史基线） | 独立 Go module、MIT 授权、help/version、跨平台 CI；当时数据库业务 CLI 只会安全拒绝，后续由 M1/M3 交付 |
| M1-A · 共享契约 | 完成 | `pkg/contracts` 冻结 `APIRevision=m1a-v1`、SQLite/MySQL 测试环境及基础类型 |
| M1-B · 四个核心组件 | 已合并 | #5/#10 数据库、#6 检测、#7/#12 脱敏、#8 验证；有真实 SQLite/MySQL 适配器与 CN 策略 SQL 组件集成、fake Reader 验证器回归。**不等于端到端 CLI** |
| M1-C · 全流程集成 [#9](https://github.com/haoyuehx/maskriver/issues/9) | **已关闭（CLOSED）** | SQLite 多表真实 Scan/Preview/签名审批/新进程 Apply/Validate 已测，AC 已由独立上下文签署，证据见 [Issue #9 验收报告](issue9-acceptance-report.md)；`--strict` 独立严格语义与 MySQL CLI 后由 M3-1/M3-2 补齐 |
| M1-D · 独立安全验收 [#11](https://github.com/haoyuehx/maskriver/issues/11) | **已裁定 ACCEPT WITH RESTRICTIONS** | 独立非作者审查已完成：P0=0、P1=0（原 P1 为许可/署名，已由 PR #58 修复并经第二独立上下文复核）；六条 P2 仍开放。当时 M1 尚未发布；后续 `v1.0.0` 已按支持范围发布 |
| M2 · 增强能力 | **本地 SQLite 预览已交付** | [#13](https://github.com/haoyuehx/maskriver/issues/13) 已关闭（SQLite 有界流式 Apply + Outbox）；[#14](https://github.com/haoyuehx/maskriver/issues/14) 本地 SQLite `m2-recovery-v1` 已实现并验收，并纳入 `v0.2.0-preview.1` 发布候选。MySQL/Windows/WAL 恢复、跨库映射与通用 exactly-once 仍不支持 |
| M3 · MySQL CLI / 稳定版 | **已发布 v1.0.0** | M3-1 strict/安全、M3-2 MySQL 8.0.46 CLI、M3-3 UX、M3-4 生成式评测、M3-5 跨平台 CI 完成；Linux amd64 + SQLite/MySQL M1，SQLite-only streaming/recovery。证据见 [M3 验收](m3/acceptance-matrix.md) |

## 已交付核心能力（M1-B）

- **go-db**：真实 SQLite 与隔离 MySQL 8.0.46 的 Reader/Writer、元数据、typed 值、复合键分页、乐观写入和事务回滚。MySQL 业务适配器已实际运行 `TestMySQLAdapter`，不是仅有 Fixture smoke（PR #21/#30）。未验证其他 MySQL 版本和通过 CLI 的完整工作流。
- **go-detect**：有界 13 类 China-first 规则与三态判断（PR #20）；默认采样下的真实世界准确率没有被独立证明，`UNKNOWN` 不可授权写入。
- **go-mask**：5 种基础策略、并发安全内存映射、9 种中国格式约束替换（PR #24/#29），以及在 SQLite/MySQL 合成表的真实 SQL 组件集成（PR #30）。无持久映射、全局唯一性、未分配号码或匿名化保证。
- **go-verify**：基于冻结 Plan 的 Schema、行数、双向主键与所选敏感列变化校验；两端有界分页，异常 fail-closed，主要测试为 fake Reader（PR #32/#33）。真实 SQLite CLI 的独立源快照验证已由 #9 覆盖；MySQL 8.0.46 CLI 由 M3-2 补齐。

## M1-C：Issue #9 的交付顺序

按依赖顺序拆成小 PR，最终由 Main 负责合并和端到端编排，不平行重写 `runner`。每个 PR 必须列出变更文件、最终 SHA、测试命令与真实 PASS/SKIP/UNVERIFIED。

| 顺序 | PR 工作包 | 主责 / 支持 | 完成门禁 |
| --- | --- | --- | --- |
| 1 | **配置、Plan 和授权门禁**：读取安全配置、组装冻结 Plan、审查决策与 Digest | A 主责；C 审核 UNKNOWN/策略，B 审核数据定位 | 配置不得激活 Apply；Plan 与 Schema/Key 对齐；无 DSN、密钥或明文泄漏 |
| 2 | **Scan 与默认 Preview**：有界样本和检测、生成待审核决策、确定性预览统计 | A 编排；C 规则门禁，B 源快照/只读支持 | `UNKNOWN`、抽样不足、无主键、非法策略均 fail-closed；Dry Run 文件快照加 SQL Spy 证明**零持久副作用** |
| 3 | **显式 Apply**：仅 `mask --apply` 可以进入 Writer，提交前重新校验 Plan、范围和 schema；分批事务 | A 唯一写入协调；B 提供 DB 事务/回滚测试，C 处理策略边界 | 无授权绝不写入；冲突/取消/提交状态不确定明确失败；Preview 与 Apply 输出一致；不承诺跨批全局原子性 |
| 4 | **Strict Validate 和 CLI 出口**：从脱敏前独立快照与目标比较，报告安全摘要、可靠退出码 | A 接入 CLI；D 主责验证用例，B 提供隔离库 | 真实 SQLite 合成库 `scan → preview → apply → strict validate` 完整通过；失败/部分覆盖不能返回成功 |
| 5 | **回归与文档**：错误注入、CI、演示脚本、真实输出证据、更新功能矩阵 | A 发布协调；B/C/D 各自交付证据 | Ubuntu/Windows Go CI、Ubuntu race、真实 SQLite 端到端；日志无原值/密钥/DSN；列出尚未验证的情况 |

首版只承诺**经测试的 SQLite CLI 全流程**。该历史门禁后续由 M3-2 补齐 MySQL 8.0.46 CLI；当前状态见 [Feature Matrix](feature-matrix.md)。

## 早期分工（历史）

早期四人 A/B/C/D 分工是计划占位，不是当前贡献者名单。该表已迁移到 [历史快照](history/four-person-collaboration-plan.md)；当前可核实贡献者见根目录 [README](../README.md#contributors--项目贡献者)。仓库不再依据“四人”推定团队人数或职责。

## M1-D：Issue #11 的最终验收

在 #9 真实 SQLite CLI 端到端通过后开展**只读**独立审查；实现作者不能自我批准。必须有文件/行号、复现命令、运行结果或明确标记 UNVERIFIED，并出具 P0/P1/P2、残余风险及 Go/数据库版本信息。

- 默认 Preview 不写源/目标数据、映射库或历史文件：文件快照及 SQL Spy 双重证明。
- 扫描 UNKNOWN/错误、无稳定主键、缺失计划/Schema、未验证输出、密钥/Scope/数据类型错误一律 fail-closed。
- `--apply` 唯一授权写入入口；乐观冲突、取消、提交歧义和批间部分成功要准确报告。
- 与脱敏前**独立源快照**比较双向键、行数、Schema 与计划字段；未完成覆盖不能 PASS，strict 模式拒绝 Warning/UNKNOWN。
- CLI stdout/stderr、日志、审计、测试报告和发布资产不得包含敏感明文、行键、DSN 或密钥。
- M1 Feature Matrix、README 示例、版权/依赖许可证与 CI 证据逐项核对，保留不支持的数据库/格式/失败场景清单。

## M2：不阻塞首版的研发

1. [#13](https://github.com/haoyuehx/maskriver/issues/13)：有界 Worker Pool/背压/取消以及 race、fuzz、重复跑的吞吐/延迟/RSS/分配基准；只发布真实测得的分布和环境。
2. [#14](https://github.com/haoyuehx/maskriver/issues/14)：绑定源快照、版本、提交批次的 checkpoint 与跨存储 commit-gap 策略；故障注入证明幂等和可恢复性前不宣称 exactly-once。（已于 2026-10-21 交付本地 SQLite `m2-recovery-v1`。）
3. 演示与竞赛材料：合成数据、可重放 CLI 录屏/命令、架构与安全边界、测试方法、来源许可清单。报名资格和赛事日程需要另行核实。

## 2026-10-21 增量：Issue #14 本地事务恢复验收

- **#14 已交付并关闭（以最终 GitHub 状态为准）**：`m2-recovery-v1` manifest/progress、同事务 checkpoint、显式 `--resume`、只读 `status`、commit_unknown 重核、纯确定性策略 allowlist、KCV key 绑定与真实 CLI 故障注入。协议、AC 矩阵、故障矩阵和 10k/100k/1M 规模数据见 [Issue #14 验收报告](issue14-acceptance-report.md)。
- **支持边界**：Linux amd64 本地 SQLite（rollback journal、`synchronous=FULL`）。MySQL/Windows/WAL 恢复、跨库持久映射的跨存储原子性、通用 exactly-once 与真实断电 durability 不在本轮范围；不支持路径 fail closed。
- **不改变 #13**：无 recovery schema 时仍是原 outbox v1 事务；`m1a-v1`、`m2-stream-v1`、`m2-approval-binding-v1` 与发布版本保持不变。

## 2026-10-21 发布候选：v0.2.0-preview.1

`v0.2.0-preview.1` 是面向 Linux amd64、本地 SQLite 的公开 Pre-release 目标版本，包含 M1 SQLite 工作流与 #13/#14 的 Streaming、Outbox、Checkpoint、Status、Resume 和幂等恢复。候选验收、测试/恢复 Smoke 与许可审计见 [v0.2.0-preview.1 验收记录](releases/v0.2.0-preview.1-acceptance.md)；最终 Tag、资产 SHA-256 与发布后下载复核记录在 GitHub Release 验收评论中。协议与限制见 [Issue #14 恢复协议](design/issue14-recovery-protocol.md) 和 [验收报告](issue14-acceptance-report.md)。`v0.1.0-dev` 的历史 Tag/Release/资产保持不变。

## 不做的承诺

当前有 SQLite 多表与 MySQL 8.0.46 合成数据库 CLI 的真实端到端路径；没有 MySQL streaming/recovery、MySQL/Windows/WAL 断点恢复、完整通用数据库 CLI、生产级 PII 识别准确率、全局匿名化、稳定公共 SDK、通用 exactly-once 或高性能并发证明；本地 SQLite 恢复的范围见 2026-10-21 增量。项目只处理获得授权的隔离/合成数据；保持 `m1a-v1` 契约，任何变化必须经 Main 审核并记录证据。发布状态必须以功能与实际测试为依据，而不是 Issue 标记或 CI 配置是否存在。

## 2026-10-18 收尾增量

#9 已交付 SQLite 多表 CLI、Ed25519 审批记录（整计划绑定、有效期、撤销）、驱动级 SQL Spy、静态安全门禁与批次故障/提交未知回归，证据见 [测试计划](test-plan.md)；早期单表收尾记录见 [2026-10-18 验收记录](m1-runner-acceptance.md)。该历史时点 `--strict` 仍 inert；其后 M3-1 补齐 strict，M3-2 补齐 MySQL CLI；断点恢复、exactly-once 与其余边界仍未完成；已接受 P2 见测试计划。#11 保持未验收，#13/#14 的阶段依赖不变。PR #36 的 Worker 组件已经合并，不能按本文件早期的“M2 未启动”重新开发。

> 上段是 2026-10-18 当日结论，作为历史记录保留不改写；其中“#11 保持未验收”已被下节裁定取代。

## 2026-10-19 增量：#9 关闭与 #11 裁定

- **#9 已 CLOSED**：AC 已由独立上下文签署（[Issue #9 验收报告](issue9-acceptance-report.md)，2026-10-18）。该报告的历史结论不改写。
- **#11 已裁定 ACCEPT WITH RESTRICTIONS**：第三方或团队内非作者审查已完成，P0=0、P1=0。原 P1（第三方许可/署名）由 PR #58 修复，并已由第二个独立上下文复核；合并记录见 PR #58。裁定、六条 P2 与发布范围见 [Issue #11 验收裁定](m1-acceptance-issue11.md)。
- **六条 P2 仍开放**，均需后续 Issue 关闭，登记表中的编号与内容不改。
- **历史时点：M1 尚未发布。** 发布门禁后续由 v0.1.0-dev/v0.2.x preview 与 v1.0.0 逐步完成；历史限制不因新版本消失，见 [测试计划](test-plan.md) 与 [v1.0.0 验收](releases/v1.0.0-acceptance.md)。

## 2026-10-19 发行增量

M1 限定预览版已作为 **GitHub Pre-release** 发布：tag `v0.1.0-dev` → 提交 `f4aeff9103ad064f57f63846cf47de587bdabeea`，工件 `maskriver-m1-linux-amd64-f4aeff9.tar.gz`，存档 SHA-256 `cf06a7e47cb7b1c558651b36f5c2de958b6c90e76f7906f4d51fce679fecc76c`，已通过上传后重新下载核对。发行证据见 [M1 Limited Preview 发行证据](releases/m1-limited-preview-v0.1.0-dev.md)。

该版本**不是**稳定版或生产就绪版本：MySQL CLI、MySQL/Windows/WAL 恢复、通用 exactly-once、真实断电 durability 与生产级检测准确率仍未验证；本地 SQLite 恢复见 2026-10-21 增量。

## 2026-10-21/22 增量：M3-1 与 M3-2

- **M3-1 已合并 `cc178aa`**：strict 全量+适配器完整性检查、Writer 非 NULL→NULL 拒绝、审批时钟偏差对齐；`v0.2.1-preview` 已发布。
- **M3-2 已合并 `71e85fd`（PR #93）**：MySQL 8.0.46 CLI `Scan→Preview→Approve→Apply→Validate`，真实隔离 CI E2E；MySQL streaming/recovery 明确拒绝。MySQL 恢复仍无协议，不得宣称支持。
- M3-3/M3-4/M3-5/M3-6 后续均已完成；`v1.0.0` 已发布。F1/生产准确率、跨平台 streaming/recovery 与稳定版门禁仍按各自验收记录执行。
