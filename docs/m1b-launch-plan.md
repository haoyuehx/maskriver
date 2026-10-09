# M1-B 启动方案与单 go-db 模型冒烟设计

本轮只准备，不运行四个正式开发 Agent。单 go-db 隔离冒烟已实际尝试一次，被 `openai-codex/gpt-5.6-sol` 的用量配额阻断（`The usage limit has been reached`，0 tokens）；隔离、基线、上下文、补丁通道已验证，模型在隔离 worktree 内的端到端完成仍未证明。用户已要求暂缓重试（详见 closeout-report.md）。

冒烟验收要求 reviewer（`review.required`），因此通过后还需一次受授权的只读审查作为该 lane 的独立门禁。

## A. 冻结前置条件

- Main 的 m1a-v1 代码/文档、两驱动许可与 SQLite 真实测试通过，提交并 push main，远程 HEAD 与本地一致且 clean。
- reload/重启 Pi 使 Native 用户配置生效；执行 `node scripts/check-native-worktree.mjs` 验证当前安装/磁盘配置，再通过模型冒烟验证实际 Agent 运行路径。
- 本轮 list/models/doctor 仅证明扩展发现、注册表模型和 async 支持；模型调用/配额/子会话加载仍必须实测。
- MySQL UNVERIFIED 暂不阻塞首轮，但最终 M1 不能通过。模型/工具/隔离失败仍是硬阻塞，不更换模型或降级共享 cwd。

## B. 单 go-db 冒烟（无自动执行）

已准备 `.pi/workflows/go-db-smoke.js`，仅一条 lane，模型 openai-codex/gpt-5.6-sol。启动需用户明确同意该小范围 smoke；不等于正式的 go-db 组件开发任务。

Main 的候选调用（本轮不执行）：

```text
subagent({
  workflow: ".pi/workflows/go-db-smoke.js",
  args: {confirm: "run-go-db-smoke"},
  cwd: "/home/haoyue/Project/maskriver",
  isolation: "worktree", worktree: true,
  baseRef: "refs/heads/main",
  context: "fresh", async: true,
  globalConcurrencyLimit: 1, maxSubagentSpawnsPerRun: 1,
  share: false, artifacts: true
})
```

启动前记录 main 的完整 SHA；运行期间不改变 main 基线。只允许创建 `internal/db/worktree_smoke_test.go`，断言冻结 APIRevision 并用新建 SQLite 合成临时库做极小测试；不写生产 adapter、不接 MySQL、不改动其他模块。

### 逐项验收与证据

1. **隔离**：runtime 返回 native provider、唯一 worktree 路径、branch/baseCommit；worker 自检 cwd/common-dir；Main 再查 git worktree list。cwd 必须在 `/home/haoyue/Project/worktrees/maskriver/`，common-dir 必须是 MaskRiver `.git`，不能等于 main 或另一 lane。
2. **模型调用**：runtime 终态、实际模型身份校验、token/usage 和 transcript 证明 openai-codex/gpt-5.6-sol 实际执行；不能以 worker 自述或 registry 存在冒充。凭证/会话不上传公开仓库。
3. **最小写入**：只增加一个 smoke test，无共享文件修改，无 git commit/push；Main 检查父仓库 HEAD 与工作区状态。
4. **真实测试**：worker gofmt/test/vet；runtime gate 再执行 go test/vet。SQLite 必须实测，MySQL SKIP 明示。模型/API/门禁失败则停止保留证据，不启动后续四 worker。
5. **补丁保存**：读取实际 handoff manifest/outputReference，保存 diff 路径、base SHA、changed-files、patch SHA-256、测试日志；Main 校验 `git apply --check` 针对正确基线以及只包含 smoke 文件。捕获器可 staging，但子 Agent 不得 staging/commit。
6. **不合并**：主分支不 git apply/merge/cherry-pick 冒烟补丁，不 push 诊断测试；补丁与会话仅保留已忽略的本地 artifact。
7. **回收**：确认终态、无活动进程/会话依赖、完整 durable handoff，再查看 `worktree.cleanup mode:plan`；若由于未合并/有改动而拒绝，这是安全行为，不捏造 merge evidence。Main 单独批准仅该 smoke 的 discard 或保留待处理；不 blanket force/prune 其他树。回收后核对 worktree 注册、分支状态、主 HEAD 未变和补丁仍存在。不能把“只分配成功”当完整 lifecycle 通过。

## C. 四 Agent 正式并行方案（后续批准后）

一次 async workflow，四条 `runs.all` lane，共同 clean/frozen 命名 ref，worktree=true/isolation=worktree，globalConcurrencyLimit=4，maxSubagentSpawnsPerRun=4，context=fresh；每条有 verb+behavior label 和独立输出名。禁止共享 cwd，不使用手工工作树替代 Native。

| Lane | Model | 独占写路径 | 接口/门禁 |
|---|---|---|---|
| go-db | openai-codex/gpt-5.6-sol | internal/db/**/*.go | contracts Reader/Writer/WriteTx/DatabaseOptions；SQLite 真实 tests；MySQL 实现保留、可 UNVERIFIED |
| go-detect | openai-codex/gpt-5.6-sol | internal/detect/**/*.go | contracts Detector/Decision；规则证据纯内存 tests |
| go-mask | openai-codex/gpt-6.1-sol | internal/mask/**/*.go | contracts Strategy/MappingReader/Writer；纯策略与内存映射，race |
| go-verify | openai-codex/gpt-5.6-sol | internal/verify/**/*.go、tests/**/*.go、自写 tests/testdata/** | contracts Validator/Reader；fake 仅验组件，不冒充 DB 集成 |

具体范围、参考绝对路径、停止条件沿用 parallel-tasks.md。不更改冻结 DTO、驱动/依赖、Main runner/config/history/testenv/tools/scripts/docs/.pi。模型不可用须报告，不能静默代换。

## D. 补丁整合

Main 收齐各 lane 的 patch + handoff + base SHA + commands/evidence；不接受只有聊天结论。先审文件所有权、无敏感数据、接口变动及 `git apply --check`，发生冲突回派同 owner，不强行覆盖。
Main 在单独集成分支按依赖/风险串行应用已审补丁并提交；worker 本身不 commit/push。每次应用执行全套 test/vet，集成后执行 SQLite 端到端与 race；MySQL 未验证项继续挂最终 M1 门禁。
经另行授权的只读 reviewer 审查具体整合 ref，Main 决定修复和发布，最后才合并/推送 main。Native/worktree 不自动合并；后续清理保留补丁及真实 provenance，不伪造合并记录。
