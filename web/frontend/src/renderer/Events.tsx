import React, { useMemo } from 'react';
import type { SectorData, MarketEvent as MarketEventType } from './types.ts';

interface EventsProps {
  sectors: SectorData[];
  frame: number;
  totalFrames: number;
  events?: MarketEventType[];
  width?: number;
  height?: number;
  format?: 'mobile' | 'tv';
}

function generateDefaultEvents(
  sectors: SectorData[],
  totalFrames: number,
): MarketEventType[] {
  const events: MarketEventType[] = [];

  events.push({
    event_type: 'market',
    frame: Math.round(totalFrames * 0.02),
    text: 'A股 开盘',
    subtext: '主力资金持续流入',
    importance: 2,
  });

  let maxPos = { name: '', net: -Infinity };
  let maxNeg = { name: '', net: Infinity };
  for (const s of sectors) {
    if (s.net > maxPos.net) maxPos = { name: s.name, net: s.net };
    if (s.net < maxNeg.net) maxNeg = { name: s.name, net: s.net };
  }

  if (maxPos.net > 20) {
    events.push({
      event_type: 'sentiment',
      frame: Math.round(totalFrames * 0.35),
      text: `${maxPos.name} 持续走强`,
      subtext: `主力资金净流入 ${maxPos.net.toFixed(1)} 亿，资金加速涌入`,
      importance: 3,
      sector: maxPos.name,
      sentiment: 'positive',
    });
  }

  if (maxNeg.net < -20) {
    events.push({
      event_type: 'aberration',
      frame: Math.round(totalFrames * 0.60),
      text: `${maxNeg.name} 承压调整`,
      subtext: `主力净流出 ${Math.abs(maxNeg.net).toFixed(1)} 亿，资金离场观望`,
      importance: 2,
      sector: maxNeg.name,
      sentiment: 'negative',
    });
  }

  events.push({
    event_type: 'market',
    frame: Math.round(totalFrames * 0.92),
    text: '今日收盘',
    subtext: '板块情绪分化明显',
    importance: 2,
  });

  return events;
}

export const Events: React.FC<EventsProps> = ({
  sectors,
  frame,
  totalFrames,
  events: passedEvents,
  width = 1080,
  height = 1920,
  format = 'mobile',
}) => {
  const isTV = format === 'tv';
  const scale = isTV ? 1.1 : 1;

  const chartLeft = isTV ? 30 : 50;
  const chartRight = isTV ? width * 0.60 : 480;
  const X_MAX = 330;

  const marketEvents = useMemo(() => {
    if (passedEvents && passedEvents.length > 0) {
      return passedEvents.map(ev => ({
        ...ev,
        frame: Math.round(totalFrames * (ev.frame / 100)),
      }));
    }
    return generateDefaultEvents(sectors, totalFrames);
  }, [passedEvents, sectors, totalFrames]);

  const FADE_IN = 10;
  const HOLD = 80;
  const FADE_OUT = 10;

  const activeEvent = marketEvents.find((ev) => {
    const start = ev.frame;
    const end = ev.frame + FADE_IN + HOLD + FADE_OUT;
    return frame >= start && frame < end;
  });

  if (!activeEvent) return null;

  const localFrame = frame - activeEvent.frame;
  let opacity = 0;
  let translateY = 30;

  if (localFrame < FADE_IN) {
    opacity = localFrame / FADE_IN;
    translateY = 30 - (localFrame / FADE_IN) * 30;
  } else if (localFrame < FADE_IN + HOLD) {
    opacity = 1;
    translateY = 0;
  } else if (localFrame < FADE_IN + HOLD + FADE_OUT) {
    const fadeProgress = (localFrame - FADE_IN - HOLD) / FADE_OUT;
    opacity = 1 - fadeProgress;
    translateY = 0 - fadeProgress * 10;
  }

  const importance = activeEvent.importance || 2;
  const accentColors = ['#4a80d0', '#e6a23c', '#f56c6c'];
  const accentColor = accentColors[Math.min(importance - 1, 2)];

  const progress = frame / totalFrames;
  const currentTimeMinutes = progress * X_MAX;
  const eventX = chartLeft + (currentTimeMinutes / X_MAX) * (chartRight - chartLeft);

  const bottomPos = isTV ? 20 : 40;

  return (
    <div
      style={{
        position: 'absolute',
        left: eventX - 80,
        bottom: bottomPos,
        zIndex: 20,
        opacity,
        transform: `translateY(${translateY}px)`,
        fontFamily: '"Helvetica Neue", "PingFang SC", sans-serif',
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: -4,
          width: 3,
          height: 48,
          background: `linear-gradient(180deg, ${accentColor}, transparent)`,
          borderRadius: 2,
        }}
      />

      {activeEvent.importance === 3 && (
        <div
          style={{
            position: 'absolute',
            left: -6,
            top: -6,
            right: -6,
            bottom: -6,
            borderRadius: 8,
            background: `radial-gradient(circle at 30% 50%, ${accentColor}18, transparent 70%)`,
            pointerEvents: 'none',
          }}
        />
      )}

      <div
        style={{
          marginLeft: 16,
          padding: `${12 * scale}px ${20 * scale}px`,
          background: 'rgba(8, 14, 22, 0.92)',
          borderLeft: `3px solid ${accentColor}`,
          borderRadius: '0 8px 8px 0',
          backdropFilter: 'blur(12px)',
          border: `1px solid ${accentColor}18`,
        }}
      >
        <div
          style={{
            fontSize: importance === 3 ? 26 * scale : 22 * scale,
            fontWeight: 700,
            color: '#ddeeff',
            letterSpacing: 2,
            marginBottom: 4 * scale,
          }}
        >
          {activeEvent.text}
        </div>
        {activeEvent.subtext && (
          <div style={{ fontSize: 14 * scale, color: '#8899aa', fontWeight: 400 }}>
            {activeEvent.subtext}
          </div>
        )}
      </div>

      {activeEvent.importance === 3 && (
        <div
          style={{
            position: 'absolute',
            left: 0,
            right: 0,
            bottom: -4,
            height: 8,
            background: `linear-gradient(180deg, ${accentColor}11, transparent)`,
            borderRadius: '50%',
            filter: 'blur(8px)',
            pointerEvents: 'none',
          }}
        />
      )}
    </div>
  );
};
