import React from 'react';
import {interpolate} from 'remotion';
import {COLORS, FONT} from '../styles/tokens';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

/** EnvTest ring motif, generalised: progress ring + core. */
export const RingMark: React.FC<{
  size?: number;
  progress?: number;
  color?: string;
  strokeWidth?: number;
  core?: boolean;
  children?: React.ReactNode;
}> = ({size = 320, progress = 1, color = COLORS.cyan, strokeWidth = 4, core = true, children}) => {
  const r = size * 0.42;
  const c = 2 * Math.PI * r;
  const p = Math.max(0, Math.min(1, progress));
  return (
    <div style={{width: size, height: size, position: 'relative'}}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`}>
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={COLORS.lineBright}
          strokeWidth={1.5}
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - p)}
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
        />
        {core && <circle cx={size / 2} cy={size / 2} r={size * 0.1} fill={color} opacity={0.14} />}
      </svg>
      {children && (
        <div
          style={{
            position: 'absolute',
            inset: 0,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          {children}
        </div>
      )}
    </div>
  );
};

/** Three-state verdict chip: SENSITIVE / NOT_SENSITIVE / UNKNOWN. */
export const VerdictChip: React.FC<{
  kind?: 'SENSITIVE' | 'NOT_SENSITIVE' | 'UNKNOWN' | 'MASKED' | 'PASS';
  size?: number;
  progress?: number;
}> = ({kind = 'SENSITIVE', size = 18, progress = 1}) => {
  const map: Record<string, {fg: string; label: string}> = {
    SENSITIVE: {fg: COLORS.cyan, label: 'SENSITIVE'},
    NOT_SENSITIVE: {fg: COLORS.muted, label: 'NOT_SENSITIVE'},
    UNKNOWN: {fg: COLORS.amber, label: 'UNKNOWN'},
    MASKED: {fg: COLORS.teal, label: 'MASKED'},
    PASS: {fg: COLORS.teal, label: 'PASS'},
  };
  const {fg, label} = map[kind];
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 7,
        padding: `${Math.round(size * 0.26)}px ${Math.round(size * 0.6)}px`,
        borderRadius: 8,
        border: `1px solid ${fg}66`,
        background: `${fg}18`,
        color: fg,
        fontFamily: FONT.mono,
        fontSize: size,
        letterSpacing: 1.2,
        opacity: progress,
      }}
    >
      <span style={{width: 5, height: 5, borderRadius: 3, background: fg}} />
      {label}
    </span>
  );
};

/** Horizontal meter with a fixed cap marker (bounded budget metaphor). */
export const Meter: React.FC<{
  progress?: number;
  cap?: number;
  width?: number;
  height?: number;
  color?: string;
  capColor?: string;
}> = ({progress = 0, cap = 0.72, width = 200, height = 4, color = COLORS.cyan, capColor = COLORS.muted2}) => (
  <div style={{position: 'relative', width, height}}>
    <div style={{position: 'absolute', inset: 0, background: COLORS.line, borderRadius: height}} />
    <div
      style={{
        position: 'absolute',
        left: 0,
        top: 0,
        bottom: 0,
        width: Math.max(0, Math.min(1, progress)) * width,
        background: color,
        borderRadius: height,
      }}
    />
    <div
      style={{
        position: 'absolute',
        left: cap * width,
        top: -3,
        width: 1.5,
        height: height + 6,
        background: capColor,
      }}
    />
  </div>
);

/** Small CLI card: `$ maskriver ...` with per-line reveal. */
export const CommandCard: React.FC<{
  lines: string[];
  revealAt: number[];
  frame: number;
  width?: number;
  size?: number;
  title?: string;
  accent?: string;
}> = ({lines, revealAt, frame, width = 1320, size = 22, title = 'shell', accent = COLORS.teal}) => (
  <div
    style={{
      width,
      borderRadius: 12,
      border: `1px solid ${COLORS.line}`,
      background: 'linear-gradient(180deg, #0a1a2a 0%, #081523 100%)',
      overflow: 'hidden',
    }}
  >
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '10px 18px',
        borderBottom: `1px solid ${COLORS.line}`,
        fontFamily: FONT.mono,
        fontSize: 15,
        letterSpacing: 1.4,
        color: COLORS.muted2,
      }}
    >
      <span style={{width: 8, height: 8, borderRadius: 4, background: accent}} />
      {title}
    </div>
    <div style={{padding: '18px 22px', display: 'flex', flexDirection: 'column', gap: 8}}>
      {lines.map((line, i) => {
        const p = interpolate(frame, [revealAt[i] ?? 0, (revealAt[i] ?? 0) + 10], [0, 1], clamp);
        return (
          <div
            key={i}
            style={{
              fontFamily: FONT.mono,
              fontSize: size,
              lineHeight: 1.5,
              color: COLORS.text,
              opacity: p,
              transform: `translateX(${(1 - p) * -10}px)`,
              whiteSpace: 'nowrap',
              overflow: 'hidden',
            }}
          >
            <span style={{color: COLORS.cyan}}>$ </span>
            <span style={{color: COLORS.muted}}>{line}</span>
          </div>
        );
      })}
    </div>
  </div>
);

/** Section title used inside scenes (below the scene headline). */
export const SceneTitle: React.FC<{
  kicker?: string;
  title: React.ReactNode;
  lead?: React.ReactNode;
  delay?: number;
  align?: 'left' | 'center';
  titleSize?: number;
}> = ({kicker, title, lead, align = 'left', titleSize = 60}) => (
  <div style={{textAlign: align, opacity: 1}}>
    {kicker && (
      <div
        style={{
          fontFamily: FONT.mono,
          fontSize: 19,
          letterSpacing: 3.2,
          color: COLORS.cyan,
          marginBottom: 16,
        }}
      >
        {kicker}
      </div>
    )}
    <div
      style={{
        fontFamily: FONT.sans,
        fontSize: titleSize,
        fontWeight: 700,
        lineHeight: 1.18,
        letterSpacing: 0.5,
        color: COLORS.text,
      }}
    >
      {title}
    </div>
    {lead && (
      <div
        style={{
          marginTop: 18,
          fontFamily: FONT.sans,
          fontSize: 28,
          lineHeight: 1.6,
          color: COLORS.muted,
          maxWidth: 1180,
          marginLeft: align === 'center' ? 'auto' : undefined,
          marginRight: align === 'center' ? 'auto' : undefined,
        }}
      >
        {lead}
      </div>
    )}
  </div>
);
