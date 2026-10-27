# MaskRiver final acceptance — competition submission

**Decision: `COMPETITION READY — DEVELOPMENT FROZEN`**

## Competition

| Item | Value |
|---|---|
| Competition | 2026 开源行业解决方案创新赛 |
| Track | 开源基础软件与解决方案 |
| Project | MaskRiver 数据脱敏引擎 |
| School | 北京航空航天大学 |
| Team lead | 卢皓天 |
| Team members | 汤斯杰、王昊涵、孙子涵 |
| Submission deadline | 2026-10-31 24:00 (Beijing time) |

GitHub Contributors and the competition team are different concepts. The four
names above are the official competition roster; they are not a mapping of
GitHub contributor accounts.

## Release

| Item | Value |
|---|---|
| Version | **v1.0.1** |
| Release | https://github.com/haoyuehx/maskriver/releases/tag/v1.0.1 |
| Tag | `v1.0.1` (annotated) → `18232f47d418202917dda6895bde45338c59d1a3` |
| Asset | `maskriver-v1.0.1-linux-amd64.tar.gz`, 4,637,907 bytes |
| SHA-256 | `bf6e839453ea7f5d962aba4cbd34c326d9740e9c21597877136cbeb1b86a3672` |
| Public re-download | `sha256sum -c` OK; `verify-release.sh` without `--allow-untagged` PASS |
| Release smoke | M1 + WAL-target refusal + M2 streaming/interruption/resume/validate PASS |

The release includes the SQLite WAL/MEMORY/OFF target refusal. The v1.0.0 tag,
Release and assets remain untouched.

## Main and CI

| Item | Value |
|---|---|
| Final application/video main before this record | `35ad9d70164c44778fae97fcac6c3058a2194b99` |
| Final video merge | PR #109, merge `35ad9d7` |
| Release CI | run `40071664665` SUCCESS |
| Final video CI | run `40501158061` (Go/MySQL/release), website `40501157957` |
| Website deploy | `40501157957` — Build + Deploy Pages SUCCESS |

## Website and video

| Item | Value |
|---|---|
| Website | https://haoyuehx.github.io/maskriver/ |
| Final video | https://haoyuehx.github.io/maskriver/media/maskriver-demo-v2.mp4 |
| Final MP4 | `website/public/media/maskriver-demo-v2.mp4` |
| SHA-256 | `44bc2418a34c003451ad2db2570e3e1dca78e597612c4e7051795887642c24c8` |
| Size / duration | 74,098,376 bytes · 260.018 s |
| Video | 1920×1080, 30 fps, H.264 Main |
| Audio | AAC-LC, 44.1 kHz, stereo, one track |
| Subtitles | 35 burned Chinese captions; optional VTT, default off |
| Human review | **PASS — project lead listened to and confirmed the final Jianying voice track (user-provided conclusion)** |
| Online check | HTTP 200; `Content-Length` 74,098,376; Range 206; first/last byte ranges match the repository file. Full live-file SHA re-download was not completed due transfer throttling, but the served file is the deployed PR artifact with the recorded SHA. |

The original Remotion project and `video/out/maskriver-demo-new.mp4` remain as
historical source/render evidence; they are not the published website video.

## Competition material

- README Contributors: four confirmed GitHub accounts only (`haoyuehx`, `emgps`,
  `whh-true`, `HerrscherofSentience555`); `PyGuy2` is not displayed.
- Competition overview, core features, technical highlights, demo guide and
  submission checklist are present and point to v1.0.1.
- The final video is online, the website uses v1.0.1 download links, and the
  submission checklist marks the final video complete.
- No new features, architecture changes, database adapters or video renders are
  planned. Future work is limited to real defects or competition requirements.

## Remaining non-blocking issues

- Video P2-A..P2-D from the earlier visual review remain accepted, non-blocking.
- The architecture diagram status-dot/label overlap and clipped GetStarted
  terminal URLs are pre-existing cosmetic P2s.
- Full SHA verification of the live 70.67 MiB media was limited by GitHub Pages
  transfer throttling; Content-Length, Range, and first/last bytes were checked.
- MySQL versions other than 8.0.46, MySQL/Windows/macOS recovery and WAL recovery
  remain outside supported scope.

## Freeze

Development is frozen: no new features, architecture changes, database support,
video re-renders or unnecessary UI refactors. Only real defects or competition
requirements may change the project.
