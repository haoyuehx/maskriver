import React from 'react';
import {AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {COLORS, FONT, SPRING} from '../styles/tokens';
import {Backdrop} from '../components/Backdrop';
import {Eyebrow, Reveal} from '../components/Reveal';
import {LogoMark, Wordmark} from '../components/Logo';
import {ONSCREEN} from '../data/script';
import {STEPS} from '../data/fixtures';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

const PILLARS = [
  {title: '安全审批', body: 'Ed25519 审批记录绑定计划摘要', color: COLORS.purple},
  {title: '可验证脱敏', body: '全量扫描 + 适配器完整性检查', color: COLORS.teal},
  {title: '故障恢复', body: 'Checkpoint · 仅本地 SQLite', color: COLORS.cyan},
];

/** S6 — 产品总结（4:05–4:20, 450 帧） */
export const Scene6Closing: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  const headP = spring({frame: frame - 1, fps, config: SPRING.settle});
  const pillarsOut = interpolate(frame, [150, 178], [1, 0], clamp);
  const lockP = spring({frame: frame - 150, fps, config: SPRING.settle});
  const ringP = interpolate(frame, [156, 214], [0, 1], clamp);
  const wordP = spring({frame: frame - 186, fps, config: SPRING.settle});
  const urlP = spring({frame: frame - 214, fps, config: SPRING.snappy});
  const noteP = spring({frame: frame - 250, fps, config: SPRING.snappy});

  return (
    <AbsoluteFill>
      <Backdrop grid="none" glow="cyan" vignette />

      <AbsoluteFill style={{opacity: interpolate(frame, [0, 14], [0, 1], clamp)}}>
        <Eyebrow index={6} label="SUMMARY · 总结" delay={2} />
      </AbsoluteFill>

      {/* Pillars */}
      <AbsoluteFill style={{opacity: pillarsOut}}>
        <div style={{position: 'absolute', left: 132, top: 150, opacity: headP}}>
          <Reveal delay={2} y={20}>
            <div style={{fontFamily: FONT.sans, fontSize: 46, fontWeight: 700, color: COLORS.text}}>
              三个价值，一条链路
            </div>
          </Reveal>
        </div>

        <div style={{position: 'absolute', left: 132, top: 250, display: 'flex', gap: 12}}>
          {STEPS.map((s, i) => {
            const p = spring({frame: frame - 6 - i * 5, fps, config: SPRING.snappy});
            return (
              <div
                key={s.key}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '7px 16px',
                  borderRadius: 8,
                  border: `1px solid ${COLORS.teal}55`,
                  background: 'rgba(88,224,186,0.08)',
                  fontFamily: FONT.mono,
                  fontSize: 15,
                  letterSpacing: 1.4,
                  color: COLORS.teal,
                  opacity: p,
                }}
              >
                <span style={{width: 5, height: 5, borderRadius: 3, background: COLORS.teal}} />
                {s.label.toUpperCase()}
              </div>
            );
          })}
        </div>

        <div style={{position: 'absolute', left: 132, top: 340, display: 'flex', gap: 24}}>
          {PILLARS.map((p, i) => {
            const sp = spring({frame: frame - 18 - i * 8, fps, config: SPRING.settle});
            return (
              <div
                key={p.title}
                style={{
                  width: 536,
                  height: 210,
                  padding: 28,
                  borderRadius: 18,
                  border: `1px solid ${p.color}55`,
                  background: `linear-gradient(155deg, ${p.color}14, rgba(11,27,44,0.94))`,
                  opacity: sp,
                  transform: `translateY(${(1 - sp) * 20}px)`,
                  display: 'flex',
                  flexDirection: 'column',
                  gap: 12,
                }}
              >
                <span style={{width: 34, height: 3, background: p.color, borderRadius: 2}} />
                <div style={{fontFamily: FONT.sans, fontSize: 34, fontWeight: 700, color: COLORS.text}}>
                  {p.title}
                </div>
                <div style={{fontFamily: FONT.sans, fontSize: 21, color: COLORS.muted, lineHeight: 1.6}}>
                  {p.body}
                </div>
              </div>
            );
          })}
        </div>
      </AbsoluteFill>

      {/* Logo lockup */}
      <AbsoluteFill
        style={{
          opacity: lockP,
          alignItems: 'center',
          justifyContent: 'center',
          flexDirection: 'column',
        }}
      >
        <div style={{display: 'flex', alignItems: 'center', gap: 34, marginTop: -60}}>
          <LogoMark size={124} progress={ringP} />
          <div style={{opacity: wordP, transform: `translateY(${(1 - wordP) * 16}px)`}}>
            <Wordmark size={98} gap={20} />
          </div>
        </div>
        <div
          style={{
            marginTop: 30,
            opacity: wordP,
            fontFamily: FONT.sans,
            fontSize: 34,
            fontWeight: 500,
            letterSpacing: 7,
            color: COLORS.cyan,
          }}
        >
          {ONSCREEN.headline}
        </div>
        <div
          style={{
            marginTop: 40,
            opacity: urlP,
            transform: `translateY(${(1 - urlP) * 12}px)`,
            display: 'flex',
            alignItems: 'center',
            gap: 16,
          }}
        >
          <span style={{fontFamily: FONT.mono, fontSize: 28, color: COLORS.text, letterSpacing: 0.6}}>
            {ONSCREEN.github}
          </span>
          <span
            style={{
              fontFamily: FONT.mono,
              fontSize: 15,
              letterSpacing: 1.6,
              color: COLORS.muted2,
              border: `1px solid ${COLORS.line}`,
              borderRadius: 6,
              padding: '5px 10px',
            }}
          >
            MIT
          </span>
        </div>
        <div
          style={{
            marginTop: 26,
            opacity: noteP,
            fontFamily: FONT.sans,
            fontSize: 18,
            color: COLORS.muted2,
            letterSpacing: 1,
          }}
        >
          演示使用合成数据 · 界面为设计示意
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};
