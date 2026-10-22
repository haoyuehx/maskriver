import type { LucideIcon } from 'lucide-react';
import { CheckCheck, Database, KeyRound, RefreshCcw, ScanSearch, ShieldCheck, Sparkles } from 'lucide-react';
import { features } from '../data/content';
import Reveal from './Reveal';

const iconMap: Record<string, LucideIcon> = {
  scan: ScanSearch,
  shield: ShieldCheck,
  wand: Sparkles,
  check: CheckCheck,
  stream: Database,
  recovery: RefreshCcw,
};

export default function FeatureGrid() {
  return (
    <section id="features" className="section section-shell" aria-labelledby="features-title">
      <div className="section-heading">
        <span className="section-kicker">CAPABILITIES</span>
        <h2 id="features-title">不是一次性的替换脚本，而是一条受控的处理链</h2>
        <p>每个能力都围绕同一件事：让“处理了哪些数据、为什么可以处理、结果是否成立”可被追踪。</p>
      </div>
      <div className="feature-grid">
        {features.map((feature, index) => {
          const Icon = iconMap[feature.icon] || KeyRound;
          return (
            <Reveal key={feature.title} className={`feature-card feature-${feature.span}`} delay={index * 60}>
              <div className="feature-top">
                <span className="feature-icon"><Icon size={21} /></span>
                <span className="feature-tag">{feature.tag}</span>
              </div>
              <h3>{feature.title}</h3>
              <p>{feature.text}</p>
              <span className="feature-index" aria-hidden="true">0{index + 1}</span>
            </Reveal>
          );
        })}
      </div>
    </section>
  );
}
