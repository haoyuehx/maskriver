import { useId } from 'react';

export default function Logo({ compact = false }: { compact?: boolean }) {
  const gradientId = `logo-${useId().replace(/:/g, '')}`;
  return (
    <span className="logo">
      <svg viewBox="0 0 64 64" aria-hidden="true" className="logo-mark">
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="1">
            <stop stopColor="#63dcf3" />
            <stop offset="1" stopColor="#b49cff" />
          </linearGradient>
        </defs>
        <rect width="64" height="64" rx="16" fill="#0b2338" stroke="#25506a" />
        <path d="M13 40c7-13 12 13 19 0s12 13 19 0" fill="none" stroke={`url(#${gradientId})`} strokeWidth="5" strokeLinecap="round" />
        <path d="M20 23h24M20 30h15" stroke="#ecf8ff" strokeWidth="4" strokeLinecap="round" opacity=".9" />
      </svg>
      {!compact && (
        <span className="logo-word">
          Mask<span>River</span>
        </span>
      )}
    </span>
  );
}
