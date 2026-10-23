import React from 'react';
import {interpolate, useCurrentFrame} from 'remotion';
import {COLORS} from '../styles/tokens';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

/**
 * Transition accents drawn above all scenes at absolute frames:
 * 735-765  vertical cyan wipe line (S1 -> S2)
 * 4486-4514 horizontal teal hairline (S3 -> S4)
 * 7336-7368 expanding teal iris ring (S5 -> S6)
 */
export const TransitionAccents: React.FC = () => {
  const frame = useCurrentFrame();

  const wipe = interpolate(frame, [735, 765], [-40, 1960], clamp);
  const wipeOn = frame >= 735 && frame <= 766;

  const hairP = interpolate(frame, [4486, 4500, 4512, 4514], [0, 1, 1, 0], clamp);
  const hairOn = frame >= 4486 && frame <= 4514;

  const iris = interpolate(frame, [7336, 7368], [0, 1], clamp);
  const irisOn = frame >= 7336 && frame <= 7369;

  return (
    <>
      {wipeOn && (
        <div
          style={{
            position: 'absolute',
            left: wipe,
            top: 0,
            width: 3,
            height: 1080,
            background: COLORS.cyan,
            opacity: 0.85,
          }}
        />
      )}
      {hairOn && (
        <div
          style={{
            position: 'absolute',
            left: 132,
            top: 540,
            width: 1656,
            height: 2,
            background: `linear-gradient(90deg, transparent, ${COLORS.teal}, transparent)`,
            transform: `scaleX(${hairP})`,
            transformOrigin: 'center',
            opacity: hairP,
          }}
        />
      )}
      {irisOn && (
        <svg width={1920} height={1080} style={{position: 'absolute', inset: 0}}>
          <circle
            cx={960}
            cy={560}
            r={iris * 1400}
            fill="none"
            stroke={COLORS.teal}
            strokeWidth={1.5}
            opacity={0.5 * (1 - iris)}
          />
        </svg>
      )}
    </>
  );
};

/** Opening fade from black and closing fade to black. */
export const OpeningClosing: React.FC = () => {
  const frame = useCurrentFrame();
  const open = interpolate(frame, [0, 30], [1, 0], clamp);
  const closeA = interpolate(frame, [7740, 7780], [0, 1], clamp);
  const closeB = interpolate(frame, [7780, 7800], [0, 1], clamp);
  return (
    <>
      {open > 0.001 && (
        <div style={{position: 'absolute', inset: 0, background: '#000', opacity: open}} />
      )}
      {closeA > 0.001 && (
        <div style={{position: 'absolute', inset: 0, background: COLORS.bgDeep, opacity: closeA}} />
      )}
      {closeB > 0.001 && (
        <div style={{position: 'absolute', inset: 0, background: '#000', opacity: closeB}} />
      )}
    </>
  );
};
