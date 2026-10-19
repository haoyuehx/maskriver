# Issue #11 P1 R-08 关闭复核（第二独立上下文）

- **复核对象**：PR #60（`fix/licence-embedded`），被复核 Head = `ddf728d`（`docs/reviews/issue11-rereview-2026-10-19.md` 并入后的提交），base `main` = `c3907fee0c95a6d4ab9014382a52af3d4167c715`。
- **复核范围**：仅 R-08（`docs/reviews/issue11-rereview-2026-10-19.md` §5、登记表 R-08 行）的关闭条件；不重审代码、不重审 R-01…R-21 其余条目。
- **上下文独立性**：本复核未参与 PR #58、PR #59、PR #60 的任何实现或撰写；结论由本轮独立执行 `cmp`、`go list` 与上游清单比对得出，不采信实现者或上一轮评审的结论。
- **仓库状态为只读**；唯一新增文件为本报告。

## 1. 结论摘要

| 项 | 结论 |
|---|---|
| 8 份新增内嵌文本 | **8/8 存在且与模块缓存逐字节一致**，无 mismatch |
| 被编译/链接模块的内嵌第三方文本 | **无遗漏**（以 `modernc.org/sqlite` 自带的扁平化 SBOM 清单为独立基准交叉核对） |
| `docs/attribution.md` 新增章节 | 模块→文件映射 **8/8 正确**；“仓库文本≠合规发行工件”的表述**准确、未过度声明**；存在 3 处措辞层面的 P2 精度问题 |
| `README.md` 状态行 | 已不再声称 #9 待验收，与 `:46`/`:47` 一致（**R-17 已修**） |
| 但 `docs/m1-acceptance-issue11.md:§8` | **未随本 PR 更新**，现与树状态直接矛盾（P2，见 §6.1） |
| **R-08（源码范围，受限 M1）** | **可判定关闭** |
| **R-08（二进制发行范围）** | **仍未关闭**，且按现有 §8 清单执行仍会产出不合规发行包 |

## 2. 新增文件逐字节核验

命令形如 `cmp -s docs/licenses/<dst> "<GOMODCACHE>/<module>@<ver>/<src>"`，`<GOMODCACHE>` 由 `go env GOMODCACHE` 取得（不在本文写出主机路径）。

| `docs/licenses/` 目标文件 | 模块缓存源 | 字节 | `cmp` |
|---|---|---|---|
| `modernc-libc-LICENSE-3RD-PARTY` | `modernc.org/libc@v1.77.1/LICENSE-3RD-PARTY.md` | 10505 | **identical** |
| `modernc-sqlite-LICENSE-3RD-PARTY` | `modernc.org/sqlite@v1.60.1/LICENSE-3RD-PARTY.md` | 74323 | **identical** |
| `modernc-sqlite-LICENSE-SQLITE` | `modernc.org/sqlite@v1.60.1/LICENSE-SQLITE` | 1506 | **identical** |
| `modernc-sqlite-LICENSE-SQLITE_VEC` | `modernc.org/sqlite@v1.60.1/LICENSE-SQLITE_VEC` | 1068 | **identical** |
| `modernc-memory-LICENSE-GO` | `modernc.org/memory@v1.12.1/LICENSE-GO` | 1479 | **identical** |
| `modernc-memory-LICENSE-MMAP-GO` | `modernc.org/memory@v1.12.1/LICENSE-MMAP-GO` | 1518 | **identical** |
| `modernc-memory-LICENSE-LOGO` | `modernc.org/memory@v1.12.1/LICENSE-LOGO` | 62 | **identical** |
| `x-sys-PATENTS` | `golang.org/x/sys@v0.48.0/PATENTS` | 1303 | **identical** |

**mismatch 列表：空。** 各文件字节数与 `issue11-rereview-2026-10-19.md` §5.2 表中记载完全一致。`docs/licenses/` 现共 **20 份**（12 份模块顶层 + 8 份内嵌），12 份顶层文本本轮未改动（`git diff --stat` 仅新增 8 份）。

## 3. 独立枚举：被链接模块里还有什么第三方文本

### 3.1 链接集合（独立测得）

`go list -m all` = 27 个模块；`go list -deps ./cmd/maskriver` 在 linux/amd64 下命中的第三方 Go 模块为 **10** 个（`edwards25519`、`go-humanize`、`google/uuid`、`go-sql-driver/mysql`、`bigfft`、`golang.org/x/sys`、`modernc.org/{libc,mathutil,memory,sqlite}`）；`GOOS=windows`、`GOOS=darwin` 另出现 `go-isatty`、`go-strftime`。故 `attribution.md:34-35` “编译或链接的 12 个 `go.mod` 依赖”是**各目标平台并集**，与 `go.mod` 的 12 条 `require` 一致，表述成立（与上一轮 §5.1 的补充事实一致）。

### 3.2 独立基准：上游自带清单

`modernc.org/sqlite@v1.60.1/SBOM.md` 与 `LICENSE-3RD-PARTY.md` 是上游用生成器产出的扁平化清单，明确区分 “linked into your program / test-only / module graph only”。其 “1. Components linked into your program” 一节列出的组件为：`go-humanize`(MIT)、`google/uuid`(BSD)、`bigfft`(BSD)、Go(BSD)、`go-netdb`(MIT)、`golang.org/x/sys`(BSD)、`modernc.org/libc`(BSD)、`modernc.org/mathutil`(BSD)、`modernc.org/memory`(BSD + attribution + mmap-go)、musl libc(MIT)、`NixOS/nixpkgs`(MIT)、SQLite(public domain)、`sqlite-vec`(MIT，仅 import `vec` 时)、`test_demovfs.c`(public domain，仅 import `vfs` 时)。

逐项对照 `docs/licenses/`：

- Go / go-netdb / musl / nixpkgs → 落在 `modernc-libc-LICENSE-3RD-PARTY`（其内部章节为 `## Go`、`## musl libc`、`## go-netdb`、`## NixOS/nixpkgs`），**已捕获**；
- SQLite public domain → `modernc-sqlite-LICENSE-SQLITE`（亦是 sqlite 3RD-PARTY 的 L7），**已捕获**；
- memory 的 attribution reference / mmap-go → L5/L6，与 `modernc-memory-LICENSE-LOGO`、`modernc-memory-LICENSE-MMAP-GO` 内容对应，**已捕获**；
- `x/sys` 的 `PATENTS`：上游 SBOM **未**单列（上游只给 `LICENSE`），本仓库额外捕获，属**超出上游**的正确补强。

**结论：在 linux/amd64（及 windows/darwin）默认构建中，凡被编译进二进制的第三方材料，均有对应文本在 `docs/licenses/`。** 以模块缓存全量 `find`（任意深度、排除 `testdata`/`docs`/`sponsors`/`vendor_libs`）复核，未发现其他带许可条款的遗漏文本。

### 3.3 仍**未**收录、但**无分发义务**的文本（不作为 R-08 缺陷）

| 文件 | 字节 | 为何无义务 |
|---|---|---|
| `modernc.org/mathutil@v1.7.1/mersenne/LICENSE` | 1486 | `go list -deps` 中**无** `modernc.org/mathutil/mersenne`；`mathutil` 父包 `rnd.go` 版权头为 “The mathutil Authors”，未从 mersenne 派生。该文本与父包 LICENSE 仅差版权人一行（“The mersenne Authors”）。包未编译 → 无二进制声明义务。**注意**：上一轮 §5.2 该行以“mathutil 已链接”为由列出它，属**上一轮的口径放宽**，本 PR 未收录是正确的。 |
| `modernc.org/libc@v1.77.1/testdata/nsz.repo.hu/libc-test/COPYRIGHT`（1314）<br>`.../src/math/crlibm/COPYING`（18007） | 1314 / 18007 | `testdata/` 目录不参与 `go build`；且 MaskRiver 不重分发模块源码。 |
| 各模块 `AUTHORS` / `CONTRIBUTORS`（mysql、libc、mathutil、memory、sqlite、uuid） | 105–5557 | 仅为版权归属人名列表，不含授权条款；BSD-3 的义务由 `LICENSE` 原文满足，MPL-2.0 亦不要求随二进制分发 AUTHORS。 |

**建议**（非阻断）：可在 `attribution.md` 新增一句说明 “未链接的子包（如 `mathutil/mersenne`、`sqlite/vec`、`sqlite/vfs`）的内嵌文本未收录”，以免下一轮复核重复提出同一问题。

## 4. `docs/attribution.md` 新增章节评价（`:41-68`）

**映射正确性**：8 行全部指向真实存在的文件，模块归属正确（libc/sqlite/memory/x-sys 各自对应本模块内的文件），无跨模块错配、无指向不存在路径的行。`modernc-sqlite-LICENSE-3RD-PARTY` 内部已内嵌 libc 的 `N1` 附录（即 libc 的 `LICENSE-3RD-PARTY.md` 原文），与 `modernc-libc-LICENSE-3RD-PARTY` 内容重复，但这只导致 NOTICE 归并时的冗余，不构成错误。

**“仓库文本不是合规发行工件”的表述（`:59-64`）**：**准确**。它明确区分了三件事——仓库文本、随包 NOTICE 归并、MPL-2.0 §3.2 的对应源码获取途径——并写明 “This repository documents the material; it does not package it”，**没有**声称合规已完成。与上一轮 §5.3 的要求方向一致。

**P2 精度问题（不阻断关闭）**：

1. `:43-46` 的引导句断言表中材料 “is compiled into the binary”，但 `:53` 的 `modernc-sqlite-LICENSE-SQLITE_VEC` 覆盖 `modernc.org/sqlite/vec`，而 `go list -deps` 中**无** `vec` 包（二进制无 `sqlite_vec` 符号）。属**过度披露**（方向无害）但措辞不准。
2. `:55` 拼写 “mmmap-go” → 应为 “mmap-go”。
3. `:56` 称 `LICENSE-LOGO` 为 “project logo terms”，实际文件只有一行 Wikimedia URL（62 B），无任何条款；上游 SBOM 称其为 “attribution reference”。建议沿用 “attribution reference / logo attribution pointer”。

## 5. `README.md` 状态行与 `:46` 一致性

- `README.md:10` 已改为 “#9 已关闭（AC1–AC5 全部通过）；#11 已裁定 ACCEPT WITH RESTRICTIONS，尚有限制项待关闭。M1 未发布。” **不再声称 #9 待验收**，上一轮 R-17（P2）针对的 `:10` vs `:46` 矛盾**已消除**。
- `:46`（M1-C 条目）称 “#9 的 AC 已由独立上下文签署”，`:47`（M1-D 条目）称 “#11 已裁定 ACCEPT WITH RESTRICTIONS”，与 `:10` 三者自洽。**一致性成立。**
- 但 `:47` 仍写 “`docs/licenses/` 已有 **12 份**文本但仍需发布时的 NOTICE/发行包载体与**内置文本核查**”，并写 “P0=0、**P1=0**”。本 PR 已把内置文本收录、且上一轮复核把 R-08 定为 P1 → 该行**已过时**。见 §6.2。

## 6. 本 PR 未更新的文档：与树状态直接矛盾

### 6.1 `docs/m1-acceptance-issue11.md:§8`（`docs/m1-acceptance-issue11.md:137-150`）—— **P2**

本 PR 未触碰该文件，导致：

| 行 | 现文本 | 与树状态 |
|---|---|---|
| `:141` | “`docs/licenses/` 现存 **12** 份文本” | **错**：现为 20 份（12 顶层 + 8 内嵌） |
| `:147` | “**内置第三方声明未捕获**：… `docs/licenses/` **未包含**该文件” | **错**：`modernc-libc-LICENSE-3RD-PARTY` 已存在 |
| `:144-150` | 发布前必须核查项只有 3 条，其中内置声明只提 libc 一份 | **仍不完整**：未列 `modernc-sqlite-LICENSE-3RD-PARTY`、`LICENSE-SQLITE`、memory 三份、`x-sys-PATENTS` |
| `:170` | “`docs/licenses/`（12 份原文）” | **错**：20 份 |

这正是上一轮把 R-08 定为 P1 的**控制面理由**（“按该清单执行仍会产出不合规的发行包”）。本 PR 修复了**材料**，但没有修复**清单**，并新引入 2 处直接为假的陈述。

严重度校准：仓库内已有先例——R-17（`README.md:10` 与 #9 CLOSED 矛盾）被本仓库自己的评审定为 **P2（文档矛盾）**，处置为 “修正前不得把该处当作状态来源”。本条与之同类，因此定 **P2**，而非 P1：当前树**没有**任何发行工件，清单缺陷不产生实际的许可违规；但它**阻塞二进制发行**（见 §8）。

### 6.2 `README.md:47` —— **P2**

“12 份文本 / 内置文本核查（待做）/ P1=0” 三处均与树状态及上一轮 R-08 定级不符。本 PR 只改了 `:10`。

## 7. 泄露与主机路径检查

- `docs/licenses/` 8 份新增文件：**无**主机绝对路径（`/home/`、`/tmp/`、`.gvm`、`pkg/mod` 均无匹配）、无凭证、无 DSN。
- 命中邮箱/URL 的行全部是上游版权人邮箱与许可正文自带链接（如 `edsrzf@gmail.com`、`commons.wikimedia.org`），属许可原文，必须保留，不算泄露。
- `docs/attribution.md`、`README.md`：无主机路径；`README.md` 中的 `haoyuehx` 为仓库所有者 GitHub 用户名（既有内容，非本 PR 引入）。
- 工作区干净，暂存区为空。

## 8. 门禁

| 命令 | 结果 |
|---|---|
| `git diff --check` | 退出码 0，无输出 |
| `gofmt -l internal cmd pkg tools` | 退出码 0，**无输出**（无未格式化文件） |
| `go test -count=1 ./...` | **退出码 0**：10 个包 `ok`（`cmd/maskriver`、`internal/{config,db,detect,history,mask,runner,testenv,verify}`、`pkg/contracts`），`tools/fixture` 无测试文件 |

本 PR 未改 Go 源码，门禁仅作为“未引入回归”的证据。

## 9. 源码范围 vs 二进制范围

### 9.1 源码范围受限 M1：**可判定关闭**

理由：(a) 凡被编译/链接模块的内嵌第三方文本**已全部存在且逐字节一致**，并以独立基准（上游扁平化 SBOM）交叉验证无遗漏；(b) 无任何二进制工件被发布，故不存在“不合规发行包”这一对象；(c) `attribution.md:59-64` 已明确声明仓库文本不是发行工件、未宣称合规。R-08 的原始风险（“被编译进二进制的 MIT/BSD 代码缺少必须随附的声明”）在仓库层面**已经消除**。

### 9.2 二进制发行：**仍未关闭**，尚需

1. **随包 NOTICE 归并**：12 份顶层文本 + `modernc-libc-LICENSE-3RD-PARTY`（Go/musl/go-netdb/nixpkgs）+ `modernc-sqlite-LICENSE-3RD-PARTY` + `modernc-sqlite-LICENSE-SQLITE` + memory 的 GO/MMAP-GO/LOGO + `x-sys-PATENTS`。若 import `sqlite/vec` 才需追加 `LICENSE-SQLITE_VEC`。需决定是原样并列还是去重（sqlite 3RD-PARTY 已内嵌 libc 的 N1）。
2. **MPL-2.0 §3.2 对应源码获取途径**：`github.com/go-sql-driver/mysql` v1.10.1 的版本号 + 公开获取地址，必须写进**随包材料**，而不只是仓库文档。
3. **修订 `docs/m1-acceptance-issue11.md:§8`**：列出全部适用文本、删除 “12 份 / 未捕获” 的假陈述，并把逐项核查表与 `attribution.md` 的 8 行对齐。**此条完成前，§8 不能作为发行前清单使用**，否则 R-08 的 P1 理由会原样复现。
4. **逐次发布的留证**：核对人、被核对文件、SHA-256；不得以“仓库有此目录”代替。
5. 若采用附源码的发行形式，需保证所附源码与二进制 exact version 对应（`go.sum` 已固定，可支持）。

## 10. 问题分级

| 级别 | 编号 | 内容 | 位置 |
|---|---|---|---|
| **P0** | — | 无 | — |
| **P1** | — | 无（R-08 的材料面已关闭；见 §9.1） | — |
| **P2** | N-1 | `§8` 未更新，现存 “12 份 / 未捕获” 两处**假陈述**，且 3 条清单仍未覆盖 8 份新增文本；修复前不得作为二进制发行前清单 | `docs/m1-acceptance-issue11.md:141,147,150,170` |
| **P2** | N-2 | `README.md:47` “12 份文本 / 内置文本核查待做 / P1=0” 已过时 | `README.md:47` |
| **P2** | N-3 | 引导句称表中材料 “compiled into the binary”，但其中 `LICENSE-SQLITE_VEC` 对应的 `vec` 包未被链接 | `docs/attribution.md:43-46,53` |
| **P2** | N-4 | 拼写 “mmmap-go” | `docs/attribution.md:55` |
| **P2** | N-5 | 称 `LICENSE-LOGO` 为 “project logo terms”，实际为一行 URL，无条款 | `docs/attribution.md:56` |
| P3 | N-6 | 未说明“未链接子包（`mathutil/mersenne`、`sqlite/vec`、`sqlite/vfs`）的内嵌文本不收录”，易被下一轮重复提出 | `docs/attribution.md:48-57` |

**P0/P1 = 0。** 上述 P2 均不构成许可违规（无发行工件），但 N-1 在二进制发行前**必须**修复。

## 11. 可合并性

PR #60 的**材料补全**部分正确、可验证、无泄露、门禁全绿，且我未发现任何字节不一致。N-1…N-5 是**同一 PR 应当收尾的文档一致性问题**（本 PR 已打开 `attribution.md` 与 `README.md`，`§8` 是同一主题的对应文档）。按“R-08 关闭需要控制面一致”的标准，建议 **N-1 + N-2 在本 PR 内一并修复**后合并；仅 `attribution.md` 的 N-3/N-4/N-5 属措辞，可与 N-1 同批处理。

若决定先合并材料、另开文档修复 PR，则 R-08 只能按“源码范围的受限 M1”关闭，且**二进制发行仍明确禁止**。

**判定的适用范围**：本节结论仅覆盖**源码范围的受限 M1（不发布任何二进制工件）**。二进制发行范围**仍未关闭**，关闭条件见 §9.2 第 1–5 项。

R-08 verdict: CLOSED
