import React, { useMemo } from 'react';
import type { TickerItem } from './types.ts';

interface TickerProps {
  tickerItems?: TickerItem[];
  frame: number;
  totalFrames: number;
  width?: number;
  height?: number;
  format?: 'mobile' | 'tv';
}

export const Ticker: React.FC<TickerProps> = ({
  tickerItems,
  frame,
  totalFrames,
  width = 1080,
  height = 1920,
  format = 'mobile',
}) => {
  const isTV = format === 'tv';
  const scale = isTV ? 1.1 : 1.1;

  const progress = frame / totalFrames;
  const mins = Math.round(progress * 330);
  const hours = Math.floor(9.5 + mins / 60);
  const minutes = mins % 60;
  const seconds = Math.round((progress * 330 * 60) % 60);
  const timeStr = `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;

  const items = useMemo(() => {
    return tickerItems && tickerItems.length > 0 ? tickerItems : [];
  }, [tickerItems]);

  const totalScrollPixels = items.length * 35 + 500;
  const scrollOffset = (progress * totalScrollPixels) * 0.7;

  const tickerLeft = isTV ? 40 : 50;
  const tickerRight = isTV ? width - 40 : width * 0.68;
  const tickerY = isTV ? height - 55 : height * 0.92;
  const itemSpacing = isTV ? 35 : 45;

  const visibleItems = useMemo(() => {
    const result: { item: TickerItem; y: number; opacity: number }[] = [];
    for (let i = 0; i < items.length; i++) {
      const baseY = tickerY - (i * itemSpacing - scrollOffset);
      if (baseY > tickerY - 80 && baseY < tickerY + 30) {
        const distFromCenter = Math.abs(baseY - tickerY);
        const opacity = Math.max(0, 1 - distFromCenter / 60) * 0.85;
        result.push({ item: items[i], y: baseY, opacity });
      }
    }
    return result;
  }, [items, scrollOffset, itemSpacing, tickerY]);

  return (
    <svg
      width={width}
      height={height}
      style={{ position: 'absolute', top: 0, left: 0, zIndex: 12, pointerEvents: 'none' }}
    >
      <rect
        x={tickerLeft}
        y={tickerY - 80}
        width={tickerRight - tickerLeft}
        height={80}
        fill="#0a0f18"
        opacity={0.92}
        rx={3}
      />

      <line
        x1={tickerLeft}
        y1={tickerY - 80}
        x2={tickerLeft}
        y2={tickerY}
        stroke="#1e3a5f"
        strokeWidth={1.5}
        opacity={0.6}
      />
      <line
        x1={tickerLeft}
        y1={tickerY}
        x2={tickerRight}
        y2={tickerY}
        stroke="#1e3a5f"
        strokeWidth={1}
        opacity={0.4}
      />

      <defs>
        <linearGradient id="tickerFade" x1="0%" y1="0%" x2="0%" y2="100%">
          <stop offset="0%" stopColor="#0a0f18" stopOpacity={0} />
          <stop offset="15%" stopColor="#0a0f18" stopOpacity={0.92} />
          <stop offset="85%" stopColor="#0a0f18" stopOpacity={0.92} />
          <stop offset="100%" stopColor="#0a0f18" stopOpacity={0} />
        </linearGradient>
      </defs>
      <rect
        x={tickerLeft}
        y={tickerY - 80}
        width={tickerRight - tickerLeft}
        height={80}
        fill="url(#tickerFade)"
        pointerEvents="none"
      />

      <rect
        x={tickerLeft}
        y={tickerY - 65}
        width={90}
        height={30}
        fill="#0f1922"
        stroke="#1e3a5f"
        strokeWidth={1}
        rx={3}
      />
      <text
        x={tickerLeft + 45}
        y={tickerY - 46}
        fill="#4a80d0"
        fontSize={13 * scale}
        fontWeight={600}
        fontFamily='"Helvetica Neue", "SF Mono", monospace'
        textAnchor="middle"
        letterSpacing={1}
      >
        {timeStr}
      </text>

      {visibleItems.map(({ item, y, opacity }, idx) => (
        <g key={`ticker-${idx}`} opacity={opacity}>
          <circle
            cx={tickerLeft + 110}
            cy={y - 4}
            r={3}
            fill="#4a80d0"
            opacity={0.6}
          />
          <text
            x={tickerLeft + 120}
            y={y - 4}
            fill="#4a80d0"
            fontSize={12 * scale}
            fontWeight={600}
            fontFamily='"Helvetica Neue", "PingFang SC", sans-serif'
          >
            {item.time}
          </text>
          <text
            x={tickerLeft + 170}
            y={y - 4}
            fill="#8899aa"
            fontSize={12 * scale}
            fontFamily='"PingFang SC", "Helvetica Neue", sans-serif'
          >
            {item.text}
          </text>
        </g>
      ))}

      <g transform={`translate(${tickerRight - 80}, ${tickerY - 55})`}>
        <rect x={0} y={0} width={28} height={22} fill="none" stroke="#2a3550" strokeWidth={1} rx={2} opacity={0.5} />
        <polyline points="6,16 10,8 14,12 18,6 22,10" fill="none" stroke="#4a80d0" strokeWidth={1.2} opacity={0.6} />
        <rect x={32} y={0} width={28} height={22} fill="none" stroke="#2a3550" strokeWidth={1} rx={2} opacity={0.5} />
        <polyline points="38,14 42,10 46,16 50,8 54,12" fill="none" stroke="#4ade80" strokeWidth={1.2} opacity={0.6} />
      </g>
    </svg>
  );
};
