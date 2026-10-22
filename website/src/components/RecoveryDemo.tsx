import { useCallback, useEffect, useRef, useState } from 'react';
import { AlertTriangle, Check, Play, RefreshCcw, ShieldCheck, Undo2 } from 'lucide-react';

type Phase = 'idle' | 'interrupted' | 'recovering' | 'recovered' | 'validated';

const committedByPhase: Record<Phase, number> = {
  idle: 3,
  interrupted: 3,
  recovering: 4,
  recovered: 6,
  validated: 6,
};

const phaseCopy: Record<Phase, { title: string; tone: string; note: string }> = {
  idle: { title: 'APPLY · 批次事务提交', tone: 'cyan', note: '业务更新、Outbox Receipt、Progress 与 Manifest CAS 在同一本地 SQLite 事务中提交。' },
  interrupted: { title: 'TASK INTERRUPTED', tone: 'amber', note: '已提交批次保留在目标数据库；任务停止，不覆盖任何外部改动。' },
  recovering: { title: 'RESUME · 核对后继续', tone: 'purple', note: '读取 Checkpoint 与回执，重算已提交前缀，跳过正确完成的部分。' },
  recovered: { title: 'RECOVERY COMPLETE', tone: 'teal', note: '未完成批次继续执行；重复恢复不会再次修改正确完成的数据。' },
  validated: { title: 'VALIDATION PASSED', tone: 'teal', note: '恢复完成后再次运行严格验证；冲突、漂移或回执不符都会拒绝继续。' },
};

export default function RecoveryDemo() {
  const [phase, setPhase] = useState<Phase>('idle');
  const timers = useRef<number[]>([]);
  const clearTimers = useCallback(() => {
    timers.current.forEach((timer) => window.clearTimeout(timer));
    timers.current = [];
  }, []);

  useEffect(() => clearTimers, [clearTimers]);

  const interrupt = () => {
    clearTimers();
    setPhase('interrupted');
  };
  const recover = () => {
    if (phase !== 'interrupted') return;
    clearTimers();
    setPhase('recovering');
    timers.current.push(window.setTimeout(() => setPhase('recovered'), 900));
    timers.current.push(window.setTimeout(() => setPhase('validated'), 1800));
  };
  const reset = () => {
    clearTimers();
    setPhase('idle');
  };

  const committed = committedByPhase[phase];
  const copy = phaseCopy[phase];
  const recovering = phase === 'recovering';

  return (
    <section id="recovery" className="section section-shell recovery-section" aria-labelledby="recovery-title">
      <div className="section-heading">
        <span className="section-kicker">CHECKPOINT · RECOVERY</span>
        <h2 id="recovery-title">中断之后，从已经核对的位置继续</h2>
        <p>本地 SQLite 的事务回执与检查点让恢复有据可查：先核对已提交部分，再继续未完成部分。这里只演示流程状态，不修改任何数据库。</p>
      </div>
      <div className="recovery-shell">
        <div className="recovery-controls">
          <div className={`phase-badge tone-${copy.tone}`} aria-live="polite">
            <span className={`dot ${copy.tone}`} />
            {copy.title}
          </div>
          <div className="recovery-buttons">
            <button type="button" className="button button-danger" onClick={interrupt} disabled={phase !== 'idle' && phase !== 'recovered'}>
              <AlertTriangle size={17} /> 模拟中断
            </button>
            <button type="button" className="button button-primary" onClick={recover} disabled={phase !== 'interrupted'}>
              <Play size={17} /> 恢复执行
            </button>
            <button type="button" className="button button-ghost" onClick={reset}>
              <Undo2 size={17} /> 重置
            </button>
          </div>
          <p className="recovery-note">{copy.note}</p>
        </div>
        <div className="batch-board" role="group" aria-label="批次恢复状态">
          {[0, 1, 2, 3, 4, 5].map((index) => {
            const isCommitted = index < committed;
            const isNext = recovering && index === committed;
            return (
              <div className={`batch-card ${isCommitted ? 'is-committed' : ''} ${isNext ? 'is-next' : ''}`} key={index}>
                <span className="batch-label">BATCH {String(index + 1).padStart(2, '0')}</span>
                <span className="batch-status">
                  {isCommitted ? <Check size={15} /> : isNext ? <RefreshCcw size={15} className="spin" /> : <span className="empty-ring" />}
                  {isCommitted ? '已提交' : isNext ? '继续中' : '待处理'}
                </span>
                <span className="batch-bar"><i style={{ width: isCommitted ? '100%' : isNext ? '42%' : '0%' }} /></span>
              </div>
            );
          })}
        </div>
        <div className="recovery-ledger">
          <div className="ledger-head"><span>LOCAL SQLITE TRANSACTION</span><span className="mono">atomic unit</span></div>
          <div className="ledger-items">
            {[
              ['Business UPDATE', '目标字段'],
              ['Outbox Receipt', '批次回执'],
              ['Progress', '检查点'],
              ['Manifest CAS', '身份绑定'],
            ].map(([name, detail]) => (
              <div key={name}><span className="ledger-check"><Check size={12} /></span><strong>{name}</strong><span>{detail}</span></div>
            ))}
          </div>
          <div className="ledger-note"><ShieldCheck size={15} /> 同一个本地事务提交；冲突时 fail-closed，不覆盖外部修改</div>
        </div>
      </div>
      <p className="fine-print">仅演示 Checkpoint 原理，不代表通用分布式 Exactly-once。恢复支持仅限 Linux amd64、rollback journal、synchronous=FULL 的本地 SQLite 与确定性策略；MySQL recovery 不支持。</p>
    </section>
  );
}
