import React from 'react';
import {AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {COLORS, FONT} from '../styles/tokens';

/**
 * Environment smoke test: 5s, 1920x1080, 30fps, SVG motion + Chinese typography.
 * Rendered to video/out/env-test.mp4 before any real scene work.
 */
export const EnvTest: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  const ring = spring({frame, fps, config: {damping: 18, mass: 0.8, stiffness: 90}});
  const dash = interpolate(frame, [0, 110], [0, 1], {extrapolateRight: 'clamp'});
  const title = spring({frame: frame - 18, fps, config: {damping: 24, stiffness: 120}});
  const sub = spring({frame: frame - 42, fps, config: {damping: 24, stiffness: 120}});

  const radius = 116;
  const circumference = 2 * Math.PI * radius;

  return (
    <AbsoluteFill style={{backgroundColor: COLORS.bgDeep, fontFamily: FONT.sans}}>
      <AbsoluteFill
        style={{
          background:
            'radial-gradient(ellipse 60% 45% at 50% 42%, #12374f 0%, transparent 65%), linear-gradient(180deg, #06111f 0%, #020812 100%)',
        }}
      />
      <AbsoluteFill style={{alignItems: 'center', justifyContent: 'center', flexDirection: 'column'}}>
        <svg width={320} height={320} viewBox="0 0 320 320" style={{opacity: ring}}>
          <circle cx={160} cy={160} r={radius} fill="none" stroke={COLORS.lineBright} strokeWidth={2} />
          <circle
            cx={160}
            cy={160}
            r={radius}
            fill="none"
            stroke={COLORS.cyan}
            strokeWidth={4}
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={circumference * (1 - dash)}
            transform="rotate(-90 160 160)"
          />
          <circle cx={160} cy={160} r={38} fill={COLORS.cyan} opacity={0.12} />
          <circle cx={160} cy={160} r={16} fill={COLORS.cyan} opacity={ring} />
        </svg>
        <div
          style={{
            marginTop: 64,
            fontSize: 96,
            fontWeight: 700,
            color: COLORS.text,
            letterSpacing: 2,
            transform: `translateY(${(1 - title) * 28}px)`,
            opacity: title,
          }}
        >
          MaskRiver
        </div>
        <div
          style={{
            marginTop: 18,
            fontSize: 34,
            fontWeight: 500,
            color: COLORS.cyan,
            letterSpacing: 6,
            opacity: sub,
          }}
        >
          安全、可验证、可恢复的数据脱敏工具
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};
