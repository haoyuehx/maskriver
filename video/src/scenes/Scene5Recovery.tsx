import React from 'react';
import {AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {COLORS, FONT, SPRING} from '../styles/tokens';
import {Backdrop} from '../components/Backdrop';
import {Eyebrow, Reveal} from '../components/Reveal';
import {BatchTimeline, BatchState} from '../components/Workflow';
import {FaultOverlay} from '../components/Compare';
import {CommandCard} from '../components/Primitives';
import {DisclaimerTag} from '../components/Subtitles';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

/** S5 — Checkpoint 与故障恢复（3:20–4:05, 1350 帧） */
export const Scene5Recovery: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  const titleP = spring({frame: frame - 1, fps, config: SPRING.settle});
  const faultAt = 560;
  const resumeAt = 760;
  const outFade = interpolate(frame, [1322, 1350], [1, 0], clamp);

  // Cell states across the story.
  const states: BatchState[] = Array.from({length: 6}).map((_, i) => {
    if (i <= 2) {
      if (frame < 120 + i * 46) return 'idle';
      if (frame >= resumeAt + 20 + i * 34 && frame < 1010) return 'verified';
      return 'committed';
    }
    if (i === 3) {
      if (frame < resumeAt + 40) return frame > faultAt && frame < resumeAt ? 'idle' : 'idle';
      if (frame < resumeAt + 120) return 'current';
      return 'committed';
    }
    if (i === 4) {
      if (frame < 950) return 'idle';
      return 'committed';
    }
    if (frame < 1010) return 'idle';
    return 'committed';
  });

  const committed = 3 + (frame > 900 ? 1 : 0) + (frame > 960 ? 1 : 0) + (frame > 1010 ? 1 : 0);
  const count = frame < 120 ? 0 : Math.min(6, committed);

  const dimmed = frame > faultAt && frame < resumeAt + 20;

  const statusP = spring({frame: frame - (faultAt + 16), fps, config: SPRING.snappy});
  const resumeP = spring({frame: frame - resumeAt, fps, config: SPRING.snappy});
  const passP = spring({frame: frame - 1120, fps, config: SPRING.crisp});

  return (
    <AbsoluteFill style={{opacity: outFade}}>
      <Backdrop grid="flat" glow="both" />

      <AbsoluteFill style={{opacity: interpolate(frame, [0, 16], [0, 1], clamp)}}>
        <Eyebrow index={5} label="RECOVERY · 中断恢复" delay={2} />
        <div style={{position: 'absolute', right: 132, top: 78}}>
          <DisclaimerTag text="本地 SQLite · 事务恢复" />
        </div>
      </AbsoluteFill>

      <div style={{position: 'absolute', left: 132, top: 138, opacity: titleP}}>
        <Reveal delay={18} y={20}>
          <div style={{fontFamily: FONT.sans, fontSize: 56, fontWeight: 700, color: COLORS.text}}>
            中断之后，从核对过的位置继续
          </div>
          <div style={{marginTop: 14, fontFamily: FONT.sans, fontSize: 26, color: COLORS.muted}}>
            先只读检查 Checkpoint、进度、回执与清单，再决定是否继续
          </div>
        </Reveal>
      </div>

      {/* commit counter */}
      <div
        style={{
          position: 'absolute',
          right: 132,
          top: 240,
          display: 'flex',
          alignItems: 'baseline',
          gap: 8,
          opacity: interpolate(frame, [180, 200], [0, 1], clamp),
        }}
      >
        <span style={{fontFamily: FONT.mono, fontSize: 56, fontWeight: 700, color: COLORS.teal}}>
          {count}
        </span>
        <span style={{fontFamily: FONT.mono, fontSize: 26, color: COLORS.muted2}}>/ 6</span>
        <span style={{fontFamily: FONT.sans, fontSize: 17, color: COLORS.muted2, marginLeft: 10}}>
          批次已提交
        </span>
      </div>

      <BatchTimeline
        states={states}
        frame={frame}
        enter={12}
        y={300}
        w={256}
        h={140}
        gap={24}
        dimmed={dimmed}
      />

      {/* Status line during normal run */}
      <div
        style={{
          position: 'absolute',
          left: 132,
          top: 470,
          fontFamily: FONT.sans,
          fontSize: 21,
          color: COLORS.muted,
          opacity:
            interpolate(frame, [200, 220], [0, 1], clamp) *
            interpolate(frame, [faultAt, faultAt + 18], [1, 0], clamp),
        }}
      >
        批次事务提交时，同步写入 Outbox 回执与检查点
      </div>

      {/* Fault state */}
      <FaultOverlay frame={frame} at={faultAt} until={resumeAt} />

      {/* Status card after fault */}
      <div
        style={{
          position: 'absolute',
          left: 132,
          top: 560,
          width: 900,
          opacity: statusP,
          transform: `translateY(${(1 - statusP) * 14}px)`,
        }}
      >
        <div
          style={{
            padding: '18px 22px',
            borderRadius: 14,
            border: `1px solid ${COLORS.amber}55`,
            background: 'rgba(11,27,44,0.94)',
          }}
        >
          <div style={{display: 'flex', alignItems: 'center', gap: 12}}>
            <span style={{fontFamily: FONT.mono, fontSize: 15, letterSpacing: 2, color: COLORS.amber}}>
              maskriver status --target target.db --run-id &lt;32-hex&gt;
            </span>
            <span
              style={{
                fontFamily: FONT.mono,
                fontSize: 12,
                letterSpacing: 1.4,
                color: COLORS.amber,
                border: `1px solid ${COLORS.amber}55`,
                background: 'rgba(255,153,106,0.10)',
                borderRadius: 6,
                padding: '3px 8px',
              }}
            >
              READ ONLY
            </span>
          </div>
          <div
            style={{
              marginTop: 12,
              display: 'flex',
              gap: 34,
              fontFamily: FONT.mono,
              fontSize: 17,
              color: COLORS.muted,
            }}
          >
            <span>
              state <span style={{color: COLORS.text}}>recoverable</span>
            </span>
            <span>
              committed <span style={{color: COLORS.teal}}>3 / 6</span>
            </span>
            <span>
              commit_unknown <span style={{color: COLORS.text}}>false</span>
            </span>
            <span>
              conflict <span style={{color: COLORS.text}}>false</span>
            </span>
          </div>
          <div style={{marginTop: 10, fontFamily: FONT.sans, fontSize: 17, color: COLORS.muted2}}>
            只读查询 · 零写入：恢复前不改变目标数据
          </div>
        </div>
      </div>

      {/* Read-only preflight checks */}
      <div
        style={{
          position: 'absolute',
          right: 132,
          top: 560,
          width: 728,
          opacity: interpolate(frame, [faultAt + 26, faultAt + 46], [0, 1], clamp),
        }}
      >
        <div
          style={{
            padding: '16px 20px',
            borderRadius: 14,
            border: `1px solid ${COLORS.line}`,
            background: 'rgba(11,27,44,0.92)',
          }}
        >
          <div style={{fontFamily: FONT.mono, fontSize: 14, letterSpacing: 2, color: COLORS.muted2}}>
            RECOVERY PREFLIGHT · read only
          </div>
          <div style={{display: 'grid', gridTemplateColumns: '1fr 1fr', columnGap: 20, rowGap: 10}}>
            {[
              ['Checkpoint 可读', 'checkpoint row'],
              ['Progress 前缀连续', 'append-only log'],
              ['Outbox 回执匹配', 'receipt match'],
              ['Manifest 校验和一致', 'manifest CAS'],
            ].map(([cn, en], i) => {
              const p = spring({frame: frame - (faultAt + 40 + i * 12), fps, config: SPRING.crisp});
              return (
                <div
                  key={en}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 10,
                    opacity: p,
                    transform: `translateX(${(1 - p) * 8}px)`,
                  }}
                >
                  <svg width={15} height={15} viewBox="0 0 16 16" style={{flex: 'none'}}>
                    <path
                      d="M3 8.5 l3.2 3.4 L13 5"
                      fill="none"
                      stroke={COLORS.teal}
                      strokeWidth={2}
                      strokeLinecap="round"
                      strokeDasharray={18}
                      strokeDashoffset={18 * (1 - p)}
                    />
                  </svg>
                  <span style={{fontFamily: FONT.sans, fontSize: 16, color: COLORS.text, whiteSpace: 'nowrap'}}>
                    {cn}
                  </span>
                  <span style={{fontFamily: FONT.mono, fontSize: 11, color: COLORS.muted2, whiteSpace: 'nowrap'}}>
                    {en}
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      </div>

      {/* Resume command + verification */}
      <div
        style={{
          position: 'absolute',
          left: 132,
          top: 730,
          opacity: resumeP,
          transform: `translateY(${(1 - resumeP) * 14}px)`,
        }}
      >
        <CommandCard
          lines={[
            'maskriver mask --streaming --apply --resume --run-id <32-hex>',
            'verify committed prefix  →  skip 3 batches  →  continue from batch 04',
          ]}
          revealAt={[resumeAt, resumeAt + 26]}
          frame={frame}
          width={1200}
          size={19}
          title="resume"
          accent={COLORS.purple}
        />
      </div>

      {/* PASS + boundaries */}
      <div
        style={{
          position: 'absolute',
          right: 132,
          top: 730,
          width: 336,
          opacity: passP,
          transform: `translateY(${(1 - passP) * 12}px)`,
        }}
      >
        <div
          style={{
            padding: '16px 20px',
            borderRadius: 14,
            border: `1px solid ${COLORS.teal}66`,
            background: 'linear-gradient(150deg, rgba(88,224,186,0.12), rgba(11,27,44,0.95))',
          }}
        >
          <div style={{fontFamily: FONT.mono, fontSize: 18, letterSpacing: 1.6, color: COLORS.teal}}>
            VALIDATION PASSED
          </div>
          <div style={{marginTop: 8, fontFamily: FONT.sans, fontSize: 16, color: COLORS.muted, lineHeight: 1.6}}>
            恢复完成后再次严格验证
          </div>
          <div style={{marginTop: 10, fontFamily: FONT.sans, fontSize: 14, color: COLORS.muted2, lineHeight: 1.8}}>
            仅本地 SQLite · MySQL 恢复不支持
            <br />
            不宣称通用 exactly-once
            <br />
            目标被外部改动时拒绝继续
          </div>
        </div>
      </div>

</AbsoluteFill>
  );
};
