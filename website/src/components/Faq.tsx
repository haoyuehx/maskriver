import { ChevronDown } from 'lucide-react';
import { faqs } from '../data/content';

export default function Faq() {
  return (
    <section id="faq" className="section section-shell" aria-labelledby="faq-title">
      <div className="section-heading">
        <span className="section-kicker">FAQ · LIMITATIONS</span>
        <h2 id="faq-title">把边界说清楚，才能安全使用</h2>
        <p>这些问题覆盖当前支持范围、审批、恢复与匿名化边界。更完整的限制见仓库文档。</p>
      </div>
      <div className="faq-list">
        {faqs.map((item) => (
          <details className="faq-item" key={item.q}>
            <summary>
              <span>{item.q}</span>
              <ChevronDown size={19} aria-hidden="true" />
            </summary>
            <p>{item.a}</p>
          </details>
        ))}
      </div>
    </section>
  );
}
