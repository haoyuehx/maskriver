# MaskRiver 演示视频评审

评审对象：`maskriver-demo-deliverables.zip` 中的 `out/maskriver-demo.mp4`（2026-10-22 交付）。
评审方式：容器信息、MP4/容器元数据、随附 SRT、旁白稿、分镜与 QC 报告、Remotion 源工程交叉核对。评审时浏览器/ffprobe 不可用，未做逐帧人工播放；这一点如实记录。

## 客观规格

| 项目 | 实测 / 交付记录 |
|---|---|
| 文件 | `maskriver-demo.mp4` |
| 大小 | 13,311,399 bytes（约 12.7 MiB） |
| 时长 | 260.000 秒（4 分 20 秒） |
| 分辨率 | 1920×1080 |
| 帧率 | 30 FPS（QC 记录完整解码 7,800 帧） |
| 视频编码 | H.264 / MP4 |
| 音轨 | **无音轨**；交付说明明确为“无配音视觉版” |
| 字幕 | 画面内中文字幕 + 独立 `maskriver-demo.srt` |
| 交付源 | Remotion + React + TypeScript 完整工程、旁白稿、分镜、素材许可、QC 报告 |

## 内容与准确性

- **时长**：4:20，满足 3–5 分钟比赛要求。
- **结构**：风险场景 → 五步流程 → 概念工作区 → 有界流式 → Checkpoint/Recovery → 品牌收束，节奏与分镜一致。
- **中文字幕**：SRT 覆盖 00:00–04:20，共 19 条；画面内也有字幕。用词与旁白稿一致，未发现明显错字或时间轴跳变。
- **技术事实**：视频明确将恢复限定为 Linux amd64 本地 SQLite，标注“操作流程示意 / 合成数据”，没有把概念 UI 说成已实现 Web 后台。审批、事务回执、Checkpoint 与 Manifest CAS 的描述与仓库实现方向一致。
- **与 v1.0.0 的差异**：视频制作早于 M3-2，**没有介绍 MySQL 8.0.46 CLI**。网站正文补充了 MySQL 支持，因此视频本身不构成完整的 v1.0.0 能力清单。
- **敏感信息**：画面数据为虚构姓名、`example.test` 邮箱、遮蔽手机号；未发现真实个人数据、凭据、密钥、DSN 或临时文件。Remotion 源工程也不包含音频/字体文件再分发。
- **清晰度/过渡**：源工程使用 1920×1080、30 FPS 与统一设计 token；QC 报告记录抽查未发现黑屏/遮挡，并修复过“非计划字段被误改”和邮箱挤压手机号列的问题。未做逐帧独立复核，因此不宣称“每帧完美”。

## 结论与改进项

**可用于比赛作品展示**，但当前是**无配音视觉版**。以“音频完整”作为硬性验收项时不通过；以“视觉演示 + 中文字幕”作为验收项时通过。

建议（本轮不擅自重制视频）：

1. **P1 — 增加中文配音**：录制旁白稿，混入 -16 LUFS 左右的语音轨，并保留现有字幕；这是当前成片与“完整产品介绍视频”之间最大的差距。
2. **P2 — 更新到 v1.0.0**：在结尾或流程段补一句 MySQL 8.0.46 CLI 已通过隔离测试，避免评委把视频误解为只支持 SQLite。
3. **P2 — 增加轻量背景音乐**：仅在获得合法授权后使用；当前无音乐是安全选择，不是缺陷。
4. **P2 — 字幕样式**：画面内字幕已经可读；若重新渲染，可把长句拆成两行并统一底部安全区，提升移动端观看体验。
5. **P3 — 版本与链接**：结尾可加 `v1.0.0` 与仓库短链接，便于评委定位。

## 网站集成决策

- 视频作为静态媒体随站点部署：13.3 MB 远低于 GitHub Pages 单文件 100 MB 限制和仓库常见容量边界，比外部对象存储更少依赖。
- 站点不自动播放；点击封面后才加载 MP4，并提供原生 `controls` 不可用时的自定义控制、加载/错误状态和直接打开 MP4 的备用链接。
- 媒体 URL 通过 `VITE_VIDEO_URL`、`VITE_POSTER_URL`、`VITE_CAPTIONS_URL` 可配置。

---

## 2026-10-27 — final Jianying video accepted for website integration

The final Jianying video is merged and deployed on the website. The project lead
listened to and approved the voice track; the technical checks below are separate
from that user-provided listening conclusion.

- Final MP4: `public/media/maskriver-demo-v2.mp4`
  - SHA-256: `44bc2418a34c003451ad2db2570e3e1dca78e597612c4e7051795887642c24c8`
  - 260.018 s · 1920×1080 · 30 fps · H.264 Main + AAC-LC 44.1 kHz stereo
  - 35 burned Chinese captions + optional 35-cue WebVTT
  - Jianying export committed by the project owner; project lead confirmed the final voice track after listening.
- New poster: `public/media/maskriver-demo-v2-poster.png` (23.0 s frame, 1920×1080).
- The external VTT is **default off** so mobile and desktop do not show burned and
  external captions at the same time. The player button is labelled as an
  optional external subtitle track.
- Old silent media remains in `public/media/` for rollback.
- Human listening: **PASS — project lead confirmed the final Jianying voice track after listening (user-provided conclusion, not an agent listening test).**
- Final technical QC is recorded in `docs/competition/final-video-qc.md` and the video PR.
