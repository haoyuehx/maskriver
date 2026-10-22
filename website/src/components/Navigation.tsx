import { useEffect, useRef, useState } from 'react';
import { Github, Menu, X } from 'lucide-react';
import Logo from './Logo';
import { GITHUB, nav } from '../data/content';

export default function Navigation() {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLElement>(null);
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 24);
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    window.addEventListener('keydown', onKey);
    const first = menuRef.current?.querySelector<HTMLAnchorElement>('a');
    first?.focus();
    return () => window.removeEventListener('keydown', onKey);
  }, [open]);

  return (
    <header className={`nav ${scrolled ? 'nav-scrolled' : ''}`}>
      <div className="nav-inner">
        <a href="#top" className="nav-brand" aria-label="MaskRiver 首页" onClick={() => setOpen(false)}>
          <Logo />
          <span className="version-pill">v1.0.0</span>
        </a>
        <nav className="nav-links" aria-label="主导航">
          {nav.map((item) => (
            <a key={item.href} href={item.href}>
              {item.label}
            </a>
          ))}
        </nav>
        <div className="nav-actions">
          <a className="button button-ghost nav-github" href={GITHUB} target="_blank" rel="noreferrer">
            <Github size={17} aria-hidden="true" />
            GitHub
          </a>
          <button
            className="nav-toggle"
            type="button"
            aria-label={open ? '关闭菜单' : '打开菜单'}
            aria-expanded={open}
            aria-controls="mobile-menu"
            onClick={() => setOpen((value) => !value)}
          >
            {open ? <X size={22} /> : <Menu size={22} />}
          </button>
        </div>
      </div>
      {open && (
        <nav id="mobile-menu" className="mobile-menu" aria-label="移动端导航" ref={menuRef}>
          {nav.map((item) => (
            <a key={item.href} href={item.href} onClick={() => setOpen(false)}>
              {item.label}
            </a>
          ))}
          <a href={GITHUB} target="_blank" rel="noreferrer" onClick={() => setOpen(false)}>
            GitHub 仓库
          </a>
        </nav>
      )}
    </header>
  );
}
