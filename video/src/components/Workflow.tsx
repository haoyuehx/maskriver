import React from 'react';
import {interpolate} from 'remotion';
import {COLORS, FONT} from '../styles/tokens';
import {STEPS} from '../data/fixtures';
import {VerdictChip, Meter} from './Primitives';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

const StepIcon: React.FC<{index: number; color: string; progress: number; size?: number}> = ({
  index,
  color,
  progress,
  size = 40,
}) => {
  const s = size;
  const common = {
    fill: 'none',
    stroke: color,
    strokeWidth: 2,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    strokeDasharray: 120,
    strokeDashoffset: 120 * (1 - Math.min(1, progress)),
    opacity: 0.4 + 0.6 * Math.min(1, progress),
  };
  return (
    <svg width={s} height={s} viewBox="0 0 40 40">
      {index === 0 && (
        <>
          <circle cx="18" cy="18" r="11" {...common} />
          <path d="M26 26 L34 34" {...common} />
          <path d="M13 18 h10" {...common} />
        </>
      )}
      {index === 1 && (
        <>
          <rect x="4" y="9" width="32" height="22" rx="3" {...common} />
          <path d="M4 20 h32" {...common} />
          <path d="M14 15 h8" {...common} />
        </>
      )}
      {index === 2 && (
        <>
          <path d="M20 5 L33 11 v10 c0 8-6 12-13 14-7-2-13-6-13-14V11Z" {...common} />
          <path d="M14 21 l5 5 9-10" {...common} />
        </>
      )}
      {index === 3 && (
        <>
          <rect x="6" y="12" width="22" height="18" rx="3" {...common} />
          <path d="M28 21 h8" {...common} />
          <path d="M31 17 l5 4 -5 4" {...common} />
        </>
      )}
      {index === 4 && (
        <>
          <circle cx="20" cy="20" r="13" {...common} />
          <path d="M13 20 l5 6 10-12" {...common} />
        </>
      )}
    </svg>
  );
};

/** Five-step rail. Used hero-sized in S2 and mini in S3/S6. */
export const WorkflowRail: React.FC<{
  active?: number;
  lit?: boolean[];
  compact?: boolean;
  y?: number;
  x0?: number;
  nodeW?: number;
  nodeH?: number;
  gap?: number;
  enter?: number;
  frame: number;
  showDesc?: boolean;
}> = ({
  active = -1,
  lit = [false, false, false, false, false],
  compact = false,
  y = 360,
  x0 = 132,
  nodeW = 312,
  nodeH = 200,
  gap = 24,
  enter = 0,
  frame,
  showDesc = false,
}) => {
  return (
    <>
      {STEPS.map((step, i) => {
        const p = interpolate(frame, [enter + i * 6, enter + i * 6 + 18], [0, 1], clamp);
        const x = x0 + i * (nodeW + gap);
        const isActive = active === i;
        const isLit = lit[i] || isActive;
        const color = isLit ? step.color : COLORS.muted2;
        return (
          <div
            key={step.key}
            style={{
              position: 'absolute',
              left: x,
              top: y,
              width: nodeW,
              height: nodeH,
              opacity: p,
              transform: `translateY(${(1 - p) * 22}px)`,
              borderRadius: 16,
              border: `1px solid ${isLit ? `${step.color}88` : COLORS.line}`,
              background: isLit
                ? `linear-gradient(150deg, ${step.color}1c 0%, rgba(11,27,44,0.96) 60%)`
                : 'rgba(11,27,44,0.82)',
              padding: compact ? 16 : 22,
              display: 'flex',
              flexDirection: 'column',
              gap: compact ? 6 : 12,
              overflow: 'hidden',
            }}
          >
            <div style={{display: 'flex', alignItems: 'center', gap: 10}}>
              <StepIcon index={i} color={color} progress={p} size={compact ? 22 : 34} />
              <span
                style={{
                  fontFamily: FONT.mono,
                  fontSize: compact ? 15 : 19,
                  letterSpacing: 2,
                  color,
                }}
              >
                {step.label.toUpperCase()}
              </span>
            </div>
            <div
              style={{
                fontFamily: FONT.sans,
                fontSize: compact ? 20 : 26,
                fontWeight: 500,
                color: isLit ? COLORS.text : COLORS.muted,
                letterSpacing: 0.5,
              }}
            >
              {step.cn}
            </div>
            {showDesc && (
              <div
                style={{
                  fontFamily: FONT.sans,
                  fontSize: 19,
                  lineHeight: 1.5,
                  color: COLORS.muted,
                }}
              >
                {step.desc}
              </div>
            )}
            <div style={{marginTop: 'auto'}}>
              <Meter
                progress={isLit ? 1 : 0}
                cap={1}
                width={compact ? nodeW - 36 : nodeW - 44}
                height={3}
                color={step.color}
                capColor="transparent"
              />
            </div>
          </div>
        );
      })}
      {/* connectors */}
      {STEPS.slice(0, 4).map((_, i) => {
        const x = x0 + i * (nodeW + gap) + nodeW;
        const on = lit[i] && lit[i + 1];
        return (
          <div
            key={`c-${i}`}
            style={{
              position: 'absolute',
              left: x,
              top: y + nodeH / 2,
              width: gap,
              height: 1.5,
              background: on ? COLORS.cyan : COLORS.line,
              opacity: on ? 0.9 : 0.6,
            }}
          />
        );
      })}
    </>
  );
};

/** Six-chapter rail for the walkthrough scene. */
export const ChapterRail: React.FC<{
  chapters: string[];
  active: number;
  frame: number;
  y?: number;
  x0?: number;
  w?: number;
  gap?: number;
  enter?: number;
}> = ({chapters, active, frame, y = 150, x0 = 132, w = 246, gap = 20, enter = 0}) => (
  <>
    {chapters.map((c, i) => {
      const p = interpolate(frame, [enter + i * 5, enter + i * 5 + 16], [0, 1], clamp);
      const on = active === i;
      const done = i < active;
      const color = on ? COLORS.cyan : done ? COLORS.teal : COLORS.muted2;
      return (
        <div
          key={c}
          style={{
            position: 'absolute',
            left: x0 + i * (w + gap),
            top: y,
            width: w,
            height: 52,
            opacity: p * (on || done ? 1 : 0.66),
            transform: `translateY(${(1 - p) * 12}px)`,
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            padding: '0 16px',
            borderRadius: 10,
            border: `1px solid ${on ? `${COLORS.cyan}88` : COLORS.line}`,
            background: on ? 'rgba(99,220,243,0.12)' : 'rgba(11,27,44,0.7)',
            fontFamily: FONT.sans,
            fontSize: 20,
            color: on ? COLORS.text : COLORS.muted,
            whiteSpace: 'nowrap',
          }}
        >
          <span
            style={{
              fontFamily: FONT.mono,
              fontSize: 14,
              color,
              letterSpacing: 1,
            }}
          >
            {String(i + 1).padStart(2, '0')}
          </span>
          {c}
        </div>
      );
    })}
  </>
);

/** Streaming pipeline: bounded reader -> queue -> workers -> txn -> outbox -> target. */
export const PipelineFlow: React.FC<{
  nodes: {label: string; detail: string}[];
  frame: number;
  enter?: number;
  y?: number;
  nodeW?: number;
  nodeH?: number;
  gap?: number;
  x0?: number;
  active?: number;
  meterAt?: number;
  showPackets?: boolean;
}> = ({
  nodes,
  frame,
  enter = 0,
  y = 386,
  nodeW = 256,
  nodeH = 216,
  gap = 24,
  x0 = 132,
  active = -1,
  meterAt = 0,
  showPackets = true,
}) => (
  <>
    {nodes.map((n, i) => {
      const p = interpolate(frame, [enter + i * 6, enter + i * 6 + 18], [0, 1], clamp);
      const x = x0 + i * (nodeW + gap);
      const on = i <= active;
      return (
        <div
          key={n.label}
          style={{
            position: 'absolute',
            left: x,
            top: y,
            width: nodeW,
            height: nodeH,
            opacity: p,
            transform: `translateY(${(1 - p) * 20}px)`,
            borderRadius: 16,
            border: `1px solid ${on ? 'rgba(99,220,243,0.55)' : COLORS.line}`,
            background: on
              ? 'linear-gradient(160deg, rgba(99,220,243,0.10), rgba(11,27,44,0.95))'
              : 'rgba(11,27,44,0.82)',
            padding: 18,
            display: 'flex',
            flexDirection: 'column',
            gap: 8,
          }}
        >
          <div
            style={{
              fontFamily: FONT.mono,
              fontSize: 13,
              letterSpacing: 2,
              color: COLORS.muted2,
            }}
          >
            {String(i + 1).padStart(2, '0')}
          </div>
          <div
            style={{
              fontFamily: FONT.sans,
              fontSize: 23,
              fontWeight: 500,
              color: COLORS.text,
              lineHeight: 1.3,
            }}
          >
            {n.label}
          </div>
          <div style={{fontFamily: FONT.sans, fontSize: 17, color: COLORS.muted, lineHeight: 1.45}}>
            {n.detail}
          </div>
          <div style={{marginTop: 'auto'}}>
            <Meter
              progress={interpolate(frame, [meterAt + i * 8, meterAt + i * 8 + 20], [0, 0.68], clamp)}
              cap={0.68}
              width={nodeW - 36}
              height={3}
              color={COLORS.cyan}
            />
          </div>
        </div>
      );
    })}
    {/* connectors */}
    {nodes.slice(0, -1).map((_, i) => {
      const x = x0 + i * (nodeW + gap) + nodeW;
      const start = enter + i * 6 + 12;
      const p = interpolate(frame, [start, start + 20], [0, 1], clamp);
      return (
        <div key={`pc-${i}`}>
          <div
            style={{
              position: 'absolute',
              left: x,
              top: y + nodeH / 2,
              width: gap * p,
              height: 1.5,
              background: COLORS.lineBright,
            }}
          />
          {showPackets && p > 0.9 && (
            <div
              style={{
                position: 'absolute',
                left: x + ((frame * 2 + i * 9) % (gap + 14)) - 7,
                top: y + nodeH / 2 - 3,
                width: 6,
                height: 6,
                borderRadius: 3,
                background: COLORS.cyan,
                opacity: 0.9,
              }}
            />
          )}
        </div>
      );
    })}
  </>
);

export type BatchState = 'idle' | 'committed' | 'verified' | 'current' | 'blocked';

/** Checkpoint timeline cells (S5). */
export const BatchTimeline: React.FC<{
  states: BatchState[];
  frame: number;
  enter?: number;
  y?: number;
  x0?: number;
  w?: number;
  h?: number;
  gap?: number;
  dimmed?: boolean;
}> = ({states, frame, enter = 0, y = 386, x0 = 132, w = 256, h = 142, gap = 24, dimmed = false}) => (
  <>
    {states.map((state, i) => {
      const p = interpolate(frame, [enter + i * 5, enter + i * 5 + 16], [0, 1], clamp);
      const color =
        state === 'committed' || state === 'verified'
          ? COLORS.teal
          : state === 'current'
            ? COLORS.purple
            : state === 'blocked'
              ? COLORS.danger
              : COLORS.muted2;
      const filled = state === 'committed' || state === 'verified' || state === 'current';
      return (
        <div
          key={i}
          style={{
            position: 'absolute',
            left: x0 + i * (w + gap),
            top: y,
            width: w,
            height: h,
            opacity: p * (dimmed && state === 'idle' ? 0.4 : 1),
            transform: `translateY(${(1 - p) * 16}px)`,
            borderRadius: 14,
            border: `1px solid ${filled ? `${color}99` : COLORS.line}`,
            background: filled ? `${color}14` : 'rgba(11,27,44,0.8)',
            padding: 18,
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'space-between',
          }}
        >
          <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center'}}>
            <span style={{fontFamily: FONT.mono, fontSize: 17, color: COLORS.muted2, letterSpacing: 1.4}}>
              BATCH {String(i + 1).padStart(2, '0')}
            </span>
            <span
              style={{
                width: 9,
                height: 9,
                borderRadius: 5,
                background: filled ? color : COLORS.line,
              }}
            />
          </div>
          <div style={{fontFamily: FONT.sans, fontSize: 20, color: filled ? COLORS.text : COLORS.muted2}}>
            {state === 'committed'
              ? '已提交'
              : state === 'verified'
                ? '已核对'
                : state === 'current'
                  ? '处理中'
                  : state === 'blocked'
                    ? '冲突'
                    : '待处理'}
          </div>
          <div style={{display: 'flex', alignItems: 'center', gap: 8}}>
            {filled && (
              <svg width={16} height={16} viewBox="0 0 16 16">
                <path
                  d="M3 8.5 l3.4 3.4 L13 5"
                  fill="none"
                  stroke={color}
                  strokeWidth={2}
                  strokeLinecap="round"
                  strokeDasharray={18}
                  strokeDashoffset={state === 'current' ? 18 : 0}
                />
              </svg>
            )}
            <span style={{fontFamily: FONT.mono, fontSize: 14, color: COLORS.muted2}}>
              {state === 'verified' ? 'receipt ok' : filled ? 'txn commit' : 'queued'}
            </span>
          </div>
        </div>
      );
    })}
  </>
);

/** Apply gate: three independent conditions. */
export const GateLocks: React.FC<{
  states: [number, number, number];
  labels: string[];
  frame: number;
  x0?: number;
  y?: number;
  gap?: number;
}> = ({states, labels, x0 = 132, y = 600, gap = 32}) => {
  const w = 420;
  return (
    <>
      {states.map((p, i) => {
        const closed = p < 0.98;
        const color = closed ? COLORS.muted2 : COLORS.teal;
        return (
          <div
            key={i}
            style={{
              position: 'absolute',
              left: x0 + i * (w + gap),
              top: y,
              width: w,
              height: 76,
              display: 'flex',
              alignItems: 'center',
              gap: 14,
              padding: '0 18px',
              borderRadius: 12,
              border: `1px solid ${closed ? COLORS.line : 'rgba(88,224,186,0.55)'}`,
              background: closed ? 'rgba(11,27,44,0.8)' : 'rgba(88,224,186,0.10)',
            }}
          >
            <svg width={26} height={26} viewBox="0 0 26 26">
              <rect x="5" y="12" width="16" height="10" rx="2" fill="none" stroke={color} strokeWidth={1.8} />
              <path
                d={closed ? 'M9 12V9a4 4 0 018 0v3' : 'M9 12V9a4 4 0 018 0'}
                fill="none"
                stroke={color}
                strokeWidth={1.8}
                strokeLinecap="round"
                strokeDasharray={22}
                strokeDashoffset={(1 - p) * 22}
              />
            </svg>
            <div style={{display: 'flex', flexDirection: 'column', gap: 2}}>
              <span style={{fontFamily: FONT.mono, fontSize: 16, color, letterSpacing: 1.2}}>
                {labels[i]}
              </span>
              <span style={{fontFamily: FONT.sans, fontSize: 16, color: COLORS.muted2}}>
                {p > 0.98 ? '条件满足' : '等待'}
              </span>
            </div>
          </div>
        );
      })}
    </>
  );
};

/** Validation checklist with staggered checks. */
export const CheckList: React.FC<{
  items: {label: string; sub?: string}[];
  frame: number;
  checkAt: number[];
  x?: number;
  y?: number;
  rowH?: number;
  width?: number;
  size?: number;
}> = ({items, frame, checkAt, x = 132, y = 400, rowH = 62, width = 720, size = 23}) => (
  <>
    {items.map((item, i) => {
      const at = checkAt[i] ?? 0;
      const p = interpolate(frame, [at, at + 12], [0, 1], clamp);
      const done = p > 0.98;
      return (
        <div
          key={item.label}
          style={{
            position: 'absolute',
            left: x,
            top: y + i * rowH,
            width,
            height: rowH - 8,
            display: 'flex',
            alignItems: 'center',
            gap: 14,
            borderBottom: `1px solid ${COLORS.line}66`,
          }}
        >
          <span
            style={{
              width: 22,
              height: 22,
              borderRadius: 11,
              border: `1.5px solid ${done ? COLORS.teal : COLORS.lineBright}`,
              background: done ? 'rgba(88,224,186,0.16)' : 'transparent',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flex: 'none',
            }}
          >
            <svg width={14} height={14} viewBox="0 0 14 14">
              <path
                d="M2.5 7.4 l3 3.1 L11.5 3.8"
                fill="none"
                stroke={COLORS.teal}
                strokeWidth={2}
                strokeLinecap="round"
                strokeDasharray={14}
                strokeDashoffset={14 * (1 - p)}
              />
            </svg>
          </span>
          <span
            style={{
              fontFamily: FONT.sans,
              fontSize: size,
              color: done ? COLORS.text : COLORS.muted,
              flex: 1,
            }}
          >
            {item.label}
          </span>
          {item.sub && (
            <span style={{fontFamily: FONT.mono, fontSize: 15, color: COLORS.muted2}}>{item.sub}</span>
          )}
          <span style={{fontFamily: FONT.mono, fontSize: 14, color: done ? COLORS.teal : COLORS.muted2}}>
            {done ? 'PASS' : '...'}
          </span>
        </div>
      );
    })}
  </>
);

/** Ed25519 approval record card (purple domain). */
export const ApprovalCard: React.FC<{
  frame: number;
  enter?: number;
  x?: number;
  y?: number;
  width?: number;
  state?: 'locked' | 'verified';
  scopes?: string[];
  digest?: string;
}> = ({
  frame,
  enter = 0,
  x = 700,
  y = 380,
  width = 1088,
  state = 'verified',
  scopes = ['people.name', 'people.email', 'people.mobile'],
  digest = 'sha256:9f2c…a41b',
}) => {
  const p = interpolate(frame, [enter, enter + 20], [0, 1], clamp);
  const ok = state === 'verified';
  const color = ok ? COLORS.purple : COLORS.muted2;
  return (
    <div
      style={{
        position: 'absolute',
        left: x,
        top: y,
        width,
        padding: 28,
        borderRadius: 16,
        border: `1px solid ${color}77`,
        background: ok
          ? 'linear-gradient(150deg, rgba(180,156,255,0.14), rgba(11,27,44,0.96))'
          : 'rgba(11,27,44,0.86)',
        opacity: p,
        transform: `translateY(${(1 - p) * 18}px)`,
        display: 'flex',
        gap: 26,
      }}
    >
      <svg width={56} height={56} viewBox="0 0 56 56" style={{flex: 'none'}}>
        <circle cx="28" cy="28" r="26" fill="none" stroke={`${color}55`} strokeWidth={1.4} />
        <path
          d="M20 28a11 11 0 1122 0"
          fill="none"
          stroke={color}
          strokeWidth={2.6}
          strokeLinecap="round"
        />
        <path d="M20 34h16" stroke={color} strokeWidth={2.6} strokeLinecap="round" />
        <path d="M20 40h10" stroke={color} strokeWidth={2.6} strokeLinecap="round" opacity={0.6} />
      </svg>
      <div style={{flex: 1, display: 'flex', flexDirection: 'column', gap: 10}}>
        <div style={{display: 'flex', alignItems: 'center', gap: 14}}>
          <span style={{fontFamily: FONT.mono, fontSize: 16, letterSpacing: 2, color}}>
            ED25519 APPROVAL RECORD
          </span>
          <VerdictChip kind={ok ? 'PASS' : 'UNKNOWN'} size={15} progress={p} />
        </div>
        <div style={{display: 'flex', gap: 40}}>
          <Field label="plan digest" value={digest} />
          <Field label="reviewer" value="reviewer-01" />
          <Field label="record" value="approval.json" />
        </div>
        <div style={{display: 'flex', gap: 10, flexWrap: 'wrap', marginTop: 2}}>
          {scopes.map((s, i) => {
            const sp = interpolate(frame, [enter + 14 + i * 6, enter + 26 + i * 6], [0, 1], clamp);
            return (
              <span
                key={s}
                style={{
                  fontFamily: FONT.mono,
                  fontSize: 16,
                  padding: '4px 12px',
                  borderRadius: 8,
                  border: `1px solid ${COLORS.line}`,
                  background: 'rgba(8,20,34,0.8)',
                  color: COLORS.muted,
                  opacity: sp,
                }}
              >
                {s}
              </span>
            );
          })}
        </div>
      </div>
    </div>
  );
};

const Field: React.FC<{label: string; value: string}> = ({label, value}) => (
  <div style={{display: 'flex', flexDirection: 'column', gap: 4}}>
    <span style={{fontFamily: FONT.mono, fontSize: 13, letterSpacing: 1.4, color: COLORS.muted2}}>
      {label}
    </span>
    <span style={{fontFamily: FONT.mono, fontSize: 19, color: COLORS.text}}>{value}</span>
  </div>
);
