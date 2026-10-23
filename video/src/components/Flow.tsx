import React from 'react';
import {COLORS} from '../styles/tokens';

export type Pt = {x: number; y: number};

/** Cubic bezier point, evaluated analytically so it is frame-deterministic. */
export const bezier = (p0: Pt, p1: Pt, p2: Pt, p3: Pt, t: number): Pt => {
  const u = 1 - t;
  const a = u * u * u;
  const b = 3 * u * u * t;
  const c = 3 * u * t * t;
  const d = t * t * t;
  return {
    x: a * p0.x + b * p1.x + c * p2.x + d * p3.x,
    y: a * p0.y + b * p1.y + c * p2.y + d * p3.y,
  };
};

/** Polyline sampling used for stroke length and packet placement. */
const sampleCubic = (p0: Pt, p1: Pt, p2: Pt, p3: Pt, steps = 48): Pt[] => {
  const out: Pt[] = [];
  for (let i = 0; i <= steps; i++) out.push(bezier(p0, p1, p2, p3, i / steps));
  return out;
};

const polyLength = (pts: Pt[]): number => {
  let len = 0;
  for (let i = 1; i < pts.length; i++) {
    len += Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y);
  }
  return len;
};

export const cubicPath = (p0: Pt, p1: Pt, p2: Pt, p3: Pt): string =>
  `M ${p0.x} ${p0.y} C ${p1.x} ${p1.y}, ${p2.x} ${p2.y}, ${p3.x} ${p3.y}`;

type FlowCurveProps = {
  points: [Pt, Pt, Pt, Pt];
  /** 0..1 draw progress. */
  progress?: number;
  color?: string;
  width?: number;
  opacity?: number;
  dashed?: boolean;
  arrow?: boolean;
};

/**
 * A data-route: bezier curve that can be drawn progressively and that
 * exposes its geometry for packets travelling along it.
 */
export const FlowCurve: React.FC<FlowCurveProps> = ({
  points,
  progress = 1,
  color = COLORS.cyan,
  width = 2,
  opacity = 0.8,
  dashed = false,
  arrow = true,
}) => {
  const [p0, p1, p2, p3] = points;
  const pts = sampleCubic(p0, p1, p2, p3);
  const len = polyLength(pts);
  const end = bezier(p0, p1, p2, p3, 0.985);
  const before = bezier(p0, p1, p2, p3, 0.95);
  const angle = Math.atan2(end.y - before.y, end.x - before.x) * (180 / Math.PI);
  const clamped = Math.max(0, Math.min(1, progress));

  return (
    <g opacity={opacity}>
      <path
        d={cubicPath(p0, p1, p2, p3)}
        fill="none"
        stroke={color}
        strokeWidth={width}
        strokeLinecap="round"
        strokeDasharray={dashed ? '10 12' : len}
        strokeDashoffset={dashed ? 0 : len * (1 - clamped)}
      />
      {arrow && clamped > 0.92 && (
        <path
          d="M -7 -5.5 L 0 0 L -7 5.5"
          fill="none"
          stroke={color}
          strokeWidth={width}
          strokeLinecap="round"
          strokeLinejoin="round"
          transform={`translate(${end.x} ${end.y}) rotate(${angle})`}
        />
      )}
    </g>
  );
};

type FlowPacketProps = {
  points: [Pt, Pt, Pt, Pt];
  /** 0..1 position along the curve. */
  at: number;
  color?: string;
  size?: number;
  opacity?: number;
  tail?: boolean;
};

/** A single data packet (dot + short tail) riding a curve. */
export const FlowPacket: React.FC<FlowPacketProps> = ({
  points,
  at,
  color = COLORS.cyan,
  size = 5,
  opacity = 1,
  tail = true,
}) => {
  const t = Math.max(0, Math.min(1, at));
  const pos = bezier(...points, t);
  const prev = bezier(...points, Math.max(0, t - 0.05));
  return (
    <g opacity={opacity}>
      {tail && (
        <line
          x1={prev.x}
          y1={prev.y}
          x2={pos.x}
          y2={pos.y}
          stroke={color}
          strokeWidth={size * 0.9}
          strokeLinecap="round"
          opacity={0.35}
        />
      )}
      <circle cx={pos.x} cy={pos.y} r={size} fill={color} />
      <circle cx={pos.x} cy={pos.y} r={size * 2.6} fill={color} opacity={0.14} />
    </g>
  );
};

/** Convenience: N evenly phased packets over one curve. */
export const FlowPackets: React.FC<{
  points: [Pt, Pt, Pt, Pt];
  count?: number;
  phase: number;
  color?: string;
  size?: number;
  opacity?: number;
}> = ({points, count = 3, phase, color = COLORS.cyan, size = 5, opacity = 1}) => (
  <>
    {Array.from({length: count}).map((_, i) => {
      const raw = (phase + i / count) % 1;
      return (
        <FlowPacket
          key={i}
          points={points}
          at={raw}
          color={color}
          size={size}
          opacity={opacity * (1 - Math.pow(Math.abs(raw - 0.5) * 1.3, 3))}
        />
      );
    })}
  </>
);
