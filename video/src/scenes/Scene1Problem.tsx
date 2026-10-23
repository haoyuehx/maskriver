import React from 'react';
import {AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {COLORS, FONT, SPRING} from '../styles/tokens';
import {Backdrop} from '../components/Backdrop';
import {DataTable} from '../components/DataTable';
import {FlowCurve, FlowPackets, Pt} from '../components/Flow';
import {LogoMark, Wordmark} from '../components/Logo';
import {Eyebrow, Reveal} from '../components/Reveal';
import {DisclaimerTag} from '../components/Subtitles';
import {RingMark, VerdictChip} from '../components/Primitives';
import {ROWS} from '../data/fixtures';
import {ONSCREEN} from '../data/script';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

/** S1 — 问题与品牌（0:00–0:25, 750 帧） */
export const Scene1Problem: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  // ---- beat windows -------------------------------------------------------
  const eyebrowP = spring({frame: frame - 40, fps, config: SPRING.snappy});
  const tableP = spring({frame: frame - 34, fps, config: SPRING.settle});
  const panX = interpolate(frame, [90, 300], [40, -60], clamp);

  const focus = interpolate(frame, [300, 356], [0, 1], clamp);
  const focusOut = interpolate(frame, [408, 432], [1, 0], clamp);
  const push = interpolate(frame, [300, 372], [1, 1.06], clamp);

  const flowP = interpolate(frame, [420, 500], [0, 1], clamp);
  const packetPhase = ((frame - 430) / 62) % 1;
  const tableOut = interpolate(frame, [408, 440], [1, 0], clamp);

  const lockP = spring({frame: frame - 596, fps, config: SPRING.settle});
  const ringP = interpolate(frame, [436, 508], [0, 1], clamp);
  const wordP = spring({frame: frame - 632, fps, config: SPRING.settle});
  const tagP = spring({frame: frame - 656, fps, config: SPRING.settle});
  const badgesP = spring({frame: frame - 676, fps, config: SPRING.snappy});
  const outFade = interpolate(frame, [735, 752], [1, 0], clamp);

  const flowA: [Pt, Pt, Pt, Pt] = [
    {x: 262, y: 430},
    {x: 520, y: 430},
    {x: 640, y: 505},
    {x: 880, y: 505},
  ];
  const flowB: [Pt, Pt, Pt, Pt] = [
    {x: 262, y: 690},
    {x: 540, y: 690},
    {x: 660, y: 612},
    {x: 880, y: 612},
  ];

  return (
    <AbsoluteFill style={{opacity: outFade}}>
      <Backdrop grid="perspective" glow="both" />

      <AbsoluteFill style={{opacity: eyebrowP}}>
        <Eyebrow index={1} label="PROBLEM · 情景演示" delay={40} />
        <div style={{position: 'absolute', right: 132, top: 78}}>
          <DisclaimerTag />
        </div>
      </AbsoluteFill>

      {/* B3 — full-frame data panorama */}
      <AbsoluteFill style={{opacity: tableOut * tableP, transform: `translateX(${panX}px)`}}>
        <div style={{position: 'absolute', left: 132, top: 162}}>
          <Reveal delay={48} y={24}>
            <div
              style={{
                fontFamily: FONT.sans,
                fontSize: 54,
                fontWeight: 700,
                color: COLORS.text,
                letterSpacing: 0.6,
              }}
            >
              每一行测试数据，
              <span style={{color: COLORS.amber}}>都可能是真实的人</span>
            </div>
            <div
              style={{
                marginTop: 16,
                fontFamily: FONT.sans,
                fontSize: 26,
                color: COLORS.muted,
                letterSpacing: 1.4,
              }}
            >
              测试需要真实结构，但不该需要真实身份
            </div>
          </Reveal>
        </div>

        <div
          style={{
            position: 'absolute',
            left: 330,
            top: 372,
            opacity: interpolate(focus, [0, 1], [1, 0], clamp),
          }}
        >
          <DataTable
            rows={ROWS}
            width={1260}
            visibleRows={6}
            frame={frame}
            enter={56}
            rowHeight={66}
            fontSize={24}
            headerSize={18}
            colState={{name: 'sensitive', email: 'sensitive', mobile: 'sensitive', city: 'unknown'}}
          />
        </div>
      </AbsoluteFill>

      {/* B4 — field close-up */}
      <AbsoluteFill style={{opacity: focus * focusOut, transform: `scale(${push})`}}>
        <div
          style={{
            position: 'absolute',
            left: 132,
            top: 372,
            width: 800,
            padding: 28,
            borderRadius: 16,
            border: `1px solid ${COLORS.lineBright}`,
            background: 'linear-gradient(150deg, #0e2235, #0a1726)',
          }}
        >
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              fontFamily: FONT.mono,
              fontSize: 15,
              letterSpacing: 1.6,
              color: COLORS.muted2,
            }}
          >
            <span>customers · row 001</span>
            <span>合成数据</span>
          </div>
          <div style={{marginTop: 22, display: 'flex', flexDirection: 'column', gap: 16}}>
            {[
              {k: 'id', v: '001', tone: COLORS.muted2, tag: null as string | null},
              {k: 'name', v: '林晓', tone: COLORS.amber, tag: '姓名'},
              {k: 'email', v: 'lin@example.test', tone: COLORS.amber, tag: '邮箱'},
              {k: 'mobile', v: '199 0000 0011', tone: COLORS.amber, tag: '手机号'},
              {k: 'city', v: '上海', tone: COLORS.amber, tag: '需核对'},
            ].map((row, i) => {
              const p = spring({frame: frame - 312 - i * 7, fps, config: SPRING.crisp});
              return (
                <div
                  key={row.k}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 18,
                    opacity: p,
                    transform: `translateX(${(1 - p) * -14}px)`,
                  }}
                >
                  <span
                    style={{
                      width: 92,
                      fontFamily: FONT.mono,
                      fontSize: 16,
                      color: COLORS.muted2,
                      letterSpacing: 0.8,
                    }}
                  >
                    {row.k}
                  </span>
                  <span style={{fontFamily: FONT.mono, fontSize: 26, color: row.tone}}>{row.v}</span>
                  {row.tag && (
                    <span
                      style={{
                        marginLeft: 'auto',
                        fontFamily: FONT.sans,
                        fontSize: 16,
                        color: row.tag === '需核对' ? COLORS.amber : COLORS.cyan,
                        border: `1px solid ${row.tag === '需核对' ? COLORS.amber : COLORS.cyan}55`,
                        borderRadius: 6,
                        padding: '3px 10px',
                        letterSpacing: 1,
                      }}
                    >
                      {row.tag}
                    </span>
                  )}
                </div>
              );
            })}
          </div>
        </div>

        <div style={{position: 'absolute', left: 984, top: 372, width: 800}}>
          <Reveal delay={344} y={22}>
            <div style={{fontFamily: FONT.sans, fontSize: 40, fontWeight: 700, color: COLORS.text, lineHeight: 1.3}}>
              直接分发副本，
              <br />
              等于分发<span style={{color: COLORS.amber}}>隐私</span>。
            </div>
          </Reveal>
          <Reveal delay={368} y={18}>
            <div
              style={{
                marginTop: 26,
                padding: '18px 22px',
                borderRadius: 12,
                border: `1px solid ${COLORS.line}`,
                background: 'rgba(11,27,44,0.8)',
                fontFamily: FONT.sans,
                fontSize: 22,
                lineHeight: 1.7,
                color: COLORS.muted,
                maxWidth: 720,
              }}
            >
              识别不足不等于数据安全。未确认的字段需要人工核对，
              而不是默认放行。
            </div>
          </Reveal>
          <Reveal delay={392} y={16}>
            <div style={{marginTop: 22, display: 'flex', gap: 12}}>
              <VerdictChip kind="SENSITIVE" size={16} />
              <VerdictChip kind="UNKNOWN" size={16} />
            </div>
          </Reveal>
        </div>
      </AbsoluteFill>

      {/* B5 — data flow into the mark */}
      <AbsoluteFill style={{opacity: interpolate(frame, [420, 452, 588, 604], [0, 1, 1, 0], clamp)}}>
        <svg width={1920} height={1080} style={{position: 'absolute', inset: 0}}>
          <FlowCurve points={flowA} progress={flowP} color={COLORS.cyan} opacity={0.55} />
          <FlowCurve points={flowB} progress={flowP} color={COLORS.purple} opacity={0.5} />
          <FlowPackets points={flowA} count={3} phase={packetPhase} color={COLORS.cyan} size={5} />
          <FlowPackets points={flowB} count={3} phase={(packetPhase + 0.42) % 1} color={COLORS.purple} size={5} />
        </svg>
        <div
          style={{
            position: 'absolute',
            left: 232,
            top: 386,
            opacity: interpolate(frame, [430, 460], [0, 1], clamp),
            fontFamily: FONT.mono,
            fontSize: 16,
            letterSpacing: 2,
            color: COLORS.amber,
          }}
        >
          原始字段
        </div>
        <div
          style={{
            position: 'absolute',
            left: 1004,
            top: 452,
            opacity: interpolate(frame, [470, 500], [0, 1], clamp),
            fontFamily: FONT.mono,
            fontSize: 16,
            letterSpacing: 2,
            color: COLORS.teal,
          }}
        >
          脱敏值
        </div>
        <div style={{position: 'absolute', left: 900, top: 430, opacity: ringP}}>
          <RingMark size={240} progress={interpolate(frame, [470, 560], [0, 1], clamp)} />
        </div>
      </AbsoluteFill>

      {/* B6 — brand lockup */}
      <AbsoluteFill
        style={{
          opacity: lockP,
          alignItems: 'center',
          justifyContent: 'center',
          flexDirection: 'column',
        }}
      >
        <div style={{display: 'flex', alignItems: 'center', gap: 34, marginTop: -40}}>
          <div style={{transform: `scale(${0.92 + lockP * 0.08})`}}>
            <LogoMark size={128} progress={interpolate(frame, [600, 648], [0, 1], clamp)} />
          </div>
          <div style={{opacity: wordP, transform: `translateY(${(1 - wordP) * 18}px)`}}>
            <Wordmark size={104} gap={20} />
          </div>
        </div>
        <div
          style={{
            marginTop: 30,
            opacity: tagP,
            transform: `translateY(${(1 - tagP) * 14}px)`,
            fontFamily: FONT.sans,
            fontSize: 36,
            fontWeight: 500,
            letterSpacing: 8,
            color: COLORS.cyan,
          }}
        >
          {ONSCREEN.tagline}
        </div>
        <div
          style={{
            marginTop: 44,
            display: 'flex',
            gap: 16,
            opacity: badgesP,
            transform: `translateY(${(1 - badgesP) * 12}px)`,
          }}
        >
          {['只读预览', 'Ed25519 审批', '严格验证'].map((t) => (
            <span
              key={t}
              style={{
                fontFamily: FONT.sans,
                fontSize: 20,
                letterSpacing: 1.4,
                color: COLORS.muted,
                border: `1px solid ${COLORS.line}`,
                background: 'rgba(11,27,44,0.7)',
                borderRadius: 999,
                padding: '9px 22px',
              }}
            >
              {t}
            </span>
          ))}
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};
