# Multi-Agent 配置

已检测当前环境 `pi-subagents` 可用：enable、list(capabilities)、models 管理查询成功；没有安装扩展，没有启动任何迁移子 Agent。

| 角色 | 精确 provider/model | 配置 |
|---|---|---|
| Main | openai-codex/gpt-6-astra | .pi/settings.json |
| go-db | openai-codex/gpt-5.6-sol | .pi/agents/go-db.md |
| go-detect | openai-codex/gpt-5.6-sol | .pi/agents/go-detect.md |
| go-mask | openai-codex/gpt-6.1-sol | .pi/agents/go-mask.md |
| go-verify | openai-codex/gpt-5.6-sol | .pi/agents/go-verify.md |
| go-reviewer | openai-codex/gpt-6-astra | .pi/agents/go-reviewer.md |

五个 Agent 已经由项目级 list 查询识别，models 查询确认每个模型来源为 project agent config。
三个精确 ID 均列于本次会话可用模型注册表；未做付费推理探测，不能保证未来配额/网络/权限。
使用已有 provider，不伪造 endpoint、models.json、API key 或模型元数据，也不改变用户级配置。

## 使用前提

从 MaskRiver 根目录启动 Pi，审阅/信任项目配置；切换目录后的会话需重新加载项目资源。
通过 `subagent({action:"list", capabilities:true, cwd:"<MaskRiver绝对路径>", agentScope:"project"})` 检查角色，
通过 `subagent({action:"models", cwd:"<MaskRiver绝对路径>"})` 核对模型解析。
Main 的启动默认不覆盖恢复会话中已记录的模型；恢复会话时另核对当前模型。
此配置不带自动启动流程、定时器、安装声明或网络服务。
其他机器未安装扩展时只保留这些定义作为建议配置；须用户评审批准再安装，不能自动安装未知扩展。

## 权限和编排

- 四个 writer 仅可编辑各自所有权文件，工具 read/grep/find/ls/bash/edit/write；没有 subagent 工具。
- reviewer 仅 read/grep/find/ls，无 bash/edit/write；报告在回复返回，测试由 Main 或 go-verify 执行。
- 当前是 M1-A 收尾/M1-B 准备，不启动四个正式 Worker；接口冻结 m1a-v1。单 go-db 冒烟方案见 m1b-launch-plan.md，未执行不宣称模型调用通过。
- 根目录 `/home/haoyue/Project/worktrees`，仅 Native；实际 worktree 位于其 `maskriver/` 子目录，一份 cwd 一个 writer，只用 MaskRiver Git，不在 dbmask 创建工作树。分配失败停止，不降级共享 cwd。
- 上游统一绝对路径 `/home/haoyue/Project/dbmask`；各任务的精确参考见 parallel-tasks.md，禁止 worktree 内 `../dbmask`。
- Agent frontmatter 和 AGENTS 是协作限制，非 OS 文件沙箱。未来运行应隔离生产凭证和网络，将上游挂载为只读。
- Main 负责调度、共享契约、整合、最终验收与发布；模型不可用时停止报告，不静默降级。

## Native 配置与分阶段门禁

已安装 pi-subagents 0.76.1 的 allocator 配置读取用户级 `~/.pi/agent/extensions/subagent/config.json`，不从项目 settings 的同名键读取。
Main 将已知配置设为 worktreeProvider=native、worktreeBaseDir=/home/haoyue/Project/worktrees；这两个设置对该 Pi 用户生效，不含凭证。优先级和复核命令见 worktree-preflight.md。
扩展初始化时 loadConfig，已有会话需 reload/重启后再执行模型冒烟；磁盘文件更新不等于旧会话热更新。
SQLite 是首轮真实门禁；MySQL 可未验证但不能冒充通过，最终 M1 仍要求真实集成。

## 初始化路径异常记录

管理工具创建 go-db 时未按传入项目 scope/cwd 落盘，曾写入用户级 agents 目录。
该新文件已立即移入本项目 `.pi/agents/`，用户级临时定义已移除；其余定义直接写入项目。
没有因此启动 Agent 或修改 dbmask。最终全部项目配置位于 MaskRiver。
