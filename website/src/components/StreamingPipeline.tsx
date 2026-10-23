import { Database, Gauge, Layers3, ListChecks, ReceiptText, Workflow } from 'lucide-react';
import Reveal from './Reveal';

const nodes = [
  { label: 'Database', detail: 'SQLite 源副本', icon: Database },
  { label: 'Bounded Reader', detail: '有界读取', icon: Gauge },
  { label: 'Batch Queue', detail: '批次队列', icon: Layers3 },
  { label: 'Workers', detail: '受控处理', icon: Workflow },
  { label: 'Transaction', detail: '事务提交', icon: ReceiptText },
  { label: 'Target', detail: '目标副本', icon: ListChecks },
];

export default function StreamingPipeline() {
  return (
    <section id="streaming" className="section section-shell streaming-section" aria-labelledby="streaming-title">
      <div className="section-heading">
        <span className="section-kicker">BOUNDED STREAMING</span>
        <h2 id="streaming-title">让数据按批次流动，而不是一次载入全部</h2>
        <p>面向较大数据规模的分批读取、资源预算与事务回执。范围仅限 SQLite。</p>
      </div>
      <Reveal className="pipeline-wrap">
        <div className="pipeline">
          {nodes.map((node, index) => {
            const Icon = node.icon;
            return (
              <div className="pipeline-item" key={node.label}>
                <div className={`pipeline-node ${index === 2 ? 'is-hot' : ''}`}>
                  <span className="pipeline-icon"><Icon size={19} /></span>
                  <strong>{node.label}</strong>
                  <span>{node.detail}</span>
                  <span className="node-meter"><i style={{ animationDelay: `${index * 0.35}s` }} /></span>
                </div>
                {index < nodes.length - 1 && (
                  <div className="pipeline-link" aria-hidden="true">
                    <span className="packet" style={{ animationDelay: `${index * 0.55}s` }} />
                    <span className="link-line" />
                  </div>
                )}
              </div>
            );
          })}
        </div>
        <div className="pipeline-stats">
          <div><span>读取方式</span><strong>有界分页</strong></div>
          <div><span>执行单位</span><strong>批次事务</strong></div>
          <div><span>资源边界</span><strong>显式预算</strong></div>
          <div><span>审计凭据</span><strong>Outbox 回执</strong></div>
        </div>
      </Reveal>
      <p className="fine-print">批次事务不等于整库原子性。资源预算与流式路径需显式配置，且不支持 MySQL streaming。</p>
    </section>
  );
}
