# MaskRiver Showcase Website

MaskRiver 的静态产品展示网站，面向比赛评委、开发者和数据工程师。站点把产品定位、五步工作流、演示视频、流式处理、Checkpoint/Recovery、架构、使用场景、快速开始与限制放在同一页中。

## 技术栈

- React 19 + TypeScript
- Vite 7
- 手写 CSS 设计系统（深色科技感、响应式、`prefers-reduced-motion`）
- Lucide 图标
- Playwright 五档视口端到端测试（1920 / 1440 / 768 / 390 / 360）

## 开发

```sh
npm ci
npm run dev
```

## 构建与验证

```sh
npm run typecheck
npm run lint
npm run build
npm test
```

## 媒体

- `public/media/maskriver-demo-v2.mp4`：新版 4:20 H.264 + AAC 中文配音视频，30.9 MB，画面烧录字幕。
- `public/media/maskriver-demo-v2.zh.vtt`：新版 35 条外挂字幕，默认关闭，避免与烧录字幕叠加。
- `public/media/maskriver-demo-v2-poster.png`：从新版视频 23.0 s 抽取的封面。
- `public/media/maskriver-demo.mp4`：旧版无配音视频，保留用于回滚。
- `public/og-image.png`：项目原创分享图。

媒体 URL 可配置：

```sh
VITE_VIDEO_URL=... VITE_POSTER_URL=... VITE_CAPTIONS_URL=... npm run build
```

## 部署

见 [docs/deployment.md](docs/deployment.md)。GitHub Pages 目标地址：

`https://haoyuehx.github.io/maskriver/`

该地址只有在 Pages 实际部署成功后才可宣称可访问。
