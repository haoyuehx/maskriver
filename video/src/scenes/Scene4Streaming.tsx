import React from 'react';
import {AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {COLORS, FONT, SPRING} from '../styles/tokens';
import {Backdrop} from '../components/Backdrop';
import {Eyebrow, Reveal} from '../components/Reveal';
import {PipelineFlow} from '../components/Workflow';
import {StatCard} from '../components/Compare';
import {Meter} from '../components/Primitives';
import {DisclaimerTag} from '../components/Subtitles';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

const NODES = [
  {label: '有界读取', detail: '分页窗口固定上限'},
  {label: '批次队列', detail: '按批入队顺序推进'},
  {label: '受控处理', detail: '确定性策略替换'},
  {label: '批次事务', detail: '每批独立提交'},
  {label: 'Outbox 回执', detail: '同事务写入证据'},
  {label: '目标副本', detail: '只写计划字段'},
];

/** S4 — 有界流式处理（2:30–3:20, 1500 帧） */
export const Scene4Streaming: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  const titleP = spring({frame: frame - 1, fps, config: SPRING.settle});
  const chipP = spring({frame: frame - 14, fps, config: SPRING.snappy});
  const closeup = interpolate(frame, [600, 690], [0, 1], clamp);
  const closeupOut = interpolate(frame, [1000, 1050], [1, 0], clamp);
  const zoom = interpolate(frame, [600, 690, 1000, 1050], [1, 1.09, 1.09, 1], clamp);
  const bracket = interpolate(frame, [620, 668], [0, 1], clamp);
  const statsP = interpolate(frame, [1050, 1080], [0, 1], clamp);
  const outFade = interpolate(frame, [1462, 1500], [1, 0], clamp);

  return (
    <AbsoluteFill style={{opacity: outFade}}>
      <Backdrop grid="perspective" glow="teal" />

      <AbsoluteFill style={{opacity: interpolate(frame, [0, 16], [0, 1], clamp)}}>
        <Eyebrow index={4} label="STREAMING · 有界流式" delay={2} />
        <div style={{position: 'absolute', right: 132, top: 78}}>
          <DisclaimerTag text="技术原理 · 本地 SQLite" />
        </div>
      </AbsoluteFill>

      <div style={{position: 'absolute', left: 132, top: 138, opacity: titleP}}>
        <Reveal delay={2} y={20}>
          <div style={{fontFamily: FONT.sans, fontSize: 56, fontWeight: 700, color: COLORS.text}}>
            让数据按批次流动
          </div>
          <div
            style={{
              marginTop: 14,
              fontFamily: FONT.sans,
              fontSize: 26,
              color: COLORS.muted,
              letterSpacing: 1,
            }}
          >
            分批读取 · 批次事务 · 显式预算
          </div>
        </Reveal>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 1080,
          top: 168,
          opacity: chipP,
          display: 'flex',
          gap: 12,
        }}
      >
        <span
          style={{
            fontFamily: FONT.mono,
            fontSize: 16,
            letterSpacing: 1.4,
            color: COLORS.teal,
            border: `1px solid ${COLORS.teal}55`,
            background: 'rgba(88,224,186,0.10)',
            borderRadius: 999,
            padding: '9px 20px',
          }}
        >
          SQLite · local copy
        </span>
      </div>

      {/* Pipeline */}
      <AbsoluteFill
        style={{
          transform: `scale(${zoom})`,
          transformOrigin: '62% 48%',
        }}
      >
        <PipelineFlow
          nodes={NODES}
          frame={frame}
          enter={26}
          y={368}
          nodeW={256}
          nodeH={176}
          gap={24}
          x0={132}
          active={frame > 520 ? 5 : frame > 420 ? 4 : frame > 340 ? 3 : frame > 280 ? 2 : frame > 240 ? 1 : 0}
          meterAt={60}
          showPackets
        />

        {/* batch queue strip */}
        <div style={{position: 'absolute', left: 132, top: 586, display: 'flex', alignItems: 'center', gap: 10, opacity: interpolate(frame, [96, 120], [0, 1], clamp)}}>
          <span style={{fontFamily: FONT.mono, fontSize: 13, letterSpacing: 2, color: COLORS.muted2, width: 92}}>
            BATCH QUEUE
          </span>
          {Array.from({length: 24}).map((_, i) => {
            const t = interpolate(frame, [110 + i * 8, 126 + i * 8], [0, 1], clamp);
            const done = frame > 110 + i * 8 + 60;
            return (
              <span
                key={i}
                style={{
                  width: 34,
                  height: 14,
                  borderRadius: 3,
                  background: done ? COLORS.teal : COLORS.cyan,
                  opacity: 0.14 + t * (done ? 0.8 : 0.5),
                }}
              />
            );
          })}
          <span style={{marginLeft: 12, fontFamily: FONT.mono, fontSize: 14, color: COLORS.muted2}}>
            bounded window
          </span>
        </div>

        {/* atomic bracket over 批次事务 + Outbox 回执 */}
        <svg
          width={1920}
          height={1080}
          style={{position: 'absolute', inset: 0, opacity: closeup * closeupOut}}
        >
          <path
            d="M 972 552 v 40 H 1500 v -40"
            fill="none"
            stroke={COLORS.purple}
            strokeWidth={2}
            strokeLinecap="round"
            strokeDasharray={1400}
            strokeDashoffset={1400 * (1 - bracket)}
            opacity={0.9}
          />
          <text
            x={1234}
            y={620}
            textAnchor="middle"
            fill={COLORS.purple}
            fontFamily={FONT.mono}
            fontSize={18}
            letterSpacing={2}
            opacity={bracket}
          >
            SAME TRANSACTION
          </text>
        </svg>
      </AbsoluteFill>

      {/* Close-up annotation */}
      <div
        style={{
          position: 'absolute',
          left: 132,
          top: 636,
          width: 720,
          opacity: closeup * closeupOut,
        }}
      >
        <div
          style={{
            padding: '20px 24px',
            borderRadius: 14,
            border: `1px solid ${COLORS.purple}55`,
            background: 'rgba(11,27,44,0.92)',
          }}
        >
          <div style={{fontFamily: FONT.sans, fontSize: 26, fontWeight: 500, color: COLORS.text, lineHeight: 1.5}}>
            业务更新与 Outbox 回执，在同一事务里提交
          </div>
          <div style={{marginTop: 10, fontFamily: FONT.sans, fontSize: 19, color: COLORS.amber}}>
            批次事务 ≠ 整库原子性
          </div>
        </div>
      </div>

      {/* Stats + boundary */}
      <AbsoluteFill style={{opacity: statsP}}>
        <StatCard label="读取方式" value="BOUNDED" unit="分页窗口" x={132} y={700} w={384} frame={frame} enter={1060} />
        <StatCard label="提交单位" value="BATCH" unit="独立事务" x={540} y={700} w={384} frame={frame} enter={1070} color={COLORS.purple} />
        <StatCard label="资源约束" value="BUDGET" unit="显式预算" x={948} y={700} w={384} frame={frame} enter={1080} color={COLORS.teal} />
        <StatCard label="提交证据" value="OUTBOX" unit="事务回执" x={1356} y={700} w={384} frame={frame} enter={1090} color={COLORS.teal} />
        <div
          style={{
            position: 'absolute',
            left: 132,
            top: 828,
            fontFamily: FONT.sans,
            fontSize: 18,
            color: COLORS.muted2,
            letterSpacing: 0.6,
            opacity: interpolate(frame, [1120, 1140], [0, 1], clamp),
          }}
        >
          范围仅限本地 SQLite；MySQL 流式与恢复不在支持范围。内存占用随批次增长，而不是随整表增长。
        </div>
      </AbsoluteFill>

      {/* Bounded meter emphasis */}
      <AbsoluteFill style={{opacity: closeup * closeupOut * 0.9}}>
        <div style={{position: 'absolute', left: 132, top: 322, display: 'flex', alignItems: 'center', gap: 12}}>
          <span style={{fontFamily: FONT.mono, fontSize: 14, letterSpacing: 2, color: COLORS.muted2}}>
            WINDOW LIMIT
          </span>
          <Meter progress={interpolate(frame, [640, 720], [0.2, 0.68], clamp)} cap={0.68} width={220} height={4} />
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};
