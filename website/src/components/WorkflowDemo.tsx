import { useState } from 'react';
import { Check, FileSearch, KeyRound, ScanSearch, ShieldCheck } from 'lucide-react';
import { steps } from '../data/content';

const icons = [ScanSearch, FileSearch, KeyRound, ShieldCheck, Check];

export default function WorkflowDemo() {
  const [active, setActive] = useState(0);
  const step = steps[active];
  const Icon = icons[active];
  return (
    <section id="workflow" className="section section-shell workflow-section" aria-labelledby="workflow-title">
      <div className="section-heading">
        <span className="section-kicker">CORE WORKFLOW</span>
        <h2 id="workflow-title">五个步骤，把一次脱敏变成可追踪的任务</h2>
        <p>点击任意步骤查看它解决的问题。实际工具通过 CLI 执行，这里的界面是流程可视化，不是 Web 管理后台。</p>
      </div>
      <div className="workflow-shell">
        <div className="workflow-tabs" role="tablist" aria-label="MaskRiver 五步流程">
          {steps.map((item, index) => {
            const StepIcon = icons[index];
            return (
              <button
                key={item.id}
                role="tab"
                id={`workflow-tab-${item.id}`}
                aria-selected={active === index}
                aria-controls={`workflow-panel-${item.id}`}
                className={`workflow-tab ${active === index ? 'is-active' : ''}`}
                onClick={() => setActive(index)}
                tabIndex={active === index ? 0 : -1}
                onKeyDown={(event) => {
                  if (event.key === 'ArrowRight' || event.key === 'ArrowLeft' || event.key === 'Home' || event.key === 'End') {
                    event.preventDefault();
                    const next = event.key === 'Home' ? 0 : event.key === 'End' ? steps.length - 1 : (active + (event.key === 'ArrowRight' ? 1 : -1) + steps.length) % steps.length;
                    setActive(next);
                    document.getElementById(`workflow-tab-${steps[next].id}`)?.focus();
                  }
                }}
                type="button"
              >
                <span className="tab-index">0{index + 1}</span>
                <StepIcon size={18} aria-hidden="true" />
                <span>{item.label}</span>
              </button>
            );
          })}
        </div>
        <div
          className={`workflow-panel panel-${step.color}`}
          role="tabpanel"
          id={`workflow-panel-${step.id}`}
          aria-labelledby={`workflow-tab-${step.id}`}
          key={step.id}
        >
          <div className="workflow-panel-copy">
            <span className="panel-icon"><Icon size={22} /></span>
            <span className="mini-kicker">STEP 0{active + 1}</span>
            <h3>{step.title}</h3>
            <p>{step.summary}</p>
            <ul className="fact-list">
              {step.facts.map((fact) => <li key={fact}><Check size={15} /> {fact}</li>)}
            </ul>
            <div className="workflow-panel-foot">
              <span className={`dot ${step.color}`} />
              {step.label} · 仅处理声明的表与字段
            </div>
          </div>
          <WorkflowVisual stepId={step.id} />
        </div>
      </div>
      <p className="fine-print">交互式流程示意，实际工具通过 CLI 执行；不会在本页面连接或修改任何数据库。</p>
    </section>
  );
}

function WorkflowVisual({ stepId }: { stepId: string }) {
  if (stepId === 'scan') {
    return (
      <div className="visual-panel">
        <div className="visual-panel-head"><span>SCAN EVIDENCE</span><span className="mono">sample · 128</span></div>
        <div className="scan-grid">
          {[
            ['email', '敏感', 'cyan'],
            ['mobile', '敏感', 'cyan'],
            ['name', '需核对', 'amber'],
            ['address', '需核对', 'amber'],
            ['order_id', '未分类', 'muted'],
          ].map(([field, verdict, tone]) => (
            <div className={`scan-row tone-${tone}`} key={field}>
              <span className="scan-field">{field}</span>
              <span className={`verdict verdict-${tone}`}>{verdict}</span>
              <span className="scan-bar"><span /></span>
            </div>
          ))}
        </div>
        <div className="visual-panel-foot">UNKNOWN 不会自动变成“安全”结论</div>
      </div>
    );
  }
  if (stepId === 'preview') {
    return (
      <div className="visual-panel">
        <div className="visual-panel-head"><span>READ-ONLY PLAN</span><span className="mono">dry run</span></div>
        <pre className="code-block"><code>{`table: customers
columns:
  email  → fake_email
  mobile → cn_mobile
preview:
  writes: 0
  target: unchanged`}</code></pre>
        <div className="visual-panel-foot"><Check size={14} /> 目标副本在预览阶段不产生写入</div>
      </div>
    );
  }
  if (stepId === 'approve') {
    return (
      <div className="visual-panel">
        <div className="visual-panel-head"><span>SIGNED DECISION</span><span className="mono">Ed25519</span></div>
        <div className="approval-card">
          <div className="approval-line"><span>Plan digest</span><strong className="mono">7f3a…b91c</strong></div>
          <div className="approval-line"><span>Scope order</span><strong>customers.email → customers.mobile</strong></div>
          <div className="approval-line"><span>Reviewer</span><strong>reviewer@example.test</strong></div>
          <div className="approval-line"><span>Status</span><strong className="status-ok"><Check size={14} /> verified</strong></div>
        </div>
        <div className="visual-panel-foot">审批与计划摘要绑定；过期、撤销或跨计划记录拒绝执行</div>
      </div>
    );
  }
  if (stepId === 'apply') {
    return (
      <div className="visual-panel">
        <div className="visual-panel-head"><span>BATCH APPLY</span><span className="mono">source read-only</span></div>
        <div className="apply-diff">
          <div><span className="diff-label">email</span><code>lin@example.test</code><span className="diff-arrow">→</span><code className="new">m1@example.test</code></div>
          <div><span className="diff-label">mobile</span><code>138****5723</code><span className="diff-arrow">→</span><code className="new">13*********</code></div>
          <div><span className="diff-label">city</span><code>上海市</code><span className="diff-arrow">→</span><code className="keep">上海市</code></div>
        </div>
        <div className="progress-strip"><span style={{ width: '68%' }} /><em>batch 4 / 6</em></div>
        <div className="visual-panel-foot">只修改计划字段；每批提交前核对源/目标值</div>
      </div>
    );
  }
  return (
    <div className="visual-panel">
      <div className="visual-panel-head"><span>STRICT VALIDATION</span><span className="mono">pass</span></div>
      <div className="validation-list">
        {['行数与双向主键集合', 'Schema 与计划字段', 'SQLite quick/FK 完整性', '覆盖完整，无截断'].map((item) => (
          <div key={item}><span className="check-ring"><Check size={13} /></span>{item}</div>
        ))}
      </div>
      <div className="validation-banner"><Check size={16} /> Validation Passed</div>
      <div className="visual-panel-foot">覆盖不足、摘要不符或完整性失败都会 fail-closed</div>
    </div>
  );
}
