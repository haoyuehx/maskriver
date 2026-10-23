#!/usr/bin/env node
/**
 * Render review stills for one or more compositions in a single bundle.
 *
 * Usage:
 *   node scripts/stills.mjs '{"Dev-1":[60,330,640],"Dev-2":[200,600]}' [outDir]
 */
import {bundle} from '@remotion/bundler';
import {renderStill, selectComposition} from '@remotion/renderer';
import {mkdirSync} from 'node:fs';
import path from 'node:path';

const spec = JSON.parse(process.argv[2] ?? '{}');
const outDir = process.argv[3] ?? 'out/stills';
mkdirSync(outDir, {recursive: true});

const serveUrl = await bundle({
  entryPoint: path.resolve('src/index.ts'),
  onProgress: () => undefined,
});

for (const [compositionId, frames] of Object.entries(spec)) {
  const composition = await selectComposition({serveUrl, id: compositionId});
  for (const frame of frames) {
    const output = path.join(outDir, `${compositionId}-f${String(frame).padStart(4, '0')}.png`);
    await renderStill({
      composition,
      serveUrl,
      output,
      frame,
      chromiumOptions: {gl: 'swiftshader'},
      imageFormat: 'png',
    });
    console.log(`still ${compositionId} @ ${frame} -> ${output}`);
  }
}
