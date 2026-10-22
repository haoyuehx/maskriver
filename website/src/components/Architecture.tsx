import Reveal from './Reveal';

const nodes = [
  { id: 'cli', x: 40, y: 72, w: 170, h: 58, label: 'CLI / Config', sub: '显式 scope · DSN / path', tone: 'cyan' },
  { id: 'detect', x: 270, y: 72, w: 170, h: 58, label: 'Detection', sub: '上下文 + 格式规则', tone: 'cyan' },
  { id: 'plan', x: 500, y: 72, w: 170, h: 58, label: 'Plan', sub: '摘要绑定 · 只读预览', tone: 'purple' },
  { id: 'approve', x: 730, y: 72, w: 170, h: 58, label: 'Approval', sub: 'Ed25519 · 有效期', tone: 'purple' },
  { id: 'runner', x: 270, y: 218, w: 170, h: 58, label: 'Runner', sub: '门禁 · 批处理', tone: 'cyan' },
  { id: 'mask', x: 500, y: 218, w: 170, h: 58, label: 'Masking', sub: '确定性策略', tone: 'cyan' },
  { id: 'adapter', x: 730, y: 218, w: 170, h: 58, label: 'DB Adapter', sub: 'SQLite · MySQL', tone: 'teal' },
  { id: 'verify', x: 500, y: 364, w: 170, h: 58, label: 'Validation', sub: '键集 · Schema · 完整性', tone: 'teal' },
  { id: 'outbox', x: 270, y: 364, w: 170, h: 58, label: 'Transaction Outbox', sub: '业务更新 + 回执', tone: 'purple' },
  { id: 'recovery', x: 40, y: 218, w: 170, h: 58, label: 'Recovery', sub: 'Checkpoint · Resume', tone: 'amber' },
];

const lines = [
  ['cli', 'detect'], ['detect', 'plan'], ['plan', 'approve'],
  ['approve', 'runner'], ['runner', 'mask'], ['mask', 'adapter'],
  ['runner', 'outbox'], ['outbox', 'recovery'], ['adapter', 'verify'],
  ['outbox', 'adapter'],
];

function center(node: (typeof nodes)[number]) {
  return { x: node.x + node.w / 2, y: node.y + node.h / 2 };
}

export default function Architecture() {
  return (
    <section id="architecture" className="section section-shell architecture-section" aria-labelledby="architecture-title">
      <div className="section-heading">
        <span className="section-kicker">ARCHITECTURE</span>
        <h2 id="architecture-title">单机、可审计的处理链，而不是想象中的云平台</h2>
        <p>从 CLI 配置到检测、计划、审批、执行、适配器与验证；Streaming 和本地恢复只在支持的 SQLite 范围内增量存在。</p>
      </div>
      <Reveal className="architecture-frame" tabIndex={0} role="region" aria-label="MaskRiver 架构图，可横向滚动">
        <div className="architecture-topline">
          <span>LOCAL / SINGLE PROCESS</span>
          <span className="mono">m1a-v1 · m2-stream-v1 · m2-recovery-v1</span>
        </div>
        <span className="scroll-hint">← 横向滑动查看完整架构 →</span>
        <svg viewBox="0 0 960 470" role="img" aria-labelledby="arch-svg-title arch-svg-desc" className="architecture-svg">
          <title id="arch-svg-title">MaskRiver 架构图</title>
          <desc id="arch-svg-desc">CLI 与配置进入检测、计划与审批，然后由 Runner 协调脱敏、数据库适配器、事务 Outbox、验证与恢复。</desc>
          <defs>
            <marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
              <path d="M0 0L10 5L0 10z" fill="#4e7895" />
            </marker>
            <linearGradient id="arch-accent" x1="0" x2="1">
              <stop stopColor="#63dcf3" /><stop offset="1" stopColor="#b49cff" />
            </linearGradient>
            <filter id="node-glow"><feGaussianBlur stdDeviation="5" result="b" /><feMerge><feMergeNode in="b" /><feMergeNode in="SourceGraphic" /></feMerge></filter>
          </defs>
          {lines.map(([fromId, toId]) => {
            const from = nodes.find((node) => node.id === fromId)!;
            const to = nodes.find((node) => node.id === toId)!;
            const a = center(from);
            const b = center(to);
            return <path key={`${fromId}-${toId}`} d={`M${a.x} ${a.y} C${(a.x + b.x) / 2} ${a.y} ${(a.x + b.x) / 2} ${b.y} ${b.x} ${b.y}`} fill="none" stroke="#3d6480" strokeWidth="1.5" markerEnd="url(#arrow)" />;
          })}
          {nodes.map((node) => (
            <g key={node.id} className={`arch-node arch-${node.tone}`}>
              <rect x={node.x} y={node.y} width={node.w} height={node.h} rx="14" fill="#0b1b2c" stroke={node.tone === 'amber' ? '#ff996a' : node.tone === 'purple' ? '#b49cff' : node.tone === 'teal' ? '#58e0ba' : '#63dcf3'} strokeWidth="1.2" />
              <text x={node.x + 18} y={node.y + 24} fill="#ecf8ff" fontSize="15" fontWeight="650">{node.label}</text>
              <text x={node.x + 18} y={node.y + 43} fill="#8da9bc" fontSize="11.5">{node.sub}</text>
              <circle cx={node.x + node.w - 16} cy={node.y + 16} r="3.5" fill={node.tone === 'amber' ? '#ff996a' : node.tone === 'purple' ? '#b49cff' : node.tone === 'teal' ? '#58e0ba' : '#63dcf3'} filter="url(#node-glow)" />
            </g>
          ))}
          <text x="40" y="444" fill="#6f8ca1" fontSize="11">本地 SQLite / MySQL 适配器 · 无消息队列 · 无分布式事务 · 无云审批服务</text>
        </svg>
        <div className="architecture-legend">
          <span><i className="legend-dot cyan" /> 识别与计划</span>
          <span><i className="legend-dot purple" /> 审批与事务</span>
          <span><i className="legend-dot teal" /> 写入与验证</span>
          <span><i className="legend-dot amber" /> 本地恢复</span>
        </div>
      </Reveal>
      <p className="fine-print">架构图基于仓库中已实现组件；没有画入 Kubernetes、Kafka、微服务网关或多节点分布式事务。</p>
    </section>
  );
}
