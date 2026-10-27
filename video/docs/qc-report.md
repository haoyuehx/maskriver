> **历史 Remotion 成片 QC。** 官网终版是项目负责人的剪映导出（260.018 s / AAC 44.1 kHz）；终版实测见 `docs/competition/final-video-qc.md`。

# MaskRiver 演示片 · 技术验收（QC Report）

全部数值来自本机实测，命令与工具见下。原始 JSON：`docs/evidence/qc-report.json`（由 `scripts/qc-video.mjs` 生成，`out/qc-report.json` 同内容）。

## 1. 交付物

| 项 | 值 |
|---|---|
| 文件 | `video/out/maskriver-demo-new.mp4` |
| 大小 | 32,415,678 bytes（30.91 MiB） |
| SHA-256 | `f7b3a07b20e26dbfbd28f2087eb5cf34976b98b02a7c53d6323213334b515ff9` |
| 时长 | 260.054 s（7800 帧 @30fps） |
| 分辨率 | 1920 × 1080 |
| 帧率 | 30/1 |
| 视频编码 | H.264 High，pixel format `yuvj420p`（与仓库现有 `website/public/media/maskriver-demo.mp4` 一致），码率 ≈ 672 kbps |
| 音频编码 | AAC-LC，48 kHz，2 声道，≈ 317 kbps，时长 260.054 s；源旁白为单声道 MP3 |
| 字幕 | 画面烧录简体中文（35 条）+ 外挂 `video/subtitles/zh-CN.srt`（35 条，末条结束 257.4 s，起始 252.4 s） |

## 2. 命令

```sh
cd video
npx remotion render MaskRiverDemo out/maskriver-demo-new.mp4 --log=error   # 实际渲染，非仅命令
node scripts/qc-video.mjs out/maskriver-demo-new.mp4                        # ffprobe + ffmpeg 校验
node scripts/review-frames.mjs out/maskriver-demo-new.mp4 out/review-full ...  # 抽帧
node scripts/collect-evidence.mjs                                            # 归集证据帧与 contact sheet
ffmpeg -i out/maskriver-demo-new.mp4 -map 0:a -af volumedetect -f null -      # 音轨电平
```

环境：Ubuntu 24.04.5 LTS / 2 vCPU / 7.4 GiB RAM / Node v22.23.3 / Remotion 4.0.427 / Chrome Headless Shell 144.0.7559.20 / FFmpeg 4.1 static（npm devDependency）/ SwiftShader 软件光栅。

## 3. 校验结果

| 检查 | 结果 |
|---|---|
| 时长在 180–300 s | PASS（260.054 s） |
| 1920×1080 | PASS |
| 30 fps | PASS（r_frame_rate = 30/1，nb_frames = 7800） |
| H.264 | PASS（High profile） |
| 完整解码 | PASS（`ffmpeg -v error -i … -f null -` 无输出、退出码 0） |
| 黑屏段（blackdetect d=0.4 pix_th=0.06） | 无 |
| 长冻结（freezedetect n=0.0005 d=2.0） | 无 |
| 音轨存在 | PASS（AAC-LC 48 kHz 双声道） |
| 综合响度 / 真峰值 | −16.8 LUFS / −4.9 dBFS（LRA 8.3 LU；无削波） |
| 语音覆盖 | PASS — 35/35 条字幕 cue 的窗口内均检测到语音，无越界；最短尾部余量 0.17 s |
| 异常静音 | 无 >10 s；最大语音间静音 6.75 s（2:24–2:31 场景转场） |
| 字幕文件 | `subtitles/zh-CN.srt`，35 条，时间轴与 `src/data/script.ts` 同源生成 |
| 场景时长 | 750 / 1200 / 2550 / 1500 / 1350 / 450 帧，合计 7800（`src/MaskRiverVideo.tsx` 的 `SCENES` 与 `docs/storyboard.md` 一致） |

## 4. 内容与安全审查

| 项 | 结果 |
|---|---|
| 软件版本号 / 里程碑代号 / Issue-PR 编号 / CI 日志 | 未出现（逐条检查屏上字符串与字幕） |
| 真实个人信息 | 仅合成数据：虚构姓名、`*.example.test` 邮箱、`199 0000 00xx` 号码、虚构城市 |
| 凭证、密钥、DSN、内部地址 | 无。审批示例摘要为 `sha256:9f2c…a41b`（省略号占位，非真实摘要） |
| 虚构 Web 后台 | 界面标注 `maskriver · concept workspace` + 右上「流程示意 · 合成数据」，并声明「界面为设计示意」 |
| 技术断言 | 五步工作流、三条件写入、SQLite-only streaming/recovery、MySQL 不支持、批次事务 ≠ 整库原子性、确定性 ≠ 匿名化 均在片中明确表述 |
| 性能/客户数据 | 未出现任何性能提升数字、真实客户或生产环境指标 |

## 5. 抽帧证据

`docs/evidence/` 下 26 张按叙事节拍抽取的全分辨率帧 + `contact-sheet-final.png`，覆盖：
品牌收束、五步各相位、概念工作区六章、流水线全景与事务特写、批次日志与故障/恢复、片尾定版。

## 6. 音轨技术验收与人工听感状态

技术数据来自 `scripts/qc-audio.mjs`，原始 JSON 为 `docs/evidence/audio-qc.json`。

| 检查 | 实测 |
|---|---|
| 编码 / 采样率 / 声道 | AAC-LC / 48 kHz / 2（源旁白单声道） |
| 综合响度 / LRA / True Peak | −16.8 LUFS / 8.3 LU / −4.9 dBFS |
| 平均 / 最大样本电平 | −21.8 dB / −4.9 dB（无削波） |
| 语音段 / 字幕 cue | 35 / 35 |
| 越界语音 | 无 |
| 最大语音起点延迟 / 最小尾部余量 | 0.33 s / 0.17 s |
| 最大语音间静音 | 6.75 s（场景转场） |
| 异常长静音（>10 s） | 无 |

**这些数值不等于听感 PASS。** 发音、自然度、机械断句、英文缩写读音和“配音是否与画面相符”
必须由人耳判断。试听包与逐项清单见 `docs/audio-review.md`；当前状态是
**HUMAN AUDIO REVIEW REQUIRED**，不是 AUDIO PASS。

## 7. 已知限制（不隐藏）

1. **音频听感：HUMAN AUDIO REVIEW REQUIRED**。旁白由本地 Piper TTS（`zh_CN-huayan-medium`）合成，已并入成片并完成技术 QC；未做人工听感验收，自然度低于专业配音，不能报告 AUDIO PASS。cue 7 在生成时做过 1.18× 时间压缩，cue 35 的语速偏快（约 7.22 字/秒），已列入人工试听重点。
2. **视觉审查模型限制**：任务指定的 `opencode-go/glm-5.3` 在本部署中 `input: ["text"]`，**无法读取图片**（已用 PNG 探测复现）。图像审查由同族、支持图片的 `glm-5.3-flash` 在独立上下文中执行，记录见 `docs/visual-review.md`；GLM-5.3 只做纯文本一致性审查。
3. **四轮渲染记录**：render #1（79/100，2×P1）→ render #2（82/100，2×P1）→ render #3（85/100 PASS）→ render #4（修复纯文本审查发现的 1 处屏上事实错误 + 2 处不精确后重渲染，见 `docs/visual-review.md` 的 Round 4 处置与复核）。
4. `yuvj420p` 为全范围 JPEG 采样；与仓库既有视频一致，主流播放器可正常解码。
