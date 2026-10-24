import { useState } from 'react';
import { Check, Copy, Download, ExternalLink, FileText, Github, ShieldCheck, Terminal } from 'lucide-react';
import { CHECKSUMS, DOWNLOAD, DOCS, GITHUB, MYSQL_DOC, QUICKSTART, RELEASE, SECURITY_DOC, SQLITE_DOC, VERSION } from '../data/content';

const command = `curl -LO ${DOWNLOAD}
curl -LO ${CHECKSUMS}
sha256sum -c SHA256SUMS
tar -xzf maskriver-${VERSION}-linux-amd64.tar.gz
cd maskriver-${VERSION}-linux-amd64
./maskriver --version`;

export default function GetStarted() {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      setCopied(false);
    }
  };

  return (
    <section id="start" className="section section-shell start-section" aria-labelledby="start-title">
      <div className="start-card">
        <div className="start-copy">
          <span className="section-kicker">GET STARTED</span>
          <h2 id="start-title">从一份隔离副本开始</h2>
          <p>下载 Linux amd64 发行包，验证 SHA-256，然后阅读 Quick Start。MySQL 路径需要两个指向不同数据库的 DSN。</p>
          <div className="start-actions">
            <a className="button button-primary" href={DOWNLOAD}>
              <Download size={17} /> 下载 {VERSION}
            </a>
            <a className="button button-secondary" href={QUICKSTART} target="_blank" rel="noreferrer">
              <FileText size={17} /> Quick Start
            </a>
            <a className="text-link" href={CHECKSUMS} target="_blank" rel="noreferrer">
              SHA256SUMS <ExternalLink size={14} aria-hidden="true" />
            </a>
          </div>
          <div className="start-links">
            <a href={SQLITE_DOC} target="_blank" rel="noreferrer">SQLite 教程</a>
            <a href={MYSQL_DOC} target="_blank" rel="noreferrer">MySQL 教程</a>
            <a href={SECURITY_DOC} target="_blank" rel="noreferrer">安全模型</a>
            <a href={DOCS} target="_blank" rel="noreferrer">全部文档</a>
            <a href={GITHUB} target="_blank" rel="noreferrer"><Github size={14} /> GitHub</a>
          </div>
          <div className="scope-note"><ShieldCheck size={16} /> 仅 Linux amd64 发行。MySQL 8.0.46 的 CLI 已通过隔离 CI；MySQL recovery 与 Windows/macOS streaming/recovery 不支持。</div>
        </div>
        <div className="command-card">
          <div className="command-head">
            <span><Terminal size={15} /> release quickstart</span>
            <button type="button" onClick={() => void copy()} aria-label="复制安装命令">
              {copied ? <Check size={15} /> : <Copy size={15} />} {copied ? '已复制' : '复制'}
            </button>
          </div>
          <pre><code>{command}</code></pre>
          <div className="command-foot">
            <span>{VERSION} · linux/amd64</span>
            <a href={RELEASE} target="_blank" rel="noreferrer">Release notes <ExternalLink size={12} /></a>
          </div>
        </div>
      </div>
    </section>
  );
}
