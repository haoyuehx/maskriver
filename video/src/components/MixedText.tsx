import React from 'react';
import {AbsoluteFill, useCurrentFrame, useVideoConfig} from 'remotion';
import {COLORS, FONT} from '../styles/tokens';

/**
 * CJK/Latin mixed typesetting. Latin runs, numbers and code tokens are set in
 * JetBrains Mono with 0.25em side margins; Chinese stays in Noto Sans SC.
 */
const SPLIT = /([A-Za-z0-9_@./#%+\-:<>=*()[\]{}|~`'"]+)/g;

export const MixedText: React.FC<{
  text: string;
  size?: number;
  color?: string;
  weight?: number;
  lineHeight?: number;
  letterSpacing?: number;
  style?: React.CSSProperties;
}> = ({text, size = 24, color = COLORS.text, weight = 400, lineHeight = 1.6, letterSpacing = 0.2, style}) => {
  const parts = text.split(SPLIT);
  return (
    <div
      style={{
        fontFamily: FONT.sans,
        fontSize: size,
        color,
        fontWeight: weight,
        lineHeight,
        letterSpacing,
        ...style,
      }}
    >
      {parts.map((part, i) =>
        part === '' ? null : SPLIT.test(part) || /^[A-Za-z0-9_@./#%+\-:<>=*()[\]{}|~`'"]+$/.test(part) ? (
          <span
            key={i}
            style={{
              fontFamily: FONT.mono,
              fontSize: size * 0.94,
              margin: '0 0.22em',
              color,
            }}
          >
            {part}
          </span>
        ) : (
          <span key={i}>{part}</span>
        ),
      )}
    </div>
  );
};

/** Single transform wrapper for camera push/pan (image only, never text). */
export const CameraRig: React.FC<{
  scale?: number;
  x?: number;
  y?: number;
  origin?: string;
  children: React.ReactNode;
}> = ({scale = 1, x = 0, y = 0, origin = '50% 50%', children}) => (
  <AbsoluteFill
    style={{
      transform: `translate(${x}px, ${y}px) scale(${scale})`,
      transformOrigin: origin,
    }}
  >
    {children}
  </AbsoluteFill>
);

/** Dev-only safe-area guides; never rendered in production builds. */
export const SafeFrame: React.FC<{children?: React.ReactNode}> = ({children}) => (
  <AbsoluteFill style={{outline: 'none'}}>{children}</AbsoluteFill>
);

/** Static scanline / vignette texture used sparingly behind panels. */
export const Hairline: React.FC<{width?: number | string; color?: string; opacity?: number; vertical?: boolean}> = ({
  width = '100%',
  color = COLORS.line,
  opacity = 1,
  vertical = false,
}) => (
  <div
    style={{
      width: vertical ? 1 : width,
      height: vertical ? width : 1,
      background: color,
      opacity,
      flex: 'none',
    }}
  />
);

export const useNow = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  return {frame, fps, time: frame / fps};
};
