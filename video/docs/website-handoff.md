# 官网终版视频上线记录

**状态：已上线。** 项目负责人使用剪映完成终版中文配音并试听确认；视频由
PR #109 合并（`35ad9d7`），Website workflow `40501157957` 的 Build 与 Deploy
Pages 均成功。

## 线上事实

| 项 | 当前值 |
|---|---|
| 网站 | https://haoyuehx.github.io/maskriver/ |
| 视频 | https://haoyuehx.github.io/maskriver/media/maskriver-demo-v2.mp4 |
| 文件 | `website/public/media/maskriver-demo-v2.mp4` |
| SHA-256 | `44bc2418a34c003451ad2db2570e3e1dca78e597612c4e7051795887642c24c8` |
| 大小 / 时长 | 74,098,376 bytes · 260.018 s |
| 视频 | 1920×1080 · 30 fps · H.264 Main |
| 音频 | AAC-LC · 44.1 kHz · stereo · 单音轨 |
| 字幕 | 35 条画面烧录中文；外挂 VTT 默认关闭 |
| 封面 | `website/public/media/maskriver-demo-v2-poster.png`（23.0 s 终版帧） |
| 播放器 | `website/src/components/VideoPlayer.tsx`；点击加载、音量/静音、进度、全屏、错误回退 |

## 验证

- 完整解码 PASS；音轨 −21.9 LUFS，True Peak −4.0 dBFS，无削波。
- 35/35 字幕窗口内有语音；语音起点约比原 cue 晚 0.2 s；无 >10 s 异常静音。
- 片头/片尾为淡入淡出；未发现剪映水印或额外片尾。
- Website CI：typecheck、lint、build、Playwright 通过。
- Pages 部署成功；线上首页 200，视频 Range 206，封面 200。
- 线上 `Content-Length` 为 74,098,376；首尾字节与仓库文件一致。受 GitHub
  Pages 传输限速影响，未完成全文件回下载 SHA 复核。

## 回滚

将 `website/src/data/content.ts` 的 `VIDEO_URL` / `POSTER_URL` /
`CAPTIONS_URL` 指回 `maskriver-demo.mp4` / `poster.png` /
`maskriver-demo.zh.vtt`，重新部署；旧文件仍保留。不要重写 Release 或 Git 历史。

## 来源与边界

终版由项目负责人使用剪映制作并试听确认，不是 Agent 重新配音或重新渲染。
原始 Remotion 工程与 `video/out/maskriver-demo-new.mp4` 仅作为历史源工程保留。
