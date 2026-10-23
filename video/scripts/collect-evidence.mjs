#!/usr/bin/env node
/**
 * Collect delivery evidence into docs/evidence/:
 *  - per-scene stills and one contact sheet from the final MP4
 *  - the QC JSON produced by scripts/qc-video.mjs
 *
 * Usage: node scripts/collect-evidence.mjs [video]
 */
import {execFileSync} from 'node:child_process';
import {createRequire} from 'node:module';
import {copyFileSync, mkdirSync, readFileSync} from 'node:fs';
import path from 'node:path';
import sharp from 'sharp';

const require = createRequire(import.meta.url);
const ffmpeg = 'node_modules/@ffmpeg-installer/linux-x64/ffmpeg';
const video = process.argv[2] ?? 'out/maskriver-demo-new.mp4';
const outDir = 'docs/evidence';
mkdirSync(outDir, {recursive: true});

// Beat list: one frame per narrative beat (timestamps in seconds).
const beats = [
  [3, 's1-01-database'],
  [12, 's1-02-field-closeup'],
  [20, 's1-03-dataflow'],
  [23, 's1-04-brand'],
  [31, 's2-01-scan'],
  [38, 's2-02-preview'],
  [44, 's2-03-approve'],
  [51, 's2-04-apply'],
  [57, 's2-05-validate'],
  [62, 's2-06-recap'],
  [70, 's3-01-datasource'],
  [82, 's3-02-scan'],
  [96, 's3-03-preview'],
  [114, 's3-04-approve'],
  [130, 's3-05-apply'],
  [144, 's3-06-validate'],
  [156, 's4-01-title'],
  [176, 's4-02-pipeline'],
  [188, 's4-03-transaction'],
  [195, 's4-04-bounds'],
  [207, 's5-01-timeline'],
  [224, 's5-02-fault'],
  [236, 's5-03-resume'],
  [243, 's5-04-pass'],
  [249, 's6-01-pillars'],
  [256, 's6-02-lockup'],
];

const files = [];
for (const [t, name] of beats) {
  const file = path.join(outDir, `${String(t).padStart(3, '0')}s-${name}.png`);
  execFileSync(ffmpeg, ['-y', '-v', 'error', '-ss', String(t), '-i', video, '-frames:v', '1', file]);
  files.push({file, t});
}

// contact sheet (4 columns)
const cellW = 480;
const cellH = 270;
const cols = 4;
const rows = Math.ceil(files.length / cols);
const composites = [];
for (const [i, f] of files.entries()) {
  const buf = await sharp(f.file).resize(cellW, cellH, {fit: 'cover'}).toBuffer();
  composites.push({input: buf, left: (i % cols) * cellW, top: Math.floor(i / cols) * cellH});
}
await sharp({create: {width: cols * cellW, height: rows * cellH, channels: 3, background: '#020812'}})
  .composite(composites)
  .png()
  .toFile(path.join(outDir, 'contact-sheet-final.png'));

copyFileSync('out/qc-report.json', path.join(outDir, 'qc-report.json'));

const qc = JSON.parse(readFileSync('out/qc-report.json', 'utf8'));
console.log(`stills: ${files.length}, contact sheet: docs/evidence/contact-sheet-final.png`);
console.log(`video: ${qc.durationSec}s ${qc.video.width}x${qc.video.height} ${qc.video.fps} ${qc.video.codec} sha256=${qc.sha256}`);
