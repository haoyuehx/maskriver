import { ArrowRight, Github, Play, ShieldCheck } from 'lucide-react';
import { GITHUB } from '../data/content';

export default function Hero() {
  return (
    <section id="top" className="hero section-shell">
      <div className="hero-glow hero-glow-one" aria-hidden="true" />
      <div className="hero-glow hero-glow-two" aria-hidden="true" />
      <div className="hero-copy">
        <div className="eyebrow">
          <span className="status-dot" aria-hidden="true" />
          Open source · v1.0.0
          <span className="eyebrow-divider" />
          Linux amd64
        </div>
        <h1>
          让数据脱敏
          <span className="gradient-text">更安全、更可控。</span>
        </h1>
        <p className="hero-lede">
          面向开发与测试场景的数据脱敏工具。将敏感信息识别、脱敏预览、安全审批、执行验证与故障恢复，
          组织成清晰、可追踪的处理流程。
        </p>
        <div className="hero-actions">
          <a className="button button-primary" href="#demo">
            <Play size={18} fill="currentColor" aria-hidden="true" />
            观看演示
          </a>
          <a className="button button-secondary" href={GITHUB} target="_blank" rel="noreferrer">
            <Github size={18} aria-hidden="true" />
            查看 GitHub
          </a>
          <a className="text-link" href="#start">
            从 Quick Start 开始 <ArrowRight size={15} aria-hidden="true" />
          </a>
        </div>
        <div className="hero-trust">
          <span>
            <ShieldCheck size={16} aria-hidden="true" /> 默认只读预览
          </span>
          <span>Ed25519 审批</span>
          <span>SQLite · MySQL 8.0.46</span>
        </div>
      </div>
      <HeroVisual />
    </section>
  );
}

function HeroVisual() {
  const rows = [
    ['001', '林晓', 'lin@example.test', '138****5723', '上海'],
    ['002', '陈雨', 'chen@example.test', '139****8241', '北京'],
    ['003', '赵明', 'zhao@example.test', '136****9032', '杭州'],
  ];
  return (
    <div className="hero-visual" role="img" aria-label="数据从源副本经过 MaskRiver 流程进入安全目标副本的示意图">
      <div className="visual-topline">
        <span className="visual-label">SYNTHETIC DATA FLOW</span>
        <span className="live-indicator">
          <span /> SAFE
        </span>
      </div>
      <svg className="data-network" viewBox="0 0 620 300" aria-hidden="true">
        <defs>
          <linearGradient id="flow" x1="0" x2="1">
            <stop stopColor="#63dcf3" stopOpacity=".2" />
            <stop offset=".5" stopColor="#63dcf3" />
            <stop offset="1" stopColor="#b49cff" stopOpacity=".2" />
          </linearGradient>
          <filter id="glow">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
        </defs>
        <path d="M60 150 C150 150 175 65 270 65 S400 150 560 150" fill="none" stroke="#25506a" strokeWidth="1.5" />
        <path d="M60 150 C150 150 175 235 270 235 S400 150 560 150" fill="none" stroke="#25506a" strokeWidth="1.5" />
        <path className="flow-line" d="M60 150 C150 150 175 65 270 65 S400 150 560 150" fill="none" stroke="url(#flow)" strokeWidth="2.5" filter="url(#glow)" />
        <path className="flow-line flow-line-delay" d="M60 150 C150 150 175 235 270 235 S400 150 560 150" fill="none" stroke="url(#flow)" strokeWidth="2.5" filter="url(#glow)" />
        <circle cx="60" cy="150" r="7" fill="#ff996a" />
        <circle cx="270" cy="65" r="6" fill="#63dcf3" />
        <circle cx="270" cy="235" r="6" fill="#b49cff" />
        <circle cx="560" cy="150" r="7" fill="#58e0ba" />
      </svg>
      <div className="visual-row visual-source">
        <span className="source-dot" /> source.db <span className="muted">read only</span>
      </div>
      <div className="visual-row visual-shield">
        <span className="shield-mark">M</span>
        <div>
          <strong>MaskRiver</strong>
          <span>approval-bound workflow</span>
        </div>
      </div>
      <div className="visual-row visual-target">
        <span className="target-dot" /> target.db <span className="muted">masked</span>
      </div>
      <div className="mini-table">
        <div className="mini-table-head">
          <span>id</span><span>name</span><span>email</span><span>mobile</span>
        </div>
        {rows.map((row, index) => (
          <div className="mini-table-row" key={row[0]}>
            <span>{row[0]}</span>
            <span>{row[1]}</span>
            <span className={index === 1 ? 'mask-cell hot' : 'mask-cell'}>{row[2].replace(/^[^@]+/, 'm' + (index + 1))}</span>
            <span className="mask-cell">{row[3]}</span>
          </div>
        ))}
      </div>
      <div className="visual-footer">
        <span>scan → preview → approve</span>
        <span className="mono">PLAN · 7f3a</span>
      </div>
    </div>
  );
}
