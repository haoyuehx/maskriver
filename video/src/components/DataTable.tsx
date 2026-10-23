import React from 'react';
import {interpolate} from 'remotion';
import {COLORS, FONT} from '../styles/tokens';
import {COLUMNS, Row} from '../data/fixtures';

const clamp = {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'} as const;

type ColState = 'none' | 'sensitive' | 'unknown' | 'masked';

type DataTableProps = {
  rows: Row[];
  visibleRows?: number;
  maskProgress?: Partial<Record<'name' | 'email' | 'mobile', number>>;
  colState?: Partial<Record<string, ColState>>;
  scanBeam?: number | null;
  rowHeight?: number;
  fontSize?: number;
  headerSize?: number;
  width?: number;
  activeRow?: number | null;
  opacity?: number;
  frame?: number;
  enter?: number;
  stagger?: number;
  compact?: boolean;
  style?: React.CSSProperties;
};

const CELL_PAD = 20;

export const DataTable: React.FC<DataTableProps> = ({
  rows,
  visibleRows = rows.length,
  maskProgress = {},
  colState = {},
  scanBeam = null,
  rowHeight = 62,
  fontSize = 24,
  headerSize = 18,
  width = 1240,
  activeRow = null,
  opacity = 1,
  frame = 0,
  enter = 0,
  stagger = 5,
  compact = false,
  style,
}) => {
  const totalWidth = COLUMNS.reduce((a, c) => a + c.width, 0);
  const scale = width / totalWidth;

  return (
    <div
      style={{
        width,
        borderRadius: 16,
        border: `1px solid ${COLORS.line}`,
        background: 'linear-gradient(180deg, #0c1e30 0%, #091726 100%)',
        overflow: 'hidden',
        position: 'relative',
        fontFamily: FONT.mono,
        opacity,
        ...style,
      }}
    >
      <div
        style={{
          display: 'flex',
          height: compact ? 40 : 48,
          alignItems: 'center',
          background: 'rgba(15,38,58,0.92)',
          borderBottom: `1px solid ${COLORS.line}`,
        }}
      >
        {COLUMNS.map((c) => {
          const state = colState[c.key] ?? 'none';
          const color =
            state === 'sensitive' ? COLORS.cyan : state === 'unknown' ? COLORS.amber : COLORS.muted2;
          return (
            <div
              key={c.key}
              style={{
                width: c.width * scale,
                padding: `0 ${CELL_PAD * scale}px`,
                fontSize: headerSize,
                letterSpacing: 1.2,
                color,
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                whiteSpace: 'nowrap',
              }}
            >
              {c.label}
              {state === 'sensitive' && (
                <span style={{width: 6, height: 6, borderRadius: 3, background: COLORS.cyan}} />
              )}
              {state === 'unknown' && (
                <span style={{width: 6, height: 6, borderRadius: 3, background: COLORS.amber}} />
              )}
            </div>
          );
        })}
      </div>

      <div style={{position: 'relative'}}>
        {rows.map((row, i) => {
          const shown = i < visibleRows;
          const p = interpolate(frame, [enter + i * stagger, enter + i * stagger + 16], [0, 1], clamp);
          const isActive = activeRow === i;
          return (
            <div
              key={row.id}
              style={{
                display: 'flex',
                height: rowHeight,
                alignItems: 'center',
                borderBottom: i === rows.length - 1 ? 'none' : `1px solid ${COLORS.line}33`,
                background: isActive ? 'rgba(88,224,186,0.08)' : 'transparent',
                opacity: shown ? p : 0,
                transform: `translateY(${shown ? (1 - p) * 10 : 10}px)`,
              }}
            >
              {COLUMNS.map((c) => {
                const state = colState[c.key] ?? 'none';
                const raw = row[c.key as 'id' | 'name' | 'email' | 'mobile' | 'city'];
                const maskedKey = c.key as 'name' | 'email' | 'mobile';
                const mp = c.sensitive ? (maskProgress[maskedKey] ?? 0) : 0;
                const masked = c.sensitive ? row.masked[maskedKey] : raw;
                const tint =
                  state === 'unknown'
                    ? 'rgba(255,153,106,'
                    : state === 'sensitive' || state === 'masked'
                      ? 'rgba(99,220,243,'
                      : 'rgba(0,0,0,';
                return (
                  <div
                    key={c.key}
                    style={{
                      width: c.width * scale,
                      padding: `0 ${CELL_PAD * scale}px`,
                      fontSize: c.key === 'id' ? fontSize - 3 : fontSize,
                      color:
                        c.key === 'id'
                          ? COLORS.muted2
                          : state === 'unknown'
                            ? COLORS.amber
                            : COLORS.text,
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      letterSpacing: c.key === 'email' || c.key === 'mobile' ? 0.2 : 0.5,
                      background:
                        state === 'none' ? 'transparent' : `${tint}${0.05 + mp * 0.03})`,
                      borderLeft:
                        state === 'sensitive' || state === 'unknown'
                          ? `2px solid ${state === 'unknown' ? COLORS.amber : COLORS.cyan}55`
                          : '2px solid transparent',
                      display: 'flex',
                      alignItems: 'center',
                      height: '100%',
                    }}
                  >
                    {mp > 0 && mp < 1 ? (
                      <RedactionBar
                        width={masked.length * fontSize * 0.6}
                        progress={mp}
                      />
                    ) : (
                      <span
                        style={{
                          opacity: mp >= 1 ? 0.95 : 1,
                          color: mp >= 1 && c.key !== 'id' ? COLORS.teal : undefined,
                        }}
                      >
                        {mp >= 1 ? masked : raw}
                      </span>
                    )}
                  </div>
                );
              })}
            </div>
          );
        })}

        {scanBeam !== null && (
          <div
            style={{
              position: 'absolute',
              left: 0,
              right: 0,
              top: `${Math.max(0, Math.min(1, scanBeam)) * (rows.length * rowHeight)}px`,
              height: 2,
              background: `linear-gradient(90deg, transparent, ${COLORS.cyan}, transparent)`,
              opacity: 0.9,
            }}
          />
        )}
      </div>
    </div>
  );
};

const RedactionBar: React.FC<{width: number; progress: number}> = ({width, progress}) => (
  <span
    style={{
      display: 'inline-block',
      width: Math.max(40, width),
      height: 14,
      borderRadius: 3,
      background: `linear-gradient(90deg, ${COLORS.cyan}, ${COLORS.purple})`,
      opacity: 0.3 + progress * 0.5,
      verticalAlign: 'middle',
    }}
  />
);
