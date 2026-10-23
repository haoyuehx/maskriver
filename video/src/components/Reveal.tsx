import React from 'react';
import {interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {COLORS, FONT, SPRING} from '../styles/tokens';

/**
 * Spring-based entrance. Every scene uses this so the acceleration curve
 * is identical film-wide.
 */
export const Reveal: React.FC<{
  children: React.ReactNode;
  delay?: number;
  y?: number;
  x?: number;
  scaleFrom?: number;
  springName?: keyof typeof SPRING;
  style?: React.CSSProperties;
}> = ({children, delay = 0, y = 26, x = 0, scaleFrom = 1, springName = 'settle', style}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const p = spring({frame: frame - delay, fps, config: SPRING[springName]});
  return (
    <div
      style={{
        opacity: interpolate(p, [0, 1], [0, 1], {extrapolateRight: 'clamp'}),
        transform: `translate(${x * (1 - p)}px, ${y * (1 - p)}px) scale(${
          scaleFrom + (1 - scaleFrom) * p
        })`,
        ...style,
      }}
    >
      {children}
    </div>
  );
};

/** Scene eyebrow: index + Chinese title + rule. Top-left of the content grid. */
export const Eyebrow: React.FC<{
  index: number;
  label: string;
  total?: number;
  color?: string;
  delay?: number;
}> = ({index, label, total = 6, color = COLORS.cyan, delay = 0}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const p = spring({frame: frame - delay, fps, config: SPRING.settle});
  const rule = interpolate(frame, [delay, delay + 26], [0, 168], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  return (
    <div
      style={{
        position: 'absolute',
        left: 132,
        top: 84,
        display: 'flex',
        alignItems: 'center',
        gap: 18,
        opacity: p,
        fontFamily: FONT.sans,
      }}
    >
      <span
        style={{
          fontFamily: FONT.mono,
          fontSize: 19,
          letterSpacing: 3,
          color,
          padding: '6px 12px',
          border: `1px solid ${color}44`,
          borderRadius: 8,
          background: `${color}12`,
        }}
      >
        {String(index).padStart(2, '0')}
      </span>
      <span style={{fontSize: 21, letterSpacing: 2.4, color: COLORS.muted, fontWeight: 500}}>
        {label}
      </span>
      <span
        style={{
          width: rule,
          height: 1,
          background: `linear-gradient(90deg, ${color}88, transparent)`,
        }}
      />
      <span style={{fontFamily: FONT.mono, fontSize: 16, letterSpacing: 2, color: COLORS.muted2}}>
        {String(index).padStart(2, '0')}/{String(total).padStart(2, '0')}
      </span>
    </div>
  );
};

/** Scene headline with an optional accent phrase. */
export const Headline: React.FC<{
  children: React.ReactNode;
  size?: number;
  delay?: number;
  color?: string;
  align?: 'left' | 'center';
  weight?: number;
  lineHeight?: number;
  maxWidth?: number;
  style?: React.CSSProperties;
}> = ({
  children,
  size = 64,
  delay = 0,
  color = COLORS.text,
  align = 'left',
  weight = 700,
  lineHeight = 1.22,
  maxWidth,
  style,
}) => (
  <Reveal delay={delay} y={30}>
    <div
      style={{
        fontFamily: FONT.sans,
        fontSize: size,
        fontWeight: weight,
        color,
        letterSpacing: 0.5,
        lineHeight,
        textAlign: align,
        maxWidth,
        ...style,
      }}
    >
      {children}
    </div>
  </Reveal>
);

/** Emphasis color span used inside headlines. */
export const Accent: React.FC<{children: React.ReactNode; color?: string}> = ({
  children,
  color = COLORS.cyan,
}) => <span style={{color}}>{children}</span>;

/** Faint section divider line. */
export const Rule: React.FC<{width?: number | string; color?: string}> = ({
  width = '100%',
  color = COLORS.line,
}) => <div style={{width, height: 1, background: color}} />;
