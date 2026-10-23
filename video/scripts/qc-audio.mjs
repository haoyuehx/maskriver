#!/usr/bin/env node
/**
 * Technical audio QC for the rendered film.
 *
 * This script measures the audio stream and checks that every subtitle cue has a
 * speech segment inside its window. It cannot judge pronunciation, tone or
 * naturalness; those require a human listener and are recorded separately.
 *
 * Usage: node scripts/qc-audio.mjs out/maskriver-demo-new.mp4 subtitles/zh-CN.srt
 */
import {spawnSync} from 'node:child_process';
import {createRequire} from 'node:module';
import {readFileSync, writeFileSync} from 'node:fs';

const require = createRequire(import.meta.url);
const ffmpeg = 'node_modules/@ffmpeg-installer/linux-x64/ffmpeg';
const ffprobe = require('ffprobe-static').path;

const file = process.argv[2] ?? 'out/maskriver-demo-new.mp4';
const srt = process.argv[3] ?? 'subtitles/zh-CN.srt';

const run = (args) => {
  const r = spawnSync(ffmpeg, args, {encoding: 'utf8'});
  return {ok: r.status === 0, stderr: r.stderr ?? '', stdout: r.stdout ?? ''};
};
const parseTime = (stamp) => {
  const m = stamp.match(/(\d\d):(\d\d):(\d\d),(\d\d\d)/);
  return m ? Number(m[1]) * 3600 + Number(m[2]) * 60 + Number(m[3]) + Number(m[4]) / 1000 : NaN;
};
const probe = JSON.parse(
  spawnSync(ffprobe, ['-v', 'error', '-show_format', '-show_streams', '-of', 'json', file], {
    encoding: 'utf8',
  }).stdout,
);
const a = probe.streams.find((s) => s.codec_type === 'audio');
if (!a) throw new Error('no audio stream');
const duration = Number(probe.format.duration);

const ebur = run([
  '-hide_banner', '-nostats', '-i', file, '-map', '0:a',
  '-af', 'ebur128=peak=true', '-f', 'null', '-',
]);
const eburTail = ebur.stderr.slice(ebur.stderr.lastIndexOf('Summary:'));
const num = (re, text = eburTail) => {
  const m = text.match(re);
  return m ? Number(m[1]) : null;
};
const integratedLufs = num(/I:\s+(-?[\d.]+) LUFS/);
const lra = num(/LRA:\s+(-?[\d.]+) LU/);
const truePeak = num(/Peak:\s+(-?[\d.]+) dBFS/);

const vol = run(['-hide_banner', '-nostats', '-i', file, '-map', '0:a', '-af', 'volumedetect', '-f', 'null', '-']);
const meanVolumeDb = num(/mean_volume:\s+(-?[\d.]+) dB/, vol.stderr);
const maxVolumeDb = num(/max_volume:\s+(-?[\d.]+) dB/, vol.stderr);

const sil = run([
  '-hide_banner', '-nostats', '-i', file, '-map', '0:a',
  '-af', 'silencedetect=noise=-45dB:d=0.30', '-f', 'null', '-',
]);
const events = [...sil.stderr.matchAll(/silence_(start|end): ([\d.]+)/g)].map((m) => ({
  type: m[1], t: Number(m[2]),
}));
const speech = [];
let start = null;
for (const e of events) {
  if (e.type === 'start') {
    if (start !== null && e.t > start + 0.05) speech.push([start, e.t]);
    start = null;
  } else start = e.t;
}
if (start !== null && duration > start + 0.05) speech.push([start, duration]);

const srtCues = readFileSync(srt, 'utf8').trim().split(/\n\n+/).map((block) => {
  const lines = block.split('\n');
  const [from, to] = lines[1].split(' --> ').map(parseTime);
  return {text: lines.slice(2).join(' '), start: from, end: to};
});

const cues = srtCues.map((cue, i) => {
  const seg = speech[i] ?? null;
  const normalizedChars = cue.text.replace(/[\s，。、：；！？,.!?:;·—–\-]/g, '').length;
  return {
    id: i + 1,
    start: cue.start,
    end: cue.end,
    speechStart: seg ? Number(seg[0].toFixed(2)) : null,
    speechEnd: seg ? Number(seg[1].toFixed(2)) : null,
    speechDuration: seg ? Number((seg[1] - seg[0]).toFixed(2)) : null,
    startDelay: seg ? Number((seg[0] - cue.start).toFixed(2)) : null,
    tailMargin: seg ? Number((cue.end - seg[1]).toFixed(2)) : null,
    chars: normalizedChars,
    charsPerSec: seg ? Number((normalizedChars / (seg[1] - seg[0])).toFixed(2)) : null,
    insideCue: seg ? seg[0] >= cue.start - 0.15 && seg[1] <= cue.end + 0.15 : false,
  };
});
const aligned = cues.filter((c) => c.speechStart !== null);
const maxStartDelay = Math.max(...aligned.map((c) => c.startDelay));
const minTailMargin = Math.min(...aligned.map((c) => c.tailMargin));
const fastCues = cues.filter((c) => c.charsPerSec !== null && c.charsPerSec >= 7).map((c) => c.id);
const gaps = speech.slice(1).map((s, i) => Number((s[0] - speech[i][1]).toFixed(2)));
const maxGap = gaps.length ? Math.max(...gaps) : 0;

const report = {
  file,
  sha256: null,
  durationSec: Number(duration.toFixed(3)),
  audio: {
    codec: a.codec_name,
    profile: a.profile ?? null,
    sampleRate: Number(a.sample_rate),
    channels: a.channels,
    channelLayout: a.channel_layout ?? null,
    bitrate: Number(a.bit_rate ?? 0),
    durationSec: Number(Number(a.duration ?? 0).toFixed(3)),
    integratedLufs,
    loudnessRangeLu: lra,
    truePeakDbfs: truePeak,
    meanVolumeDb,
    maxVolumeDb,
  },
  speech: {
    segmentCount: speech.length,
    cueCount: srtCues.length,
    alignedCueCount: aligned.length,
    maxStartDelaySec: maxStartDelay,
    minTailMarginSec: minTailMargin,
    maxInterSpeechGapSec: maxGap,
    fastCueIds: fastCues,
    segments: speech.map(([s, e]) => [Number(s.toFixed(2)), Number(e.toFixed(2))]),
  },
  cues,
  checks: {
    hasAac48kStereo: a.codec_name === 'aac' && Number(a.sample_rate) === 48000 && a.channels === 2,
    audioDurationMatchesVideoWithin100ms: Math.abs(Number(a.duration) - duration) <= 0.1,
    loudnessWithinMinus20ToMinus12Lufs: integratedLufs >= -20 && integratedLufs <= -12,
    truePeakBelowMinus1Dbfs: truePeak <= -1,
    noSampleClipping: maxVolumeDb < 0,
    everyCueHasSpeech: speech.length === srtCues.length,
    everySpeechInsideItsCue: cues.every((c) => c.insideCue),
    noUnexpectedGapOver10s: maxGap <= 10,
  },
  limitations: [
    'Technical measurements only; no subjective listening was performed by this script.',
    'Pronunciation, prosody, naturalness, misreadings and sync with the picture require a human listener.',
  ],
};

const hash = spawnSync('sha256sum', [file], {encoding: 'utf8'}).stdout.split(/\s+/)[0];
report.sha256 = hash;
writeFileSync('out/audio-qc.json', JSON.stringify(report, null, 2) + '\n');
console.log(JSON.stringify(report, null, 2));
