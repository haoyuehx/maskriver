# Contributors / 贡献者

MaskRiver 是四人协作的开源项目。下面仅列出目前能够从公开 PR 与 Git 提交核实的贡献；名单将随真实开发和审核工作持续补充，而不是按参与人数自动生成。

| GitHub | 可核实的贡献 |
| --- | --- |
| [@haoyuehx](https://github.com/haoyuehx) | 仓库维护与集成、中国敏感信息检测 [#20](https://github.com/haoyuehx/maskriver/pull/20)、数据库 Count 范围校验及整合 [#21](https://github.com/haoyuehx/maskriver/pull/21) |
| [@HerrscherofSentience555](https://github.com/HerrscherofSentience555) | 原始 SQLite/MySQL 适配器实现 [#16](https://github.com/haoyuehx/maskriver/pull/16)，随后由 [#21](https://github.com/haoyuehx/maskriver/pull/21) 纳入主线 |
| [@emgps](https://github.com/emgps) | 通过 [PR #18](https://github.com/haoyuehx/maskriver/pull/18) 提交 M1 五种脱敏策略和并发内存映射；安全修复与整合见 [PR #24](https://github.com/haoyuehx/maskriver/pull/24) |

## GitHub 贡献者统计

GitHub 的 **Insights → Contributors** 由默认分支上可归属的实际 Git 提交决定，**不是**由此 Markdown 名单直接决定。为了保留来自 fork 的真实作者身份，维护者合并 PR 时优先采用保留原始提交作者的 Merge 或 Rebase；确有必要 Squash 时，应在合并前核对作者/共同作者信息。

原始 PR #16 的作者提交是 [b8c9c5f](https://github.com/haoyuehx/maskriver/commit/b8c9c5f8be8d5b944213f25d296d3312abffc4c4)。此前数据库整合采用 Squash Merge，造成原始作者提交未进入 `main` 的祖先历史。本次采用**保留当前主分支文件树的历史合并**补回该真实作者提交，不重新应用旧代码，也不回滚 Count 安全修复。

**PR #18 的自动统计说明**：PR 创建者是 `emgps`，但原始 Git 提交 `515d628` 的 Git author 为 `Compiler Bot <noreply@example.com>`，GitHub 当前将该提交关联为 `PyGuy2`；二者不能自动视为同一个账号。本名单按实际 PR 贡献署名 `emgps`，而 Insights → Contributors 的账号统计仍取决于提交邮箱与账号绑定。需要在该图中归属 `emgps` 时，应由贡献者本人通过已关联邮箱提交后续真实代码贡献，不应替换他人作者记录。保留原始 commit 只能确保历史不丢失，不能保证其 GitHub 展示归属。

贡献者必须使用已关联本人 GitHub 账号的提交邮箱；GitHub 的 Contributors 统计可能延迟刷新。
