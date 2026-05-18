import React from 'react';
import { AbsoluteFill, useCurrentFrame, useVideoConfig, getInputProps } from 'remotion';
import { Background } from './Background.tsx';
import { Header } from './Header.tsx';
import { Timeline } from './Timeline.tsx';
import { RankingPanel } from './RankingPanel.tsx';
import { Ticker } from './Ticker.tsx';
import { Particles } from './Particles.tsx';
import { Disclaimer } from './Disclaimer.tsx';
import { TickChart } from './TickChart.tsx';
import type { BloombergVideoProps, TimelineEvent, TickerItem, SectorTick } from './types.ts';

export const BloombergVideoTick: React.FC = () => {
  const frame = useCurrentFrame();
  const { durationInFrames, width, height } = useVideoConfig();
  const inputProps = (getInputProps() ?? {}) as unknown as BloombergVideoProps & { sectorTicks?: SectorTick[] };

  const dateStr = inputProps.dateStr || '2026-05-11';
  const displayDate = inputProps.displayDate || '05-11';
  const sectorTicks = inputProps.sectorTicks || [];
  const totalFrames = inputProps.totalFrames || durationInFrames;
  const events = inputProps.events;
  const timelineEvents = inputProps.timelineEvents || [];
  const tickerItems = inputProps.tickerItems || [];
  const format = inputProps.format || 'mobile';
  const isTV = format === 'tv';
  const session = inputProps.session || 'full';
  const xLim = inputProps.xLim || [0, 330];

  const sentiment = React.useMemo(() => {
    if (sectorTicks.length === 0) return 'neutral' as const;
    const lastValues = sectorTicks.map(s => s.data[s.data.length - 1] || 0);
    const negativeRatio = lastValues.filter(v => v < 0).length / lastValues.length;
    if (negativeRatio > 0.65) return 'bearish' as const;
    if (negativeRatio < 0.35) return 'bullish' as const;
    return 'neutral' as const;
  }, [sectorTicks]);

  const progress = frame / totalFrames;
  const activeEventSector = React.useMemo(() => {
    if (!events || events.length === 0) return null;
    const eventFrame = Math.round(progress * 100);
    for (const ev of events) {
      if (Math.abs(ev.frame - eventFrame) < 5) {
        return ev.sector || null;
      }
    }
    return null;
  }, [events, progress]);

  const sectorsForRanking = React.useMemo(() => {
    return sectorTicks.map(s => ({
      name: s.name,
      net: s.data.reduce((a, b) => a + b, 0),
      color: s.color || '#888888',
    }));
  }, [sectorTicks]);

  return (
    <AbsoluteFill>
      <Background frame={frame} totalFrames={totalFrames} sentiment={sentiment} width={width} height={height} format={format} />
      <Header displayDate={displayDate} frame={frame} totalFrames={totalFrames} sentiment={sentiment} width={width} height={height} format={format} />
      <Particles frame={frame} width={width} height={height} />

      <TickChart
        sectorTicks={sectorTicks}
        frame={frame}
        totalFrames={totalFrames}
        activeEventSector={activeEventSector}
        width={width}
        height={height}
        format={format}
        sentiment={sentiment}
        session={session}
        xLim={xLim}
      />

      <Timeline
        timelineEvents={timelineEvents}
        frame={frame}
        totalFrames={totalFrames}
        width={width}
        height={height}
        format={format}
        session={session}
        xLim={xLim}
      />

      <RankingPanel
        sectors={sectorsForRanking}
        frame={frame}
        totalFrames={totalFrames}
        highlightId={sectorsForRanking[0]?.name}
        width={width}
        height={height}
        format={format}
      />

      <Ticker
        tickerItems={tickerItems}
        frame={frame}
        totalFrames={totalFrames}
        width={width}
        height={height}
        format={format}
      />

      <Disclaimer
        frame={frame}
        totalFrames={totalFrames}
        width={width}
        height={height}
        format={format}
      />
    </AbsoluteFill>
  );
};
