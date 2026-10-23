import React from 'react';
import {AbsoluteFill, interpolate, useCurrentFrame} from 'remotion';
import {COLORS} from '../styles/tokens';

type BackdropProps = {
  /** Soft radial light pools. */
  glow?: 'cyan' | 'purple' | 'both' | 'teal' | 'none';
  /** Grid treatment. `perspective` tilts the plane away from the camera. */
  grid?: 'perspective' | 'flat' | 'none';
  /** Extra darkening at the edges to focus the frame. */
  vignette?: boolean;
  /** Darken the whole plate (used when a scene puts dense UI on top). */
  dim?: number;
  children?: React.ReactNode;
};

/**
 * Full-frame plate: layered gradient, optional grid, vignette.
 * Deliberately flat (no CSS blur) so software rasterization stays fast.
 */
export const Backdrop: React.FC<BackdropProps> = ({
  glow = 'both',
  grid = 'flat',
  vignette = true,
  dim = 0,
  children,
}) => {
  const frame = useCurrentFrame();
  const drift = interpolate(frame, [0, 7800], [0, -40], {extrapolateRight: 'extend'});

  const glowLayer = () => {
    if (glow === 'none') return null;
    const cyan = 'radial-gradient(ellipse 62% 48% at 50% 8%, #12374f 0%, transparent 66%)';
    const purple = 'radial-gradient(ellipse 46% 40% at 92% 22%, #251a45 0%, transparent 64%)';
    const teal = 'radial-gradient(ellipse 52% 40% at 12% 78%, #0d3a3a 0%, transparent 66%)';
    const layers =
      glow === 'both' ? `${cyan}, ${purple}` : glow === 'cyan' ? cyan : glow === 'purple' ? purple : teal;
    return <AbsoluteFill style={{background: layers}} />;
  };

  return (
    <AbsoluteFill style={{backgroundColor: COLORS.bgDeep, overflow: 'hidden'}}>
      <AbsoluteFill
        style={{
          background: `linear-gradient(180deg, ${COLORS.bg} 0%, #04101c 58%, ${COLORS.bgDeep} 100%)`,
        }}
      />
      {glowLayer()}
      {grid !== 'none' && (
        <AbsoluteFill style={{opacity: grid === 'flat' ? 0.5 : 0.85}}>
          {grid === 'flat' ? (
            <AbsoluteFill
              style={{
                backgroundImage: `linear-gradient(${COLORS.line} 1px, transparent 1px), linear-gradient(90deg, ${COLORS.line} 1px, transparent 1px)`,
                backgroundSize: '80px 80px',
                backgroundPosition: `0px ${drift}px`,
                opacity: 0.28,
                maskImage:
                  'radial-gradient(ellipse 70% 62% at 50% 42%, #000 0%, transparent 78%)',
                WebkitMaskImage:
                  'radial-gradient(ellipse 70% 62% at 50% 42%, #000 0%, transparent 78%)',
              }}
            />
          ) : (
            <AbsoluteFill style={{perspective: 1400, perspectiveOrigin: '50% 8%'}}>
              <div
                style={{
                  position: 'absolute',
                  left: '-40%',
                  right: '-40%',
                  top: '34%',
                  height: '190%',
                  transform: 'rotateX(74deg)',
                  transformOrigin: '50% 0%',
                  backgroundImage: `linear-gradient(${COLORS.lineBright} 1px, transparent 1px), linear-gradient(90deg, ${COLORS.lineBright} 1px, transparent 1px)`,
                  backgroundSize: '96px 96px',
                  opacity: 0.22,
                  maskImage: 'linear-gradient(180deg, #000 0%, transparent 72%)',
                  WebkitMaskImage: 'linear-gradient(180deg, #000 0%, transparent 72%)',
                }}
              />
            </AbsoluteFill>
          )}
        </AbsoluteFill>
      )}
      {vignette && (
        <AbsoluteFill
          style={{
            background:
              'radial-gradient(ellipse 78% 70% at 50% 44%, transparent 38%, #01060e 100%)',
            opacity: 0.72,
          }}
        />
      )}
      {dim > 0 && <AbsoluteFill style={{backgroundColor: COLORS.bgDeep, opacity: dim}} />}
      {children}
    </AbsoluteFill>
  );
};
