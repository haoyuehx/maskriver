import React from 'react';
import {AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {COLORS, FONT, SPRING} from '../styles/tokens';
import {Backdrop} from '../components/Backdrop';
import {DataTable} from '../components/DataTable';
import {FlowCurve, FlowPackets, Pt} from '../components/Flow';
import {LogoMark, Wordmark} from '../components/Logo';
import {ROWS} from '../data/fixtures';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

/** 15-second style gate: dark data space -> scan -> flow -> brand lockup. */
export const StyleTest: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  // Camera: slow push in over the whole piece, settle at the lockup.
  const push = interpolate(frame, [0, 300, 450], [1.0, 1.035, 1.015], clamp);

  const tableIn = spring({frame: frame - 18, fps, config: SPRING.settle});
  const rowsIn = interpolate(frame, [46, 122], [0, 6], clamp);
  const beam = interpolate(frame, [58, 150], [-0.1, 1.1], clamp);
  const beamOpacity = interpolate(frame, [58, 66, 142, 152], [0, 1, 1, 0], clamp);

  const maskP = interpolate(frame, [168, 224], [0, 1], clamp);
  const flowP = interpolate(frame, [196, 250], [0, 1], clamp);
  const packetPhase = ((frame - 200) / 46) % 1;

  // Brand lockup
  const lock = spring({frame: frame - 286, fps, config: SPRING.soft});
  const tableOut = interpolate(frame, [276, 318], [1, 0], clamp);
  const logoDraw = interpolate(frame, [300, 344], [0, 1], clamp);
  const wordIn = spring({frame: frame - 318, fps, config: SPRING.settle});
  const taglineIn = spring({frame: frame - 342, fps, config: SPRING.settle});
  const taglineOut = interpolate(frame, [428, 448], [1, 0], clamp);

  const labelIn = (d: number) => spring({frame: frame - d, fps, config: SPRING.snappy});

  const cols: Array<{text: string; x: number; y: number; color: string; delay: number}> = [
    {text: '姓名', x: 356, y: 262, color: COLORS.cyan, delay: 74},
    {text: '邮箱', x: 566, y: 262, color: COLORS.cyan, delay: 82},
    {text: '手机号', x: 966, y: 262, color: COLORS.cyan, delay: 90},
  ];

  const flowA: [Pt, Pt, Pt, Pt] = [
    {x: 1150, y: 470},
    {x: 1300, y: 470},
    {x: 1330, y: 520},
    {x: 1470, y: 520},
  ];
  const flowB: [Pt, Pt, Pt, Pt] = [
    {x: 1150, y: 620},
    {x: 1320, y: 620},
    {x: 1330, y: 700},
    {x: 1470, y: 700},
  ];

  return (
    <AbsoluteFill style={{backgroundColor: COLORS.bgDeep}}>
      <AbsoluteFill style={{transform: `scale(${push})`}}>
        <Backdrop grid="perspective" glow="both" />

        {/* Scene table */}
        <AbsoluteFill style={{opacity: tableOut}}>
          <div
            style={{
              position: 'absolute',
              left: 196,
              top: 306,
              opacity: tableIn,
              transform: `translateY(${(1 - tableIn) * 34}px) scale(${0.985 + tableIn * 0.015})`,
            }}
          >
            <DataTable
              rows={ROWS}
              width={1010}
              visibleRows={Math.floor(rowsIn)}
              rowHeight={66}
              fontSize={23}
              headerSize={17}
              colState={{
                name: 'sensitive',
                email: 'sensitive',
                mobile: 'sensitive',
                city: 'unknown',
              }}
              maskProgress={{name: maskP, email: maskP, mobile: maskP}}
              scanBeam={beamOpacity > 0.01 ? beam : null}
            />
            <div
              style={{
                marginTop: 16,
                display: 'flex',
                gap: 14,
                alignItems: 'center',
                fontFamily: FONT.mono,
                fontSize: 16,
                letterSpacing: 2,
                color: COLORS.muted2,
                opacity: beamOpacity,
              }}
            >
              <span style={{color: COLORS.cyan}}>SCAN</span>
              <span>customers · 6 rows · synthetic</span>
            </div>
          </div>

          {/* Field chips */}
          {cols.map((c) => {
            const p = labelIn(c.delay);
            return (
              <div
                key={c.text}
                style={{
                  position: 'absolute',
                  left: c.x,
                  top: c.y - 8,
                  opacity: p,
                  transform: `translateY(${(1 - p) * 10}px)`,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '5px 12px',
                  borderRadius: 8,
                  border: `1px solid ${c.color}55`,
                  background: 'rgba(8,28,44,0.9)',
                  fontFamily: FONT.sans,
                  fontSize: 18,
                  color: c.color,
                  letterSpacing: 1,
                }}
              >
                <span style={{width: 5, height: 5, borderRadius: 3, background: c.color}} />
                {c.text}
              </div>
            );
          })}

          {/* Target environment */}
          <div
            style={{
              position: 'absolute',
              left: 1470,
              top: 372,
              width: 300,
              opacity: interpolate(flowP, [0, 0.4], [0, 1], clamp),
              transform: `translateX(${(1 - flowP) * 24}px)`,
            }}
          >
            <div
              style={{
                padding: 20,
                borderRadius: 16,
                border: `1px solid ${COLORS.lineBright}`,
                background: 'rgba(11,27,44,0.92)',
              }}
            >
              <div
                style={{
                  fontFamily: FONT.mono,
                  fontSize: 15,
                  letterSpacing: 2,
                  color: COLORS.teal,
                }}
              >
                TARGET
              </div>
              <div
                style={{
                  marginTop: 8,
                  fontFamily: FONT.sans,
                  fontSize: 22,
                  color: COLORS.text,
                  fontWeight: 500,
                }}
              >
                测试环境副本
              </div>
              {[0, 1, 2].map((i) => (
                <div
                  key={i}
                  style={{
                    marginTop: 14,
                    height: 8,
                    borderRadius: 4,
                    width: `${70 - i * 12}%`,
                    background: i === 0 ? COLORS.teal : COLORS.line,
                    opacity: interpolate(flowP, [0.3 + i * 0.15, 0.6 + i * 0.15], [0.2, 1], clamp),
                  }}
                />
              ))}
            </div>
          </div>

          {/* Flow paths */}
          <svg
            width={1920}
            height={1080}
            style={{position: 'absolute', inset: 0, pointerEvents: 'none'}}
          >
            <FlowCurve points={flowA} progress={flowP} color={COLORS.cyan} opacity={0.55} width={2} />
            <FlowCurve points={flowB} progress={flowP} color={COLORS.purple} opacity={0.5} width={2} />
            {flowP > 0.25 && (
              <>
                <FlowPackets points={flowA} count={3} phase={packetPhase} color={COLORS.cyan} size={5} />
                <FlowPackets
                  points={flowB}
                  count={3}
                  phase={(packetPhase + 0.4) % 1}
                  color={COLORS.purple}
                  size={5}
                />
              </>
            )}
          </svg>
        </AbsoluteFill>

        {/* Brand lockup */}
        <AbsoluteFill
          style={{
            alignItems: 'center',
            justifyContent: 'center',
            flexDirection: 'column',
            opacity: lock,
          }}
        >
          <div style={{transform: `scale(${0.94 + lock * 0.06})`}}>
            <LogoMark size={132} progress={logoDraw} />
          </div>
          <div style={{marginTop: 46, opacity: wordIn, transform: `translateY(${(1 - wordIn) * 20}px)`}}>
            <Wordmark size={104} gap={22} letterSpacing={1} />
          </div>
          <div
            style={{
              marginTop: 26,
              opacity: taglineIn * taglineOut,
              transform: `translateY(${(1 - taglineIn) * 16}px)`,
              fontFamily: FONT.sans,
              fontSize: 34,
              fontWeight: 500,
              letterSpacing: 7,
              color: COLORS.cyan,
            }}
          >
            安全、可验证、可恢复的数据脱敏工具
          </div>
        </AbsoluteFill>

        {/* Corner marks for a filmic frame */}
        <AbsoluteFill style={{pointerEvents: 'none', opacity: 0.5}}>
          {[
            {left: 64, top: 64, borderLeft: 1, borderTop: 1},
            {right: 64, top: 64, borderRight: 1, borderTop: 1},
            {left: 64, bottom: 64, borderLeft: 1, borderBottom: 1},
            {right: 64, bottom: 64, borderRight: 1, borderBottom: 1},
          ].map((s, i) => (
            <div
              key={i}
              style={{
                position: 'absolute',
                width: 26,
                height: 26,
                borderColor: COLORS.lineBright,
                borderStyle: 'solid',
                borderWidth: 0,
                ...s,
              }}
            />
          ))}
        </AbsoluteFill>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};
