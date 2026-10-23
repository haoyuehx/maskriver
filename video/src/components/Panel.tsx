import React from 'react';
import {COLORS, FONT} from '../styles/tokens';

type PanelProps = {
  width?: number | string;
  height?: number | string;
  padding?: number;
  radius?: number;
  /** Border accent: neutral line or a semantic colour. */
  accent?: 'line' | 'cyan' | 'teal' | 'purple' | 'danger' | 'amber' | 'none';
  fill?: string;
  children?: React.ReactNode;
  style?: React.CSSProperties;
};

const ACCENT: Record<string, string> = {
  line: COLORS.line,
  cyan: 'rgba(99,220,243,0.55)',
  teal: 'rgba(88,224,186,0.55)',
  purple: 'rgba(180,156,255,0.55)',
  danger: 'rgba(255,123,107,0.55)',
  amber: 'rgba(255,153,106,0.55)',
  none: 'transparent',
};

/** Flat glass plate used for every surface in the film. */
export const Panel: React.FC<PanelProps> = ({
  width = '100%',
  height = 'auto',
  padding = 28,
  radius = 20,
  accent = 'line',
  fill = COLORS.panel,
  children,
  style,
}) => {
  return (
    <div
      style={{
        width,
        height,
        padding,
        borderRadius: radius,
        background: fill,
        border: `1px solid ${ACCENT[accent]}`,
        boxShadow: '0 24px 80px rgba(0,0,0,0.45)',
        position: 'relative',
        overflow: 'hidden',
        fontFamily: FONT.sans,
        ...style,
      }}
    >
      <div
        style={{
          position: 'absolute',
          inset: '0 0 auto 0',
          height: 1,
          background:
            'linear-gradient(90deg, transparent, rgba(99,220,243,0.35), transparent)',
        }}
      />
      {children}
    </div>
  );
};

/** Small uppercase mono label, e.g. "SCAN" or "03 / 05". */
export const MicroLabel: React.FC<{
  children: React.ReactNode;
  color?: string;
  size?: number;
  tracking?: number;
  style?: React.CSSProperties;
}> = ({children, color = COLORS.muted2, size = 17, tracking = 3, style}) => (
  <div
    style={{
      fontFamily: FONT.mono,
      fontSize: size,
      letterSpacing: tracking,
      textTransform: 'uppercase',
      color,
      ...style,
    }}
  >
    {children}
  </div>
);

/** Status pill: 已通过 / 待审批 / 需核对 ... */
export const Badge: React.FC<{
  children: React.ReactNode;
  tone?: 'cyan' | 'teal' | 'amber' | 'danger' | 'purple' | 'muted';
  size?: number;
}> = ({children, tone = 'cyan', size = 18}) => {
  const map = {
    cyan: {fg: COLORS.cyan, bg: 'rgba(99,220,243,0.12)', bd: 'rgba(99,220,243,0.42)'},
    teal: {fg: COLORS.teal, bg: 'rgba(88,224,186,0.12)', bd: 'rgba(88,224,186,0.42)'},
    amber: {fg: COLORS.amber, bg: 'rgba(255,153,106,0.12)', bd: 'rgba(255,153,106,0.42)'},
    danger: {fg: COLORS.danger, bg: 'rgba(255,123,107,0.14)', bd: 'rgba(255,123,107,0.46)'},
    purple: {fg: COLORS.purple, bg: 'rgba(180,156,255,0.12)', bd: 'rgba(180,156,255,0.42)'},
    muted: {fg: COLORS.muted, bg: 'rgba(147,174,193,0.10)', bd: 'rgba(147,174,193,0.32)'},
  }[tone];
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 8,
        padding: `${Math.round(size * 0.24)}px ${Math.round(size * 0.62)}px`,
        borderRadius: 999,
        border: `1px solid ${map.bd}`,
        background: map.bg,
        color: map.fg,
        fontFamily: FONT.sans,
        fontSize: size,
        fontWeight: 500,
        letterSpacing: 0.5,
        whiteSpace: 'nowrap',
      }}
    >
      {children}
    </span>
  );
};
