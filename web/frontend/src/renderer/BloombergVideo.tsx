import React from 'react';
import { AbsoluteFill, useCurrentFrame, useVideoConfig, getInputProps } from 'remotion';
import { Background } from './Background.tsx';
import { Header } from './Header.tsx';
import { Chart } from './Chart.tsx';
import { Timeline } from './Timeline.tsx';
import { RankingPanel } from './RankingPanel.tsx';
import { Ticker } from './Ticker.tsx';
import { Particles } from './Particles.tsx';
import type { BloombergVideoProps, TimelineEvent, TickerItem, SectorData } from './types.ts';

export const BloombergVideo: React.FC = () => {
  const frame = useCurrentFrame();
  const { durationInFrames, width, height } = useVideoConfig();
  const inputProps = (getInputProps() ?? {}) as unknown as BloombergVideoProps;

  const dateStr = inputProps.dateStr || '2026-05-11';
  const displayDate = inputProps.displayDate || '05-11';
  const typeLabel = inputProps.typeLabel || '行业板块';
  const sectors = inputProps.sectors || [];
  const totalFrames = inputProps.totalFrames || durationInFrames;
  const events = inputProps.events;
  const timelineEvents = inputProps.timelineEvents || [];
  const tickerItems = inputProps.tickerItems || [];
  const format = inputProps.format || 'mobile';
  const isTV = format === 'tv';

  const mainLineId = React.useMemo(() => {
    if (sectors.length === 0) return null;
    let maxAbs = 0;
    let main = sectors[0].name;
    for (const s of sectors) {
      if (Math.abs(s.net) > maxAbs) {
        maxAbs = Math.abs(s.net);
        main = s.name;
      }
    }
    return main;
  }, [sectors]);

  const sentiment = React.useMemo(() => {
    if (sectors.length === 0) return 'neutral' as const;
    const negativeRatio = sectors.filter(s => s.net < 0).length / sectors.length;
    if (negativeRatio > 0.65) return 'bearish' as const;
    if (negativeRatio < 0.35) return 'bullish' as const;
    return 'neutral' as const;
  }, [sectors]);

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

  return (
    <AbsoluteFill>
      <Background frame={frame} totalFrames={totalFrames} sentiment={sentiment} width={width} height={height} format={format} />
      <Header displayDate={displayDate} typeLabel={typeLabel} frame={frame} totalFrames={totalFrames} sentiment={sentiment} width={width} height={height} format={format} />
      <Particles frame={frame} width={width} height={height} />

      <Chart
        sectors={sectors}
        mainLineId={mainLineId}
        frame={frame}
        totalFrames={totalFrames}
        activeEventSector={activeEventSector}
        width={width}
        height={height}
        format={format}
        sentiment={sentiment}
      />

      <Timeline
        timelineEvents={timelineEvents}
        frame={frame}
        totalFrames={totalFrames}
        width={width}
        height={height}
        format={format}
      />

      <RankingPanel
        sectors={sectors}
        frame={frame}
        totalFrames={totalFrames}
        highlightId={mainLineId || undefined}
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
    </AbsoluteFill>
  );
};
