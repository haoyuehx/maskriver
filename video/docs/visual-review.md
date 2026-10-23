<!--
Persisted review artifact.

Reviewer: GLM-5.3-Flash (image input enabled, independent context)
Reason this model and not GLM-5.3: the requested reviewer model `opencode-go/glm-5.3`
advertises `input: ["text"]` in this deployment — it cannot receive images. A probe
(reading a rendered PNG) returned "[Current model does not support images. The image
will be omitted from this request.]". Its review is therefore recorded separately as a
text-only consistency review, and the image-based review below was executed by the
image-capable sibling model so that no claim of "visual review" is unsupported.

FINAL STATUS (2026-10-23): render #4 is the delivered cut. The independent image review
at the end of this file records 85/100, PASS, with no P0/P1. The four P2 items remain
accepted, non-blocking. The earlier Round 3 limitation note below is retained as history
and is explicitly superseded by Round 4.

Round 1 reviewed the FIRST full render (render #1, 260.05s). Fixes applied afterwards
are listed in section "Round 1 处置". A second, independent image review of the final
render is recorded in the same file after that section.
-->

# MaskRiver 产品片 · 视觉 QA / 创意评审报告

- 评审对象：`out/maskriver-demo-new.mp4`（260.05s · 1920×1080 · 30fps · 7800 帧）
- 评审模型：GLM-5.3-Flash（图像输入，独立上下文）
- 评审日期：2026-10-23（文件首轮记录；最终交付以本文件末尾 Round 4 为准）

## 实际打开的证据文件

逐张查看的图像（共 25 张）：
- `video/out/review-full/contact-sheet.png`（24 帧标注缩略总表）
- `video/out/review-full/t3s.png`, `t12s.png`, `t20s.png`, `t25s.png`, `t30s.png`, `t45s.png`, `t60s.png`, `t65s.png`, `t72s.png`, `t85s.png`, `t100s.png`, `t115s.png`, `t130s.png`, `t145s.png`, `t150s.png`, `t155s.png`, `t170s.png`, `t185s.png`, `t200s.png`, `t215s.png`, `t230s.png`, `t245s.png`, `t252s.png`, `t258s.png`

参考文件：
- `video/out/review-full/frame-stats.json`（逐帧均值/方差）
- `video/src/data/script.ts`（字幕时间轴单一事实源）
- `video/docs/storyboard.md`（分镜与节拍表）

未打开：`video/docs/visual-direction.md`（未读取，不影响以下结论）。未运行任何渲染命令。

## 评分

| 维度 | 满分 | 得分 |
|---|---|---|
| 整体专业质感 | 20 | 16 |
| 构图与视觉层次 | 20 | 15 |
| 字体与排版 | 15 | 12 |
| 动画节奏 | 15 | 11 |
| 色彩统一 | 10 | 9 |
| 创意与辨识度 | 10 | 8 |
| 信息可读性 | 10 | 8 |
| **总分** | **100** | **79** |

**结论：79 / 100，低于 85 分通过线，且存在 2 项 P1 → 当前版本不通过，需修改后再交付。**
（动画节奏仅能由静帧 + frame-stats + storyboard 节拍表间接推断，无法逐帧判断运动质量；已按保守口径计分。）

## Findings

### P1（交付前必须修复）

**P1-1 · 字幕节拍与画面系统性错位（约一个节拍）**
- 时间戳 / 帧：0:45.0（t45s.png）；1:55.0（t115s.png）；2:10.0（t130s.png）；2:25.0（t145s.png）
- 屏幕问题：0:45.0 屏幕显示 APPROVE「签名审批」Ed25519 审批记录卡，字幕却是「第二步，预览：先给出处理计划…」；1:55.0 屏幕高亮「05 执行」并展示 `--approved/--apply ok`，字幕仍是「审批把计划摘要签进 Ed25519…」；2:10.0 屏幕显示「06 验证」清单，字幕却是「授权之后，敏感值按批次替换…」；2:25.0 屏幕已是收束页「从扫描到验证，全程留痕」，字幕仍是「最后是严格验证…」。
- 证据：对照 `script.ts` 与 `storyboard.md`：Scene 3 CH5 执行起于场景帧 1440 = 全局 113.0s，而 cue 16（审批）持续到 116.6s；CH6 验证起于 128.3s，cue 18（执行）持续到 131.6s。画面节拍比字幕快约 2–3.5 秒，五个关键步骤中有四个出现"画面是第 N+1 步、旁白在讲第 N 步"。对一部以五步工作流为核心叙事的影片，步骤与解说错位会直接误导评委。
- 修复建议（Remotion/TS 工程师）：二选一并保持一致——(a) 将 Scene 3 各章节起始帧按字幕 cue 右移约 80–105 帧（例如 CH5 从 1440 移至 ≥1540），或 (b) 将 `CUES` 中 Scene 2/3 的每条 cue 结束时间提前到对应画面节拍开始前 ≤1s（如 cue 16 end 改 112.5）。修复后抽验 t=42s/45s、113s/115s、129s/131s、143s/145s 四组帧，要求字幕主题与屏上章节一致。

**P1-2 · 邮箱列数值被逐字符硬裁切、无省略号**
- 时间戳 / 帧：1:25.0（t85s.png）；1:40.0（t100s.png）；1:55.0（t115s.png）
- 屏幕问题：扫描视图 email 列显示 `lin.xiao@example.te`、`zhao.ming@example.t`；预览视图 SOURCE 列 `chen.yu@example.1`、PLANNED 列 `l***@example.tes!`（末字符被裁成 "!"）；执行视图 `wu.tong@example.tes`。值在单元格边缘被直接截断，无 `…`。本片的核心卖点之一是 SOURCE/PLANNED 对照的"确定性脱敏"，而被截断的值使对照无法读全，观感是渲染 bug 而非设计。
- 修复建议：概念工作区表格中 email 列宽度不足以容纳 22 字符的 `lin.xiao@example.test`。任选其一：(a) 将 workspace 表格 mono 字号由当前尺寸下调 1–2px 或将 email 列宽增加约 90–110px；(b) 对单元格套用 `overflow:hidden; text-overflow:ellipsis; white-space:nowrap`，保证截断处出现 `…` 而非半个字符。修复后检查 t85/t100/t115 三帧 SOURCE 与 PLANNED 两列均可读全。

### P2（建议改进）

**P2-1 · 场景转场中点出现整幅空白帧**
- 1:05.0（t65s.png）、3:20.0（t200s.png）、4:05.0（t245s.png）：整幅画面只剩背景渐变，无标题、无角标、无字幕（frame-stats stdev 3.2/5.0/3.4 为全片最低）；0:25.0（t25s.png）与 2:30.0（t150s.png）仅有一根细线。每处约 1 秒。五次转场三次读作"死屏"。
- 修复：转场中点叠加下一场景的章节编号/眉标（如「05 RECOVERY」淡入），或将交叉淡化缩短到 ≤0.5s，避免中点双透明。

**P2-2 · 1:00.0（t60s.png）标题字帧：下部 2/3 全空**
- 画面仅有「五步闭环」标题 + 五枚步骤 chip（VALIDATE 高亮），正文区完全空白，而该 cue（57.2–64.6s）长达 7.4s；storyboard B8 的"三张指标卡"进场太晚。修复：指标卡进场提前到场景帧 ≈1060，或在标题下方保留五步链的 hero 形态。
- 同类问题（下沿大面积留白）：0:45.0（t45s.png，内容压在上半部，下方约 45% 空）、2:25.0（t145s.png，大面板内仅 6 枚 chip）、2:35.0（t155s.png，仅标题 + 一行 BATCH QUEUE）。修复方向：将内容块垂直居中或放大主卡片，保持 8:3 的内容/留白比。

**P2-3 · 0:03.0（t3s.png）空表帧**
- 表格只有表头（id/name/email/mobile/city），表体全空。属进场动画起点可接受，但请确保首行数据在 ~1s 内进场，避免评委第一印象是"空表格"。

**P2-4 · 3:50.0（t230s.png）「TASK INTERRUPTED」徽标挤压**
- 红色徽标叠在 BATCH 01 卡片下边框上，且其下缘与横向红色警示线间距不足数像素；红线下方的「批次事务提交时…」说明行被压暗。修复：徽标与红条间留 ≥12px，或将徽标移到红条下方独立行。

**P2-5 · Scene 2 数据卡缺少「流程示意 · 合成数据」角标**
- 0:30.0（t30s.png）、0:45.0（t45s.png）、1:00.0（t60s.png）右上角无任何角标，而 0:45 画面包含 plan digest（sha256:9f2c…a41b）与审批记录等"数据卡"。storyboard 规定"出现数据表/概念工作区的镜头右上角恒显「流程示意 · 合成数据」"。建议将角标扩展到 Scene 2 的 digest/审批卡镜头，保持全片一致性。

## 专项判定（按要求逐项）

- **场景入场空帧**：0:25 / 1:05 / 2:30 / 3:20 / 4:05 五个转场中点均为近空帧（见 P2-1）；1:00 为标题字空帧（P2-2）。属节拍设计+采样命中，但三次完全空白超出必要。
- **文字重叠**：未发现标题/正文/字幕相互重叠。唯一挤压是 3:50 的 TASK INTERRUPTED 徽标（P2-4）。
- **1920×1080 表格可读性**：概念工作区表格（t72s）行高、字号、列距良好，1080p 全部可读；但 email 列在 t85/t100/t115 被硬裁切（P1-2），是唯一的可读性硬伤。
- **字幕与内容碰撞**：未发生。底部字幕胶囊稳定在 y≈985–1035，内容最深到 y≈830（t230s），留有约 150px 安全边。
- **是否读作产品片而非 PPT**：总体是。Scene 3 概念工作区（章节轨 + 六态视图）、Scene 4 流水线 meter、Scene 5 中断/恢复都是 UI 叙事，不是列表页轮播。最接近 PPT 感的是 Scene 2（0:30–1:05）：五张静态卡 + 大量留白（t45s、t60s），建议给五步链增加持续动效或数据 token 流。
- **色彩统一**：优秀。深藏青底 + 青（SENSITIVE/PASS）、琥珀（REVIEW/UNKNOWN/INTERRUPTED）、紫（accent）三语义色贯穿 0:03–4:18，无越界用色；End frame 的蓝紫渐变 wordmark 与整体协调。
- **仅凭画面能否读懂工作流故事**：基本能。五步 chip、六章节轨、BATCH 01–06 状态卡、"3/6 批次已提交"、`--apply --resume` 终端都自解释；但 P1-1 的字幕错位会让"边看边听"的观众接收到互相矛盾的步骤编号，必须修复。

## What works

- 一套完整、可辨识的深色"概念工作区"UI（t72–t145）：表列表 + 章节轨 + read only / write session · gated 状态栏，是全片辨识度核心。
- 语义色系统严格一致：UNKNOWN≠安全 用琥珀表达（t85s「UNKNOWN 不等于安全」、t100s「确定性…不等于匿名化」），与产品契约口径一致。
- 3:30–3:55 的中断—恢复叙事（TASK INTERRUPTED 红条 → 只读 status 卡 → `--resume` 终端 → skip 3 batches）画面语言清晰、有戏剧性。
- 五步与六章节的进度指示（步骤 chip、章节 tab、BATCH 状态卡 + 3/6 计数）让观众随时知道走到了哪一步。
- 合规细节到位：合成数据全部使用 `example.test` / `199 0000 xxxx`；工作区镜头恒显「流程示意 · 合成数据」；片尾有「演示使用合成数据 · 界面为设计示意」；全片未见版本号。
- 字幕胶囊样式统一、对比充足、位置安全，静帧内未与任何内容碰撞。

---

## Round 1 处置（Flash 实现者，按 P0/P1 优先修复）

| 编号 | 结论 | 处理方式 | 复核方式 |
|---|---|---|---|
| P1-1 字幕节拍与画面错位 | **已修复** | 重写 Scene 2 的旁白 cue，使其对齐画面五步节拍（30/36/42/48/54s）；重写 Scene 3 六个章节窗口，使章节切换与旁白 cue 一一对应（数据源 66.2s / 扫描 72.6s / 预览 87.0s / 审批 109.2s / 执行 124.2s / 验证 140.2s）；`subtitles/zh-CN.srt` 由同一份 `src/data/script.ts` 重新生成 | 抽帧 45s/115s/130s 复核章节高亮与字幕主题一致 |
| P1-2 邮箱列硬裁切 | **已修复** | 合成邮箱缩短为 `lin@example.test` 等 17 字符；`DataTable` / `MiniTable` 增加 `text-overflow: ellipsis`，避免半个字符 | 抽帧 85s/100s/115s 复核 SOURCE/PLANNED 可读 |
| P2-1 转场中点空白帧 | **已修复** | 六个场景的入场延迟从 16–20 帧压缩到 1–2 帧，眉标/标题/主视觉在场景第 1 帧即开始进场；出画淡出缩短到 14–26 帧 | 抽帧 25s/65s/150s/200s/245s 复核非空帧 |
| P2-2 1:00 下部空白 | **已修复** | Scene 2 recap 指标卡进场从 1100/1110/1120 提前到 1040/1052/1064；recap 相位起点从 1050 提前到 1030 | 抽帧 60s |
| P2-3 0:03 空表 | **已修复** | Scene 1 表格进场 78→34 帧，首行 56 帧进场 | 抽帧 3s |
| P2-4 TASK INTERRUPTED 挤压 | **已修复** | 危险横条移到 y=452，徽标移到横条下方 y=486 并加背景胶囊；运行态说明行在故障时淡出；右侧新增「RECOVERY PREFLIGHT · read only」四项只读检查 | 抽帧 200s/215s/230s |
| P2-5 Scene 2 缺少免责角标 | **已修复** | Scene 2 右上角常显「流程示意 · 合成数据」 | 抽帧 30s/45s/60s |
| P2-6（评审意见）Scene 2 PPT 感 | **部分改善** | 步骤条加入持续流动的 token；审批相位移除穿过卡片的杂线，改为「trust boundary」三张说明卡；五步链 hero 形态仍保留 | 抽帧 600/760 帧 |

> 说明：评审报告的「动画节奏」一项只能由静帧与 frame-stats 间接推断，不代表逐帧运动质量评估。
> 音轨在 Round 1 时尚未生成；最终渲染已加入本地 TTS 旁白（见 `docs/qc-report.md`）。

---

## Round 3 — render #3 独立视觉复核（GLM-5.3-Flash，图像输入）

**说明**：`docs/visual-review.md` 中实际没有编号为 N-1..N-4 的 "Round 2" 章节——Round 2 的两条 P1（PREFLIGHT 面板被裁、游离数字 780）仅记录在 `docs/progress.md` 渲染轮次表中。本轮按任务给出的修复声明清单逐条用图像复核，编号沿用任务口径。此为文档一致性备注，非视频缺陷。

**实际打开的图像（27 张）**：`docs/evidence/contact-sheet-final.png` + 全部 26 张全分辨率帧（003s/012s/020s/023s/031s/038s/044s/051s/057s/062s/070s/082s/096s/114s/130s/144s/156s/176s/188s/195s/207s/224s/236s/243s/249s/256s）；另核对 `out/qc-report.json`、`src/data/script.ts`、`subtitles/zh-CN.srt`。QC JSON 与声明一致（260.054s / 1920×1080 / 30fps / h264+aac / 无黑帧无冻结段，sha256 27c51947…）。

### 逐项修复声明复核

| 声明 | 判定 | 证据帧 | 实际所见 |
|---|---|---|---|
| N-1 PREFLIGHT 改 2×2 紧凑布局、卡下移、删冗余 chip、故障遮罩撤除 | **PASS** | 224s / 236s / 243s | 224s：RECOVERY PREFLIGHT 为右侧紧凑 2×2 四项（Checkpoint 可读 / Progress 前缀连续 / Outbox 回执匹配 / Manifest 校验和一致），y≈530–670，完整在画布内无裁切；画面中已无「已提交批次不重写 / 回执与检查点同事务 / 冲突时拒绝继续」三联 chip 行；无遮罩压暗。236s：resume 终端卡（`maskriver mask --streaming --apply --resume --run-id <32-hex>` 等 2 行）位于 y≈730–870，完整可读。243s：VALIDATION PASSED 卡位于右侧 y≈730–905，与居中字幕胶囊（x≈575–1350）无碰撞 |
| N-2 移除泄漏的调试节点「780」 | **PASS** | 051s（+全 26 帧 + contact sheet） | 051s 画布原点区域干净，无游离数字；逐张检查 26 帧，未发现任何游离帧号/调试文本残留 |
| N-3 SCAN 相位样本行提前 32 帧进场、面板移至 y=318 | **PASS** | 031s | SCAN 视图已完整呈现：章节头 + 五 chip（SCAN 高亮）+ 标题「先识别，再决定」+ 两个面板顶边 y≈318，SAMPLE 面板已有 199 0000 0011/0012/0013 三行样本进场，无迟到空白相 |
| P2-1 残留：邮箱缩至 17 字符、SOURCE 全读 | **PASS（含保留项）** | 038s / 051s / 096s | 038s、051s：SOURCE 列完整显示 `lin@example.test` / `chen@example.test` / `zhao@example.test`，PLANNED 列 `l***@example.test` 等同样完整，无半字符、无"!"截断。**保留项**：096s 的 Scene 3 迷你表列宽更窄，SOURCE 仍截为 `lin@example.te…`——是规范的省略号截断（无半字符），但未做到声明所称"全读"，见 P2-A |

### 字幕/画面节拍对齐（7 个抽验点，对照 `script.ts` / `zh-CN.srt`）

| 时间 | 屏上高亮 | 字幕（cue / 时间窗） | 判定 |
|---|---|---|---|
| 044s | APPROVE chip 高亮 + Ed25519 审批记录卡（plan digest sha256:9f2c…a41b） | cue 7「第三步，审批：Ed25519 记录绑定计划摘要，签名不符就无法执行。」(42.4–47.6) | ✓ |
| 070s | 章节「01 数据源」高亮 + people 合成数据表 | cue 11「下面走一遍完整流程，左边是一份合成的数据源。」(66.2–72.0) | ✓ |
| 082s | 章节「02 扫描」高亮 + 字段判定面板（city=UNKNOWN 上下文不足） | cue 13「证据不足的字段标记为需核对，而不是当作安全。」(79.6–86.4) | ✓ |
| 096s | 章节「03 预览」高亮 + plan / DRY RUN · WRITES = 0 | cue 15「计划之外的数据一律不动，这是范围约束。」(94.2–100.6) | ✓ |
| 114s | 章节「04 审批」高亮 + ED25519 APPROVAL RECORD ·PASS | cue 17「审批把计划摘要签进 Ed25519 记录，执行侧只加载公钥。」(109.2–116.6) | ✓ |
| 130s | 章节「05 执行」高亮 + apply · batch transaction（值列已换为渐变遮蔽条） | cue 19「授权之后，敏感值按批次替换，非计划字段保持不变。」(124.2–131.6) | ✓ |
| 144s | 章节「06 验证」高亮 + validate · strict 五项全 PASS | cue 21「最后是严格验证：行数、主键集合、Schema 与计划字段。」(140.2–148.6) | ✓ |

7/7 全部对齐，Round 1 P1-1 的系统性错位无复发。

### 回归检查（26 帧全量）

- **无 P0**：无文字重叠、无裁切卡片、无内容侵入字幕带（最深内容为 243s VALIDATION PASSED 卡下缘 ≈y905，在 y912 线之上；字幕胶囊稳定于 y≈950–1035）。
- **无 illegible 文本**：1080p 下全部 mono 标签、终端行、chip 可读。
- **空帧**：无 >45% 无理由空帧。转场中点（25s/65s/150s 附近）在 contact sheet 中均已有眉标/内容进场，Round 1 P2-1 的"死屏"未复发。
- 03s 首行数据已进场（P2-3 维持修复）；230s 帧（contact sheet 内）TASK INTERRUPTED 徽标与红条间距正常，无挤压复发。

### 评分

| 维度 | 满分 | 得分 |
|---|---|---|
| 整体专业质感 | 20 | 17 |
| 构图与视觉层次 | 20 | 16 |
| 字体与排版 | 15 | 13 |
| 动画节奏 | 15 | 13 |
| 色彩统一 | 10 | 9 |
| 创意与辨识度 | 10 | 8 |
| 信息可读性 | 10 | 9 |
| **总分** | **100** | **85** |

（动画节奏仍按静帧 + 节拍表间接口径保守计分。85 ≥ 85 通过线，且无 P0/P1。）

### 剩余 Findings（均非阻塞）

- **P2-A** · 096s：Scene 3 迷你表 SOURCE/PLANNED 仍省略号截断为 `lin@example.te…`。修复：迷你表 email 列宽 +约 24px 或该处字号 −1px，使 17 字符完整可读；或接受为设计截断并在文档注明。
- **P2-B** · 176s：SAME TRANSACTION 括注与说明卡已完整可见，但其对应旁白 cue 26 起于 182.2s；代码中括注进场为场景相对帧 620–668（≈170.7s，`Scene4Streaming.tsx:32`），早于专用 cue 约 11 秒。修复：进场移至相对帧 ≥960，或对齐 cue 23 窗口（158.2–165.6s）。
- **P2-C** · 207s / 249s（224s 较轻）：下半幅留白偏大（内容止于 y≈510/545，字幕前全空）。修复：内容块垂直居中或放大主卡，维持内容/留白约 8:3。
- **P2-D** · 044s：Ed25519 审批记录卡标「•UNKNOWN」，与检测语义的 UNKNOWN（证据不足）存在词汇碰撞。修复：审批未定状态改用「PENDING / 等待」，与 051s 第三卡「等待」措辞统一。

**最终判定：PASS**

---

## 审查基础设施限制与处置说明（Orchestrator 记录）

1. **模型能力**：任务指定的视觉审查模型 `opencode-go/glm-5.3` 在本部署中 `input: ["text"]`，无法接收图片。
   实测探针（读取 `out/still-060.png`）返回 `[Current model does not support images. The image will be omitted from this request.]`。
   因此三轮**图像**审查均由同族、支持图像输入的 `opencode-go/glm-5.3-flash` 在独立上下文中执行；
   GLM-5.3 另行执行纯文本的事实与一致性审查。任何"视觉确认"的结论都只来自实际读图的那一轮。
2. **轮次**：render #1 → 79/100（2×P1）；render #2 → 82/100（2×P1）；render #3 → **85/100，PASS，无 P0/P1**。
   三轮均按评审意见修改后重新渲染，未通过删除断言或跳过检查提高分数。
3. **交付一致性（历史段落，已由 Round 4 取代）**：该段写于 Round 3，当时审查的成品是
   render #3，SHA-256 为 `27c51947713888fd797c667f552822a8e1c4375e86ccf304e1d90d46c0bc1ed7`。
   最终交付是 **render #4**，SHA-256 为
   `f7b3a07b20e26dbfbd28f2087eb5cf34976b98b02a7c53d6323213334b515ff9`，见本文件末尾 Round 4
   与 `docs/evidence/qc-report.json`。Round 3 的 P2-A…P2-D 在 render #4 中仍作为已记录、
   未修复的非阻塞项保留；修复它们需要再次完整渲染并重新审查，本轮未执行。
4. **证据文件格式**：`docs/evidence/` 中的 26 张逐帧证据以 JPEG（质量 90，原始 1920×1080）保存以控制仓库体积，
   文件名与上述评审引用一致（仅扩展名由 `.png` 改为 `.jpg`）；`contact-sheet-final.png` 保持 PNG。

---

## 事实与一致性审查（GLM-5.3，纯文本；未查看任何图像）

### 已读文件

- 仓库文档：`README.md`、`docs/feature-matrix.md`、`docs/architecture.md`、`docs/guides/approval.md`、`docs/guides/streaming-recovery.md`、`docs/guides/security-model.md`
- 影片源：`video/src/data/script.ts`、`video/src/data/fixtures.ts`、`video/subtitles/zh-CN.srt`、`video/src/scenes/Scene1Problem.tsx`–`Scene6Closing.tsx`、`video/src/components/Workflow.tsx`/`Primitives.tsx`（grep）
- 影片文档：`video/docs/storyboard.md`、`narration.md`、`visual-direction.md`（全文）、`visual-review.md`（Round 1–3）、`qc-report.md`、`video/out/qc-report.json`、`video/scripts/qc-video.mjs`
- CLI 佐证：`cmd/maskriver/main.go` 及各 `*_test.go` 的真实 flag 形态

### 逐项判定

| 检查项 | 判定 | 证据 |
|---|---|---|
| 五步工作流命名（scan / dry-run 预览 / approve / apply / validate） | PASS | `script.ts:24-28`（cue 5–9）；`fixtures.ts` STEPS（Scan/Preview/Approve/Apply/Validate）与 README「Scan → Preview → Approve → Apply → Validate」一致；`Scene2Workflow.tsx` 各相位窗口（SCAN 150–330 / PREVIEW 330–510 / APPROVE 510–690 / APPLY 690–870 / VALIDATE 870–1050 场景帧）恰为 30/36/42/48/54s，与旁白节拍一一对应 |
| 三条件写入门（`--approved` + `--apply` + 有效签名记录） | PASS | `Scene2Workflow.tsx:447-459`（GateLocks 标签 `--approved` / `--apply` / `approval record`）；`Scene3Walkthrough.tsx` CH5 三 chip；与 `docs/guides/approval.md:5` 及 feature-matrix「三者缺一即拒绝」一致；flag 均真实存在（`cmd/maskriver/main.go:127-132`） |
| Ed25519 记录绑定计划摘要 | PASS | `Scene2Workflow.tsx:361`、`Scene3Walkthrough.tsx:409`（`sha256:9f2c…a41b`）+「计划一旦变化，签名即失效」；与 `approval.md:3-5`（绑定 plan/digest/strategy/scope）一致；`script.ts:26,38` |
| 执行侧只加载公钥 | PASS | `Scene2Workflow.tsx:405`「执行侧只加载公钥 / public key only」、APPROVE lead「执行侧只加载可信公钥」；`script.ts:38`；与 `approval.md:2-3` 一致 |
| 「预览零写入」证据表述 | PASS | `Scene2Workflow.tsx:312`、`Scene3Walkthrough.tsx:348`「DRY RUN · WRITES = 0」；cue 16「零写入由驱动级证据证明」对应 feature-matrix 的 SQL Spy + 文件 SHA-256 证据链 |
| 严格验证内容（行数/键集/Schema/计划字段/适配器完整性 quick_check+foreign_key_check） | PASS | `Scene2Workflow.tsx:514`「适配器完整性检查 / quick_check / foreign_key_check」五项清单；`Scene3Walkthrough.tsx:525`；与 feature-matrix strict 语义（SQLite `quick_check`+数据库级 `foreign_key_check`）逐项吻合 |
| 有界流式 + Outbox 回执仅 SQLite | PASS | `Scene4Streaming.tsx:215`「范围仅限本地 SQLite；MySQL 流式与恢复不在支持范围」；同事务括注「SAME TRANSACTION」+「批次事务 ≠ 整库原子性」；`script.ts:48`；与 README 支持范围表一致 |
| Checkpoint/Resume 语义（只读 status、跳过已提交前缀、目标漂移拒绝） | PASS | `Scene5Recovery.tsx:147`（`maskriver status --target target.db --run-id <32-hex>`，与 `main.go:523` 的真实形态一致）、`:256-257`（resume 命令与 `main.go:522` 一致；`verify committed prefix → skip 3 batches`）、`:294-296`（仅本地 SQLite / MySQL 恢复不支持 / 不宣称通用 exactly-once / 目标被外部改动时拒绝继续）；四项 PREFLIGHT 与 manifest CAS/append-only progress/回执协议吻合 |
| 豁免/边界声明（确定性 ≠ 匿名化、不承诺无碰撞） | PASS | `Scene3Walkthrough.tsx:385`；`Scene5Recovery.tsx:296`；与 security-model.md、architecture.md 口径一致 |
| 过度宣称排查（MySQL 恢复 / 通用 exactly-once / 法律匿名化 / 无碰撞 / 生产准确率 / 性能数字 / 真实 Web 后台） | PASS（无发现） | 全部为反向声明；Scene 4 指标卡为 BOUNDED/BATCH/BUDGET/OUTBOX 描述性文案，无性能数字；工作区标注 `maskriver · concept workspace` +「流程示意 · 合成数据」+「界面为设计示意」 |
| 禁止内容（版本号 / 里程碑代号 / Issue-PR / commit SHA / CI 日志 / 已交付后端） | PASS（无发现） | 逐文件 grep 无命中；`sha256:9f2c…a41b` 为省略号占位摘要且 `qc-report.md` §4 明示非真实值；`v1.0.0` 仅出现在 `video/package.json`（非屏上内容） |
| Cue 表 vs SRT vs 分镜一致性 | PASS（一处 QC 工具缺陷，见错误 2） | `script.ts` 35 条 = SRT 35 条 = `narration.md` 35 行，逐条时间抽验一致；Scene 3 六章窗口（36–228/228–660/660–1326/1326–1776/1776–2256/2256–2550）与 cue 11–21 时间窗一一对应；场景时长 750/1200/2550/1500/1350/450=7800 帧与 storyboard 及 qc-report 一致 |
| 屏上 CLI flag vs 真实 CLI | PASS | `status --target … --run-id <32-hex>`、`mask --streaming --apply --resume --run-id <32-hex>`、`--approved/--apply/--streaming/--budget-file/--receipt-page-size/--run-id/--trust-file/--history-dir/--key-file` 全部为 `cmd/maskriver/main.go` 及测试中的真实 flag；`<32-hex>` 一律占位 |
| 音频诚实性 | PASS | 无人宣称 AUDIO PASS：`narration.md`「AUDIO PENDING（未通过人工听感验收），不得宣称 AUDIO PASS」；`qc-report.md` §6.1 同口径；`qc-report.json` 仅事实性 `hasAudio: true`（AAC-LC 48kHz）；TTS 来源（Piper `zh_CN-huayan-medium`、-16 LUFS、`scripts/generate-narration.mjs` + Remotion 渲染命令）可复现。README「当前无配音」指网站侧独立静音版视频（`website/docs/video-review.md`:「无音轨」），与本片（含 AAC 旁白）是两个产物，无矛盾——但建议在文档中显式区分两个视频，避免误读 |
| 数据安全（全合成、无凭证/DSN） | PASS | `fixtures.ts` 头注与全部值：`*.example.test` 邮箱、`199 0000 00xx`、虚构姓名（林晓/陈雨/赵明/周宁/吴桐/郑一）；全 video/ 目录 grep 无 password/secret/DSN/token 凭证类字符串 |

### 事实错误（含修正措辞）

**错误 1（P1 · 事实性错误）：屏上判定态「REVIEW」是产品中不存在的状态**
- 错误原文：`{f: 'name', v: 'REVIEW' as const, note: 'cn_person_name · 83%'}`（`video/src/scenes/Scene3Walkthrough.tsx:271`）；`{field: 'name', rule: 'cn_person_name', verdict: 'REVIEW', samples: 6, hit: '83%'}`（`video/src/data/fixtures.ts:139`）；「右侧三态判定（SENSITIVE / REVIEW / UNKNOWN）」（`video/docs/storyboard.md:54`）。
- 事实依据：冻结契约的三态是 `Unknown / NotSensitive / Sensitive`（`pkg/contracts/contracts.go:125-130`），且无任何「REVIEW」判定态；`internal/detect/detect.go:135-153` 明确：命中率低于 90% 阈值（83% < 90%）即不满足 formatStrong，返回的 Decision 敏感度为零值 `Unknown`。即真实产品中该 name 字段的判定是 **UNKNOWN**，不是 REVIEW。影片自己的规范也写对了：`video/docs/visual-direction.md` §9 S3-B3「字段判定：SENSITIVE / NOT_SENSITIVE / UNKNOWN」，`video/src/components/Primitives.tsx:61` 注释同——实现与 storyboard 偏离了自家规范与产品契约。
- 修正措辞：把 Scene 3 CH2 判定卡与 `fixtures.ts` FIELD_EVIDENCE 中 name 的 chip 改为 `UNKNOWN`（note 可写「cn_person_name · 83% 低于阈值」），storyboard.md:54 改为「三态判定（SENSITIVE / NOT_SENSITIVE / UNKNOWN）」。旁白本身（cue 13「证据不足的字段标记为需核对」）无需改动。

**错误 2（P2 · QC 报告数值错误）：「末条 252.0 s」实为末条字幕的起始秒，且丢弃毫秒**
- 错误原文：`"lastCueEndSec": 252`（`video/out/qc-report.json:33`）；「外挂 …zh-CN.srt（35 条，末条 252.0 s）」（`video/docs/qc-report.md:17`）。
- 事实依据：SRT 第 35 条为 `00:04:12,400 --> 00:04:17,400`，真实末条结束时间为 **257.4 s**；`video/scripts/qc-video.mjs:71-73` 的正则 `(\d\d):(\d\d):(\d\d),(\d\d\d) --> ` 捕获的是 `-->` **前**的起始时间戳，且未计入毫秒，字段名却叫 `lastCueEndSec`。
- 修正措辞：正则改为捕获 `--> ` 之后的时间戳（含毫秒换算），或把字段改名 `lastCueStartSec`；qc-report.md 对应改为「末条 257.4 s（起始 252.4 s）」。

**错误 3（P2 · 旁白范围表述不精确）：cue 20 在非流式走查场景中宣称批次回执**
- 错误原文：「每个批次提交时写下事务回执，为中断后的判断留下依据。」（`video/src/data/script.ts:41`，132.2–139.6s，Scene 3 CH5；SRT 第 20 条）。
- 事实依据：事务性 Outbox 回执仅存在于 `--streaming` 路径（feature-matrix「SQLite 流式回执 Outbox（WP2）…显式 opt-in」；`docs/guides/streaming-recovery.md`），而 Scene 3 CH5 屏上三条件锁只显示 `--approved/--apply/approval record`，无 `--streaming`；普通 M1 apply 不写回执。紧接的 Scene 4 已正确限定范围，但该 cue 单独读会让人以为默认批次提交就有回执。
- 修正措辞：改为「（在流式路径中，）每个批次提交时同步写下事务回执，为中断后的判断留下依据。」或将该句并入 Scene 4 语境。

### 非错误备注

- `video/src/data/script.ts:74` 的 `SCENE_LABELS`（含「核心能力」与 storyboard「五步核心流程」措辞不同）是未被任何场景渲染的死导出，不影响画面事实；可顺手删除或对齐。
- 仓库现有两个演示视频（网站静音版 `website/public/media/maskriver-demo.mp4` 与本片含旁白版），README 的「当前无配音」仅描述前者，准确但建议加一句区分。

### 本审查无法判断的内容

纯文本审查，未查看任何图像或视频帧：无法判断画面视觉质量、动效节奏、屏上文字可读性/是否被裁切、免责角标是否实际可见、字幕与画面的实际渲染对齐（仅能核对源码时间轴与视觉评审报告 `visual-review.md` 的记录）；无法试听音轨的听感质量（仅能核对 qc-report 的电平与编码数据）；`out/maskriver-demo-new.mp4` 本体的实际内容以 `qc-report.json` 与视觉评审记录为据，未逐帧复核。

**结论：FAIL（文本层面）** —— 事实与一致性整体良好，但存在 1 处屏上事实性错误（虚构判定态「REVIEW」）与 2 处轻微不准确（QC「末条 252.0 s」数值/字段错位；cue 20 回执声明缺流式限定）。
**错误计数：共 3 处（1 × P1 事实错误 + 2 × P2）。**

---

## Round 4 处置（事实修正后重渲染）

GLM-5.3 的纯文本审查发现 1 处屏上事实错误与 2 处不精确，均在 render #4 中修正：

| 编号 | 结论 | 处理 |
|---|---|---|
| 错误 1（P1）「REVIEW」判定态在产品中不存在 | **已修复** | `src/data/fixtures.ts` 与 `Scene3Walkthrough` 的 name 字段判定改为 `UNKNOWN`（注：「cn_person_name · 83% 低于阈值」）；`VerdictChip` 的 `REVIEW` 类型整体删除，Scene 5 的只读标记改为 `READ ONLY` 徽标；`docs/storyboard.md` 改为「SENSITIVE / NOT_SENSITIVE / UNKNOWN」 |
| 错误 2（P2）QC 报「末条 252.0 s」 | **已修复** | `scripts/qc-video.mjs` 的正则改为捕获 `-->` 之后的结束时间并计入毫秒，现报 `lastCueEndSec = 257.4` |
| 错误 3（P2）cue 20 未限定流式路径 | **已修复** | 旁白改为「在流式路径中，每个批次提交都会写下事务回执。」并重新合成配音；SRT 同步重新生成 |
| 备注：`SCENE_LABELS` 死导出 | **已删除** | 该常量未被任何场景渲染 |

render #4 与 render #3 的差异仅限上述文案与一处徽标；场景结构、镜头、时序未变。

---

所有 26 帧全分辨率帧以及缩略图总表（contact sheet）已打开。以下是完整报告。

## 第 4 轮 — render #4 事实修正复核（GLM-5.3-Flash，图像输入）

**已打开的图像 (27)**：`docs/evidence/contact-sheet-final.png` + 全部 26 张全分辨率帧（003s/012s/020s/023s/031s/038s/044s/051s/057s/062s/070s/082s/096s/114s/130s/144s/156s/176s/188s/195s/207s/224s/236s/243s/249s/256s）。另核对 `out/qc-report.json`：sha256 = `f7b3a07b…5ff9`（与任务口径一致）、260.054s / 1920×1080 / 30fps / h264+aac、`lastCueEndSec: 257.4`（错误 2 的修正已在 QC 数据中体现）、无黑帧无冻结段。

### 核验表

| # | 检查项 | 判定 | 实际所见（帧） |
|---|---|---|---|
| 1a | 082s 判定 chip 为 UNKNOWN，非 REVIEW | **PASS** | `082s-s3-02-scan.jpg`：字段判定面板 name 行 chip 为「•UNKNOWN」，注「cn_person_name · 83% 低于阈值」；email/mobile 为「•SENSITIVE」，city 为「•UNKNOWN 上下文不足」，脚注「UNKNOWN 不等于安全」。全面板无 REVIEW 字样，文案连贯 |
| 1b | 031s 无 REVIEW 残留，UNKNOWN 语境连贯 | **PASS（带说明）** | `031s-s2-01-scan.jpg`：判定面板此时仅 email/mobile 两条「•SENSITIVE」chip 进场，加琥珀色说明行「UNKNOWN 不是安全，需要人工确认后才进入计划」；该帧本就不含 name 判定 chip（031s 为相位早段），故不存在 REVIEW，也不存在 REVIEW→UNKNOWN 的残留。UNKNOWN 语义与旁白「证据不足时保留 UNKNOWN」一致 |
| 1c | 26 帧内无任何 "REVIEW" 文本 | **PASS** | 逐张检查全部 26 帧 + contact sheet：屏上判定态仅出现 SENSITIVE / NOT_SENSITIVE（070s）/ UNKNOWN / PASS 四种，与冻结契约三态一致；REVIEW 字样零命中 |
| 2 | 224/236/243s Scene 5 只读标记为 READ ONLY 徽标，无裁切/重叠 | **PASS** | 三帧的 status 卡命令行 `maskriver status --target target.db --run-id <32-hex>` 旁均为独立「READ ONLY」胶囊徽标（琥珀描边），完整在卡内无裁切；224s TASK INTERRUPTED 徽标与红条间距正常；236s resume 终端卡（y≈730–870）完整；243s VALIDATION PASSED 卡（右侧，下缘 ≈y905）与字幕胶囊无碰撞 |
| 3 | 全片回归（重叠/裁切/y>912/可读性/杂散/无理由空帧） | **PASS** | 最深内容为 243s 卡片下缘 ≈y905，在线上；字幕胶囊稳定 y≈950–1035；1080p 下全部 mono 文本可读；无游离数字或调试残留；无 >45% 无理由空帧（20s/156s 的留白属标题/网格 hero 构图，有叙事理由） |
| 4 | 与 Round 3 相比无其他变化 | **PASS** | 26 帧逐张对照 Round 3 记录：场景结构、章节轨、镜头节拍、布局、配色、角标（流程示意·合成数据 / 技术原理·本地 SQLite / 本地 SQLite·事务恢复）完全一致；差异仅限于任务声明的四处（UNKNOWN chip、READ ONLY 徽标、cue 20 措辞、字幕重生成）。历史 P2 原样保留（见下），无新增回归 |

### 评分

| 维度 | 满分 | 得分 | 说明 |
|---|---|---|---|
| 整体专业质感 | 20 | 17 | 与 render #3 持平；事实修正后 UI 更可信 |
| 构图与视觉层次 | 20 | 16 | 持平；207s/249s 下半幅留白仍偏大 |
| 字体与排版 | 15 | 13 | 持平；096s 省略号截断保留 |
| 动画节奏 | 15 | 13 | 静帧 + 节拍表间接口径，保守计分 |
| 色彩统一 | 10 | 9 | 语义色系统一致（SENSITIVE 青 / UNKNOWN 琥珀 / NOT_SENSITIVE 灰） |
| 创意与辨识度 | 10 | 8 | 概念工作区 + 语义色仍是辨识度核心 |
| 信息可读性 | 10 | 9 | READ ONLY 徽标比原判定 chip 语义更准，略优于 #3 |
| **总分** | **100** | **85** | |

### 剩余 Findings（均为 Round 3 已记录、未修复、非阻塞项，非本轮 diff 引入）

- **P2-A** · `096s-s3-03-preview.jpg`：迷你表 SOURCE/PLANNED 仍为规范省略号截断 `l***@example.te…`。
- **P2-B** · `176s-s4-02-pipeline.jpg`：SAME TRANSACTION 括注进场仍早于专用 cue 约 11 秒。
- **P2-C** · `207s` / `249s`：下半幅留白偏大（内容止于 y≈510/545）。
- **P2-D** · `044s-s2-03-approve.jpg`：ED25519 APPROVAL RECORD 仍标「•UNKNOWN」（审批语境与检测语义的 UNKNOWN 存在词汇碰撞；114s 同卡为 PASS）。属词汇复用而非契约违规。

无 P0，无 P1。

**最终判定：PASS**

---

**回复摘要**：总分 **85/100**。四项核验全部 **PASS**：(1) 082s name 判定 chip 为 UNKNOWN（83% 低于阈值）、26 帧内 REVIEW 零命中，031s 帧本无 name chip、UNKNOWN 语境连贯；(2) 224/236/243s 均为 READ ONLY 徽标、无裁切无重叠；(3) 回归扫描干净，最深内容 ≈y905 在安全线内；(4) 与 Round 3 相比仅任务声明的四处差异，历史 P2-A…P2-D 原样保留。**无剩余 P0/P1**。

## 交付结论

- 交付版本：render #4，260.054s / 1920×1080 / 30fps / H.264+AAC，SHA-256 `f7b3a07b20e26dbfbd28f2087eb5cf34976b98b02a7c53d6323213334b515ff9`。
- 图像审查（GLM-5.3-Flash，独立上下文，实际读图）：85/100，PASS，**无 P0/P1**。
- 文本审查（GLM-5.3，无法读图）：发现 1 处屏上事实错误 + 2 处不精确，均已修复（见 Round 4 处置）。
- 遗留 P2-A…P2-D 已记录、非阻塞，未在交付版本中修复（修复需再次渲染，会使被审查版本与交付版本不一致）。
