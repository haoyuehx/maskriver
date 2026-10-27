# 官网新视频替换方案（待人工听感确认后执行）

**当前状态：终版已准备，待 PR #109 合并与 Pages 部署。** 项目负责人已完成剪映中文配音并试听确认；
候选文件为 `website/public/media/maskriver-demo-v2.mp4`（SHA-256
`44bc2418a34c003451ad2db2570e3e1dca78e597612c4e7051795887642c24c8`）。在部署成功前，
线上仍是旧的无配音视频。

## 当前线上事实（本轮实际检查）

| 项 | 当前值 |
|---|---|
| 线上站点 | `https://haoyuehx.github.io/maskriver/`（HTTP 200） |
| 线上视频 | `/maskriver/media/maskriver-demo.mp4`，Range 请求 206 |
| 线上视频规格 | 260.000 s / 1920×1080 / H.264 / **无音轨** / 13,311,399 bytes |
| 线上视频 SHA-256 | `efa1790674f8559daa5af64c73c387515e8494a2b7ef5316d636caa7e098dd66` |
| 线上字幕 | 外挂 `maskriver-demo.zh.vtt`，同时画面烧录中文 |
| 播放器 | `website/src/components/VideoPlayer.tsx`；点击封面才加载；有字幕、音量、全屏控制 |
| 部署 | `.github/workflows/website.yml`，仅 `main` push 到 `website/**` 时构建/部署 |
| 媒体 URL | `website/src/data/content.ts` 的 `VIDEO_URL` / `POSTER_URL` / `CAPTIONS_URL` |

## 候选新片

| 项 | 值 |
|---|---|
| 文件 | `video/out/maskriver-demo-new.mp4`（已提交到 PR #104） |
| SHA-256 | `f7b3a07b20e26dbfbd28f2087eb5cf34976b98b02a7c53d6323213334b515ff9` |
| 规格 | 260.054 s / 1920×1080 / 30 fps / H.264 + AAC-LC 48 kHz / 32,415,678 bytes |
| 字幕 | 35 条中文已烧录进画面 + `video/subtitles/zh-CN.srt` |
| 音轨 | 本地 Piper TTS，技术 QC 通过；**人工听感未验收** |
| 推荐 URL 文件名 | `maskriver-demo-v2.mp4`（新文件名，避免 CDN/浏览器缓存旧片） |

## 替换前必须满足

1. 人工按 `docs/audio-review.md` 完整试听，结论为 PASS 或 CONDITIONAL。
2. 若为 CONDITIONAL，记录具体问题、时间点和是否可接受；不得把技术 QC 当成听感 PASS。
3. 视频 SHA-256 与视觉审查/技术 QC 指向的产物一致；若重新渲染，必须重新抽帧、重新 QC、更新 SHA。
4. 用户明确确认可以覆盖官网演示视频。

## 建议的最小改动（下一轮执行）

### 1. 媒体文件

- 新增 `website/public/media/maskriver-demo-v2.mp4`（新片，保留旧文件直到上线回滚不再需要）。
- 新增封面 `website/public/media/maskriver-demo-v2-poster.png`：从新片抽取 4.0 s 全分辨率帧，
  确保与新片视觉一致；旧 `poster.png` 是旧片风格，不应继续使用。
- 不复制新片的 SRT/VTT 作为默认外挂字幕：新片已烧录 35 条字幕，默认再显示外挂字幕会形成双层。
  如仍需无障碍外挂字幕，必须默认关闭，并在 UI 明示“外挂字幕（画面已含字幕）”，不能默认开启。

### 2. `website/src/data/content.ts`

```ts
export const VIDEO_URL =
  import.meta.env.VITE_VIDEO_URL ?? `${mediaBase}maskriver-demo-v2.mp4`;
export const POSTER_URL =
  import.meta.env.VITE_POSTER_URL ?? `${mediaBase}maskriver-demo-v2-poster.png`;
```

`CAPTIONS_URL` 仅在决定保留“默认关闭的外挂字幕”时改为新 VTT；否则从播放器移除 track。

### 3. `website/src/components/VideoPlayer.tsx`

- 文案：`04:20 · 1920×1080 · 中文画面字幕 · 无配音` → `04:20 · 1920×1080 · 中文配音 · 烧录字幕`。
- 底部提示：`视频为无配音视觉版；中文 SRT 随站点提供` → `视频含中文旁白与烧录字幕；音量可调`。
- 若移除外挂字幕：删除 `<track>`、`captions` 状态、Captions 图标按钮和 `textTracks` effect；
  同步删除 `onLoadedMetadata` 之外的字幕相关分支。
- 若保留外挂字幕：`captions` 初始值必须为 `false`，按钮 label 改为“外挂字幕（画面已含）”。
- 保留现有的点击加载、错误回退、音量/静音、进度、全屏和移动端行为；新片有音轨，默认
  音量沿用 0.85，不要强制静音。

### 4. 文档与测试

- `website/docs/video-review.md`：保留旧片评审为历史，新增新片规格、SHA、视觉/事实/音频状态。
- `website/README.md`：媒体清单改为新文件名和大小；明确“中文配音 + 烧录字幕”。
- `.github/workflows/website.yml` 的 post-deploy smoke：把 Range 检查路径改为新视频；
  继续检查首页 200 与 `og-image.png` 200。若封面改名，OG 图不必跟随视频封面。
- `website/tests/site.spec.ts`：现有播放器测试检查封面和播放按钮；新增/调整断言以覆盖
  新文案与“无默认双层字幕”。不要把 260 s 写死为其它时长（新片 260.054 s）。
- `website/docs/deployment.md`：记录新片部署与回滚步骤。

### 5. 部署与验证

1. 本地 `npm run typecheck && npm run lint && npm run build`。
2. Playwright 全视口回归；至少检查 360/390/768/1440/1920 的播放器与字幕开关。
3. 合并后等待 Website workflow 的 `Deploy GitHub Pages` 成功。
4. 线上验证：
   - 首页 200；
   - 新 MP4 Range 206；
   - 新封面 200；
   - 无默认双层字幕；
   - 音量按钮可听、可静音；
   - 移动端 360 px 无溢出；
   - 旧视频文件仍在仓库作为回滚，直到确认不再需要。

## 回滚

若新片上线后发现音画问题：将 `content.ts` 的 `VIDEO_URL` / `POSTER_URL` 指回旧文件，
重新部署即可；不要重写 Git 历史或删除旧 Release。正式比赛提交前必须完成一次线上播放验证。

## 本轮结论

**WAITING FOR HUMAN AUDIO REVIEW**：工程 PR 可合并，但官网替换必须等试听结论；当前线上仍是旧无声版。
