import { ArrowRight, ShieldAlert, ShieldCheck } from 'lucide-react';
import Reveal from './Reveal';

const rows = [
  ['001', '林晓', 'lin@example.test', '138****5723', '上海市'],
  ['002', '陈雨', 'chen@example.test', '139****8241', '北京市'],
  ['003', '赵明', 'zhao@example.test', '136****9032', '杭州市'],
];

export default function ProblemSolution() {
  return (
    <section className="section section-shell" aria-labelledby="problem-title">
      <div className="section-heading">
        <span className="section-kicker">WHY MASKRIVER</span>
        <h2 id="problem-title">敏感数据不该以原样进入开发和测试</h2>
        <p>
          开发、测试与数据共享经常需要数据库副本。直接复制真实数据会把敏感信息扩散到更多环境；
          手工替换又难以证明处理范围、审批和结果。
        </p>
      </div>
      <div className="problem-grid">
        <Reveal className="problem-card risk-card">
          <div className="problem-card-head">
            <span className="card-icon risk"><ShieldAlert size={20} /></span>
            <div>
              <span className="mini-kicker">BEFORE</span>
              <h3>原始副本</h3>
            </div>
          </div>
          <p>真实字段、凭据和可关联数据被复制到低信任环境，风险随副本数量扩散。</p>
          <DataTable masked={false} />
        </Reveal>
        <div className="problem-connector" aria-hidden="true">
          <span className="connector-pulse" />
          <div className="connector-chip">
            <span className="shield-mark">M</span>
            <span>MaskRiver</span>
          </div>
          <ArrowRight size={20} />
        </div>
        <Reveal className="problem-card safe-card" delay={120}>
          <div className="problem-card-head">
            <span className="card-icon safe"><ShieldCheck size={20} /></span>
            <div>
              <span className="mini-kicker">AFTER</span>
              <h3>受控目标副本</h3>
            </div>
          </div>
          <p>只修改计划中的敏感字段，保留非计划列、键集和结构，并通过严格验证。</p>
          <DataTable masked />
        </Reveal>
      </div>
      <p className="fine-print">
        表格为合成数据示意。MaskRiver 处理的是获授权、隔离、可丢弃的副本；脱敏不等于匿名化。
      </p>
    </section>
  );
}

function DataTable({ masked }: { masked: boolean }) {
  return (
    <div className={`demo-table ${masked ? 'is-masked' : ''}`} role="img" aria-label={masked ? '脱敏后的合成数据表' : '包含敏感字段的合成数据表'}>
      <div className="demo-table-head">
        <span>id</span><span>name</span><span>email</span><span>mobile</span><span>city</span>
      </div>
      {rows.map((row, index) => (
        <div className="demo-table-row" key={row[0]}>
          <span>{row[0]}</span>
          <span>{row[1]}</span>
          <span className={masked ? 'value-masked' : 'value-sensitive'}>
            {masked ? `m${index + 1}@example.test` : row[2]}
          </span>
          <span className={masked ? 'value-masked' : 'value-sensitive'}>
            {masked ? '13*********' : row[3]}
          </span>
          <span>{row[4]}</span>
        </div>
      ))}
      <div className="demo-table-footer">
        <span>{masked ? '计划字段已替换 · 非计划字段保持' : '敏感字段暴露在副本中'}</span>
        <span className="mono">{masked ? 'VALIDATED' : 'RISK'}</span>
      </div>
    </div>
  );
}
