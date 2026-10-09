# Native Worktree 策略与预检

## 已批准的唯一布局

- 源 Git：`/home/haoyue/Project/maskriver/.git`
- dedicated root：`/home/haoyue/Project/worktrees`
- Native 实际叶：`/home/haoyue/Project/worktrees/maskriver/pi-worktree-<runId>-<index>`
- provider：**native**，不使用 auto/Worktrunk，也不使用仓库内 `.local/worktrees/`。

每个 writer 必须明确 `worktree:true` / `isolation:"worktree"`，验证返回路径、独占性、branch/baseCommit 和 git-common-dir。分配/配置失败即停止，不允许 `worktree:false`、共享主工作树或外部 CLI 替代。

## 实际安装与配置优先级

检查版本：pi-subagents **0.76.1**，Pi host **1.1.0**。
配置来源是 `~/.pi/agent/extensions/subagent/config.json`，不是项目 `.pi/settings.json` 的同名字段。
Main 已在原先不存在的该文件创建以下配置；没有覆盖其他已有设置，也没有安装或修改扩展：

```json
{
  "worktreeProvider": "native",
  "worktreeBaseDir": "/home/haoyue/Project/worktrees"
}
```

这两个配置为当前 OS 用户的 pi-subagents 设置，会影响其他项目的 Native 根选择（各项目仍有独立 projectName 子目录）；项目文档保留可复核快照，不提交用户目录、认证文件或环境配置。

依据已安装源码：
- `src/extension/config.js`：getConfigPath/loadConfig，只加载上述用户配置。
- `src/extension/index.js`：扩展初始化时 loadConfig；**已有会话必须 reload/重启**，磁盘更新不等于内存热更新。
- `src/runs/shared/worktree.js`：resolveWorktreeDedicatedRoot 使用 `configuredBaseDir ?? PI_SUBAGENTS_WORKTREE_DIR`，再默认 `{dirname(repoRoot)}/worktrees`。
- 非空显式配置优先于环境变量；配置未设才用环境变量；两者都未设才用默认。空白配置不是“回退”，可能报错。
- 非空 base 配置/环境在 provider=auto 时选择 native；显式 worktrunk 与 base 冲突会失败。我们固定 native，不依赖 auto 的探测结果。
- allocator 的 `options.baseDir/provider` 来自运行时配置；不要往 subagent 工具添加不存在的同名参数。
- Native 在任何 Git mutation 前拒绝 repository checkout 内部、Pi 扩展目录及不合法叶目录，不能用相对路径/软链绕过。

本轮 shell 未设置 PI_SUBAGENTS_WORKTREE_DIR / PI_CODING_AGENT_DIR。预检还用进程内临时环境值验证“配置 > 环境”，不写任何环境变量文件。

## 可复核命令

```sh
cd /home/haoyue/Project/maskriver
node scripts/check-native-worktree.mjs
# Main 在已提交的干净 main 上运行，不启动模型：
node scripts/check-native-worktree.mjs --allocate
```

脚本调用实际安装的 Native allocator API，而非模拟 `git worktree add`；为独立 Node 进程映射 Pi host 已安装的四个 peer 包，不下载模块或修改扩展。普通 Node import 起初因缺少 Pi peer 解析失败，采用现有 host 的明确模块映射后可复核；这是 Main 环境检查，不是 Agent 运行协议降级。
版本被固定为 0.76.1；扩展/host 升级应先重新审查 API，禁止静默使用旧脚本。

无参数检查：真实 loadConfig、provider、实际路径解析、配置优先级、非法仓库内路径拒绝及 worktrunk 冲突拒绝。
`--allocate`：要求 clean main；Native 新建唯一 worktree/branch，检查共同 Git 根；仅在新工作树生成一个 Main 诊断测试，运行 Go 测试，Native 捕获补丁并验证与工作树一致；保存证据后只移除这一个诊断文件，再 Native 回收已干净工作树。无 Agent、无模型请求、无 merge/push。
证据在主仓库已忽略 `.local/main-native-probe-*/`，包含 setup、tests、diffs/patch、cleanup。出错保留状态，不强制删除未知改动。

这证明 allocator/隔离/补丁/清理链，不证明模型调用；单 go-db 模型冒烟方案见 [m1b-launch-plan.md](m1b-launch-plan.md)。本轮结果见 [closeout-report.md](closeout-report.md)。
