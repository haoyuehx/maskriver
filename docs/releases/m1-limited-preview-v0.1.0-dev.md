# M1 Limited Preview 发行证据（linux/amd64）

- 发行日期：2026-10-19
- 类型：**GitHub Pre-release**（非稳定版、非生产就绪）
- Release：<https://github.com/haoyuehx/maskriver/releases/tag/v0.1.0-dev>
- Tag：`v0.1.0-dev` → 提交 `f4aeff9103ad064f57f63846cf47de587bdabeea`
- 资产：`maskriver-m1-linux-amd64-f4aeff9.tar.gz`（4 505 396 字节）
- **批准的归档 SHA-256**：`cf06a7e47cb7b1c558651b36f5c2de958b6c90e76f7906f4d51fce679fecc76c`

## 1. 发布前工件校验（发布前执行）

| 检查 | 结果 |
|---|---|
| 归档存在且摘要一致 | SHA-256 = `cf06a7e4…`，与独立复核报告一致 |
| `sha256sum -c SHA256SUMS` | 全部 OK |
| `MANIFEST` 与文件集合双向比对 | 一致 |
| `licenses/` 数量 | 20 |
| NOTICE 引用解析 | 20/20 存在，无悬空引用 |
| 二进制 `vcs.revision` | `f4aeff9103ad064f57f63846cf47de587bdabeea`，`vcs.modified=false` |
| `--version` | `MaskRiver 0.1.0-dev`，退出 0 |

工件未经任何修改。摘要与 `docs/reviews/m1-candidate-v-final-delta-review-2026-10-19.md` 中被独立复核的工件一致。

## 2. 标签

- 发布前 `v0.1.0-dev` **不存在**（本地与远端均为 0）
- 在 `f4aeff9` 创建附注标签并推送
- Tag 解引用提交 = `f4aeff9103ad064f57f63846cf47de587bdabeea`，与二进制内嵌 `vcs.revision` 一致
- 未移动、未覆盖任何既有标签

## 3. 上传后复核

重新从 Release 下载资产到独立目录：

| 项 | 值 |
|---|---|
| 下载文件大小 | 4 505 396 字节（与上传前一致） |
| 下载后 SHA-256 | `cf06a7e47cb7b1c558651b36f5c2de958b6c90e76f7906f4d51fce679fecc76c` |
| 与批准摘要比对 | **完全一致** |
| Release 状态 | `prerelease=true`，`draft=false`，资产数 1 |

未出现上传后工件不一致的情况，无需撤回。

## 4. 最终 Release Notes

标题：`MaskRiver M1 Limited Preview — Linux amd64`

正文（即 GitHub Release 上的正式说明）：

- 面向授权、隔离的 SQLite 测试快照的敏感信息发现与脱敏工具预览版本
- **已验证功能**：SQLite 多表 Scan/Preview/Approve/Apply/Validate；Ed25519 签名审批记录绑定完整计划；显式写入授权需同时满足 `--apply`、`--approved` 与有效记录；驱动级 SQL Spy、写入前安全门禁、批次事务与故障报告；通过独立复现构建、实际发行包检查与合成 SQLite 冒烟
- **限制与已知风险**：仅限授权/隔离/可弃置 SQLite 快照；MySQL CLI 未完成端到端；`--strict` 无独立数据库层严格语义；无 Checkpoint/恢复/exactly-once；无生产级准确率、匿名化、无碰撞、跨批全局原子性保证；审批记录不能检测整体回滚与撤销回滚，本地签名信任边界不等于 RBAC；被拒 WAL 源仍可能产生 `-wal`/`-shm`；CLI 拒绝当前无法通过验证的非 NULL→NULL，但 `internal/db` Writer 未提供同等保证；构建脚本仍有 umask 敏感性、链接集合流水线错误传播不足与 NOTICE 标题精度问题，本次只批准摘要已独立验证的工件
- **许可**：MIT；包内含 NOTICE、适用第三方许可文本与 MPL-2.0 组件源码获取说明

## 5. 接受并公示的 P2

| P2 | 说明 |
|---|---|
| 构建脚本 umask 敏感性 | `MANIFEST`/`SHA256SUMS`/目录模式未固定，不同 umask 会产生不同归档摘要（内容相同）；本次只批准摘要 `cf06a7e4…` |
| 链接集合管道缺 `pipefail` | `go list -deps` 管道失败时可能被下游吞掉，属 fail-open 风险 |
| NOTICE 标题精度 | 「Further texts shipped inside those modules」对 `modernc.org/memory` 的 `LICENSE-GO` 描述过度概括 |
| 历史核查 | v1 工件（`e50ec597…`）字节已未保留，其 READY 结论不可再复核；v2（`662bd601…`）因需手工 `chmod` 不可复现而作废 |

## 6. 使用限制

本版本为限定范围 Pre-release：

- 仅面向授权、隔离、可弃置的 SQLite 测试快照
- 不提供生产环境保证，不应视为稳定版
- 未实现能力（MySQL CLI、checkpoint、exactly-once、生产级准确率）不得对外宣称
- 后续若发布新工件，必须重新走独立复核，不得以本次批准替代

## 7. 复现命令

```sh
git checkout f4aeff9103ad064f57f63846cf47de587bdabeea
./scripts/build-release.sh /tmp/rel/maskriver-m1-linux-amd64-f4aeff9
cd /tmp/rel
tar --sort=name --owner=0 --group=0 --numeric-owner \
    --mtime='2026-10-19 00:00Z' \
    -czf maskriver-m1-linux-amd64-f4aeff9.tar.gz maskriver-m1-linux-amd64-f4aeff9
sha256sum maskriver-m1-linux-amd64-f4aeff9.tar.gz
# 期望：cf06a7e47cb7b1c558651b36f5c2de958b6c90e76f7906f4d51fce679fecc76c
```
