import React from 'react';
import {AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {COLORS, FONT, SPRING} from '../styles/tokens';
import {Backdrop} from '../components/Backdrop';
import {DataTable} from '../components/DataTable';
import {Eyebrow} from '../components/Reveal';
import {ChapterRail, ApprovalCard} from '../components/Workflow';
import {CheckList} from '../components/Workflow';
import {MiniTable} from '../components/Compare';
import {VerdictChip, Meter} from '../components/Primitives';
import {DisclaimerTag} from '../components/Subtitles';
import {ROWS} from '../data/fixtures';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

const WS = {x: 300, y: 244, w: 1320, h: 592};

const CHAPTERS = ['数据源', '扫描', '预览', '审批', '执行', '验证'];

const Workspace: React.FC<{
  frame: number;
  active: number;
  opacity: number;
  children: React.ReactNode;
  headerRight?: string;
  mainLabel: string;
}> = ({frame, active, opacity, children, headerRight, mainLabel}) => (
  <div
    style={{
      position: 'absolute',
      left: WS.x,
      top: WS.y,
      width: WS.w,
      height: WS.h,
      borderRadius: 20,
      border: `1px solid ${COLORS.lineBright}`,
      background: 'linear-gradient(160deg, #0c1e30 0%, #081624 60%, #06111d 100%)',
      opacity,
      overflow: 'hidden',
    }}
  >
    {/* window header */}
    <div
      style={{
        height: 46,
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        padding: '0 18px',
        borderBottom: `1px solid ${COLORS.line}`,
        background: 'rgba(14,34,53,0.9)',
      }}
    >
      {['#ff7b6b', '#ff996a', '#58e0ba'].map((c) => (
        <span key={c} style={{width: 9, height: 9, borderRadius: 5, background: `${c}88`}} />
      ))}
      <span
        style={{
          marginLeft: 8,
          fontFamily: FONT.mono,
          fontSize: 14,
          letterSpacing: 1.6,
          color: COLORS.muted2,
        }}
      >
        maskriver · concept workspace
      </span>
      <span style={{marginLeft: 'auto', fontFamily: FONT.mono, fontSize: 13, color: COLORS.muted2}}>
        {headerRight ?? 'read only session'}
      </span>
    </div>

    <div style={{display: 'flex', height: WS.h - 46}}>
      {/* sidebar */}
      <div
        style={{
          width: 286,
          borderRight: `1px solid ${COLORS.line}`,
          padding: '16px 14px',
          display: 'flex',
          flexDirection: 'column',
          gap: 10,
          background: 'rgba(9,22,37,0.7)',
        }}
      >
        <div style={{fontFamily: FONT.mono, fontSize: 12, letterSpacing: 2, color: COLORS.muted2}}>
          TABLES
        </div>
        {[
          {name: 'people', rows: '6 rows'},
          {name: 'orders', rows: '12 rows'},
          {name: 'addresses', rows: '6 rows'},
        ].map((t, i) => {
          const on = i === 0;
          return (
            <div
              key={t.name}
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                padding: '9px 12px',
                borderRadius: 8,
                border: `1px solid ${on ? 'rgba(99,220,243,0.45)' : 'transparent'}`,
                background: on ? 'rgba(99,220,243,0.10)' : 'transparent',
                fontFamily: FONT.mono,
                fontSize: 15,
                color: on ? COLORS.text : COLORS.muted2,
              }}
            >
              {t.name}
              <span style={{fontSize: 12, color: COLORS.muted2}}>{t.rows}</span>
            </div>
          );
        })}

        <div style={{marginTop: 18, fontFamily: FONT.mono, fontSize: 12, letterSpacing: 2, color: COLORS.muted2}}>
          CHAPTER
        </div>
        {CHAPTERS.map((c, i) => {
          const on = i === active;
          return (
            <div
              key={c}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                fontFamily: FONT.sans,
                fontSize: 17,
                color: on ? COLORS.cyan : COLORS.muted2,
              }}
            >
              <span
                style={{
                  width: 4,
                  height: 4,
                  borderRadius: 2,
                  background: on ? COLORS.cyan : COLORS.line,
                }}
              />
              {c}
            </div>
          );
        })}
      </div>

      {/* main */}
      <div style={{flex: 1, position: 'relative', padding: 18, overflow: 'hidden'}}>
        <div
          style={{
            fontFamily: FONT.mono,
            fontSize: 13,
            letterSpacing: 2,
            color: COLORS.muted2,
            marginBottom: 12,
          }}
        >
          {mainLabel}
        </div>
        <div style={{position: 'absolute', left: 18, right: 18, top: 46, bottom: 12}}>{children}</div>
      </div>
    </div>
    <div style={{height: 0}}>{frame}</div>
  </div>
);

/** S3 — 完整脱敏流程（1:05–2:30, 2550 帧） */
export const Scene3Walkthrough: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  const win = (a: number, b: number) => interpolate(frame, [a, a + 18, b - 18, b], [0, 1, 1, 0], clamp);

  // Chapter windows follow the narration cues (see src/data/script.ts) so the
  // spoken step and the picture step never disagree.
  const ch1 = win(36, 228);
  const ch2 = win(228, 660);
  const ch3 = win(660, 1326);
  const ch4 = win(1326, 1776);
  const ch5 = win(1776, 2256);
  const ch6 = win(2256, 2550);

  const active = frame < 228 ? 0 : frame < 660 ? 1 : frame < 1326 ? 2 : frame < 1776 ? 3 : frame < 2256 ? 4 : 5;

  const wsIn = spring({frame: frame - 1, fps, config: SPRING.settle});
  const outFade = interpolate(frame, [2510, 2550], [1, 0], clamp);

  const scanBeam = interpolate(frame, [262, 500], [-0.05, 1.05], clamp);
  const maskP = interpolate(frame, [1846, 1996], [0, 1], clamp);
  const digestP = interpolate(frame, [1336, 1396], [0, 1], clamp);

  return (
    <AbsoluteFill style={{opacity: outFade}}>
      <Backdrop grid="flat" glow="cyan" />

      <AbsoluteFill style={{opacity: interpolate(frame, [0, 16], [0, 1], clamp)}}>
        <Eyebrow index={3} label="WALKTHROUGH · 完整走查" delay={2} />
        <div style={{position: 'absolute', right: 132, top: 78}}>
          <DisclaimerTag />
        </div>
      </AbsoluteFill>

      <div style={{position: 'absolute', left: 132, top: 132, opacity: wsIn}}>
        <div style={{fontFamily: FONT.sans, fontSize: 30, fontWeight: 700, color: COLORS.text}}>
          一次完整脱敏，从数据源到验证
        </div>
      </div>

      <AbsoluteFill style={{opacity: wsIn}}>
        <ChapterRail chapters={CHAPTERS} active={active} frame={frame} enter={6} y={186} w={238} gap={22} x0={132} />
      </AbsoluteFill>

      <AbsoluteFill style={{opacity: wsIn}}>
        <Workspace
          frame={frame}
          active={active}
          opacity={1}
          headerRight={frame < 1080 ? 'read only' : 'write session · gated'}
          mainLabel={
            active === 0
              ? 'people · 合成数据源'
              : active === 1
                ? 'scan · 字段判定'
                : active === 2
                  ? 'plan · 处理计划'
                  : active === 3
                    ? 'approval · Ed25519'
                    : active === 4
                      ? 'apply · batch transaction'
                      : 'validate · strict'
          }
        >
          {/* CH1 data source */}
          <AbsoluteFill style={{opacity: ch1}}>
            <DataTable
              rows={ROWS}
              width={960}
              visibleRows={6}
              frame={frame}
              enter={46}
              rowHeight={52}
              fontSize={19}
              headerSize={15}
              colState={{name: 'none', email: 'none', mobile: 'none', city: 'none'}}
            />
            <div style={{marginTop: 14, display: 'flex', gap: 14, alignItems: 'center'}}>
              <VerdictChip kind="NOT_SENSITIVE" size={14} />
              <span style={{fontFamily: FONT.sans, fontSize: 17, color: COLORS.muted}}>
                隔离副本 · 只读连接 · 合成数据
              </span>
            </div>
          </AbsoluteFill>

          {/* CH2 scan */}
          <AbsoluteFill style={{opacity: ch2}}>
            <DataTable
              rows={ROWS}
              width={700}
              visibleRows={6}
              frame={frame}
              enter={248}
              rowHeight={52}
              fontSize={18}
              headerSize={14}
              colState={{name: 'sensitive', email: 'sensitive', mobile: 'sensitive', city: 'unknown'}}
              scanBeam={frame > 262 && frame < 505 ? scanBeam : null}
            />
            <div style={{position: 'absolute', right: 6, top: 24, width: 290}}>
              {[
                {f: 'name', v: 'REVIEW' as const, note: 'cn_person_name · 83%'},
                {f: 'email', v: 'SENSITIVE' as const, note: 'email · 100%'},
                {f: 'mobile', v: 'SENSITIVE' as const, note: 'cn_mobile · 100%'},
                {f: 'city', v: 'UNKNOWN' as const, note: '上下文不足'},
              ].map((r, i) => {
                const p = spring({frame: frame - 262 - i * 12, fps, config: SPRING.snappy});
                return (
                  <div
                    key={r.f}
                    style={{
                      marginBottom: 12,
                      padding: '10px 12px',
                      borderRadius: 10,
                      border: `1px solid ${COLORS.line}`,
                      background: 'rgba(9,22,37,0.85)',
                      opacity: p,
                      transform: `translateX(${(1 - p) * 12}px)`,
                    }}
                  >
                    <div style={{fontFamily: FONT.mono, fontSize: 15, color: COLORS.text}}>{r.f}</div>
                    <div style={{display: 'flex', alignItems: 'center', gap: 8, marginTop: 6}}>
                      <VerdictChip kind={r.v} size={12} />
                      <span style={{fontFamily: FONT.mono, fontSize: 11, color: COLORS.muted2}}>{r.note}</span>
                    </div>
                  </div>
                );
              })}
              <div style={{marginTop: 6, fontFamily: FONT.sans, fontSize: 15, color: COLORS.amber, lineHeight: 1.6}}>
                UNKNOWN 不等于安全
              </div>
            </div>
          </AbsoluteFill>

          {/* CH3 preview */}
          <AbsoluteFill style={{opacity: ch3}}>
            <div style={{display: 'flex', gap: 22}}>
              <div style={{width: 330}}>
                {[
                  ['表', 'people'],
                  ['字段', 'name · email · mobile'],
                  ['策略', 'name_mask · fake_email'],
                  ['范围', '6 rows'],
                  ['写入', 'planned columns only'],
                ].map(([k, v], i) => {
                  const p = spring({frame: frame - 678 - i * 8, fps, config: SPRING.snappy});
                  return (
                    <div
                      key={k}
                      style={{
                        display: 'flex',
                        gap: 12,
                        padding: '10px 0',
                        borderBottom: `1px solid ${COLORS.line}55`,
                        opacity: p,
                      }}
                    >
                      <span style={{fontFamily: FONT.sans, fontSize: 16, color: COLORS.muted2, width: 44}}>
                        {k}
                      </span>
                      <span style={{fontFamily: FONT.mono, fontSize: 16, color: COLORS.text}}>{v}</span>
                    </div>
                  );
                })}
                <div
                  style={{
                    marginTop: 18,
                    padding: '10px 14px',
                    borderRadius: 10,
                    border: `1px solid ${COLORS.teal}55`,
                    background: 'rgba(88,224,186,0.10)',
                    fontFamily: FONT.mono,
                    fontSize: 14,
                    color: COLORS.teal,
                    opacity: interpolate(frame, [736, 756], [0, 1], clamp),
                    letterSpacing: 1.2,
                  }}
                >
                  DRY RUN · WRITES = 0
                </div>
              </div>
              <div style={{flex: 1}}>
                <div style={{display: 'flex', gap: 12}}>
                  <MiniTable
                    title="SOURCE"
                    columns={['id', 'email']}
                    rows={[
                      [{text: '001'}, {text: 'lin@example.test', tone: 'source'}],
                      [{text: '002'}, {text: 'chen@example.test', tone: 'source'}],
                      [{text: '003'}, {text: 'zhao@example.test', tone: 'source'}],
                    ]}
                    width={300}
                    fontSize={13}
                    rowHeight={38}
                    frame={frame}
                    enter={694}
                  />
                  <MiniTable
                    title="PLANNED"
                    columns={['id', 'email']}
                    rows={[
                      [{text: '001'}, {text: 'l***@example.test', tone: 'masked'}],
                      [{text: '002'}, {text: 'c***@example.test', tone: 'masked'}],
                      [{text: '003'}, {text: 'z***@example.test', tone: 'masked'}],
                    ]}
                    width={300}
                    fontSize={13}
                    rowHeight={38}
                    frame={frame}
                    enter={712}
                  />
                </div>
                <div style={{marginTop: 18, fontFamily: FONT.sans, fontSize: 16, color: COLORS.muted2, lineHeight: 1.7}}>
                  确定性脱敏：同一输入产生同一输出。
                  <br />
                  但确定性不等于匿名化，也不承诺全局无碰撞。
                </div>
              </div>
            </div>
          </AbsoluteFill>

          {/* CH4 approve */}
          <AbsoluteFill style={{opacity: ch4}}>
            <div style={{display: 'flex', gap: 26, alignItems: 'flex-start'}}>
              <div style={{width: 330}}>
                <div style={{fontFamily: FONT.mono, fontSize: 13, letterSpacing: 2, color: COLORS.muted2}}>
                  PLAN DIGEST
                </div>
                <div
                  style={{
                    marginTop: 10,
                    padding: '12px 14px',
                    borderRadius: 10,
                    border: `1px solid ${COLORS.line}`,
                    fontFamily: FONT.mono,
                    fontSize: 15,
                    color: COLORS.text,
                  }}
                >
                  sha256:9f2c…a41b
                </div>
                <div style={{marginTop: 16, fontFamily: FONT.sans, fontSize: 15, color: COLORS.muted, lineHeight: 1.7}}>
                  审批记录绑定计划摘要与策略范围；执行侧只加载可信公钥。
                </div>
                <div style={{marginTop: 16, opacity: digestP}}>
                  <VerdictChip kind="PASS" size={13} />
                </div>
              </div>
              <div style={{position: 'relative', flex: 1, height: 240}}>
                <div style={{position: 'absolute', left: 0, top: 6, width: 640}}>
                  <ApprovalCard frame={frame - 1326} enter={18} x={0} y={0} width={640} state="verified" />
                </div>
              </div>
            </div>
            <div style={{position: 'absolute', left: 0, bottom: 4, fontFamily: FONT.sans, fontSize: 16, color: COLORS.muted2}}>
              未获授权的计划无法进入写入通道
            </div>
          </AbsoluteFill>

          {/* CH5 apply */}
          <AbsoluteFill style={{opacity: ch5}}>
            <DataTable
              rows={ROWS}
              width={660}
              visibleRows={6}
              frame={frame}
              enter={1792}
              rowHeight={54}
              fontSize={17}
              headerSize={13}
              colState={{name: 'masked', email: 'masked', mobile: 'masked', city: 'none'}}
              maskProgress={{name: maskP, email: maskP, mobile: maskP}}
            />
            <div style={{position: 'absolute', right: 6, top: 4, width: 290}}>
              <div style={{display: 'flex', flexDirection: 'column', gap: 10}}>
                {[
                  ['--approved', 1800],
                  ['--apply', 1820],
                  ['approval record', 1840],
                ].map(([label, at], i) => {
                  const p = spring({frame: frame - (at as number), fps, config: SPRING.crisp});
                  return (
                    <div
                      key={label as string}
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 10,
                        padding: '8px 12px',
                        borderRadius: 8,
                        border: `1px solid ${COLORS.teal}55`,
                        background: 'rgba(88,224,186,0.08)',
                        opacity: p,
                        fontFamily: FONT.mono,
                        fontSize: 14,
                        color: COLORS.teal,
                      }}
                    >
                      <span style={{width: 6, height: 6, borderRadius: 3, background: COLORS.teal}} />
                      {label}
                      <span style={{marginLeft: 'auto', fontSize: 11, color: COLORS.muted2}}>ok</span>
                      {i < 0 && null}
                    </div>
                  );
                })}
              </div>
              <div style={{marginTop: 16}}>
                <div style={{fontFamily: FONT.mono, fontSize: 13, color: COLORS.muted2, letterSpacing: 1.2}}>
                  batch commit
                </div>
                <div style={{marginTop: 6}}>
                  <Meter
                    progress={interpolate(frame, [1860, 2140], [0, 1], clamp)}
                    cap={1}
                    width={280}
                    height={5}
                    color={COLORS.teal}
                    capColor="transparent"
                  />
                </div>
              </div>
            </div>
            <div style={{position: 'absolute', left: 0, bottom: 42, display: 'flex', gap: 12}}>
              {['行数一致', '主键不变', '仅计划列写入'].map((t, i) => (
                <span
                  key={t}
                  style={{
                    fontFamily: FONT.mono,
                    fontSize: 13,
                    letterSpacing: 1.2,
                    color: COLORS.teal,
                    border: `1px solid ${COLORS.teal}55`,
                    background: 'rgba(88,224,186,0.08)',
                    borderRadius: 6,
                    padding: '5px 10px',
                    opacity: spring({frame: frame - 1880 - i * 10, fps, config: SPRING.crisp}),
                  }}
                >
                  {t}
                </span>
              ))}
            </div>
            <div style={{position: 'absolute', left: 0, bottom: 8, fontFamily: FONT.sans, fontSize: 16, color: COLORS.muted2}}>
              仅写入计划字段；非计划列保持原样
            </div>
          </AbsoluteFill>

          {/* CH6 validate */}
          <AbsoluteFill style={{opacity: ch6}}>
            <CheckList
              items={[
                {label: '行数一致', sub: '6 = 6'},
                {label: '主键集合双向一致', sub: 'diff = 0'},
                {label: 'Schema 与类型一致', sub: 'fingerprint ok'},
                {label: '仅计划字段发生变化', sub: 'planned only'},
                {label: '适配器完整性检查', sub: 'quick_check'},
              ]}
              frame={frame}
              checkAt={[2272, 2286, 2300, 2314, 2328]}
              x={8}
              y={10}
              rowH={62}
              width={620}
              size={19}
            />
            <div
              style={{
                position: 'absolute',
                right: 6,
                top: 16,
                width: 290,
                padding: 18,
                borderRadius: 14,
                border: `1px solid ${COLORS.teal}66`,
                background: 'linear-gradient(150deg, rgba(88,224,186,0.12), rgba(11,27,44,0.95))',
                opacity: interpolate(frame, [2342, 2364], [0, 1], clamp),
                transform: `translateY(${interpolate(frame, [2342, 2364], [12, 0], clamp)}px)`,
              }}
            >
              <div style={{fontFamily: FONT.mono, fontSize: 17, letterSpacing: 1.6, color: COLORS.teal}}>
                VALIDATE · PASS
              </div>
              <div style={{marginTop: 8, fontFamily: FONT.sans, fontSize: 15, color: COLORS.muted, lineHeight: 1.6}}>
                严格验证要求全量扫描与完整性检查，任一失败即 fail closed。
              </div>
            </div>
          </AbsoluteFill>

          {/* closing line inside the validation chapter */}
          <div
            style={{
              position: 'absolute',
              left: 0,
              bottom: 6,
              display: 'flex',
              alignItems: 'center',
              gap: 16,
              opacity: interpolate(frame, [2400, 2430], [0, 1], clamp),
            }}
          >
            <span style={{fontFamily: FONT.sans, fontSize: 26, fontWeight: 500, color: COLORS.text}}>
              从扫描到验证，全程留痕
            </span>
            {['数据源', '扫描', '预览', '审批', '执行', '验证'].map((s2, i) => {
              const p = spring({frame: frame - 2408 - i * 8, fps, config: SPRING.snappy});
              return (
                <span
                  key={s2}
                  style={{
                    fontFamily: FONT.mono,
                    fontSize: 14,
                    letterSpacing: 1.2,
                    color: COLORS.teal,
                    border: `1px solid ${COLORS.teal}55`,
                    background: 'rgba(88,224,186,0.08)',
                    borderRadius: 6,
                    padding: '4px 10px',
                    opacity: p,
                  }}
                >
                  {String(i + 1).padStart(2, '0')} {s2}
                </span>
              );
            })}
          </div>
        </Workspace>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};
