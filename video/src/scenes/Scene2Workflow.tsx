import React from 'react';
import {AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {COLORS, FONT, SPRING} from '../styles/tokens';
import {Backdrop} from '../components/Backdrop';
import {Eyebrow, Reveal} from '../components/Reveal';
import {CheckList, GateLocks, ApprovalCard, WorkflowRail} from '../components/Workflow';
import {BeforeAfterSplit, StatCard} from '../components/Compare';
import {VerdictChip, Meter} from '../components/Primitives';
import {DisclaimerTag} from '../components/Subtitles';
import {FIELD_EVIDENCE, STEPS} from '../data/fixtures';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

const PhaseHeader: React.FC<{label: string; title: string; lead?: string; frame: number; at: number}> = ({
  label,
  title,
  lead,
  frame,
  at,
}) => {
  const p = interpolate(frame, [at, at + 16], [0, 1], clamp);
  return (
    <div style={{position: 'absolute', left: 132, top: 236, opacity: p, transform: `translateY(${(1 - p) * 14}px)`}}>
      <div style={{display: 'flex', alignItems: 'center', gap: 14}}>
        <span
          style={{
            fontFamily: FONT.mono,
            fontSize: 15,
            letterSpacing: 2.4,
            color: COLORS.cyan,
            border: `1px solid ${COLORS.cyan}55`,
            borderRadius: 6,
            padding: '4px 10px',
          }}
        >
          {label}
        </span>
        <span style={{fontFamily: FONT.sans, fontSize: 30, fontWeight: 700, color: COLORS.text}}>
          {title}
        </span>
      </div>
      {lead && (
        <div style={{marginTop: 8, fontFamily: FONT.sans, fontSize: 21, color: COLORS.muted, letterSpacing: 0.6}}>
          {lead}
        </div>
      )}
    </div>
  );
};

const PanelBox: React.FC<{
  x: number;
  y: number;
  w: number;
  h: number;
  opacity?: number;
  title?: string;
  accent?: string;
  children?: React.ReactNode;
}> = ({x, y, w, h, opacity = 1, title, accent = COLORS.line, children}) => (
  <div
    style={{
      position: 'absolute',
      left: x,
      top: y,
      width: w,
      height: h,
      padding: 22,
      borderRadius: 16,
      border: `1px solid ${accent}`,
      background: 'linear-gradient(155deg, rgba(14,34,53,0.92), rgba(9,22,37,0.94))',
      opacity,
      overflow: 'hidden',
    }}
  >
    {title && (
      <div
        style={{
          fontFamily: FONT.mono,
          fontSize: 14,
          letterSpacing: 2,
          color: COLORS.muted2,
          marginBottom: 14,
        }}
      >
        {title}
      </div>
    )}
    {children}
  </div>
);

/** S2 — 五步核心流程（0:25–1:05, 1200 帧） */
export const Scene2Workflow: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  const intro = spring({frame: frame - 1, fps, config: SPRING.settle});
  const railHeroOut = interpolate(frame, [150, 176], [1, 0], clamp);
  const railMiniIn = interpolate(frame, [156, 180], [0, 1], clamp);

  const phase = (start: number, end: number) =>
    interpolate(frame, [start, start + 14, end - 14, end], [0, 1, 1, 0], clamp);

  const scanP = phase(150, 330);
  const previewP = phase(330, 510);
  const approveP = phase(510, 690);
  const applyP = phase(690, 870);
  const validateP = phase(870, 1050);
  const recapP = phase(1030, 1180);

  const activeIndex = frame < 330 ? 0 : frame < 510 ? 1 : frame < 690 ? 2 : frame < 870 ? 3 : 4;

  const swapRows = [1, 2, 3];
  const swapAt = [760, 800, 840];

  const outFade = interpolate(frame, [1184, 1200], [1, 0], clamp);

  return (
    <AbsoluteFill style={{opacity: outFade}}>
      <Backdrop grid="flat" glow="both" />

      <AbsoluteFill style={{opacity: interpolate(frame, [0, 12], [0, 1], clamp)}}>
        <Eyebrow index={2} label="WORKFLOW · 五步流程" delay={2} />
        <div style={{position: 'absolute', right: 132, top: 78}}>
          <DisclaimerTag />
        </div>
      </AbsoluteFill>

      {/* Title */}
      <div style={{position: 'absolute', left: 132, top: 118, opacity: intro * railHeroOut}}>
        <Reveal delay={2} y={22}>
          <div style={{fontFamily: FONT.sans, fontSize: 46, fontWeight: 700, color: COLORS.text}}>
            五步闭环，处处有据可查
          </div>
        </Reveal>
      </div>

      {/* Hero rail */}
      <AbsoluteFill style={{opacity: railHeroOut}}>
        <WorkflowRail frame={frame} enter={8} y={330} showDesc active={activeIndex} />
      </AbsoluteFill>

      {/* Mini step strip */}
      <AbsoluteFill style={{opacity: railMiniIn}}>
        <div style={{position: 'absolute', left: 132, top: 180, display: 'flex', gap: 12}}>
          {STEPS.map((s, i) => {
            const on = i === activeIndex;
            const done = i < activeIndex;
            const color = on ? s.color : done ? COLORS.teal : COLORS.muted2;
            return (
              <div
                key={s.key}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '8px 16px',
                  borderRadius: 8,
                  border: `1px solid ${on ? `${s.color}88` : COLORS.line}`,
                  background: on ? `${s.color}18` : 'rgba(11,27,44,0.7)',
                  fontFamily: FONT.mono,
                  fontSize: 16,
                  letterSpacing: 1.4,
                  color,
                }}
              >
                <span style={{width: 5, height: 5, borderRadius: 3, background: color}} />
                {s.label.toUpperCase()}
              </div>
            );
          })}
        </div>
      </AbsoluteFill>

      {/* SCAN */}
      <AbsoluteFill style={{opacity: scanP}}>
        <PhaseHeader
          label="SCAN"
          title="先识别，再决定"
          lead="13 类格式与上下文规则；证据不足保留 UNKNOWN"
          frame={frame}
          at={158}
        />
        <PanelBox x={132} y={318} w={820} h={468} title="SAMPLE · customers.mobile">
          {['199 0000 0011', '199 0000 0012', '199 0000 0013', '199 0000 0014', '199 0000 0015'].map(
            (v, i) => {
              const p = spring({frame: frame - 158 - i * 8, fps, config: SPRING.snappy});
              const hit = interpolate(frame, [230 + i * 10, 244 + i * 10], [0, 1], clamp);
              return (
                <div
                  key={v}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 16,
                    height: 58,
                    opacity: p,
                    borderBottom: `1px solid ${COLORS.line}44`,
                  }}
                >
                  <span style={{fontFamily: FONT.mono, fontSize: 22, color: COLORS.muted}}>{v}</span>
                  <span style={{marginLeft: 'auto', display: 'flex', gap: 8, alignItems: 'center'}}>
                    <span
                      style={{
                        width: 7,
                        height: 7,
                        borderRadius: 4,
                        background: COLORS.cyan,
                        opacity: hit,
                      }}
                    />
                    <span style={{fontFamily: FONT.mono, fontSize: 15, color: COLORS.cyan, opacity: hit}}>
                      cn_mobile
                    </span>
                  </span>
                </div>
              );
            },
          )}
          <div style={{marginTop: 18, display: 'flex', alignItems: 'center', gap: 14}}>
            <span style={{fontFamily: FONT.sans, fontSize: 19, color: COLORS.muted2}}>抽样 5 / 6 命中</span>
            <Meter progress={0.9} cap={0.9} width={260} height={4} />
          </div>
        </PanelBox>

        <PanelBox x={988} y={318} w={800} h={468} title="VERDICTS">
          {FIELD_EVIDENCE.map((f, i) => {
            const p = spring({frame: frame - 168 - i * 10, fps, config: SPRING.snappy});
            return (
              <div
                key={f.field}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 14,
                  height: 74,
                  opacity: p,
                  transform: `translateX(${(1 - p) * 12}px)`,
                  borderBottom: `1px solid ${COLORS.line}44`,
                }}
              >
                <span style={{fontFamily: FONT.mono, fontSize: 21, color: COLORS.text, width: 120}}>
                  {f.field}
                </span>
                <span style={{fontFamily: FONT.mono, fontSize: 16, color: COLORS.muted2, width: 190}}>
                  {f.rule}
                </span>
                <span style={{marginLeft: 'auto'}}>
                  <VerdictChip
                    kind={
                      f.verdict === 'SENSITIVE'
                        ? 'SENSITIVE'
                        : f.verdict === 'REVIEW'
                          ? 'REVIEW'
                          : 'UNKNOWN'
                    }
                    size={15}
                  />
                </span>
              </div>
            );
          })}
          <div style={{marginTop: 16, fontFamily: FONT.sans, fontSize: 19, color: COLORS.amber, letterSpacing: 0.4}}>
            UNKNOWN 不是安全，需要人工确认后才进入计划
          </div>
        </PanelBox>
      </AbsoluteFill>

      {/* PREVIEW */}
      <AbsoluteFill style={{opacity: previewP}}>
        <PhaseHeader
          label="PREVIEW"
          title="先出计划，再说写入"
          lead="默认 Dry Run；预览由驱动级证据证明零写入"
          frame={frame}
          at={338}
        />
        <BeforeAfterSplit
          columns={['id', 'name', 'email']}
          before={[
            [{text: '001'}, {text: '林晓', tone: 'source'}, {text: 'lin@example.test', tone: 'source'}],
            [{text: '002'}, {text: '陈雨', tone: 'source'}, {text: 'chen@example.test', tone: 'source'}],
            [{text: '003'}, {text: '赵明', tone: 'source'}, {text: 'zhao@example.test', tone: 'source'}],
          ]}
          after={[
            [{text: '001'}, {text: '林**', tone: 'masked'}, {text: 'l***@example.test', tone: 'masked'}],
            [{text: '002'}, {text: '陈**', tone: 'masked'}, {text: 'c***@example.test', tone: 'masked'}],
            [{text: '003'}, {text: '赵**', tone: 'masked'}, {text: 'z***@example.test', tone: 'masked'}],
          ]}
          frame={frame}
          enter={360}
          x={132}
          y={330}
          tableW={720}
          gap={120}
        />
        <div style={{position: 'absolute', left: 132, top: 716, display: 'flex', gap: 16, alignItems: 'center'}}>
          <span
            style={{
              fontFamily: FONT.mono,
              fontSize: 16,
              letterSpacing: 1.6,
              color: COLORS.teal,
              border: `1px solid ${COLORS.teal}55`,
              background: 'rgba(88,224,186,0.10)',
              borderRadius: 8,
              padding: '8px 16px',
              opacity: interpolate(frame, [400, 418], [0, 1], clamp),
            }}
          >
            DRY RUN · WRITES = 0
          </span>
          <span
            style={{
              fontFamily: FONT.sans,
              fontSize: 20,
              color: COLORS.muted,
              opacity: interpolate(frame, [414, 430], [0, 1], clamp),
            }}
          >
            计划摘要绑定表、字段、策略与范围
          </span>
        </div>
      </AbsoluteFill>

      {/* APPROVE */}
      <AbsoluteFill style={{opacity: approveP}}>
        <PhaseHeader
          label="APPROVE"
          title="签名与计划绑定"
          lead="Ed25519 审批记录；执行侧只加载可信公钥"
          frame={frame}
          at={518}
        />
        <div
          style={{
            position: 'absolute',
            left: 132,
            top: 400,
            width: 500,
            opacity: interpolate(frame, [528, 546], [0, 1], clamp),
          }}
        >
          <div style={{fontFamily: FONT.mono, fontSize: 15, letterSpacing: 2, color: COLORS.muted2}}>
            PLAN DIGEST
          </div>
          <div
            style={{
              marginTop: 12,
              padding: '18px 20px',
              borderRadius: 12,
              border: `1px solid ${COLORS.line}`,
              background: 'rgba(11,27,44,0.85)',
              fontFamily: FONT.mono,
              fontSize: 20,
              color: COLORS.text,
              letterSpacing: 1,
            }}
          >
            sha256:9f2c…a41b
          </div>
          <div style={{marginTop: 18, fontFamily: FONT.sans, fontSize: 19, color: COLORS.muted, lineHeight: 1.7}}>
            记录绑定当前计划摘要与策略范围；
            <br />
            计划一旦变化，签名即失效。
          </div>
        </div>
        <ApprovalCard
          frame={frame}
          enter={546}
          x={700}
          y={392}
          width={1088}
          state={frame > 600 ? 'verified' : 'locked'}
        />
        <div
          style={{
            position: 'absolute',
            left: 700,
            top: 626,
            display: 'flex',
            gap: 14,
            alignItems: 'center',
            opacity: interpolate(frame, [606, 624], [0, 1], clamp),
          }}
        >
          <VerdictChip kind="PASS" size={16} />
          <span style={{fontFamily: FONT.sans, fontSize: 19, color: COLORS.muted}}>
            未获授权时写入通道保持锁定
          </span>
        </div>
        <div
          style={{
            position: 'absolute',
            left: 132,
            top: 690,
            display: 'flex',
            gap: 24,
            opacity: interpolate(frame, [618, 636], [0, 1], clamp),
          }}
        >
          {[
            ['记录绑定计划摘要', 'plan digest'],
            ['执行侧只加载公钥', 'public key only'],
            ['计划变化即失效', 'signature invalid'],
          ].map(([cn, en]) => (
            <div
              key={en}
              style={{
                width: 500,
                padding: '14px 18px',
                borderRadius: 12,
                border: `1px solid ${COLORS.line}`,
                background: 'rgba(11,27,44,0.8)',
              }}
            >
              <div style={{fontFamily: FONT.sans, fontSize: 20, color: COLORS.text}}>{cn}</div>
              <div style={{fontFamily: FONT.mono, fontSize: 14, color: COLORS.muted2, marginTop: 4}}>
                {en}
              </div>
            </div>
          ))}
        </div>
      </AbsoluteFill>

      {/* APPLY */}
      <AbsoluteFill style={{opacity: applyP}}>
        <PhaseHeader
          label="APPLY"
          title="三个条件同时成立"
          lead="--approved 声明 + --apply 意图 + 有效签名记录"
          frame={frame}
          at={698}
        />
        <GateLocks
          frame={frame}
          states={[
            interpolate(frame, [720, 740], [0, 1], clamp),
            interpolate(frame, [744, 764], [0, 1], clamp),
            interpolate(frame, [768, 788], [0, 1], clamp),
          ]}
          labels={['--approved', '--apply', 'approval record']}
          x0={132}
          y={330}
          gap={24}
        />
        <BeforeAfterSplit
          columns={['id', 'name', 'email']}
          before={[
            [{text: '001'}, {text: '林晓', tone: 'source'}, {text: 'lin@example.test', tone: 'source'}],
            [{text: '002'}, {text: '陈雨', tone: 'source'}, {text: 'chen@example.test', tone: 'source'}],
            [{text: '003'}, {text: '赵明', tone: 'source'}, {text: 'zhao@example.test', tone: 'source'}],
          ]}
          after={[
            [{text: '001'}, {text: '林**', tone: 'masked'}, {text: 'l***@example.test', tone: 'masked'}],
            [{text: '002'}, {text: '陈**', tone: 'masked'}, {text: 'c***@example.test', tone: 'masked'}],
            [{text: '003'}, {text: '赵**', tone: 'masked'}, {text: 'z***@example.test', tone: 'masked'}],
          ]}
          frame={frame}
          enter={720}
          x={132}
          y={440}
          tableW={680}
          gap={140}
          swapRows={swapRows}
          swapAt={swapAt}
        />
        <div style={{position: 'absolute', left: 132, top: 800, width: 1656}}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              fontFamily: FONT.mono,
              fontSize: 16,
              color: COLORS.muted2,
              letterSpacing: 1.2,
            }}
          >
            <span>batch commit progress</span>
            <span style={{color: COLORS.teal}}>
              {Math.round(interpolate(frame, [740, 860], [0, 100], clamp))}%
            </span>
          </div>
          <div style={{marginTop: 8}}>
            <Meter
              progress={interpolate(frame, [740, 860], [0, 1], clamp)}
              cap={1}
              width={1656}
              height={5}
              color={COLORS.teal}
              capColor="transparent"
            />
          </div>
        </div>
      </AbsoluteFill>

      {/* VALIDATE */}
      <AbsoluteFill style={{opacity: validateP}}>
        <PhaseHeader
          label="VALIDATE"
          title="验证通过，才算完成"
          lead="严格验证 = 全量扫描 + 适配器完整性检查"
          frame={frame}
          at={878}
        />
        <PanelBox x={132} y={330} w={900} h={470} title="STRICT CHECKLIST">
          <CheckList
            items={[
              {label: '行数一致', sub: 'count(source) = count(target)'},
              {label: '主键集合双向一致', sub: 'keyset symmetric diff = 0'},
              {label: 'Schema 与类型一致', sub: 'schema fingerprint'},
              {label: '仅计划字段发生变化', sub: 'planned columns only'},
              {label: '适配器完整性检查', sub: 'quick_check / foreign_key_check'},
            ]}
            frame={frame}
            checkAt={[900, 912, 924, 936, 948]}
            x={20}
            y={34}
            rowH={64}
            width={840}
            size={20}
          />
        </PanelBox>
        <div style={{position: 'absolute', left: 1084, top: 330, width: 704}}>
          <div
            style={{
              padding: 24,
              borderRadius: 16,
              border: `1px solid ${COLORS.teal}66`,
              background: 'linear-gradient(150deg, rgba(88,224,186,0.12), rgba(11,27,44,0.95))',
              opacity: interpolate(frame, [962, 988], [0, 1], clamp),
              transform: `translateY(${interpolate(frame, [962, 988], [16, 0], clamp)}px)`,
            }}
          >
            <div style={{display: 'flex', alignItems: 'center', gap: 14}}>
              <svg width={44} height={44} viewBox="0 0 44 44">
                <circle cx="22" cy="22" r="19" fill="none" stroke={COLORS.teal} strokeWidth={2} />
                <path
                  d="M13 22.5 l6 6.5 L31 15"
                  fill="none"
                  stroke={COLORS.teal}
                  strokeWidth={3}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeDasharray={36}
                  strokeDashoffset={interpolate(frame, [968, 996], [36, 0], clamp)}
                />
              </svg>
              <div>
                <div style={{fontFamily: FONT.mono, fontSize: 20, letterSpacing: 2, color: COLORS.teal}}>
                  VALIDATE · PASS
                </div>
                <div style={{fontFamily: FONT.sans, fontSize: 18, color: COLORS.muted, marginTop: 4}}>
                  未验证输出不会被当作合格结果
                </div>
              </div>
            </div>
          </div>
          <div
            style={{
              marginTop: 20,
              padding: 22,
              borderRadius: 16,
              border: `1px solid ${COLORS.line}`,
              background: 'rgba(11,27,44,0.8)',
              opacity: interpolate(frame, [990, 1008], [0, 1], clamp),
            }}
          >
            <div style={{fontFamily: FONT.sans, fontSize: 19, color: COLORS.muted, lineHeight: 1.8}}>
              验证失败即 fail closed：
              <br />
              不生成完成状态，也不继续写入。
            </div>
          </div>
        </div>
      </AbsoluteFill>

      {/* RECAP */}
      <AbsoluteFill style={{opacity: recapP}}>
        <div style={{position: 'absolute', left: 132, top: 320}}>
          <div style={{fontFamily: FONT.sans, fontSize: 46, fontWeight: 700, color: COLORS.text}}>
            扫描 → 预览 → 审批 → 执行 → 验证
          </div>
          <div style={{marginTop: 10, fontFamily: FONT.sans, fontSize: 24, color: COLORS.muted, letterSpacing: 1.2}}>
            一条可审计的脱敏链路
          </div>
        </div>
        <WorkflowRail
          frame={frame}
          enter={1064}
          y={470}
          nodeW={312}
          nodeH={190}
          lit={[true, true, true, true, frame > 1130]}
          active={frame > 1130 ? 4 : 4}
        />
        <div style={{position: 'absolute', left: 132, top: 730, display: 'flex', gap: 24}}>
          <StatCard label="计划绑定" value="1" unit="份摘要" x={0} y={0} frame={frame} enter={1040} w={384} />
        </div>
        <div style={{position: 'absolute', left: 548, top: 730}}>
          <StatCard label="写入条件" value="3" unit="项同时成立" x={0} y={0} frame={frame} enter={1052} w={384} />
        </div>
        <div style={{position: 'absolute', left: 964, top: 730}}>
          <StatCard label="验证方式" value="STRICT" x={0} y={0} frame={frame} enter={1064} w={384} />
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};
