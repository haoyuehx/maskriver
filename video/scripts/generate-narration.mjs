#!/usr/bin/env node
/**
 * Generate the Simplified Chinese narration track with the local Piper TTS voice
 * `zh_CN-huayan-medium`, fit each line into its subtitle cue window, and write
 * public/audio/narration.m4a (AAC, 48 kHz mono).
 *
 * Requirements (kept outside the repository):
 *   PYTHONPATH=/home/admin/.cache/pylibs python3 -m piper ...
 *   voice: /home/admin/.cache/tts/zh_CN-huayan-medium.onnx (+ .json)
 *
 * Usage: node scripts/generate-narration.mjs
 */
import {execFileSync} from 'node:child_process';
import {createRequire} from 'node:module';
import {existsSync, mkdirSync, readFileSync, rmSync, writeFileSync} from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const ffmpeg = 'node_modules/@ffmpeg-installer/linux-x64/ffmpeg';
const ffprobe = require('ffprobe-static').path;

const PIPER_PY = process.env.PIPER_PYTHON ?? 'python3';
const PYLIB = process.env.PIPER_PYLIB ?? '/home/admin/.cache/pylibs';
const VOICE = process.env.PIPER_VOICE ?? '/home/admin/.cache/tts/zh_CN-huayan-medium.onnx';
const VOICE_CFG = `${VOICE}.json`;
const WORK = 'out/narration';
const OUT = 'public/audio/narration.mp3';
const TOTAL = 260;

if (!existsSync(VOICE)) throw new Error(`voice model missing: ${VOICE}`);
mkdirSync(WORK, {recursive: true});
mkdirSync(path.dirname(OUT), {recursive: true});

// ---- read cues from the single source of truth -------------------------------
const src = readFileSync('src/data/script.ts', 'utf8');
const cueRe = /\{id:\s*(\d+),\s*start:\s*([\d.]+),\s*end:\s*([\d.]+),\s*scene:\s*(\d+),\s*text:\s*'([^']*)'\}/g;
const cues = [];
let m;
while ((m = cueRe.exec(src))) {
  cues.push({id: Number(m[1]), start: Number(m[2]), end: Number(m[3]), text: m[5]});
}

const probeDuration = (file) =>
  Number(
    execFileSync(ffprobe, ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', file], {
      encoding: 'utf8',
    }).trim(),
  );

const synth = (text, lengthScale, outFile) => {
  const txt = `${outFile}.txt`;
  writeFileSync(txt, text, 'utf8');
  execFileSync(
    PIPER_PY,
    ['-m', 'piper', '-m', VOICE, '-c', VOICE_CFG, '-i', txt, '-f', outFile, '--length-scale', String(lengthScale)],
    {env: {...process.env, PYTHONPATH: PYLIB}, stdio: ['ignore', 'ignore', 'pipe']},
  );
  return probeDuration(outFile);
};

const fitted = [];
for (const cue of cues) {
  const window = cue.end - cue.start;
  const target = Math.max(1.2, window - 0.25); // leave a small breath before the next line
  let raw = path.join(WORK, `raw-${cue.id}.wav`);
  let duration = synth(cue.text, 1.0, raw);

  if (duration > target) {
    // Re-synthesise slower/faster rather than time-stretching when possible.
    const scale = Math.max(0.82, Math.min(1.3, (1.0 * target) / duration));
    const retry = path.join(WORK, `fit-${cue.id}.wav`);
    duration = synth(cue.text, scale, retry);
    raw = retry;
  }

  // Final safety: trim to the window and pad with silence to the exact length.
  const out = path.join(WORK, `cue-${cue.id}.wav`);
  const tempo = duration > target ? Math.min(1.18, duration / target) : 1;
  execFileSync(ffmpeg, [
    '-y',
    '-v',
    'error',
    '-i',
    raw,
    '-af',
    `atempo=${tempo.toFixed(3)},aresample=48000`, '-ac','1','-ar','48000','-c:a','pcm_s16le',
    '-t',
    String(Math.min(window, duration / tempo).toFixed(3)),
    out,
  ]);
  const finalDur = probeDuration(out);
  fitted.push({...cue, file: out, duration: finalDur, tempo, window});
  console.log(
    `cue ${String(cue.id).padStart(2)} ${cue.start.toFixed(1)}s window=${window.toFixed(1)}s audio=${finalDur.toFixed(2)}s tempo=${tempo.toFixed(2)}`,
  );
}

// ---- assemble the timeline by concatenating silence and cue audio -----------
// Cues are strictly ordered and non-overlapping, so a concat of gaps + lines
// reproduces the narration timeline exactly without mixing attenuation.
const parts = [];
let cursor = 0;
const silence = (seconds, file) => {
  execFileSync(ffmpeg, [
    '-y', '-v', 'error',
    '-f', 'lavfi',
    '-t', seconds.toFixed(3),
    '-i', 'anullsrc=r=48000:cl=mono',
    '-ac', '1', '-ar', '48000', '-c:a', 'pcm_s16le',
    file,
  ]);
  return file;
};
fitted.forEach((c) => {
  if (c.start > cursor + 0.01) {
    parts.push(silence(c.start - cursor, path.join(WORK, `gap-${c.id}.wav`)));
    cursor = c.start;
  }
  parts.push(c.file);
  cursor += c.duration;
});
if (TOTAL > cursor) parts.push(silence(TOTAL - cursor, path.join(WORK, 'gap-tail.wav')));

const listFile = path.join(WORK, 'concat.txt');
writeFileSync(listFile, parts.map((p) => `file '${path.resolve(p)}'`).join('\n') + '\n');

execFileSync(ffmpeg, [
  '-y', '-v', 'error',
  '-f', 'concat', '-safe', '0', '-i', listFile,
  '-af', 'loudnorm=I=-16:TP=-1.5:LRA=11',
  '-c:a', OUT.endsWith('.mp3') ? 'libmp3lame' : 'aac', '-b:a', '128k', '-ar', '48000', '-ac', '1',
  '-t', String(TOTAL),
  OUT,
]);

const total = probeDuration(OUT);
writeFileSync(
  path.join(WORK, 'narration.json'),
  JSON.stringify({voice: path.basename(VOICE), out: OUT, duration: total, cues: fitted}, null, 2),
);
rmSync(path.join(WORK, 'raw-0.wav'), {force: true});
console.log(`wrote ${OUT} (${total.toFixed(2)}s, ${fitted.length} lines)`);
