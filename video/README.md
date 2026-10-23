# MaskRiver 演示片（Remotion 工程）

MaskRiver 的产品概念片：**安全、可验证、可恢复的数据脱敏工具**。1920×1080 / 30fps / 260 秒 / H.264 MP4。
全部画面由 React + SVG + CSS 在 Remotion 中实时绘制，无位图素材、无终端录屏、无虚构后台。

## 快速开始

```sh
cd video
npm ci                 # 锁定版本（package-lock.json）
npx remotion browser ensure   # 首次渲染前下载 Chrome Headless Shell
npm run studio         # 打开 Remotion Studio 预览
npm run typecheck      # TypeScript 检查
npm run subtitles      # 由 src/data/script.ts 生成 subtitles/zh-CN.srt
npm run render         # 渲染正片 -> out/maskriver-demo-new.mp4
npm run qc             # ffprobe/ffmpeg 技术验收 -> out/qc-report.json
```

首次渲染在 2 vCPU / 无 GPU 机器上约需 45–60 分钟（SwiftShader 软件光栅，约 2–4 帧/秒）。
`remotion.config.ts` 固定 `gl=swiftshader`、`concurrency=2`；有 GPU 的机器可调高并发。

## 目录

```
src/
  index.ts               Remotion 入口
  Root.tsx               合成注册（EnvTest / StyleTest / MaskRiverDemo / Dev-1..6 / Poster）
  MaskRiverVideo.tsx     六幕编排 + 字幕 + 转场 + 音轨
  scenes/                Scene1Problem … Scene6Closing
  components/            Backdrop / DataTable / Panel / Reveal / Flow / Workflow / Compare / Subtitles / Transitions …
  data/script.ts         旁白与字幕的唯一事实源（cue 时间轴）
  data/fixtures.ts       合成数据（虚构姓名、example.test 邮箱、199 0000 00xx 号码）
  styles/tokens.ts       设计 token（颜色、字号、间距、栅格、spring 预设、场景时长）
  styles/fonts.ts        Noto Sans SC + JetBrains Mono（@fontsource，OFL）
scripts/
  build-subtitles.mjs    由 script.ts 生成 zh-CN.srt
  stills.mjs             批量渲染审片静帧
  review-frames.mjs      从 MP4 抽帧 + contact sheet + 帧统计
  sheet.mjs              静帧拼图
  qc-video.mjs           时长/分辨率/帧率/编码/音轨/解码/黑屏/冻结/字幕检查
  generate-narration.mjs 本地 TTS 旁白合成与时间轴拼装（可选，需要 Piper 与音色模型）
docs/                    视觉规范、分镜、旁白、评审、素材来源、进度、QC
subtitles/zh-CN.srt      外挂字幕（与画面烧录字幕同源）
```

## 设计来源

- 视觉规范：`docs/visual-direction.md`（GLM-5.3 视觉导演输出）
- 分镜：`docs/storyboard.md`；旁白：`docs/narration.md`
- 视觉审查：`docs/visual-review.md`；技术验收：`docs/qc-report.md`
- 素材与许可：`docs/asset-sources.md`

## 音轨与人工验收

成片含 AAC-LC 48 kHz 旁白（Piper `zh_CN-huayan-medium`，35 条）。技术 QC 已通过：
−16.8 LUFS、True Peak −4.9 dBFS、35/35 cue 有语音、无削波。**尚未做人工听感验收**，
因此状态是 `HUMAN AUDIO REVIEW REQUIRED`，不能报告 AUDIO PASS。

- 精简试听包：`review/audio-review.mp3`（52.9 s）
- 逐项试听清单：`docs/audio-review.md`
- 原始测量：`docs/evidence/audio-qc.json`

## 事实边界（片中已声明）

- 五步工作流：scan → mask（dry-run 预览）→ approve（Ed25519 绑定计划）→ apply → validate。
- 写入需 `--approved` + `--apply` + 有效签名记录三者同时成立。
- 有界流式、Outbox 回执、Checkpoint/Resume **仅限本地 SQLite**；MySQL 流式/恢复不支持。
- 确定性脱敏 ≠ 匿名化，≠ 全局无碰撞。
- 画面数据全部为合成数据；概念工作区标注「流程示意 · 合成数据」。

## 依赖与许可

Remotion（公司许可）、React/ReactDOM（MIT）、TypeScript（Apache-2.0）、
Noto Sans SC / JetBrains Mono（OFL-1.1，经 @fontsource 以 npm 依赖形式引入，不入库字体文件）、
FFmpeg/FFprobe 静态包（仅本地 QC 使用，不作为产品分发）。
详见 `docs/asset-sources.md`。
