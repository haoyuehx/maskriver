# MaskRiver 演示片 · 执行进度（Progress）

> 若会话上下文中断，从这里恢复。只记录**实际发生**的事情与实际生成的文件。

## 当前阶段

PHASE I — 完成（render #4 已交付并通过技术验收与第 4 轮视觉复核；PR #104 仍为 OPEN，等待本次合并门禁）

## 环境（实测）

| 项 | 值 |
|---|---|
| OS | Ubuntu 24.04.5 LTS，Linux 6.8，2 vCPU / 7.4 GiB RAM / 27 GiB 可用磁盘 |
| Node.js | v22.23.3，npm 10.9.9 |
| Remotion | 4.0.427（@remotion/cli 4.0.427，React 19.2.0，TypeScript 5.9.3） |
| 浏览器 | Chrome Headless Shell 144.0.7559.20（`npx remotion browser ensure` 下载，86.6 MB） |
| FFmpeg | 4.1 static（`@ffmpeg-installer/linux-x64`，npm devDependency，免 sudo） |
| FFprobe | 4.0.2 static（`ffprobe-static`，npm devDependency） |
| 字体 | Noto Sans SC 400/500/700 + JetBrains Mono 400/500/700（@fontsource，OFL-1.1，npm 管理，不落库） |
| 中文字形检查 | 已渲染 still 并目视确认：无方框、标点正常、中英混排正常 |
| 渲染后端 | `gl=swiftshader`，`concurrency=2`，实测约 2.3–4 fps（SwiftShader，无 GPU） |

## 阶段记录

| 阶段 | 状态 | 证据 |
|---|---|---|
| A 环境 | PASS | `out/env-test.mp4` 5.06s 1920×1080 30fps h264（ffprobe 实测） |
| B 设计 | PASS | `docs/visual-direction.md`（GLM-5.3 输出，425 行，含 tokens/网格/镜头/转场/逐场景 beat 表） |
| C 样片 | 已渲染 | `out/maskriver-style-test.mp4`（15.06s），抽帧 `out/review-style/contact-sheet.png` |
| D 正片前半 | 已实现 | `src/scenes/Scene1Problem.tsx`、`Scene2Workflow.tsx`、`Scene3Walkthrough.tsx` |
| E 正片后半 | 已实现 | `Scene4Streaming.tsx`、`Scene5Recovery.tsx`、`Scene6Closing.tsx` |
| F 声音 | 技术 QC PASS；**HUMAN AUDIO REVIEW REQUIRED** | `public/audio/narration.mp3`（Piper `zh_CN-huayan-medium`，35 条，260.04s）；成片 AAC 48 kHz / −16.8 LUFS / TP −4.9 dBFS；试听包与清单见 `docs/audio-review.md` |
| G 成片 | 完成（render #4） | `out/maskriver-demo-new.mp4`：260.054s / 7800 帧 / 30.91 MiB / sha256 f7b3a07b…5ff9 |
| H 独立 QA | 第 1–4 轮 + 交付前独立复核完成（第 4 轮 85/100；最终复核 88/100 PASS，无 P0/P1） | `docs/visual-review.md`、`docs/qc-report.md`、`docs/audio-review.md`、`docs/evidence/` |
| I 交付 | PR OPEN，等待门禁 | 分支 `feat/remotion-video-v2`；内容提交 `bd93099`（工程与成片）+ `139dffa`（事实修正与重渲染）+ `bc6b4ed`（交付文档）；音轨人工试听提交随后加入；PR https://github.com/haoyuehx/maskriver/pull/104 |

## 渲染轮次

| 轮次 | 变更 | 结果 |
|---|---|---|
| render #1 | 六幕首版 | 260.05s，QC 通过；视觉评审 79/100，2×P1（字幕节拍错位、邮箱硬裁切） |
| render #2 | 重排 Scene 2 旁白 cue 与 Scene 3 章节窗口；缩短合成邮箱；修 5 项 P2；加入 TTS 旁白 | QC 通过（含音轨）；视觉评审 82/100，P1-1/P1-2 判 PASS，新增 2×P1（PREFLIGHT 面板被裁、游离数字 780） |
| render #3 | 移除泄漏到画布的调试文本节点；PREFLIGHT 改 2×2 紧凑布局；resume/PASS 卡下移；故障遮罩在 resume 时撤除；删除冗余 chip 与游离 token；SCAN 相位内容提前进场 | QC 通过；视觉评审 85/100 **PASS**（无 P0/P1），剩 4 项 P2 |
| render #4 | 纯文本审查（GLM-5.3）发现屏上「REVIEW」判定态与产品契约不符：改为 `UNKNOWN`，删除 `VerdictChip` 的 REVIEW 类型，Scene 5 只读标记改 `READ ONLY`；cue 20 增加流式限定并重新合成配音；QC 脚本修正末条字幕时间 | 交付版本：260.054s / 30.91 MiB / sha256 f7b3a07b…5ff9；QC 通过 |

## 模型分工（实际）

- **DeepSeek V4.1 Flash**（`opencode-go/deepseek-v4.1-flash`）：环境、工程搭建、全部 React/TS 动画实现、渲染、抽帧、修复、字幕生成脚本、QC 脚本。
- **GLM-5.3**（`opencode-go/glm-5.3`）：视觉导演，输出 `docs/visual-direction.md`；另做一次**纯文本**的事实/一致性审查（该模型在本部署中 `input: ["text"]`，**不能接收图片**）。
- **视觉审查限制（必须如实记录）**：GLM-5.3 无法读取 PNG 帧，因此它的任何结论都不是"看过画面"的结论。实际看图由 `opencode-go/glm-5.3-flash`（同族、支持图片输入）执行，结论写入 `docs/visual-review.md` 并明确标注模型与证据路径。

## 最终状态（本次收尾）

- **视觉：** render #4，GLM-5.3-Flash 第 4 轮 85/100、交付前独立复核 88/100，均 PASS，无 P0/P1。
- **事实：** GLM-5.3 纯文本审查发现的 1 处 P1（虚构判定态 REVIEW）与 2 处 P2 已在 render #4 修正；本轮再复核无 P0/P1。
- **音轨：** 技术 QC 通过（AAC-LC 48 kHz 双声道，260.054 s，−16.8 LUFS，true peak −4.9 dBFS，35/35 cue 有语音、无削波、无 >10 s 异常静音）；**未做人工听感验收**，因此仍为 `HUMAN AUDIO REVIEW REQUIRED`，不报告 AUDIO PASS。
- **保留 P2：** P2-A（096 s 邮箱省略号）、P2-B（176 s SAME TRANSACTION 提前）、P2-C（207/249 s 留白）、P2-D（044 s 审批卡 UNKNOWN 词汇复用）。四项均不影响技术事实与主线理解；修复需要再次完整渲染并重新审查，本轮保留已审查的 render #4，不以未复核的新渲染替代。
- **样片：** `out/maskriver-style-test.mp4` 本地存在（15.06 s），未入库；不影响正式片，本轮不重做。
- **网站：** 仍使用旧静音视频。新片替换方案见 `video/docs/website-handoff.md`；在人工听感确认前不覆盖 `website/public/media/*`。

## 下一步（未完成，不由本 PR 冒充完成）

1. 人工试听 `review/audio-review.mp3` 或完整 MP4，并按 `docs/audio-review.md` 填写 PASS/CONDITIONAL/FAIL。
2. 若音轨需要修复，单独做音频重混与 QC；若保留，记录试听结论。
3. PR #104 合并后，按 `docs/website-handoff.md` 执行网站替换（需用户确认）。
4. 不修改 Go、v1.0.0 Tag、Release 资产或旧网站视频，直到网站替换步骤被授权。

## 交付状态（最终）

| 项 | 状态 |
|---|---|
| Remotion 工程可构建（`npm ci` + `npm run typecheck`） | PASS |
| 15 s 风格样片真实渲染 | PASS（`out/maskriver-style-test.mp4`，15.06 s，未入库） |
| 六幕全部实现 | PASS（750+1200+2550+1500+1350+450 = 7800 帧） |
| 完整 MP4 生成 | PASS（`out/maskriver-demo-new.mp4`，sha256 `f7b3a07b…5ff9`） |
| 可完整解码 / 无黑屏 / 无长冻结 | PASS |
| 时长 3–5 分钟 | PASS（260.054 s） |
| 中文字体与字幕 | PASS（Noto Sans SC 渲染正常；35 条烧录字幕 + 外部 SRT） |
| 产品内容准确性 | PASS（4 轮评审 + 1 轮文本事实审查；1 处事实错误已修复） |
| 无真实敏感信息 | PASS（全合成数据；无凭证/DSN/密钥） |
| 无软件版本号 / 里程碑 / Issue-PR 编号 | PASS |
| 独立视觉审查 | PASS（第 4 轮 85/100；交付前复核 88/100；均无 P0/P1；模型限制已记录） |
| 音轨 | **AUDIO PENDING**（已生成并混入，未人工听感验收） |
| 不影响 Go 项目与网站 | PASS（仅新增 `video/**`） |
