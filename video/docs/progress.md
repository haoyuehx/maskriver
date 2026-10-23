# MaskRiver 演示片 · 执行进度（Progress）

> 若会话上下文中断，从这里恢复。只记录**实际发生**的事情与实际生成的文件。

## 当前阶段

PHASE I — 交付（render #4 已产出并通过技术验收；第 4 轮视觉复核进行中；分支已提交，推送因网络受阻待重试）

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
| F 声音 | AUDIO PENDING（已生成，未做人工听感验收） | `public/audio/narration.mp3`（Piper `zh_CN-huayan-medium`，35 条，260.04s，−16 LUFS） |
| G 成片 | 完成（render #4） | `out/maskriver-demo-new.mp4`：260.054s / 7800 帧 / 30.91 MiB / sha256 f7b3a07b…5ff9 |
| H 独立 QA | 第 1–3 轮完成；第 4 轮（事实修正复核）进行中 | `docs/visual-review.md`、`docs/qc-report.md`、`docs/evidence/` |
| I 交付 | 提交完成，推送受阻待重试 | 分支 `feat/remotion-video-v2`，提交 `bd93099` + 事实修正提交 |

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

## 未解决问题 / 下一步

1. render #3 完成后重跑 `scripts/qc-video.mjs` 与 `scripts/collect-evidence.mjs`。
2. 第 3 轮独立视觉复核（`glm-5.3-flash`，图像输入）；若仍有 P0/P1，按修复上限（三轮）记录并决定是否再渲染。
3. GLM-5.3（纯文本）做一次事实/一致性审查，结论并入 `docs/visual-review.md`。
4. 提交、推送 `feat/remotion-video-v2`、创建 PR（不合并，不覆盖官网旧视频）。
