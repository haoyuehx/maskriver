# Contributors / 贡献者

MaskRiver 是四人协作的开源项目。下面仅列出目前能够从公开 PR 与 Git 提交核实的贡献；名单将随真实开发和审核工作持续补充，而不是按参与人数自动生成。

| GitHub | 可核实的贡献 |
| --- | --- |
| [@haoyuehx](https://github.com/haoyuehx) | 仓库维护与集成、中国敏感信息检测 [#20](https://github.com/haoyuehx/maskriver/pull/20)、数据库 Count 范围校验及整合 [#21](https://github.com/haoyuehx/maskriver/pull/21) |
| [@HerrscherofSentience555](https://github.com/HerrscherofSentience555) | 原始 SQLite/MySQL 适配器实现 [#16](https://github.com/haoyuehx/maskriver/pull/16)，随后由 [#21](https://github.com/haoyuehx/maskriver/pull/21) 纳入主线 |
| [@emgps](https://github.com/emgps) | 提交脱敏策略 [#18](https://github.com/haoyuehx/maskriver/pull/18)（以该 PR 的实际状态为准，尚不代表已合并） |

## GitHub 贡献者统计

GitHub 的 **Insights → Contributors** 由默认分支上可归属的实际 Git 提交决定，**不是**由此 Markdown 名单直接决定。为了保留来自 fork 的真实作者身份，维护者合并 PR 时优先采用保留原始提交作者的 Merge 或 Rebase；确有必要 Squash 时，应在合并前核对作者/共同作者信息。

原始 PR #16 的作者提交是 [b8c9c5f](https://github.com/haoyuehx/maskriver/commit/b8c9c5f8be8d5b944213f25d296d3312abffc4c4)。此前数据库整合采用 Squash Merge，造成原始作者提交未进入 `main` 的祖先历史。本次采用**保留当前主分支文件树的历史合并**补回该真实作者提交，不重新应用旧代码，也不回滚 Count 安全修复。

贡献者必须使用已关联本人 GitHub 账号的提交邮箱；GitHub 的 Contributors 统计可能延迟刷新。
