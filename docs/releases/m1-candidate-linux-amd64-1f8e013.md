# M1 候选发布包验证记录（linux/amd64）

- 日期：2026-10-19
- 提交：`1f8e0130f71a3273a19f10d23d1189241c902fff`
- 平台：linux/amd64 · Go go1.26.3 linux/amd64
- 构建方式：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOFLAGS=-trimpath`，`-buildvcs=true -ldflags "-s -w"`
- 包名：`maskriver-m1-linux-amd64-1f8e013.tar.gz`
- 包大小：4 505 193 字节
- **包 SHA-256**：`e50ec59744547888d6143f71538319020807dddad7431e70727df9d708ef8b6f`
- 本轮性质：**候选包**。未创建 GitHub Release、未打 Tag、未宣称生产可用。

可复现构建：

```sh
git checkout 1f8e0130f71a3273a19f10d23d1189241c902fff
./scripts/build-release.sh /tmp/rel/maskriver-m1-linux-amd64-1f8e013
tar --sort=name --owner=0 --group=0 --numeric-owner \
    --mtime='2026-10-19 00:00Z' \
    -czf maskriver-m1-linux-amd64-1f8e013.tar.gz maskriver-m1-linux-amd64-1f8e013
sha256sum maskriver-m1-linux-amd64-1f8e013.tar.gz
```

## 1. 包内容

| 文件 | 说明 |
|---|---|
| `maskriver` | 可执行文件（linux/amd64，静态，`-trimpath`） |
| `LICENSE` | 项目 MIT 许可证原文 |
| `NOTICE.md` | 归属索引：默认二进制实际链接 / 依赖内嵌并被编译 / 上游自带但无链接代码，三组分开 |
| `licenses/` | 20 份第三方许可证与声明原文 |
| `MPL-2.0-SOURCE.md` | `go-sql-driver/mysql` v1.10.1 对应源码获取方式 |
| `BUILD-INFO.md` | 版本、提交、平台、构建模式、已验证/禁止宣称范围、已知限制 |
| `MANIFEST` | 包内全部文件清单 |
| `SHA256SUMS` | 除自身外每个文件的摘要 |

**不含**：密钥、真实 PII、审批记录、测试数据库、构建缓存、开发残留。

## 2. 在全新目录解压后的独立核验

解压到 `/tmp/verify/`（与构建目录分离），逐项执行：

| 检查 | 命令 | 结果 |
|---|---|---|
| 摘要完整性 | `sha256sum -c SHA256SUMS` | **24/24 OK** |
| 清单与实际文件一致 | `diff` MANIFEST 与 `find` | **一致** |
| NOTICE 每个引用真实存在 | 提取 `licenses/*` 引用逐个 `test -f` | **20/20 存在** |
| 许可证内容未被改动 | 与 `<GOMODCACHE>/<module>@<ver>/…` 做 `cmp` | 抽样 3/3 **逐字节一致**（libc 3RD-PARTY、sqlite 3RD-PARTY、mysql LICENSE） |
| MPL-2.0 源码获取路径真实 | `go mod download github.com/go-sql-driver/mysql@v1.10.1` | 版本与 `go.mod` 一致，模块缓存可取 |
| 可执行文件启动 | `./maskriver --version` | `MaskRiver 0.1.0-dev`，退出 **0** |
| help | `./maskriver --help` | 退出 **0** |
| 无参数拒绝 | `./maskriver scan` | 退出 **2**，输出已脱敏 |
| 包内敏感残留 | `grep -rnE '/home/\|/Users/\|C:\\\\Users\|PRIVATE\|…@…'` | **0 命中** |

## 3. 合成 SQLite 冒烟（用包内二进制执行）

fixture：`people(id INTEGER PRIMARY KEY, email TEXT NOT NULL, full_name TEXT)`，3 行合成数据（`user{1,2,3}@example.invalid`），源与目标为两份独立数据库副本。

| 步骤 | 退出码 | 输出（脱敏） |
|---|---|---|
| `scan --min-samples 3` | **0** | `scan: complete=true columns=1 issues=0` |
| `mask --strategy fake_email --min-samples 3`（默认 Dry Run） | **0** | `preview: dry-run=true rows=3 complete=true (no row values shown)` |
| `approve`（独立签发，Ed25519） | **0** | `approve: stored=true scopes=1 valid_for=24h0m0s` |
| 新进程 `mask --apply --approved` | **0** | `apply: complete=true committed_batches=1 committed_rows=3` |
| `validate --approved --strict` | **0** | `validate: pass=true tables=1 rows=3 issues=0` |

补充观察：`email` 带 `UNIQUE` 的 fixture 被拒绝（`scan: complete=false … issues=1`，退出 1），与「被脱敏列位于 UNIQUE/CHECK/FK/触发器即拒绝」的既定不变量一致。这条负例说明冒烟不是只挑能通过的路径。

## 4. 与 #11 裁决的关系

#11 的 `R-08`（发行工件许可声明集合）在**源码范围**已关闭；本记录完成的是**二进制范围**的四项发行前检查中的一部分：

| 二进制发行门禁 | 本轮状态 |
|---|---|
| NOTICE 式归并全部适用文本 | **已在本包内完成**（`NOTICE.md`，三组分开） |
| 随包附带完整许可证文本 | **已在本包内完成**（`licenses/` 20 份） |
| MPL-2.0 源码获取途径落地 | **已在本包内完成**（`MPL-2.0-SOURCE.md`，版本匹配） |
| 逐次发行 SHA-256 留证 | **已在本记录内完成**（包摘要与逐文件摘要） |

但「本轮完成」不等于「可发布」：仍须由独立 Reviewer 检查**实际压缩包**（而非仓库文件）后才能判定 `RELEASE CANDIDATE READY`。

## 5. 明示的适用范围与禁止宣称

可用范围：合成或经授权的可弃置 SQLite 快照，linux/amd64。

不得宣称：MySQL CLI 端到端 · checkpoint/恢复 · exactly-once · 生产级检测准确率 · macOS/Windows CLI · `--strict` 独立严格语义 · 跨批全局原子性 · 匿名化 · 能抵御审批记录整体回滚或撤销回滚 · 拒绝 WAL 源时零文件变化。
