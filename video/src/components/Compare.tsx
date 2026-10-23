import React from 'react';
import {interpolate} from 'remotion';
import {COLORS, FONT} from '../styles/tokens';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

const mono = (size: number, color: string): React.CSSProperties => ({
  fontFamily: FONT.mono,
  fontSize: size,
  color,
  letterSpacing: 0.3,
  whiteSpace: 'nowrap',
});

type Cell = {text: string; tone?: 'plain' | 'source' | 'masked' | 'unknown'};

/** Compact flat table used for before/after comparisons and plan views. */
export const MiniTable: React.FC<{
  title: string;
  columns: string[];
  rows: Cell[][];
  colWidths?: number[];
  width?: number;
  titleColor?: string;
  visibleRows?: number;
  rowHeight?: number;
  fontSize?: number;
  frame?: number;
  enter?: number;
  opacity?: number;
}> = ({
  title,
  columns,
  rows,
  colWidths,
  width = 640,
  titleColor = COLORS.amber,
  visibleRows = rows.length,
  rowHeight = 46,
  fontSize = 19,
  enter = 0,
  frame = 0,
  opacity = 1,
}) => {
  const widths = colWidths ?? columns.map(() => 1 / columns.length);
  const total = widths.reduce((a, b) => a + b, 0);
  return (
    <div
      style={{
        width,
        borderRadius: 12,
        border: `1px solid ${COLORS.line}`,
        background: 'linear-gradient(180deg, #0c1e30, #091625)',
        overflow: 'hidden',
        opacity,
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          padding: '10px 16px',
          borderBottom: `1px solid ${COLORS.line}`,
          ...mono(14, titleColor),
          letterSpacing: 1.6,
        }}
      >
        <span style={{width: 6, height: 6, borderRadius: 3, background: titleColor}} />
        {title}
      </div>
      <div style={{display: 'flex', padding: '8px 16px', borderBottom: `1px solid ${COLORS.line}66`}}>
        {columns.map((c, i) => (
          <div
            key={c}
            style={{
              width: `${(widths[i] / total) * 100}%`,
              ...mono(13, COLORS.muted2),
              letterSpacing: 1.1,
            }}
          >
            {c}
          </div>
        ))}
      </div>
      {rows.map((row, ri) => {
        const p = interpolate(frame, [enter + ri * 6, enter + ri * 6 + 14], [0, 1], clamp);
        return (
          <div
            key={ri}
            style={{
              display: 'flex',
              height: rowHeight,
              alignItems: 'center',
              padding: '0 16px',
              borderBottom: ri === rows.length - 1 ? 'none' : `1px solid ${COLORS.line}22`,
              opacity: ri < visibleRows ? p : 0,
            }}
          >
            {row.map((cell, ci) => {
              const color =
                cell.tone === 'source'
                  ? COLORS.amber
                  : cell.tone === 'masked'
                    ? COLORS.teal
                    : cell.tone === 'unknown'
                      ? COLORS.amber
                      : COLORS.text;
              return (
                <div
                  key={ci}
                  style={{
                    width: `${(widths[ci] / total) * 100}%`,
                    ...mono(fontSize, color),
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                  }}
                >
                  {cell.text}
                </div>
              );
            })}
          </div>
        );
      })}
    </div>
  );
};

/** Before/after split with a centre arrow column. */
export const BeforeAfterSplit: React.FC<{
  beforeTitle?: string;
  afterTitle?: string;
  columns: string[];
  before: Cell[][];
  after: Cell[][];
  frame: number;
  enter?: number;
  x?: number;
  y?: number;
  tableW?: number;
  gap?: number;
  swapRows?: number[];
  swapAt?: number[];
  opacity?: number;
}> = ({
  beforeTitle = 'SOURCE · 原始副本',
  afterTitle = 'TARGET · 脱敏副本',
  columns,
  before,
  after,
  frame,
  enter = 0,
  x = 132,
  y = 380,
  tableW = 760,
  gap = 64,
  swapRows = [],
  swapAt = [],
  opacity = 1,
}) => {
  const arrowP = interpolate(frame, [enter + 16, enter + 34], [0, 1], clamp);
  return (
    <>
      <div style={{position: 'absolute', left: x, top: y, opacity}}>
        <MiniTable
          title={beforeTitle}
          columns={columns}
          rows={before}
          width={tableW}
          titleColor={COLORS.amber}
          frame={frame}
          enter={enter}
        />
      </div>
      <div
        style={{
          position: 'absolute',
          left: x + tableW + (gap - 56) / 2,
          top: y + 150,
          width: 56,
          opacity: arrowP,
        }}
      >
        <svg width={56} height={40} viewBox="0 0 56 40">
          <path
            d="M4 20 H44"
            stroke={COLORS.cyan}
            strokeWidth={2}
            strokeLinecap="round"
            strokeDasharray={40}
            strokeDashoffset={40 * (1 - arrowP)}
          />
          <path
            d="M36 13 L45 20 L36 27"
            fill="none"
            stroke={COLORS.cyan}
            strokeWidth={2}
            strokeLinecap="round"
            strokeLinejoin="round"
            opacity={arrowP > 0.8 ? 1 : 0}
          />
        </svg>
      </div>
      <div style={{position: 'absolute', left: x + tableW + gap, top: y, opacity}}>
        <MiniTable
          title={afterTitle}
          columns={columns}
          rows={after}
          width={tableW}
          titleColor={COLORS.teal}
          frame={frame}
          enter={enter + 10}
        />
        {/* per-row swap highlights */}
        {swapRows.map((r, i) => {
          const p = interpolate(frame, [swapAt[i] ?? 0, (swapAt[i] ?? 0) + 18], [0, 1], clamp);
          return (
            <div
              key={r}
              style={{
                position: 'absolute',
                left: 0,
                right: 0,
                top: 40 + r * 46,
                height: 46,
                background: `rgba(88,224,186,${0.16 * p})`,
                borderLeft: `2px solid ${COLORS.teal}`,
                opacity: p,
              }}
            />
          );
        })}
      </div>
    </>
  );
};

/** Numeric stat card (bottom metrics row). */
export const StatCard: React.FC<{
  label: string;
  value: string;
  unit?: string;
  x: number;
  y: number;
  w?: number;
  h?: number;
  frame: number;
  enter?: number;
  color?: string;
}> = ({label, value, unit, x, y, w = 384, h = 110, frame, enter = 0, color = COLORS.cyan}) => {
  const p = interpolate(frame, [enter, enter + 18], [0, 1], clamp);
  return (
    <div
      style={{
        position: 'absolute',
        left: x,
        top: y,
        width: w,
        height: h,
        padding: '16px 20px',
        borderRadius: 12,
        border: `1px solid ${COLORS.line}`,
        background: 'rgba(11,27,44,0.82)',
        opacity: p,
        transform: `translateY(${(1 - p) * 14}px)`,
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'space-between',
      }}
    >
      <span style={{fontFamily: FONT.sans, fontSize: 18, color: COLORS.muted2, letterSpacing: 0.6}}>
        {label}
      </span>
      <span style={{display: 'flex', alignItems: 'baseline', gap: 6}}>
        <span style={{fontFamily: FONT.mono, fontSize: 34, fontWeight: 700, color}}>{value}</span>
        {unit && <span style={{fontFamily: FONT.sans, fontSize: 17, color: COLORS.muted}}>{unit}</span>}
      </span>
    </div>
  );
};

/** Fault overlay: dim veil + danger bar. No blur, no shake. */
export const FaultOverlay: React.FC<{
  frame: number;
  at: number;
  /** Frame at which the fault state starts lifting (resume begins). */
  until?: number;
  label?: string;
}> = ({frame, at, until, label = 'TASK INTERRUPTED'}) => {
  const lift = until ?? Infinity;
  const out = interpolate(frame, [lift, lift + 14], [1, 0], clamp);
  const veil = interpolate(frame, [at, at + 6], [0, 0.55], clamp) * out;
  const bar = interpolate(frame, [at + 6, at + 18], [0, 1], clamp);
  const labelP = interpolate(frame, [at + 14, at + 24], [0, 1], clamp) * out;
  if (frame < at || out <= 0) return null;
  return (
    <>
      <div style={{position: 'absolute', inset: 0, background: COLORS.bgDeep, opacity: veil}} />
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 452,
          width: 1920,
          height: 8,
          background: `linear-gradient(90deg, ${COLORS.dangerDeep}, ${COLORS.danger})`,
          transform: `scaleX(${bar})`,
          transformOrigin: 'left center',
          opacity: out,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: 132,
          top: 486,
          display: 'flex',
          alignItems: 'center',
          gap: 14,
          padding: '8px 18px',
          borderRadius: 8,
          background: 'rgba(6,14,26,0.9)',
          border: `1px solid ${COLORS.danger}55`,
          opacity: labelP,
          fontFamily: FONT.mono,
          fontSize: 24,
          letterSpacing: 6,
          color: COLORS.danger,
        }}
      >
        <span style={{width: 10, height: 10, borderRadius: 5, background: COLORS.danger}} />
        {label}
      </div>
    </>
  );
};
