import { ClipboardCheck, Database, FlaskConical, Layers3 } from 'lucide-react';
import { useCases } from '../data/content';
import Reveal from './Reveal';

const icons = [Database, Layers3, ClipboardCheck, FlaskConical];

export default function UseCases() {
  return (
    <section id="use-cases" className="section section-shell" aria-labelledby="use-cases-title">
      <div className="section-heading">
        <span className="section-kicker">USE CASES</span>
        <h2 id="use-cases-title">适合授权的副本，而不是生产数据库</h2>
        <p>MaskRiver 面向开发、测试、治理验证与受控实验；它不是生产数据安全认证或法律合规认证。</p>
      </div>
      <div className="use-grid">
        {useCases.map((item, index) => {
          const Icon = icons[index];
          return (
            <Reveal key={item.title} className="use-card" delay={index * 70}>
              <span className="use-icon"><Icon size={19} /></span>
              <h3>{item.title}</h3>
              <p>{item.text}</p>
            </Reveal>
          );
        })}
      </div>
    </section>
  );
}
