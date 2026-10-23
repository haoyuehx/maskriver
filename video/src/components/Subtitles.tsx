import React from 'react';
import {AbsoluteFill, useCurrentFrame, useVideoConfig, interpolate} from 'remotion';
import {COLORS, FONT} from '../styles/tokens';
import {CUES} from '../data/script';

/**
 * Burned-in Simplified Chinese captions. Optional spoken-word audio is a
 * separate track; this component renders the picture subtitles only.
 * Band geometry: bottom 56px margin, 36px type, max two lines.
 */
export const Subtitles: React.FC<{opacity?: number}> = ({opacity = 1}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const time = frame / fps;

  const cue = CUES.find((c) => time >= c.start && time <= c.end);
  if (!cue) return null;

  // 4-frame fade in/out so caption changes do not pop.
  const fadeIn = interpolate(time, [cue.start, cue.start + 4 / fps], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  const fadeOut = interpolate(time, [cue.end - 5 / fps, cue.end], [1, 0], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  const alpha = Math.min(fadeIn, fadeOut) * opacity;

  return (
    <AbsoluteFill style={{pointerEvents: 'none'}}>
      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          bottom: 56,
          display: 'flex',
          justifyContent: 'center',
        }}
      >
        <div
          style={{
            maxWidth: 1480,
            padding: '12px 34px',
            borderRadius: 12,
            background: 'rgba(2,8,18,0.62)',
            border: '1px solid rgba(29,61,85,0.55)',
            opacity: alpha,
            fontFamily: FONT.sans,
            fontSize: 34,
            fontWeight: 500,
            lineHeight: 1.38,
            letterSpacing: 0.8,
            color: COLORS.text,
            textAlign: 'center',
            textShadow: '0 2px 18px rgba(0,0,0,0.8)',
          }}
        >
          {cue.text}
        </div>
      </div>
    </AbsoluteFill>
  );
};

/** Small "流程示意 · 合成数据" marker required on concept-UI shots. */
export const DisclaimerTag: React.FC<{text?: string; style?: React.CSSProperties}> = ({
  text = '流程示意 · 合成数据',
  style,
}) => (
  <div
    style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: 8,
      padding: '6px 14px',
      borderRadius: 8,
      border: `1px solid ${COLORS.line}`,
      background: 'rgba(11,27,44,0.72)',
      color: COLORS.muted2,
      fontFamily: FONT.sans,
      fontSize: 17,
      letterSpacing: 1.2,
      ...style,
    }}
  >
    <span style={{width: 5, height: 5, borderRadius: 3, background: COLORS.muted2}} />
    {text}
  </div>
);
