import React, { useMemo } from 'react';
import type { SectorTick } from './types.ts';

interface TickChartProps {
  sectorTicks: SectorTick[];
  frame: number;
  totalFrames: number;
  activeEventSector?: string | null;
  width?: number;
  height?: number;
  format?: 'mobile' | 'tv';
  sentiment?: 'bullish' | 'bearish' | 'neutral';
  session?: 'morning' | 'full';
  xLim?: [number, number];
}

const SECTOR_COLORS: Record<string, string> = {
  '半导体': '#00d4ff',
  'AI应用': '#00ffaa',
  'CPO概念': '#00ff88',
  '有色金属': '#ffc107',
  '锂矿概念': '#66bb6a',
  '商业航天': '#ff8a80',
  '电池': '#4caf50',
  '机器人': '#00ffcc',
  '创新药': '#ba68c8',
  '白酒': '#ff9800',
  '消费电子': '#00c8ff',
  '银行': '#ffb300',
  '人工智能': '#00b4ff',
  '云计算': '#ce93d8',
  '低空经济': '#ff6b9d',
  '电网设备': '#7aa2ff',
  '通信设备': '#89dceb',
  '传媒': '#f4a261',
  '国产芯片': '#e07a5f',
};

function getSectorColor(name: string, fallback: string): string {
  for (const key in SECTOR_COLORS) {
    if (name.includes(key)) return SECTOR_COLORS[key];
  }
  return fallback;
}

export const TickChart: React.FC<TickChartProps> = ({
  sectorTicks,
  frame,
  totalFrames,
  activeEventSector,
  width = 1080,
  height = 1920,
  format = 'mobile',
  sentiment = 'neutral',
  session = 'full',
  xLim: propXLim,
}) => {
  const isTV = format === 'tv';
  const xMax = propXLim ? propXLim[1] : 330;
  const isMorning = session === 'morning';

  const chartLeft = isTV ? 30 : 50;
  const chartRight = isTV ? width * 0.60 : 480;
  const chartTop = isTV ? 100 : 180;
  const chartBottom = isTV ? height * 0.78 : height * 0.83;

  const chartW = chartRight - chartLeft;
  const chartH = chartBottom - chartTop;

  const coloredTicks = useMemo(() => {
    return sectorTicks.map(s => ({
      ...s,
      color: s.color || getSectorColor(s.name, '#888888'),
    }));
  }, [sectorTicks]);

  const cumulativeData = useMemo(() => {
    return coloredTicks.map(s => {
      const cum: number[] = [];
      let sum = 0;
      for (const v of s.data) {
        sum += v;
        cum.push(sum);
      }
      return { ...s, cum };
    });
  }, [coloredTicks]);

  const sortedByAbs = useMemo(() => {
    return [...cumulativeData].sort((a, b) => {
      const lastA = a.cum.length > 0 ? Math.abs(a.cum[a.cum.length - 1]) : 0;
      const lastB = b.cum.length > 0 ? Math.abs(b.cum[b.cum.length - 1]) : 0;
      return lastB - lastA;
    });
  }, [cumulativeData]);

  const yBounds = useMemo(() => {
    if (cumulativeData.length === 0) return { min: -100, max: 300 };
    let minVal = 0, maxVal = 0;
    for (const s of cumulativeData) {
      for (const v of s.cum) {
        if (v < minVal) minVal = v;
        if (v > maxVal) maxVal = v;
      }
    }
    const padding = Math.max((maxVal - minVal) * 0.15, 30);
    return {
      max: Math.ceil((maxVal + padding) / 10) * 10,
      min: Math.floor((minVal - padding) / 10) * 10,
    };
  }, [cumulativeData]);

  const xScale = (v: number) => chartLeft + (v / xMax) * chartW;
  const yScale = (v: number) => {
    const range = yBounds.max - yBounds.min;
    if (range === 0) return (chartTop + chartBottom) / 2;
    const normalized = (v - yBounds.min) / range;
    return chartBottom - normalized * chartH;
  };

  const progress = frame / totalFrames;
  const numPoints = cumulativeData.length > 0 ? cumulativeData[0].cum.length : 1;
  const currentIdx = Math.min(Math.floor(progress * numPoints), numPoints - 1);

  const yTickStep = useMemo(() => {
    const dataRange = yBounds.max - yBounds.min;
    if (dataRange <= 100) return 10;
    if (dataRange <= 300) return 30;
    if (dataRange <= 600) return 30;
    return 50;
  }, [yBounds]);

  const yTicks: number[] = [];
  const startTick = Math.ceil(yBounds.min / yTickStep) * yTickStep;
  for (let v = startTick; v <= yBounds.max; v += yTickStep) {
    yTicks.push(Math.round(v));
  }

  const yZero = yScale(0);

  const MIN_LABEL_GAP = isTV ? 18 : 22;
  const labelPositions = React.useMemo(() => {
    const positions = new Map<string, { rawY: number; adjY: number }>();
    if (currentIdx < 2) return positions;

    const items = cumulativeData
      .map((s, rankIdx) => {
        if (rankIdx >= 18) return null;
        const yVal = s.cum[currentIdx];
        const rawY = yScale(yVal);
        return { name: s.name, rawY, rank: rankIdx };
      })
      .filter(Boolean) as { name: string; rawY: number; rank: number }[];

    items.sort((a, b) => a.rawY - b.rawY);
    const adjusted = items.map(it => ({ ...it, adjY: it.rawY }));

    for (let pass = 0; pass < 6; pass++) {
      let shifted = false;
      for (let i = 1; i < adjusted.length; i++) {
        const gap = adjusted[i].adjY - adjusted[i - 1].adjY;
        if (gap < MIN_LABEL_GAP && gap > -MIN_LABEL_GAP) {
          const push = (MIN_LABEL_GAP - gap) / 2 + 0.5;
          adjusted[i - 1].adjY -= push;
          adjusted[i].adjY += push;
          shifted = true;
        }
      }
      if (!shifted) break;
    }

    for (const it of adjusted) {
      const clampedY = Math.max(chartTop + 10, Math.min(chartBottom - 10, it.adjY));
      positions.set(it.name, { rawY: it.rawY, adjY: clampedY });
    }

    return positions;
  }, [cumulativeData, currentIdx, yScale, chartTop, chartBottom, isTV]);

  const curvesJSX = cumulativeData.map((sector) => {
    const rankIdx = sortedByAbs.findIndex(s => s.name === sector.name);
    const isActiveEvent = activeEventSector === sector.name;

    let lineWidth: number;
    let glowWidth: number;
    let glowOpacity: number;
    let mainOpacity: number;
    let pointR: number;
    let pointOpacity: number;

    if (rankIdx < 5) {
      lineWidth = isTV ? 2.5 : 3.8;
      glowWidth = isTV ? 8 : 12;
      glowOpacity = 0.12;
      mainOpacity = 0.85;
      pointR = isTV ? 4.5 : 6.5;
      pointOpacity = 0.85;
    } else if (rankIdx < 12) {
      lineWidth = isTV ? 1.8 : 3.0;
      glowWidth = isTV ? 4 : 8;
      glowOpacity = 0.08;
      mainOpacity = 0.6;
      pointR = isTV ? 3.5 : 5.5;
      pointOpacity = 0.6;
    } else if (rankIdx < 20) {
      lineWidth = isTV ? 1.2 : 2.2;
      glowWidth = isTV ? 2 : 4;
      glowOpacity = 0.04;
      mainOpacity = 0.4;
      pointR = isTV ? 2.5 : 4.5;
      pointOpacity = 0.4;
    } else {
      lineWidth = isTV ? 0.7 : 1.4;
      glowWidth = 0;
      glowOpacity = 0;
      mainOpacity = 0.15;
      pointR = 0;
      pointOpacity = 0;
    }

    const eventPulse = isActiveEvent ? Math.sin(frame * 0.4) * 0.4 + 0.8 : 1;

    const visibleCum = sector.cum.slice(0, currentIdx + 1);
    const visibleTimes = sector.times.slice(0, currentIdx + 1);

    let pathD = '';
    for (let i = 0; i < visibleCum.length; i++) {
      const xVal = visibleTimes.length > 1
        ? (i / (visibleTimes.length - 1)) * xMax
        : xMax / 2;
      const px = xScale(xVal);
      const py = yScale(visibleCum[i]);
      pathD += (i === 0 ? 'M' : 'L') + ` ${px} ${py}`;
    }

    const endX = visibleCum.length > 0 ? xScale(xMax) : 0;
    const endY = visibleCum.length > 0 ? yScale(visibleCum[visibleCum.length - 1]) : 0;

    const showLabel = currentIdx > 2 && rankIdx < 18 && pointR > 0;
    const showValueLabel = currentIdx > 4 && rankIdx < 18 && pointR > 0;
    const labelPos = labelPositions.get(sector.name);
    const labelY = labelPos ? labelPos.adjY : endY;

    return (
      <g key={sector.name}>
        {glowWidth > 0 && (
          <path d={pathD} fill="none" stroke={sector.color} strokeWidth={glowWidth * eventPulse} opacity={glowOpacity * eventPulse} strokeLinecap="round" strokeLinejoin="round" style={{ filter: 'blur(8px)' }} />
        )}
        <path d={pathD} fill="none" stroke={sector.color} strokeWidth={lineWidth * 1.2} opacity={mainOpacity * 0.15 * eventPulse} strokeLinecap="round" strokeLinejoin="round" />
        <path d={pathD} fill="none" stroke={sector.color} strokeWidth={lineWidth} opacity={mainOpacity * eventPulse} strokeLinecap="round" strokeLinejoin="round" />
        {isActiveEvent && (
          <path d={pathD} fill="none" stroke="#ffffff" strokeWidth={lineWidth * 0.4} opacity={0.25 * eventPulse} strokeLinecap="round" strokeLinejoin="round" />
        )}

        {currentIdx > 1 && pointR > 0 && (
          <>
            <circle cx={endX} cy={endY} r={pointR * 2.5} fill={sector.color} opacity={pointOpacity * 0.12 * eventPulse} style={{ filter: 'blur(4px)' }} />
            <circle cx={endX} cy={endY} r={pointR} fill={sector.color} stroke="#ffffff" strokeWidth={rankIdx < 5 ? 1.2 : 0.8} opacity={Math.min((currentIdx - 1) / 4, 1) * pointOpacity * eventPulse} />
          </>
        )}

        {showLabel && (
          <>
            {Math.abs(labelY - endY) > 2 && (
              <line x1={endX + pointR + 2} y1={endY} x2={endX + pointR + 8} y2={labelY} stroke={sector.color} strokeWidth={rankIdx < 5 ? 1.5 : 1.0} opacity={Math.min((currentIdx - 2) / 4, 1) * pointOpacity * 0.5 * eventPulse} />
            )}
            <line x1={endX + pointR + 8} y1={labelY} x2={endX + pointR + 16} y2={labelY} stroke={sector.color} strokeWidth={rankIdx < 5 ? 1.6 : 1.2} opacity={Math.min((currentIdx - 2) / 4, 1) * pointOpacity * 0.5 * eventPulse} />
            <text x={endX + pointR + 19} y={labelY + 1} fill={sector.color} fontSize={rankIdx < 5 ? (isTV ? 12 : 20) : (isTV ? 10 : 18)} fontWeight={rankIdx < 5 ? 600 : 400} textAnchor="start" dominantBaseline="middle" opacity={Math.min((currentIdx - 2) / 4, 1) * (rankIdx < 5 ? 0.85 : 0.6)} style={{ textShadow: `0 0 4px ${sector.color}33`, fontFamily: '"PingFang SC", "Helvetica Neue", sans-serif' }}>
              {sector.name}
            </text>
          </>
        )}

        {showValueLabel && (
          <text x={endX + pointR + 19 + (rankIdx < 5 ? 62 : 48)} y={labelY + 1} fill={visibleCum[currentIdx] >= 0 ? '#4ade80' : '#f87171'} fontSize={rankIdx < 5 ? (isTV ? 12 : 20) : (isTV ? 10 : 18)} fontWeight={rankIdx < 5 ? 600 : 400} textAnchor="start" dominantBaseline="middle" opacity={Math.min((currentIdx - 4) / 4, 1) * (rankIdx < 5 ? 0.8 : 0.55)} style={{ fontFamily: '"Helvetica Neue", Arial, sans-serif', fontVariantNumeric: 'tabular-nums' }}>
            {visibleCum[currentIdx] >= 0 ? '+' : ''}{visibleCum[currentIdx].toFixed(1)}
          </text>
        )}
      </g>
    );
  });

  const XTICKS = isMorning
    ? [{ pos: 0, label: '09:30' }, { pos: 30, label: '10:00' }, { pos: 60, label: '10:30' }, { pos: 90, label: '11:00' }, { pos: 120, label: '11:30' }]
    : [{ pos: 0, label: '09:30' }, { pos: 60, label: '10:30' }, { pos: 120, label: '11:30' }, { pos: 180, label: '13:00' }, { pos: 240, label: '14:00' }, { pos: 300, label: '15:00' }];

  return (
    <svg width={width} height={height} style={{ position: 'absolute', top: 0, left: 0, zIndex: 5, pointerEvents: 'none', overflow: 'visible' }}>
      {XTICKS.map((t, i) => (
        <line key={`xgrid${i}`} x1={xScale(t.pos)} y1={chartTop} x2={xScale(t.pos)} y2={chartBottom} stroke="#1e2d45" strokeWidth={0.8} opacity={0.5} />
      ))}
      {yTicks.map((v) => (
        <line key={`ygrid${v}`} x1={chartLeft} y1={yScale(v)} x2={chartRight} y2={yScale(v)} stroke="#1e2d45" strokeWidth={0.8} opacity={0.6} />
      ))}
      {yZero >= chartTop && yZero <= chartBottom && (
        <>
          <line x1={chartLeft} y1={yZero} x2={chartRight} y2={yZero} stroke="#3a5570" strokeWidth={1.5} opacity={0.7} strokeDasharray="6 4" />
          <text x={chartRight + 8} y={yZero + 4} fill="#6b7280" fontSize={isTV ? 12 : 16} fontWeight={600} textAnchor="start" fontFamily='"Helvetica Neue", Arial, sans-serif'>0</text>
        </>
      )}
      {XTICKS.map((t, i) => (
        <text key={`xlabel${i}`} x={xScale(t.pos)} y={chartBottom + 22} fill="#5a6577" fontSize={isTV ? 14 : 20} fontWeight={500} textAnchor="middle" fontFamily='"Helvetica Neue", Arial, sans-serif'>{t.label}</text>
      ))}
      {yTicks.map((v) => {
        const y = yScale(v);
        if (y < chartTop || y > chartBottom) return null;
        return (
          <text key={`ylabel${v}`} x={chartLeft - 8} y={y + 4} fill={v === 0 ? '#6b7280' : '#4a5568'} fontSize={isTV ? 13 : 18} fontWeight={v === 0 ? 600 : 400} textAnchor="end" fontFamily='"Helvetica Neue", Arial, sans-serif'>{v >= 0 ? `+${v}` : `${v}`}</text>
        );
      })}
      <text x={chartLeft} y={chartTop - 12} fill="#6b7280" fontSize={isTV ? 14 : 15} fontWeight={500} textAnchor="start" fontFamily='"Helvetica Neue", Arial, sans-serif'>主力资金净流入（亿）</text>
      {curvesJSX}
      <line x1={xScale(progress * xMax)} y1={chartTop - 8} x2={xScale(progress * xMax)} y2={chartBottom + 8} stroke="#5a90d0" strokeWidth={1.2} opacity={0.6} strokeDasharray="4 4" />
    </svg>
  );
};
