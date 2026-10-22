# 网站构建与部署

## 本地开发

```sh
cd website
npm ci
npm run dev
# http://localhost:5173/maskriver/
```

## 质量门禁

```sh
npm run typecheck
npm run lint
npm run build
npx playwright install chromium
npm test
```

`playwright.config.ts` 使用生产预览服务器，覆盖 1440、768、390 三档视口；测试会实际加载 MP4 并检查播放进度、字幕切换、流程切换和恢复状态迁移。

## 静态构建

```sh
cd website
npm run build
# 产物：website/dist/
```

Vite `base` 为 `/maskriver/`，适用于 `https://haoyuehx.github.io/maskriver/`。媒体路径通过 `import.meta.env.BASE_URL` 生成，并可用 `VITE_VIDEO_URL`、`VITE_POSTER_URL`、`VITE_CAPTIONS_URL` 覆盖。

## GitHub Pages

工作流：`.github/workflows/website.yml`

- PR：构建、类型检查、ESLint、Playwright 测试，不上传部署。
- push 到 `main`：在测试通过后上传 Pages artifact 并部署；功能分支只跑构建和浏览器测试，不发布生产 Pages。
- 部署使用官方 `actions/upload-pages-artifact` + `actions/deploy-pages`，不修改 Go CI。
- 需要仓库管理员在 Settings → Pages 将 Source 设置为 **GitHub Actions**，并允许 `main` 部署。若未启用，部署 job 会失败，不能声称已上线。
- `deploy` job 有独立的 `pages: write` / `id-token: write` 权限、`github-pages` 环境、不可取消的并发组，并在部署后执行 HTTP/Range/OG smoke test。

## 部署后检查

1. `https://haoyuehx.github.io/maskriver/` 返回 200。
2. 页面刷新与直接访问子路径正常。
3. 视频请求为 `206 Partial Content` 或完整 200，能拖动进度。
4. 控制台无严重错误，移动端无横向滚动。
5. 下载、GitHub、文档链接可访问。
