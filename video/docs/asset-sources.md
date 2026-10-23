# 素材与来源（Asset Sources）

本片只使用可追溯、可再分发的素材。仓库中**不包含**任何第三方音频、字体二进制、真实数据或截图。

| 素材 | 来源 | 许可 | 使用方式 |
|---|---|---|---|
| Remotion 4.0.427 / @remotion/cli / @remotion/renderer | npm 官方包 | Remotion License（公司许可，个人/小团队免费） | 视频渲染框架 |
| React 19.2.0 / react-dom 19.2.0 | npm | MIT | UI 组件 |
| TypeScript 5.9.3 | npm | Apache-2.0 | 类型检查 |
| Noto Sans SC 5.2.8（@fontsource/noto-sans-sc） | npm | SIL OFL-1.1 | 中文为主字体；`public/` 不存字体二进制，由 lockfile 锁定 |
| JetBrains Mono 5.2.8（@fontsource/jetbrains-mono） | npm | SIL OFL-1.1 | 等宽/代码/数据字体 |
| Chrome Headless Shell 144.0.7559.20 | Remotion 官方下载（`remotion.media`） | Chrome for Testing 许可 | 渲染浏览器，不提交仓库 |
| FFmpeg 4.1 static | npm `@ffmpeg-installer/linux-x64` | GPL-2.0+（构建启用 GPL），仅用于本地 QC/抽帧 | 不作为产品分发 |
| FFprobe 4.0.2 static | npm `ffprobe-static` | GPL-2.0+/LGPL | 仅本地 QC |
| 图片/图标 | 无外部图片；全部为 React/SVG/CSS 现场绘制 | 本项目 MIT | 无位图资产 |
| 背景音乐 | **未使用**（无可用授权音源） | — | 避免未授权音轨 |
| 配音（若生成） | Piper TTS 本地合成，voice `zh_CN-huayan-medium` | 模型来自 rhasspy/piper-voices（见其 MODEL_CARD；huayan-medium 源自开源中文语料训练） | 本地合成，不含真人克隆；模型权重不提交仓库 |
| 数据 | 本项目自编合成数据（`.test` 邮箱域、`199 0000 00xx` 号码、虚构姓名/城市） | 本项目 MIT | 画面内所有记录均为合成 |

## 明确未使用

- 未使用任何真实个人信息、真实企业数据、真实数据库导出。
- 未使用未授权的流行音乐、影视音效或第三方配音。
- 未使用未授权的 API Key / 云 TTS 服务；未克隆真人声音。
- 未复制其他项目的源码、词典、配置或测试数据。

## 字体与语义

- 中文：Noto Sans SC（400/500/700）。英文与数字：JetBrains Mono（400/500/700）。
- 字体通过 npm 依赖（`@fontsource/*`）加载，`package-lock.json` 锁定版本；字体二进制不入库，符合 OFL 的可再分发要求。
