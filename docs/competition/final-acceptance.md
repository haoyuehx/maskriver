# MaskRiver final acceptance — competition freeze

**Decision: `RELEASE READY — WEBSITE VIDEO AWAITING AUDIO APPROVAL`**

The code, release, download entry points and website are frozen. The only
remaining publication step is the user's human listening verdict for the new
Chinese voice track. No AI listening result is claimed.

## Release

| Item | Value |
|---|---|
| Version | **v1.0.1** |
| Release | https://github.com/haoyuehx/maskriver/releases/tag/v1.0.1 |
| Release ID | `RE_kwDOVCBgws4ZL-0v` |
| Tag | `v1.0.1` (annotated) → `18232f47d418202917dda6895bde45338c59d1a3` |
| Release commit | `18232f47d418202917dda6895bde45338c59d1a3` |
| Asset | `maskriver-v1.0.1-linux-amd64.tar.gz`, 4,637,907 bytes |
| SHA-256 | `bf6e839453ea7f5d962aba4cbd34c326d9740e9c21597877136cbeb1b86a3672` |
| Public re-download | `sha256sum -c` OK; `verify-release.sh` without `--allow-untagged` PASS |
| Release smoke | M1 + WAL-target refusal + M2 streaming/interruption/resume/validate PASS |

The included fix is the SQLite WAL/MEMORY/OFF target refusal in legacy Apply.
The release-package black-box test confirms a real WAL target is refused before
write and that the target main file and rows stay unchanged.

## Final application state

| Item | Value |
|---|---|
| Final code/website main before this record | `d8acfcb053d26caee3f6bebb6b5da961104fa168` |
| Main CI | run `40073851968` SUCCESS |
| Website workflow | run `40073851964` SUCCESS (Build + Deploy Pages) |
| Website | https://haoyuehx.github.io/maskriver/ |
| Public download entry | v1.0.1 in README and website |
| Frozen contracts | `m1a-v1`, `m2-stream-v1`, `m2-approval-binding-v1`, `m2-recovery-v1` |

## Video

| Item | Value |
|---|---|
| New MP4 | `video/out/maskriver-demo-new.mp4` |
| SHA-256 | `f7b3a07b20e26dbfbd28f2087eb5cf34976b98b02a7c53d6323213334b515ff9` |
| Spec | 260.054 s, 1920×1080, 30 fps, H.264 + AAC-LC 48 kHz stereo |
| Subtitles | 35 burned Chinese cues + matching SRT/VTT |
| Visual review | 88/100 PASS, no P0/P1 |
| Audio technical QC | PASS — −16.8 LUFS, −4.9 dBFS true peak, 35/35 cue coverage, no clipping |
| Audio human review | **PENDING — HUMAN AUDIO REVIEW REQUIRED** |
| Website video | **Still the old silent video**; new v2 integration is prepared in draft PR #109 and not deployed |

Do not claim the new voice track is accepted, and do not replace the live video
until the user records PASS or CONDITIONAL in `video/docs/audio-review.md`.

## Competition material

- README Contributors: four confirmed accounts only (`haoyuehx`, `emgps`,
  `whh-true`, `HerrscherofSentience555`); `PyGuy2` not displayed.
- Competition overview, core features, technical highlights, demo guide and
  submission checklist are present and point to v1.0.1.
- Submission checklist keeps the final video item unchecked pending human audio
  approval.
- v1.0.0 tag, Release, assets and historical previews remain untouched.

## Remaining non-blocking issues

- Audio: human listening approval pending (the only publication gate).
- Video P2-A..P2-D from the visual review remain accepted, non-blocking.
- Architecture diagram status-dot/label overlap and clipped GetStarted terminal
  URLs are pre-existing cosmetic P2s, outside the release fix.
- MySQL versions other than 8.0.46, MySQL/Windows/macOS recovery and WAL recovery
  remain outside supported scope.

## Freeze

No new features, architecture changes, database adapters, video renders or
unnecessary UI refactors. Future work is limited to real defects or competition
requirements. The new video may be published only after the human audio gate.
