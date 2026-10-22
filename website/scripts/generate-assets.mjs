import sharp from 'sharp';
import { readFile } from 'node:fs/promises';

for (const [source, target, width, height] of [
  ['public/poster.svg', 'public/media/poster.png', 1920, 1080],
  ['public/og-image.svg', 'public/og-image.png', 1200, 630],
]) {
  const svg = await readFile(source);
  await sharp(svg, { density: 144 }).resize(width, height).png({ compressionLevel: 9 }).toFile(target);
  console.log(`generated ${target}`);
}
