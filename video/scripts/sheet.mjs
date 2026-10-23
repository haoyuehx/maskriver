#!/usr/bin/env node
/** Build a contact sheet from a directory of review stills. */
import sharp from 'sharp';
import {readdirSync} from 'node:fs';
import path from 'node:path';

const dir = process.argv[2] ?? 'out/stills';
const cols = Number(process.argv[3] ?? 3);
const prefix = process.argv[4] ?? '';
const cellW = 640;
const cellH = 360;
const files = readdirSync(dir)
  .filter((f) => f.endsWith('.png') && f.startsWith(prefix) && !f.includes('sheet'))
  .sort();
const rows = Math.ceil(files.length / cols);
const comps = [];
for (const [i, f] of files.entries()) {
  const buf = await sharp(path.join(dir, f)).resize(cellW, cellH, {fit: 'cover'}).toBuffer();
  comps.push({input: buf, left: (i % cols) * cellW, top: Math.floor(i / cols) * cellH});
}
const out = path.join(dir, 'sheet.png');
await sharp({create: {width: cols * cellW, height: rows * cellH, channels: 3, background: '#020812'}})
  .composite(comps)
  .png()
  .toFile(out);
console.log(`${files.length} stills -> ${out}`);
