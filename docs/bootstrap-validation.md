# 初始化验收记录

## 环境与仓库

- 初始 cwd：`/home/haoyue/Project`。
- 只读上游：`/home/haoyue/Project/dbmask`，HEAD `7d8789ef4883a423ddba2f8934b95997d1aaf099`，初始工作区干净。
- 新项目：`/home/haoyue/Project/maskriver`，独立 `git init -b main`。
- `gh auth status` 成功，`gh api user` 用户为 `haoyuehx`；创建前仓库 API 返回 404，本地目录不存在。
- Go：`go1.26.3 linux/amd64`；module：`github.com/haoyuehx/maskriver`。
- 首次提交：`0d01c2f`（CLI/文档骨架）。
- `gh repo create --public ... --source . --remote origin --push` 成功。
- `gh repo view` 验证 URL 为 https://github.com/haoyuehx/maskriver，visibility=PUBLIC，isFork=false，默认分支 main。
- origin：`https://github.com/haoyuehx/maskriver.git`（fetch/push）。

## 本机验证

- `gofmt -w cmd internal` 完成。
- `go test ./...` 通过：cmd/maskriver、internal/config；其余包为无测试的占位包。
- `go vet ./...` 通过，无诊断。
- `go build -o bin/maskriver ./cmd/maskriver` 通过；版本输出 `MaskRiver 0.0.0-dev`。
- 暂存区 diff whitespace 检查通过；常见 token/private-key 格式筛查未命中。简单模式筛查不等于完整安全审计。
- 上游 MIT 文本与 `docs/upstream-LICENSE` 逐字节一致。
- 上游 143 个文件（含 .git）在骨架创建前快照与创建后 SHA-256 比对一致；只读 Git status 仍干净。哈希清单位于本地已忽略 `.local/`，不公开上传。
- 五个项目 Agent 被 pi-subagents list 正确识别，models 解析与指定模型一致；未启动子 Agent，未验证实际模型推理调用。

本记录只证明初始化骨架。没有运行上游 pytest、数据库集成、性能基准、恢复测试或远程 CI；不得据此宣传这些功能已完成。
