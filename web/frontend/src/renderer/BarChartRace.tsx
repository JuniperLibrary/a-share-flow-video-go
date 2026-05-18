import React, { useMemo, useRef, useEffect } from 'react';
import { AbsoluteFill, interpolate, useCurrentFrame, spring } from 'remotion';
import type { BarSnapshot, MultiDayAnalysis, SectorData } from './types.ts';

// ============================================================
// Cinematic Financial Bar Chart Race
// Bloomberg Terminal / TradingView Premium Style
// ============================================================

const BAR_HEIGHT = 44;
const BAR_GAP = 8;
const LABEL_WIDTH = 110;
const VALUE_WIDTH = 100;
const MAX_BAR_WIDTH_RATIO = 0.52;
const TOP_SECTION_HEIGHT_RATIO = 0.70;

// Financial color palette (low saturation, institutional feel)
const FINANCIAL_COLORS: Record<string, string> = {
  inflow: '#00B4D8',     // Cyan-blue for inflow
  inflowBright: '#00F0FF', // Brighter cyan for main line
  outflow: '#E63946',    // Muted red for outflow
  neutral: '#4A90D9',    // Steel blue
  grid: 'rgba(74, 144, 217, 0.08)',
};

// Sector-specific colors (financial terminal style)
const SECTOR_COLOR_MAP: Record<string, string> = {
  '半导体': '#00B4D8',
  'AI应用': '#00F0FF',
  'CPO概念': '#48CAE4',
  '人工智能': '#0096C7',
  '机器人': '#F4A261',
  '商业航天': '#E76F51',
  '创新药': '#2A9D8F',
  '电池': '#264653',
  '银行': '#E9C46A',
  '白酒': '#D4A574',
  '消费电子': '#81B29A',
  '低空经济': '#F2CC8F',
  '锂矿概念': '#95D5B2',
  '有色金属': '#B5838D',
  '云计算': '#6D6875',
  '国产芯片': '#E07A5F',
};

function easeInOutCubic(t: number): number {
  return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
}

function formatNet(net: number): string {
  const sign = net >= 0 ? '+' : '';
  return `${sign}${net.toFixed(1)}亿`;
}

// Animated position tracker for smooth transitions
interface SectorPosition {
  name: string;
  targetY: number;
  currentY: number;
  targetNet: number;
  currentNet: number;
  color: string;
  rank: number;
  isMainLine: boolean;
  prevRank: number;
}

interface BarRaceChartProps {
  snapshots: BarSnapshot[];
  analysis: MultiDayAnalysis;
  frame: number;
  totalFrames: number;
  width: number;
  height: number;
}

export const BarRaceChart: React.FC<BarRaceChartProps> = ({
  snapshots,
  analysis,
  frame,
  totalFrames,
  width,
  height,
}) => {
  if (snapshots.length === 0) return null;

  // Frame ranges
  const RACE_START = 60;
  const RACE_END = 839;
  const TRANSITION_DURATION = 50;

  const numDays = snapshots.length;
  const raceDuration = RACE_END - RACE_START;
  const holdDuration = Math.floor((raceDuration - (numDays - 1) * TRANSITION_DURATION) / numDays);

  // Determine current phase
  const raceFrame = frame - RACE_START;
  let currentDayIndex = 0;
  let transitionProgress = 0;
  let isTransition = false;
  let accumulated = 0;

  for (let i = 0; i < numDays; i++) {
    if (raceFrame < accumulated + holdDuration) {
      currentDayIndex = i;
      transitionProgress = 0;
      isTransition = false;
      break;
    }
    accumulated += holdDuration;
    if (i < numDays - 1 && raceFrame < accumulated + TRANSITION_DURATION) {
      currentDayIndex = i;
      transitionProgress = (raceFrame - accumulated) / TRANSITION_DURATION;
      isTransition = true;
      break;
    }
    accumulated += TRANSITION_DURATION;
  }
  if (!isTransition && raceFrame >= accumulated) {
    currentDayIndex = numDays - 1;
  }

  const easedProgress = isTransition ? easeInOutCubic(Math.min(1, Math.max(0, transitionProgress))) : 0;

  const currentSnapshot = snapshots[currentDayIndex];
  const nextSnapshot = isTransition ? snapshots[Math.min(currentDayIndex + 1, numDays - 1)] : currentSnapshot;

  // Build sector data with interpolated positions
  const sectorMap = useMemo(() => {
    const map = new Map<string, { currentNet: number; nextNet: number; currentRank: number; nextRank: number; color: string }>();

    for (const bar of currentSnapshot.bars) {
      const rank = currentSnapshot.bars.indexOf(bar) + 1;
      const nextBar = nextSnapshot.bars.find(b => b.name === bar.name);
      const nextRank = nextBar ? nextSnapshot.bars.indexOf(nextBar) + 1 : rank;

      const color = SECTOR_COLOR_MAP[bar.name] || (bar.net >= 0 ? FINANCIAL_COLORS.inflow : FINANCIAL_COLORS.outflow);

      map.set(bar.name, {
        currentNet: bar.net,
        nextNet: nextBar?.net ?? bar.net,
        currentRank: rank,
        nextRank,
        color,
      });
    }

    if (isTransition) {
      for (const bar of nextSnapshot.bars) {
        if (!map.has(bar.name)) {
          const rank = nextSnapshot.bars.indexOf(bar) + 1;
          const currentBar = currentSnapshot.bars.find(b => b.name === bar.name);
          const color = SECTOR_COLOR_MAP[bar.name] || (bar.net >= 0 ? FINANCIAL_COLORS.inflow : FINANCIAL_COLORS.outflow);
          map.set(bar.name, {
            currentNet: currentBar?.net ?? 0,
            nextNet: bar.net,
            currentRank: currentBar ? currentSnapshot.bars.indexOf(currentBar) + 1 : rank,
            nextRank: rank,
            color,
          });
        }
      }
    }

    return map;
  }, [currentSnapshot, nextSnapshot, isTransition]);

  // Interpolate and sort
  const sectors = useMemo(() => {
    return Array.from(sectorMap.entries()).map(([name, data]) => {
      const net = data.currentNet + (data.nextNet - data.currentNet) * easedProgress;
      const rank = data.currentRank + (data.nextRank - data.currentRank) * easedProgress;
      return { name, net, rank, color: data.color, prevRank: data.currentRank, targetRank: data.nextRank };
    }).sort((a, b) => a.rank - b.rank).slice(0, 18);
  }, [sectorMap, easedProgress]);

  // Identify main line (sector with highest absolute net flow)
  const mainLineName = useMemo(() => {
    let maxAbs = 0;
    let main = sectors[0]?.name || '';
    for (const s of sectors) {
      if (Math.abs(s.net) > maxAbs) {
        maxAbs = Math.abs(s.net);
        main = s.name;
      }
    }
    return main;
  }, [sectors]);

  // Calculate max bar width
  const maxAbsNet = Math.max(1, ...sectors.map(s => Math.abs(s.net)));
  const maxBarWidth = width * MAX_BAR_WIDTH_RATIO;

  // Top section height
  const topSectionHeight = height * TOP_SECTION_HEIGHT_RATIO;
  const chartTop = topSectionHeight * 0.12;

  // Current date display
  const currentDate = currentSnapshot.date;
  const nextDate = isTransition ? nextSnapshot.date : currentDate;
  const dateDisplay = isTransition ? `${currentDate} → ${nextDate}` : currentDate;

  // Time simulation based on race progress
  const timeProgress = (frame - RACE_START) / (RACE_END - RACE_START);
  const simulatedHour = 9 + Math.floor(timeProgress * 5.5);
  const simulatedMin = Math.floor((timeProgress * 330) % 60);
  const timeStr = `${String(simulatedHour).padStart(2, '0')}:${String(simulatedMin).padStart(2, '0')}`;

  // Sentiment calculation
  const inflowCount = sectors.filter(s => s.net > 0).length;
  const sentiment = inflowCount > sectors.length * 0.6 ? 'bullish' : inflowCount < sectors.length * 0.4 ? 'bearish' : 'neutral';

  return (
    <AbsoluteFill style={{ backgroundColor: '#0a0e17' }}>
      {/* Background */}
      <div style={{
        position: 'absolute',
        inset: 0,
        background: 'linear-gradient(180deg, #0a0e17 0%, #0d1220 40%, #0a0e17 100%)',
      }} />

      {/* HUD Grid */}
      <div style={{ position: 'absolute', inset: 0, opacity: 0.05 }}>
        {Array.from({ length: Math.floor(topSectionHeight / 30) }).map((_, i) => (
          <div key={i} style={{
            position: 'absolute',
            left: 0,
            right: 0,
            top: i * 30,
            height: 1,
            backgroundColor: '#4a90d9',
          }} />
        ))}
      </div>

      {/* Scan line */}
      <div style={{
        position: 'absolute',
        left: 0,
        right: 0,
        top: interpolate(frame % 100, [0, 100], [0, topSectionHeight]),
        height: 2,
        background: 'linear-gradient(90deg, transparent, rgba(0, 240, 255, 0.1), transparent)',
      }} />

      {/* Top Header Bar */}
      <div style={{
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        height: 36,
        display: 'flex',
        alignItems: 'center',
        padding: '0 20px',
        borderBottom: '1px solid rgba(0, 240, 255, 0.1)',
        background: 'rgba(10, 14, 23, 0.8)',
        zIndex: 10,
      }}>
        <div style={{
          fontSize: 14,
          fontWeight: 700,
          color: '#00F0FF',
          letterSpacing: 2,
          fontFamily: 'monospace',
        }}>
          A股资金流监控系统
        </div>
        <div style={{ flex: 1 }} />
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: 16,
          fontSize: 13,
          fontFamily: 'monospace',
        }}>
          <span style={{ color: '#5a6577' }}>{dateDisplay}</span>
          <span style={{ color: '#e0e4e8', fontWeight: 600 }}>{timeStr}</span>
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
          }}>
            <div style={{
              width: 6,
              height: 6,
              borderRadius: '50%',
              backgroundColor: sentiment === 'bullish' ? '#00F0FF' : sentiment === 'bearish' ? '#FF6B8A' : '#8892a4',
              boxShadow: `0 0 8px ${sentiment === 'bullish' ? '#00F0FF' : sentiment === 'bearish' ? '#FF6B8A' : '#8892a4'}80`,
              animation: 'pulse 1.5s ease-in-out infinite',
            }} />
            <span style={{ color: '#8892a4', fontSize: 11, letterSpacing: 1 }}>LIVE</span>
          </div>
        </div>
      </div>

      {/* Column Headers */}
      <div style={{
        position: 'absolute',
        top: 44,
        left: LABEL_WIDTH + 10,
        right: VALUE_WIDTH + 20,
        height: 20,
        display: 'flex',
        alignItems: 'center',
        fontSize: 12,
        fontWeight: 600,
        color: '#4a5568',
        letterSpacing: 1.5,
        textTransform: 'uppercase',
        borderBottom: '1px solid rgba(74, 144, 217, 0.1)',
      }}>
        <div style={{ flex: 1 }}>主力资金净流入</div>
        <div style={{ textAlign: 'right', minWidth: VALUE_WIDTH }}>实时数值</div>
      </div>

      {/* Bars */}
      {sectors.map((sector, index) => {
        const y = chartTop + 28 + index * (BAR_HEIGHT + BAR_GAP);
        const barWidth = (Math.abs(sector.net) / maxAbsNet) * maxBarWidth;
        const isMainLine = sector.name === mainLineName;
        const color = sector.color;
        const rankChange = sector.prevRank - (index + 1);
        const isRising = rankChange > 0;
        const isFalling = rankChange < 0;

        // Spring animation for entry
        const entryOpacity = spring({ frame: Math.max(0, frame - 60 - index * 5), fps: 30, config: { damping: 12, stiffness: 80 } });

        return (
          <div key={sector.name} style={{
            position: 'absolute',
            top: y,
            left: 0,
            right: 0,
            height: BAR_HEIGHT,
            opacity: entryOpacity,
          }}>
            {/* Rank number */}
            <div style={{
              position: 'absolute',
              left: 8,
              top: 0,
              width: 20,
              height: BAR_HEIGHT,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 13,
              fontWeight: 700,
              color: isMainLine ? '#00F0FF' : '#4a5568',
              fontFamily: 'monospace',
            }}>
              {String(index + 1).padStart(2, '0')}
            </div>

            {/* Sector name */}
            <div style={{
              position: 'absolute',
              left: 28,
              top: 0,
              width: LABEL_WIDTH - 20,
              height: BAR_HEIGHT,
              display: 'flex',
              alignItems: 'center',
              fontSize: isMainLine ? 18 : 16,
              fontWeight: isMainLine ? 700 : 500,
              color: isMainLine ? '#ffffff' : '#c0c8d4',
              fontFamily: isMainLine ? 'sans-serif' : 'sans-serif',
              textShadow: isMainLine ? '0 0 12px rgba(0, 240, 255, 0.4)' : 'none',
            }}>
              {sector.name}
              {/* Main line indicator */}
              {isMainLine && (
                <span style={{
                  marginLeft: 6,
                  fontSize: 9,
                  padding: '1px 4px',
                  borderRadius: 2,
                  backgroundColor: 'rgba(0, 240, 255, 0.2)',
                  color: '#00F0FF',
                  letterSpacing: 0.5,
                }}>
                  主线
                </span>
              )}
            </div>

            {/* Bar container */}
            <div style={{
              position: 'absolute',
              left: LABEL_WIDTH + 10,
              right: VALUE_WIDTH + 10,
              top: 0,
              height: BAR_HEIGHT,
              display: 'flex',
              alignItems: 'center',
            }}>
              {/* Bar background track */}
              <div style={{
                position: 'absolute',
                left: 0,
                right: 0,
                height: BAR_HEIGHT * 0.6,
                borderRadius: 2,
                backgroundColor: 'rgba(74, 144, 217, 0.06)',
              }} />

              {/* Bar fill */}
              <div style={{
                position: 'absolute',
                left: 0,
                width: barWidth,
                height: BAR_HEIGHT * 0.6,
                borderRadius: 2,
                background: isMainLine
                  ? `linear-gradient(90deg, ${color}80, ${color})`
                  : `linear-gradient(90deg, ${color}60, ${color}90)`,
                boxShadow: isMainLine
                  ? `0 0 16px ${color}40, inset 0 0 8px ${color}20`
                  : `0 0 8px ${color}20`,
                transition: 'width 0.3s ease-out',
              }}>
                {/* Bar inner glow / HUD shine */}
                <div style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  right: 0,
                  height: '50%',
                  background: 'linear-gradient(180deg, rgba(255,255,255,0.12), transparent)',
                  borderRadius: '2px 2px 0 0',
                }} />
              </div>
            </div>

            {/* Value */}
            <div style={{
              position: 'absolute',
              right: 10,
              top: 0,
              width: VALUE_WIDTH,
              height: BAR_HEIGHT,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'flex-end',
              fontSize: isMainLine ? 17 : 15,
              fontWeight: 700,
              color: color,
              fontFamily: 'monospace',
              textShadow: isMainLine ? `0 0 10px ${color}60` : 'none',
            }}>
              {formatNet(sector.net)}
            </div>

            {/* Rank change indicator */}
            {rankChange !== 0 && (
              <div style={{
                position: 'absolute',
                right: VALUE_WIDTH + 15,
                top: 0,
                height: BAR_HEIGHT,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontSize: 12,
                fontWeight: 700,
                color: isRising ? '#00F0FF' : '#FF6B8A',
                fontFamily: 'monospace',
                textShadow: `0 0 6px ${isRising ? '#00F0FF' : '#FF6B8A'}40`,
              }}>
                {isRising ? '▲' : '▼'}{Math.abs(rankChange)}
              </div>
            )}

            {/* Separator line */}
            <div style={{
              position: 'absolute',
              bottom: -BAR_GAP / 2,
              left: LABEL_WIDTH + 10,
              right: VALUE_WIDTH + 10,
              height: 1,
              backgroundColor: 'rgba(74, 144, 217, 0.04)',
            }} />
          </div>
        );
      })}

      {/* Bottom separator for 70/30 split */}
      <div style={{
        position: 'absolute',
        top: topSectionHeight,
        left: 0,
        right: 0,
        height: 1,
        background: 'linear-gradient(90deg, transparent, rgba(0, 240, 255, 0.2), transparent)',
      }} />

      {/* Progress bar at top of chart area */}
      <div style={{
        position: 'absolute',
        top: 38,
        left: 20,
        right: 20,
        height: 2,
        backgroundColor: 'rgba(74, 144, 217, 0.1)',
        borderRadius: 1,
      }}>
        <div style={{
          width: `${timeProgress * 100}%`,
          height: '100%',
          backgroundColor: '#00F0FF',
          borderRadius: 1,
          boxShadow: '0 0 8px rgba(0, 240, 255, 0.4)',
        }} />
      </div>

      {/* Date markers on progress bar */}
      <div style={{
        position: 'absolute',
        top: 42,
        left: 20,
        right: 20,
        display: 'flex',
        justifyContent: 'space-between',
        fontSize: 11,
        color: '#3a4557',
        fontFamily: 'monospace',
      }}>
        {snapshots.map((s, i) => (
          <span key={s.date} style={{
            color: i === currentDayIndex ? '#00F0FF' : '#3a4557',
            fontWeight: i === currentDayIndex ? 600 : 400,
          }}>
            {s.date}
          </span>
        ))}
      </div>
    </AbsoluteFill>
  );
};
