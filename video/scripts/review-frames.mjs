#!/usr/bin/env node
/**
 * Extract review frames from a rendered MP4 and build a contact sheet.
 *
 * Usage:
 *   node scripts/review-frames.mjs <video.mp4> <outDir> <t1,t2,...>
 *
 * Uses the ffmpeg / ffprobe binaries installed as devDependencies, so the
 * review workflow needs no system ffmpeg.
 */
import {execFileSync} from 'node:child_process';
import {createRequire} from 'node:module';
import {mkdirSync, writeFileSync} from 'node:fs';
import path from 'node:path';
import sharp from 'sharp';

const require = createRequire(import.meta.url);
const ffmpeg = path.resolve('node_modules/@ffmpeg-installer/linux-x64/ffmpeg');
const ffprobe = require('ffprobe-static').path;

const [, , video, outDir = 'out/review', timesArg = '0,3,6,9,12,14.5'] = process.argv;
if (!video) {
  console.error('usage: node scripts/review-frames.mjs <video.mp4> [outDir] [times]');
  process.exit(2);
}
const times = timesArg.split(',').map((t) => Number(t.trim())).filter((n) => !Number.isNaN(n));

mkdirSync(outDir, {recursive: true});

const probe = JSON.parse(
  execFileSync(
    ffprobe,
    ['-v', 'error', '-show_entries', 'format=duration', '-show_entries', 'stream=width,height,r_frame_rate,codec_name', '-of', 'json', video],
    {encoding: 'utf8'},
  ),
);
const duration = Number(probe.format?.duration ?? 0);
const videoStream = (probe.streams ?? []).find((s) => s.width);
console.log(`probe: ${duration.toFixed(2)}s ${videoStream.width}x${videoStream.height} ${videoStream.r_frame_rate}`);

const frames = [];
for (const [i, t] of times.entries()) {
  const file = path.join(outDir, `t${String(t).replace('.', '_')}s.png`);
  execFileSync(ffmpeg, ['-y', '-v', 'error', '-ss', String(t), '-i', video, '-frames:v', '1', file]);
  frames.push({file, t, index: i});
}
console.log(`extracted ${frames.length} frames into ${outDir}`);

// Contact sheet: 3 columns, each cell 640x360 with a label strip.
const cols = 3;
const cellW = 640;
const cellH = 360;
const labelH = 34;
const rows = Math.ceil(frames.length / cols);
const sheetW = cols * cellW;
const sheetH = rows * (cellH + labelH);

const composites = [];
for (const [i, f] of frames.entries()) {
  const x = (i % cols) * cellW;
  const y = Math.floor(i / cols) * (cellH + labelH);
  const buf = await sharp(f.file).resize(cellW, cellH, {fit: 'cover'}).toBuffer();
  composites.push({input: buf, left: x, top: y + labelH});
  const label = Buffer.from(
    `<svg width="${cellW}" height="${labelH}"><rect width="${cellW}" height="${labelH}" fill="#0b1b2c"/><text x="14" y="23" font-family="monospace" font-size="16" fill="#93aec1">${f.t.toFixed(1)}s · frame ${Math.round(f.t * 30)}</text></svg>`,
  );
  composites.push({input: label, left: x, top: y});
}

const sheetPath = path.join(outDir, 'contact-sheet.png');
await sharp({create: {width: sheetW, height: sheetH, channels: 3, background: '#020812'}})
  .composite(composites)
  .png()
  .toFile(sheetPath);
console.log(`contact sheet: ${sheetPath}`);

// Machine-readable frame statistics used by the QC report (mean luma + colour).
const stats = [];
for (const f of frames) {
  const s = await sharp(f.file).stats();
  stats.push({
    t: f.t,
    mean: s.channels.map((c) => Number(c.mean.toFixed(2))),
    stdev: s.channels.map((c) => Number(c.stdev.toFixed(2))),
  });
}
writeFileSync(path.join(outDir, 'frame-stats.json'), JSON.stringify({video, duration, stats}, null, 2));
console.log('wrote frame-stats.json');
