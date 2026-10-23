#!/usr/bin/env node
/**
 * Generate video/subtitles/zh-CN.srt from src/data/script.ts.
 *
 * The cue list is the single source of truth for the burned-in captions and the
 * narration timing, so the sidecar SRT can never drift from the picture.
 */
import {readFileSync, writeFileSync, mkdirSync} from 'node:fs';

const src = readFileSync('src/data/script.ts', 'utf8');
const cueRe = /\{id:\s*(\d+),\s*start:\s*([\d.]+),\s*end:\s*([\d.]+),\s*scene:\s*(\d+),\s*text:\s*'([^']*)'\}/g;

const cues = [];
let m;
while ((m = cueRe.exec(src))) {
  cues.push({id: Number(m[1]), start: Number(m[2]), end: Number(m[3]), scene: Number(m[4]), text: m[5]});
}
if (cues.length === 0) throw new Error('no cues parsed from src/data/script.ts');

const pad = (n, w = 2) => String(n).padStart(w, '0');
const stamp = (seconds) => {
  const ms = Math.round(seconds * 1000);
  const h = Math.floor(ms / 3600000);
  const mi = Math.floor((ms % 3600000) / 60000);
  const s = Math.floor((ms % 60000) / 1000);
  const rest = ms % 1000;
  return `${pad(h)}:${pad(mi)}:${pad(s)},${pad(rest, 3)}`;
};

// Guard: never emit overlapping cues.
let prevEnd = -1;
for (const c of cues) {
  if (c.start < prevEnd) throw new Error(`overlapping cue ${c.id} starts before previous end`);
  prevEnd = c.end;
}

const body = cues
  .map((c) => `${c.id}\n${stamp(c.start)} --> ${stamp(c.end)}\n${c.text}\n`)
  .join('\n');

mkdirSync('subtitles', {recursive: true});
writeFileSync('subtitles/zh-CN.srt', `${body}`, 'utf8');
writeFileSync(
  'src/data/cues.generated.json',
  JSON.stringify(cues, null, 2) + '\n',
  'utf8',
);
console.log(`wrote subtitles/zh-CN.srt (${cues.length} cues, last cue ends ${cues[cues.length - 1].end}s)`);
