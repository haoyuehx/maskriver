#!/usr/bin/env node
/**
 * Technical QC for the rendered MP4.
 *
 * Usage: node scripts/qc-video.mjs out/maskriver-demo-new.mp4 [subtitles/zh-CN.srt]
 *
 * Uses the ffmpeg/ffprobe binaries installed as devDependencies. Every number in
 * the generated report comes from this run, not from the design intent.
 */
import {execFileSync} from 'node:child_process';
import {createRequire} from 'node:module';
import {readFileSync, writeFileSync, statSync} from 'node:fs';
import {createHash} from 'node:crypto';

const require = createRequire(import.meta.url);
const ffmpeg = 'node_modules/@ffmpeg-installer/linux-x64/ffmpeg';
const ffprobe = require('ffprobe-static').path;

const file = process.argv[2] ?? 'out/maskriver-demo-new.mp4';
const srt = process.argv[3] ?? 'subtitles/zh-CN.srt';

const probe = JSON.parse(
  execFileSync(
    ffprobe,
    ['-v', 'error', '-show_format', '-show_streams', '-of', 'json', file],
    {encoding: 'utf8'},
  ),
);
const v = probe.streams.find((s) => s.codec_type === 'video');
const a = probe.streams.find((s) => s.codec_type === 'audio');

const runFilter = (filter) => {
  try {
    const out = execFileSync(
      ffmpeg,
      ['-hide_banner', '-nostats', '-i', file, '-vf', filter, '-an', '-f', 'null', '-'],
      {encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe']},
    );
    return {ok: true, log: out};
  } catch (err) {
    const stderr = String(err.stderr ?? '');
    return {ok: false, log: stderr};
  }
};

// Decode every frame; any error is a hard failure.
let decodeOk = true;
let decodeErr = '';
try {
  execFileSync(ffmpeg, ['-v', 'error', '-i', file, '-f', 'null', '-'], {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  });
} catch (err) {
  decodeOk = false;
  decodeErr = String(err.stderr ?? '').slice(0, 800);
}

const black = runFilter('blackdetect=d=0.4:pix_th=0.06');
const freeze = runFilter('freezedetect=n=0.0005:d=2.0');
const parseIntervals = (log, re) => [...log.matchAll(re)].map((m) => Number(m[1]));

const blackIntervals = parseIntervals(black.log, /black_start:([\d.]+)/g);
const freezeStarts = parseIntervals(freeze.log, /freeze_start: *([\d.]+)/g);

const hash = createHash('sha256').update(readFileSync(file)).digest('hex');
const size = statSync(file).size;

const srtText = readFileSync(srt, 'utf8');
const srtCues = srtText.trim().split(/\n\n+/).length;
const lastStamp = [...srtText.matchAll(/ --> (\d\d):(\d\d):(\d\d),(\d\d\d)/g)].pop();
const lastCueEnd = lastStamp
  ? Number(lastStamp[1]) * 3600 +
    Number(lastStamp[2]) * 60 +
    Number(lastStamp[3]) +
    Number(lastStamp[4]) / 1000
  : 0;

const report = {
  file,
  bytes: size,
  mb: Number((size / 1024 / 1024).toFixed(2)),
  sha256: hash,
  durationSec: Number(Number(probe.format.duration).toFixed(3)),
  video: {
    codec: v.codec_name,
    profile: v.profile,
    width: v.width,
    height: v.height,
    fps: v.r_frame_rate,
    frames: Number(v.nb_frames ?? 0),
    pixelFormat: v.pix_fmt,
    bitrate: Number(v.bit_rate ?? 0),
  },
  audio: a
    ? {
        codec: a.codec_name,
        channels: a.channels,
        sampleRate: a.sample_rate,
        bitrate: Number(a.bit_rate ?? 0),
        durationSec: Number(Number(a.duration ?? 0).toFixed(3)),
      }
    : null,
  decode: {ok: decodeOk, error: decodeErr},
  blackFrameStarts: blackIntervals,
  freezeStarts,
  subtitles: {file: srt, cues: srtCues, lastCueEndSec: lastCueEnd},
  assertions: {
    durationInRange180to300: Number(probe.format.duration) >= 180 && Number(probe.format.duration) <= 300,
    is1080p: v.width === 1920 && v.height === 1080,
    is30fps: v.r_frame_rate === '30/1',
    isH264: v.codec_name === 'h264',
    hasAudio: Boolean(a),
    decodesCleanly: decodeOk,
    noBlackSegments: blackIntervals.length === 0,
    noLongFreezes: freezeStarts.length === 0,
  },
};

writeFileSync('out/qc-report.json', JSON.stringify(report, null, 2) + '\n');
console.log(JSON.stringify(report, null, 2));
