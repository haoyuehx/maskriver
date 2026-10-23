import React from 'react';
import {COLORS, FONT} from '../styles/tokens';

/**
 * MaskRiver brand mark: a river wave (masked by two redaction rules)
 * inside a rounded tile. Geometrically compatible with the website logo,
 * re-drawn as Remotion-safe SVG with animatable draw progress.
 */
export const LogoMark: React.FC<{size?: number; progress?: number; showTile?: boolean}> = ({
  size = 64,
  progress = 1,
  showTile = true,
}) => {
  const s = size;
  const p = Math.max(0, Math.min(1, progress));
  // wave path length is ~ 44 units; keep it generous for dash animation
  const waveLen = 56;
  const gradId = React.useId().replace(/:/g, '');

  return (
    <svg width={s} height={s} viewBox="0 0 64 64" style={{display: 'block'}}>
      <defs>
        <linearGradient id={gradId} x1="0" y1="0" x2="1" y2="1">
          <stop stopColor={COLORS.cyan} />
          <stop offset="1" stopColor={COLORS.purple} />
        </linearGradient>
      </defs>
      {showTile && (
        <rect
          width="64"
          height="64"
          rx="16"
          fill="#0b2338"
          stroke={COLORS.lineBright}
          strokeWidth="1.2"
        />
      )}
      <path
        d="M13 40c7-13 12 13 19 0s12 13 19 0"
        fill="none"
        stroke={`url(#${gradId})`}
        strokeWidth="5"
        strokeLinecap="round"
        strokeDasharray={waveLen}
        strokeDashoffset={waveLen * (1 - p)}
      />
      <path
        d="M20 23h24"
        stroke={COLORS.text}
        strokeWidth="4"
        strokeLinecap="round"
        opacity={0.9}
        strokeDasharray={24}
        strokeDashoffset={24 * (1 - Math.min(1, p * 1.4))}
      />
      <path
        d="M20 30h15"
        stroke={COLORS.text}
        strokeWidth="4"
        strokeLinecap="round"
        opacity={0.62}
        strokeDasharray={15}
        strokeDashoffset={15 * (1 - Math.max(0, Math.min(1, p * 1.6 - 0.3)))}
      />
    </svg>
  );
};

/** Wordmark: "Mask" in white, "River" in the cyan/purple gradient. */
export const Wordmark: React.FC<{
  size?: number;
  opacity?: number;
  gap?: number;
  letterSpacing?: number;
  weight?: number;
}> = ({size = 56, opacity = 1, gap = 16, letterSpacing = 0.5, weight = 700}) => (
  <div
    style={{
      display: 'flex',
      alignItems: 'center',
      gap,
      opacity,
      fontFamily: FONT.sans,
      fontSize: size,
      fontWeight: weight,
      letterSpacing,
      lineHeight: 1,
    }}
  >
    <span style={{color: COLORS.text}}>Mask</span>
    <span
      style={{
        background: `linear-gradient(96deg, ${COLORS.cyan} 0%, ${COLORS.purple} 100%)`,
        WebkitBackgroundClip: 'text',
        backgroundClip: 'text',
        color: 'transparent',
      }}
    >
      River
    </span>
  </div>
);
