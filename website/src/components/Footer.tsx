import { Github } from 'lucide-react';
import Logo from './Logo';
import { CHECKSUMS, DOCS, GITHUB, RELEASE, VERSION } from '../data/content';

export default function Footer() {
  return (
    <footer className="footer">
      <div className="footer-inner section-shell">
        <div className="footer-brand">
          <Logo />
          <p>安全、可验证、可恢复的数据脱敏工具，面向开发、测试与数据共享场景。</p>
          <a href={GITHUB} target="_blank" rel="noreferrer"><Github size={16} /> github.com/haoyuehx/maskriver</a>
        </div>
        <div className="footer-col">
          <span className="footer-title">产品</span>
          <a href="#workflow">核心流程</a>
          <a href="#streaming">Streaming</a>
          <a href="#recovery">Recovery</a>
          <a href="#start">快速开始</a>
        </div>
        <div className="footer-col">
          <span className="footer-title">资源</span>
          <a href={DOCS} target="_blank" rel="noreferrer">文档</a>
          <a href={RELEASE} target="_blank" rel="noreferrer">{VERSION} Release</a>
          <a href={CHECKSUMS} target="_blank" rel="noreferrer">SHA256SUMS</a>
          <a href={`${GITHUB}/blob/main/LICENSE`} target="_blank" rel="noreferrer">MIT License</a>
        </div>
      </div>
      <div className="footer-bottom section-shell">
        <span>© 2026 MaskRiver contributors · MIT</span>
        <span>Built as a static product showcase. No analytics, no tracking.</span>
      </div>
    </footer>
  );
}
