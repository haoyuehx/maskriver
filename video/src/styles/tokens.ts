/**
 * Design tokens derived from the MaskRiver website visual system
 * (website/src/styles/global.css) and refined for motion.
 * See video/docs/visual-direction.md for the specification.
 */

export const COLORS = {
  bgDeep: '#020812',
  bg: '#06111f',
  bgSoft: '#0a1a2a',
  panel: '#0b1b2c',
  panel2: '#0e2235',
  line: '#1d3d55',
  lineBright: '#2b5874',
  text: '#ecf8ff',
  muted: '#93aec1',
  muted2: '#6f8ca1',
  cyan: '#63dcf3',
  cyanDeep: '#1e8fb0',
  teal: '#58e0ba',
  purple: '#b49cff',
  amber: '#ff996a',
  danger: '#ff7b6b',
  dangerDeep: '#a8321f',
} as const;

export const FONT = {
  sans: "'Noto Sans SC', 'Noto Sans CJK SC', sans-serif",
  mono: "'JetBrains Mono', 'Noto Sans SC', monospace",
} as const;

export const TYPE = {
  hero: 92,
  h1: 64,
  h2: 46,
  h3: 34,
  lead: 30,
  body: 24,
  small: 20,
  micro: 16,
  mono: 22,
  caption: 32,
} as const;

/** 16:9 frame geometry. */
export const FRAME = {
  width: 1920,
  height: 1080,
  fps: 30,
  marginX: 132,
  marginY: 84,
  safeBottom: 168,
  contentTop: 132,
  contentBottom: 912,
} as const;

/** Shared spring presets so every scene accelerates the same way. */
export const SPRING = {
  soft: {damping: 200, mass: 0.9, stiffness: 110},
  snappy: {damping: 26, mass: 0.7, stiffness: 150},
  crisp: {damping: 20, mass: 0.6, stiffness: 190},
  settle: {damping: 32, mass: 1, stiffness: 92},
} as const;

export const DURATION = {
  /** Scene boundaries in frames at 30fps. */
  scene1: 750,
  scene2: 1200,
  scene3: 2550,
  scene4: 1500,
  scene5: 1350,
  scene6: 450,
  total: 7800,
} as const;

export const SCENE_START = {
  scene1: 0,
  scene2: 750,
  scene3: 1950,
  scene4: 4500,
  scene5: 6000,
  scene6: 7350,
} as const;

/** 8px spacing scale (visual-direction.md §1). */
export const SPACING = {
  s1: 8,
  s2: 16,
  s3: 24,
  s4: 32,
  s5: 40,
  s6: 48,
  s8: 64,
  s10: 80,
  s12: 96,
  s16: 128,
} as const;

export const RADIUS = {
  sm: 8,
  md: 12,
  lg: 16,
  xl: 24,
  pill: 999,
} as const;

export const STROKE = {
  hairline: 1,
  line: 1.5,
  emph: 2,
  icon: 3,
  ring: 4,
} as const;

export const OPACITY = {
  grid: 0.15,
  dim: 0.55,
  soft: 0.8,
  full: 1,
  sub: 0.92,
} as const;

/** Named stage regions from the layout grid (§4). */
export const REGION = {
  eyebrow: {x: 132, y: 84, w: 560, h: 32},
  disclaimer: {x: 1452, y: 84, w: 336, h: 36},
  title: {x: 132, y: 150, w: 1656, h: 130},
  stage: {x: 132, y: 320, w: 1656, h: 560},
  metrics: {x: 132, y: 820, w: 1656, h: 80},
  subtitle: {x: 0, y: 912, w: 1920, h: 168},
} as const;
