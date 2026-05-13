import React, { useMemo } from 'react';
import type { SectorData } from './types.ts';

interface ChartProps {
  sectors: SectorData[];
  mainLineId?: string | null;
  frame: number;
  totalFrames: number;
  activeEventSector?: string | null;
  width?: number;
  height?: number;
  format?: 'mobile' | 'tv';
  sentiment?: 'bullish' | 'bearish' | 'neutral';
}

const X_MAX = 330;
const NUM_POINTS = 300;

const SECTOR_COLORS: Record<string, string> = {
  '半导体': '#00d4ff',
  'AI应用': '#00ffaa',
  'AI智能体': '#00ff88',
  '人形机器人': '#00ffcc',
  '集成电路': '#00c8ff',
  '软件开发': '#00b4ff',
  '证券': '#ffc107',
  '证券板块': '#ffc107',
  '银行': '#ffb300',
  '白酒': '#ff9800',
  '新能源汽车': '#4caf50',
  '锂电池': '#66bb6a',
  '光伏': '#81c784',
  '储能': '#a5d6a7',
  '军工': '#ff6b9d',
  '航天航空': '#ff8a80',
  '石油': '#ffab40',
  '煤炭': '#ffca28',
  '医药': '#ba68c8',
  '生物制品': '#ce93d8',
};

function getSectorColor(name: string, fallback: string): string {
  for (const key in SECTOR_COLORS) {
    if (name.includes(key)) return SECTOR_COLORS[key];
  }
  return fallback;
}

function createRNG(seed: number) {
  let s = seed | 0;
  return () => {
    s = (s * 1664525 + 1013904223) | 0;
    return (s >>> 0) / 4294967296;
  };
}

function generateCurve(net: number, numPts: number, seed: number): Float64Array {
  const rng = createRNG(seed);
  const base = new Float64Array(numPts);
  const noise = new Float64Array(numPts);

  for (let i = 0; i < numPts; i++) {
    const t = i / (numPts - 1);
    let rhythm: number;
    if (net > 0) {
      if (t < 0.25) {
        rhythm = Math.pow(t / 0.25, 0.7) * 0.35;
      } else if (t < 0.7) {
        rhythm = 0.35 + (t - 0.25) / 0.45 * 0.5;
      } else {
        rhythm = 0.85 + Math.pow((t - 0.7) / 0.3, 1.6) * 0.15;
      }
      base[i] = net * rhythm;
    } else {
      if (t < 0.25) {
        rhythm = Math.pow(t / 0.25, 0.6) * 0.3;
      } else if (t < 0.65) {
        rhythm = 0.3 + (t - 0.25) / 0.4 * 0.55;
      } else {
        rhythm = 0.85 + Math.pow((t - 0.65) / 0.35, 1.4) * 0.15;
      }
      base[i] = net * rhythm;
    }
    noise[i] = (rng() - 0.5) * Math.abs(net) * 0.02;
  }

  const data = new Float64Array(numPts);
  let cumNoise = 0;
  for (let i = 0; i < numPts; i++) {
    cumNoise += noise[i];
    data[i] = base[i] + cumNoise * 0.03;
  }
  return data;
}

function pointsToPath(
  xVals: Float64Array | number[],
  yVals: Float64Array | number[],
  xScale: (v: number) => number,
  yScale: (v: number) => number,
): string {
  const n = xVals.length;
  if (n < 2) return '';
  let d = `M ${xScale(xVals[0])} ${yScale(yVals[0])}`;
  for (let i = 0; i < n - 1; i++) {
    const x1 = xScale(xVals[i]);
    const y1 = yScale(yVals[i]);
    const x2 = xScale(xVals[i + 1]);
    const y2 = yScale(yVals[i + 1]);
    const cx = (x1 + x2) / 2;
    const cy = (y1 + y2) / 2;
    d += ` Q ${x1} ${y1} ${cx} ${cy}`;
  }
  return d;
}

function normalizeY(value: number, yMin: number, yMax: number): number {
  const absVal = Math.abs(value);
  const isNegative = value < 0;

  if (isNegative) {
    // 负值区域：更紧凑的压缩，避免"瀑布式暴跌"视觉效果
    // 0 ~ -50: 放大显示（保持结构层次）
    // -50 ~ -300: 强压缩显示（深度负值压缩）
    if (absVal <= 50) {
      // 小负值：幂函数放大 (0.6 指数使靠近零的值被放大)
      const amplified = Math.pow(absVal / 50, 0.6) * 0.25;
      return -amplified;
    } else {
      // 大负值：强压缩 (0.35 指数使超50部分被大幅压缩)
      const compressed = 0.25 + Math.pow((absVal - 50) / Math.max(Math.abs(yMin) - 50, 1), 0.35) * 0.35;
      return -Math.min(compressed, 0.6);
    }
  } else {
    // 正值区域：更展开，突出资金流入
    // 0 ~ 80: 线性增长
    // 80 ~ 300: 对数增长（突出高值）
    const positiveMax = Math.max(yMax, 100);
    if (absVal <= 80) {
      // 线性增长区域
      return (absVal / 80) * 0.6;
    } else {
      // 对数增长区域：突出高值但不过度展开
      const logScale = Math.log(1 + (absVal - 80) / 80) / Math.log(1 + (positiveMax - 80) / 80);
      return 0.6 + logScale * 0.4;
    }
  }
}

export const Chart: React.FC<ChartProps> = ({
  sectors,
  mainLineId: propMainLineId,
  frame,
  totalFrames,
  activeEventSector,
  width = 1080,
  height = 1920,
  format = 'mobile',
  sentiment = 'neutral',
}) => {
  const isTV = format === 'tv';

  // TV layout: chart ~60%, events ~12%, ranking ~20%
  const chartLeft = isTV ? 30 : 50;
  const chartRight = isTV ? width * 0.60 : 480;
  const chartTop = isTV ? 100 : 180;
  const chartBottom = isTV ? height * 0.78 : height * 0.83;

  const chartW = chartRight - chartLeft;
  const chartH = chartBottom - chartTop;

  const sectorsWithColor = useMemo(() => {
    return sectors.map(s => ({
      ...s,
      color: getSectorColor(s.name, s.color || '#888888'),
    }));
  }, [sectors]);

  const bearishRatio = useMemo(() => {
    if (sectors.length === 0) return 0;
    const negativeCount = sectors.filter(s => s.net < 0).length;
    return negativeCount / sectors.length;
  }, [sectors]);

  const isBearishMarket = bearishRatio > 0.6;

  const zeroBias = isBearishMarket ? 0.35 : 0;

  const yBounds = useMemo(() => {
    if (sectors.length === 0) return { min: -100, max: 300 };
    const nets = sectors.map((s) => s.net);
    const maxIn = Math.max(...nets);
    const minOut = Math.min(...nets);
    const padding = Math.max((maxIn - minOut) * 0.15, 30);
    return {
      max: Math.ceil((maxIn + padding) / 10) * 10,
      min: Math.floor((minOut - padding) / 10) * 10,
    };
  }, [sectors]);

  const curves = useMemo(() => {
    return sectorsWithColor.map((s, i) => ({
      ...s,
      data: generateCurve(s.net, NUM_POINTS, i * 9999 + 42),
    }));
  }, [sectorsWithColor]);

  const sortedByAbs = useMemo(() => {
    return [...sectorsWithColor].sort((a, b) => Math.abs(b.net) - Math.abs(a.net));
  }, [sectorsWithColor]);

  const xScale = (v: number) => chartLeft + (v / X_MAX) * chartW;

  const yScale = (v: number) => {
    const range = yBounds.max - yBounds.min;
    if (range === 0) return chartBottom;
    const normMin = normalizeY(yBounds.min, yBounds.min, yBounds.max);
    const normMax = normalizeY(yBounds.max, yBounds.min, yBounds.max);
    const normRange = normMax - normMin;
    if (normRange === 0) return (chartTop + chartBottom) / 2;
    const normalized = (normalizeY(v, yBounds.min, yBounds.max) - normMin) / normRange;
    const adjusted = normalized * (1 - zeroBias) + zeroBias;
    return chartBottom - adjusted * chartH;
  };

  const yTickStep = useMemo(() => {
    const dataRange = yBounds.max - yBounds.min;
    if (dataRange <= 100) return 10;
    if (dataRange <= 300) return 30;
    if (dataRange <= 600) return 30;
    return 50;
  }, [yBounds]);

  const yTicks: number[] = [];
  for (let v = yBounds.min; v <= yBounds.max; v += yTickStep) {
    yTicks.push(v);
  }

  const yZero = yScale(0);
  const progress = frame / totalFrames;
  const currentIdx = Math.round(progress * (NUM_POINTS - 1));

  const MIN_LABEL_GAP = isTV ? 18 : 22;

  const labelPositions = React.useMemo(() => {
    const positions = new Map<string, { rawY: number; adjY: number }>();
    if (currentIdx < 8) return positions;

    const items: { name: string; rawY: number; rank: number }[] = [];
    for (const sector of curves) {
      const rankIdx = sortedByAbs.findIndex(s => s.name === sector.name);
      if (rankIdx >= 15) continue;
      const ptR = rankIdx < 5 ? (isTV ? 4.5 : 4) : rankIdx < 12 ? (isTV ? 3.5 : 3) : (isTV ? 2.5 : 2);
      if (ptR <= 0) continue;
      const yVal = sector.data[currentIdx];
      const rawY = yScale(yVal);
      items.push({ name: sector.name, rawY, rank: rankIdx });
    }

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
  }, [curves, sortedByAbs, currentIdx, yScale, chartTop, chartBottom, isTV]);

  const curvesJSX = curves.map((sector) => {
    const xValues = new Float64Array(currentIdx + 1);
    const yValues = new Float64Array(currentIdx + 1);
    for (let i = 0; i <= currentIdx; i++) {
      xValues[i] = (i / (NUM_POINTS - 1)) * X_MAX;
      yValues[i] = sector.data[i];
    }

    const pathD = pointsToPath(xValues, yValues, xScale, yScale);
    const rankIdx = sortedByAbs.findIndex(s => s.name === sector.name);
    const isActiveEvent = activeEventSector === sector.name;

    let lineWidth: number;
    let glowWidth: number;
    let glowOpacity: number;
    let mainOpacity: number;
    let pointR: number;
    let pointOpacity: number;

    if (rankIdx < 5) {
      lineWidth = isTV ? 2.5 : 2.2;
      glowWidth = isTV ? 8 : 6;
      glowOpacity = 0.08;
      mainOpacity = 0.7;
      pointR = isTV ? 4.5 : 4;
      pointOpacity = 0.7;
    } else if (rankIdx < 12) {
      lineWidth = isTV ? 1.8 : 1.5;
      glowWidth = isTV ? 4 : 3;
      glowOpacity = 0.04;
      mainOpacity = 0.45;
      pointR = isTV ? 3.5 : 3;
      pointOpacity = 0.45;
    } else if (rankIdx < 20) {
      lineWidth = isTV ? 1.2 : 1;
      glowWidth = isTV ? 2 : 1.5;
      glowOpacity = 0.02;
      mainOpacity = 0.3;
      pointR = isTV ? 2.5 : 2;
      pointOpacity = 0.3;
    } else {
      lineWidth = isTV ? 0.7 : 0.5;
      glowWidth = 0;
      glowOpacity = 0;
      mainOpacity = 0.12;
      pointR = 0;
      pointOpacity = 0;
    }

    const eventPulse = isActiveEvent
      ? Math.sin(frame * 0.4) * 0.4 + 0.8
      : 1;

    const endX = currentIdx > 0 ? xScale(xValues[currentIdx]) : 0;
    const endY = currentIdx > 0 ? yScale(yValues[currentIdx]) : 0;

    const showLabel = currentIdx > 8 && rankIdx < 15 && pointR > 0;
    const showValueLabel = currentIdx > 12 && rankIdx < 15 && pointR > 0;
    const labelPos = labelPositions.get(sector.name);
    const labelY = labelPos ? labelPos.adjY : endY;

    return (
      <g key={sector.name}>
        {glowWidth > 0 && (
          <path
            d={pathD}
            fill="none"
            stroke={sector.color}
            strokeWidth={glowWidth * eventPulse}
            opacity={glowOpacity * eventPulse}
            strokeLinecap="round"
            strokeLinejoin="round"
            style={{ filter: 'blur(8px)' }}
          />
        )}
        <path
          d={pathD}
          fill="none"
          stroke={sector.color}
          strokeWidth={lineWidth * 1.2}
          opacity={mainOpacity * 0.15 * eventPulse}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d={pathD}
          fill="none"
          stroke={sector.color}
          strokeWidth={lineWidth}
          opacity={mainOpacity * eventPulse}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        {isActiveEvent && (
          <path
            d={pathD}
            fill="none"
            stroke="#ffffff"
            strokeWidth={lineWidth * 0.4}
            opacity={0.25 * eventPulse}
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        )}

        {currentIdx > 3 && pointR > 0 && (
          <>
            <circle
              cx={endX}
              cy={endY}
              r={pointR * 2.5}
              fill={sector.color}
              opacity={pointOpacity * 0.12 * eventPulse}
              style={{ filter: 'blur(4px)' }}
            />
            <circle
              cx={endX}
              cy={endY}
              r={pointR}
              fill={sector.color}
              stroke="#ffffff"
              strokeWidth={rankIdx < 5 ? 1.2 : 0.8}
              opacity={Math.min((currentIdx - 3) / 8, 1) * pointOpacity * eventPulse}
            />
            {isActiveEvent && (
              <circle
                cx={endX}
                cy={endY}
                r={pointR * 3}
                fill="none"
                stroke="#ffffff"
                strokeWidth={1}
                opacity={0.35 * eventPulse}
              />
            )}
          </>
        )}

        {showLabel && (
          <>
            {Math.abs(labelY - endY) > 2 && (
              <line
                x1={endX + pointR + 2}
                y1={endY}
                x2={endX + pointR + 8}
                y2={labelY}
                stroke={sector.color}
                strokeWidth={rankIdx < 5 ? 1 : 0.6}
                opacity={Math.min((currentIdx - 8) / 8, 1) * pointOpacity * 0.35 * eventPulse}
              />
            )}
            <line
              x1={endX + pointR + 8}
              y1={labelY}
              x2={endX + pointR + 16}
              y2={labelY}
              stroke={sector.color}
              strokeWidth={rankIdx < 5 ? 1.2 : 0.8}
              opacity={Math.min((currentIdx - 8) / 8, 1) * pointOpacity * 0.5 * eventPulse}
            />
            <text
              x={endX + pointR + 19}
              y={labelY + 1}
              fill={sector.color}
              fontSize={rankIdx < 5 ? (isTV ? 12 : 13) : (isTV ? 10 : 11)}
              fontWeight={rankIdx < 5 ? 600 : 400}
              textAnchor="start"
              dominantBaseline="middle"
              opacity={Math.min((currentIdx - 8) / 8, 1) * (rankIdx < 5 ? 0.85 : 0.6)}
              style={{
                textShadow: `0 0 4px ${sector.color}33`,
                fontFamily: '"PingFang SC", "Helvetica Neue", sans-serif',
              }}
            >
              {sector.name}
            </text>
          </>
        )}

        {showValueLabel && (
          <text
            x={endX + pointR + 19 + (rankIdx < 5 ? 62 : 48)}
            y={labelY + 1}
            fill={sector.net >= 0 ? '#4ade80' : '#f87171'}
            fontSize={rankIdx < 5 ? (isTV ? 12 : 13) : (isTV ? 10 : 11)}
            fontWeight={rankIdx < 5 ? 600 : 400}
            textAnchor="start"
            dominantBaseline="middle"
            opacity={Math.min((currentIdx - 12) / 8, 1) * (rankIdx < 5 ? 0.8 : 0.55)}
            style={{
              fontFamily: '"Helvetica Neue", Arial, sans-serif',
              fontVariantNumeric: 'tabular-nums',
            }}
          >
            {sector.net >= 0 ? '+' : ''}{sector.net.toFixed(1)}
          </text>
        )}
      </g>
    );
  });

  const XTICKS = [
    { pos: 0, label: '09:30' },
    { pos: 60, label: '10:30' },
    { pos: 120, label: '11:30' },
    { pos: 180, label: '13:00' },
    { pos: 240, label: '14:00' },
    { pos: 300, label: '15:00' },
  ];

  return (
    <svg width={width} height={height} style={{ position: 'absolute', top: 0, left: 0, zIndex: 5, pointerEvents: 'none', overflow: 'visible' }}>
      {XTICKS.map((t, i) => {
        const x = xScale(t.pos);
        return (
          <line key={`xgrid${i}`} x1={x} y1={chartTop} x2={x} y2={chartBottom} stroke="#1e2d45" strokeWidth={0.5} opacity={0.5} />
        );
      })}
      {yTicks.map((v) => (
        <line key={`ygrid${v}`} x1={chartLeft} y1={yScale(v)} x2={chartRight} y2={yScale(v)} stroke="#1e2d45" strokeWidth={0.5} opacity={0.6} />
      ))}
      {yZero >= chartTop && yZero <= chartBottom && (
        <>
          <line x1={chartLeft} y1={yZero} x2={chartRight} y2={yZero} stroke="#3a5570" strokeWidth={1.2} opacity={0.7} strokeDasharray="6 4" />
          <text
            x={chartRight + 8}
            y={yZero + 4}
            fill="#6b7280"
            fontSize={12}
            fontWeight={600}
            textAnchor="start"
            fontFamily='"Helvetica Neue", Arial, sans-serif'
          >
            0
          </text>
        </>
      )}

      {XTICKS.map((t, i) => (
        <text
          key={`xlabel${i}`}
          x={xScale(t.pos)}
          y={chartBottom + 22}
          fill="#5a6577"
          fontSize={isTV ? 14 : 16}
          fontWeight={500}
          textAnchor="middle"
          fontFamily='"Helvetica Neue", Arial, sans-serif'
        >
          {t.label}
        </text>
      ))}

      {yTicks.map((v) => {
        const y = yScale(v);
        if (y < chartTop || y > chartBottom) return null;
        return (
          <text
            key={`ylabel${v}`}
            x={chartLeft - 8}
            y={y + 4}
            fill={v === 0 ? '#6b7280' : '#4a5568'}
            fontSize={isTV ? 13 : 15}
            fontWeight={v === 0 ? 600 : 400}
            textAnchor="end"
            fontFamily='"Helvetica Neue", Arial, sans-serif'
          >
            {v >= 0 ? `+${v}` : `${v}`}
          </text>
        );
      })}

      <text
        x={chartLeft}
        y={chartTop - 12}
        fill="#6b7280"
        fontSize={isTV ? 14 : 15}
        fontWeight={500}
        textAnchor="start"
        fontFamily='"Helvetica Neue", Arial, sans-serif'
      >
        主力资金净流入（亿）
      </text>

      {curvesJSX}

      <line
        x1={xScale(progress * X_MAX)}
        y1={chartTop - 8}
        x2={xScale(progress * X_MAX)}
        y2={chartBottom + 8}
        stroke="#5a90d0"
        strokeWidth={1.2}
        opacity={0.6}
        strokeDasharray="4 4"
      />
    </svg>
  );
};
