<!--
Persisted review artifact.

Reviewer: GLM-5.3-Flash (image input enabled, independent context)
Reason this model and not GLM-5.3: the requested reviewer model `opencode-go/glm-5.3`
advertises `input: ["text"]` in this deployment — it cannot receive images. A probe
(reading a rendered PNG) returned "[Current model does not support images. The image
will be omitted from this request.]". Its review is therefore recorded separately as a
text-only consistency review, and the image-based review below was executed by the
image-capable sibling model so that no claim of "visual review" is unsupported.

Round 1 reviewed the FIRST full render (render #1, 260.05s). Fixes applied afterwards
are listed in section "Round 1 处置". A second, independent image review of the final
render is recorded in the same file after that section.
-->

# MaskRiver 产品片 · 视觉 QA / 创意评审报告

- 评审对象：`out/maskriver-demo-new.mp4`（260.05s · 1920×1080 · 30fps · 7800 帧）
- 评审模型：GLM-5.3-Flash（图像输入，独立上下文）
- 评审日期：2026-05-27

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
3. **交付一致性**：本报告审查的成品是 **render #3**，其 SHA-256 为
   `27c51947713888fd797c667f552822a8e1c4375e86ccf304e1d90d46c0bc1ed7`，
   与 `docs/qc-report.md` 及仓库中交付的 `out/maskriver-demo-new.mp4` 完全相同。
   Round 3 剩余的 P2-A…P2-D 为**已记录但未修复**的非阻塞项：修复它们需要再渲染一次，
   会使"被审查的成品"与"交付的成品"不一致，因此不在本轮执行，交由后续版本处理。
4. **证据文件格式**：`docs/evidence/` 中的 26 张逐帧证据以 JPEG（质量 90，原始 1920×1080）保存以控制仓库体积，
   文件名与上述评审引用一致（仅扩展名由 `.png` 改为 `.jpg`）；`contact-sheet-final.png` 保持 PNG。
