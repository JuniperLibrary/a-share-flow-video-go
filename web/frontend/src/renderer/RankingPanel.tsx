import React from 'react';
import type { SectorData } from './types.ts';

interface RankingPanelProps {
  sectors: SectorData[];
  frame: number;
  totalFrames: number;
  highlightId?: string;
  width?: number;
  height?: number;
  format?: 'mobile' | 'tv';
}

const RankingGroup: React.FC<{
  title: string;
  sectors: SectorData[];
  isPositive: boolean;
  visibleCount: number;
  highlightId?: string;
  frame: number;
  scale: number;
}> = ({ title, sectors, isPositive, visibleCount, highlightId, frame, scale }) => {
  const color = isPositive ? '#4ade80' : '#f87171';
  const arrow = isPositive ? '↑' : '↓';
  const displaySectors = sectors.slice(0, 18);

  return (
    <div style={{ marginBottom: 20 * scale }}>
      <div
        style={{
          fontSize: 16 * scale,
          color: '#8899aa',
          fontWeight: 600,
          letterSpacing: 1,
          paddingBottom: 6 * scale,
          marginBottom: 4 * scale,
          borderBottom: `1px solid ${color}33`,
        }}
      >
        {title}
      </div>

      {displaySectors.map((sector, i) => {
        const isVisible = i < visibleCount;
        const opacity = isVisible ? Math.min(1, (frame - i * 5) / 10) : 0;
        const isHighlighted = sector.name === highlightId;
        const pulseOpacity = isHighlighted ? Math.min(0.4, (frame % 30) / 30) : 0;

        return (
          <div
            key={sector.name}
            style={{
              display: 'flex',
              alignItems: 'center',
              padding: `${5 * scale}px 0`,
              opacity: Math.max(0, opacity),
              borderBottom: '1px solid rgba(42,53,80,0.15)',
              background: isHighlighted ? `linear-gradient(90deg, ${sector.color}18, transparent)` : undefined,
              position: 'relative',
            }}
          >
            {isHighlighted && (
              <div
                style={{
                  position: 'absolute',
                  left: 0,
                  top: 0,
                  right: 0,
                  bottom: 0,
                  borderRadius: 4,
                  background: `radial-gradient(circle at 30% 50%, ${sector.color}${Math.round(pulseOpacity * 50).toString(16).padStart(2, '0')}, transparent 70%)`,
                  pointerEvents: 'none',
                }}
              />
            )}

            <span
              style={{
                fontSize: 14 * scale,
                fontWeight: 700,
                color: isHighlighted ? sector.color : (i < 3 ? '#aabbcc' : '#556677'),
                width: 22 * scale,
                fontVariantNumeric: 'tabular-nums' as const,
                position: 'relative',
                zIndex: 1,
              }}
            >
              {i + 1}
            </span>

            <span
              style={{
                fontSize: 15 * scale,
                fontWeight: 600,
                color: isHighlighted ? '#ddeeff' : '#ccddee',
                flex: 1,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                position: 'relative',
                zIndex: 1,
              }}
            >
              {sector.name}
            </span>

            <span
              style={{
                fontSize: 15 * scale,
                fontWeight: 700,
                color,
                fontVariantNumeric: 'tabular-nums' as const,
                textShadow: `0 0 8px ${color}44`,
                minWidth: 70 * scale,
                textAlign: 'right',
                position: 'relative',
                zIndex: 1,
              }}
            >
              {isPositive ? '+' : ''}{sector.net.toFixed(1)}亿 {arrow}
            </span>
          </div>
        );
      })}
    </div>
  );
};

export const RankingPanel: React.FC<RankingPanelProps> = ({
  sectors,
  frame,
  totalFrames,
  highlightId,
  width = 1080,
  height = 1920,
  format = 'mobile',
}) => {
  const isTV = format === 'tv';
  const scale = isTV ? 1.1 : 1.5;

  const panelLeft = isTV ? width * 0.78 : width * 0.70;
  const panelTop = isTV ? 110 : 180;
  const panelWidth = isTV ? width * 0.18 : width * 0.28;

  const inflowSectors = sectors.filter((s) => s.net >= 0).sort((a, b) => b.net - a.net);
  const outflowSectors = sectors.filter((s) => s.net < 0).sort((a, b) => a.net - b.net);

  const progress = frame / totalFrames;
  const inflowVisible = Math.min(
    inflowSectors.length,
    Math.floor(progress * inflowSectors.length * 1.6),
  );
  const outflowVisible = Math.min(
    outflowSectors.length,
    Math.floor(progress * outflowSectors.length * 1.6),
  );

  return (
    <div
      style={{
        position: 'absolute',
        left: panelLeft,
        top: panelTop,
        width: panelWidth,
        zIndex: 10,
        fontFamily: '"PingFang SC", "Helvetica Neue", sans-serif',
      }}
    >
      <div
        style={{
          fontSize: 18 * scale,
          color: '#8899aa',
          fontWeight: 600,
          letterSpacing: 2,
          marginBottom: 12 * scale,
          paddingBottom: 6 * scale,
          borderBottom: '1px solid #2a3550',
        }}
      >
        资金排行榜
      </div>

      {inflowSectors.length > 0 && (
        <RankingGroup
          title="净流入 TOP"
          sectors={inflowSectors}
          isPositive={true}
          visibleCount={inflowVisible}
          highlightId={highlightId}
          frame={frame}
          scale={scale}
        />
      )}

      {outflowSectors.length > 0 && (
        <RankingGroup
          title="净流出 TOP"
          sectors={outflowSectors}
          isPositive={false}
          visibleCount={outflowVisible}
          highlightId={highlightId}
          frame={frame}
          scale={scale}
        />
      )}
    </div>
  );
};
