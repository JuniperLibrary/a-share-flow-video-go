import React, { useMemo } from 'react';
import type { TimelineEvent as TimelineEventType } from './types.ts';

interface TimelineProps {
  timelineEvents?: TimelineEventType[];
  frame: number;
  totalFrames: number;
  width?: number;
  height?: number;
  format?: 'mobile' | 'tv';
}

const SENTIMENT_COLORS: Record<string, string> = {
  positive: '#4ade80',
  negative: '#f87171',
  neutral: '#60a5fa',
};

export const Timeline: React.FC<TimelineProps> = ({
  timelineEvents,
  frame,
  totalFrames,
  width = 1080,
  height = 1920,
  format = 'mobile',
}) => {
  const isTV = format === 'tv';
  const scale = isTV ? 1.0 : 1.05;

  const timelineLeft = isTV ? width * 0.64 : width * 0.50;
  const timelineTop = isTV ? 110 : 170;
  const timelineWidth = isTV ? width * 0.12 : 170;
  const timelineBottom = isTV ? height * 0.78 : height * 0.82;

  const events = useMemo(() => {
    return (timelineEvents && timelineEvents.length > 0) ? timelineEvents.slice(0, 12) : [];
  }, [timelineEvents]);

  const progress = frame / totalFrames;

  const visibleEvents = useMemo(() => {
    return events.filter(ev => {
      const eventProgress = ev.timeMinutes / 330;
      return eventProgress <= progress + 0.02 && eventProgress > 0;
    });
  }, [events, progress]);

  const latestEventTime = useMemo(() => {
    if (visibleEvents.length === 0) return '';
    return visibleEvents[visibleEvents.length - 1].time;
  }, [visibleEvents]);

  return (
    <div
      style={{
        position: 'absolute',
        left: timelineLeft,
        top: timelineTop,
        width: timelineWidth,
        bottom: height - timelineBottom,
        zIndex: 10,
        fontFamily: '"PingFang SC", "Helvetica Neue", sans-serif',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          fontSize: 15 * scale,
          color: '#8899aa',
          fontWeight: 600,
          letterSpacing: 2,
          marginBottom: 12 * scale,
          paddingBottom: 8 * scale,
          borderBottom: '1px solid #1e2d45',
          display: 'flex',
          alignItems: 'center',
          gap: 6 * scale,
        }}
      >
        市场事件
        <span style={{ fontSize: 12 * scale, color: '#4a5568', fontWeight: 400 }}>ⓘ</span>
      </div>

      <div style={{ position: 'relative' }}>
        <div
          style={{
            position: 'absolute',
            left: 6 * scale,
            top: 0,
            bottom: 0,
            width: 1,
            background: 'linear-gradient(180deg, #1e2d45, #2a3550, #1e2d45)',
          }}
        />

        {visibleEvents.map((ev, idx) => {
          const eventProgress = ev.timeMinutes / 330;
          const fadeStart = progress - 0.06;
          const opacity = progress > fadeStart ? Math.min(1, (progress - fadeStart) / 0.04) * 0.95 : 0;
          const color = SENTIMENT_COLORS[ev.sentiment] || '#60a5fa';
          const isLatest = ev.time === latestEventTime;
          const isHighlighted = isLatest && ev.sentiment === 'positive';

          if (opacity <= 0) return null;

          return (
            <div
              key={`event-${idx}`}
              style={{
                display: 'flex',
                gap: 10 * scale,
                marginBottom: 14 * scale,
                opacity,
                position: 'relative',
              }}
            >
              <div
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                  flexShrink: 0,
                  paddingTop: 3 * scale,
                }}
              >
                <div
                  style={{
                    width: isHighlighted ? 10 * scale : 7 * scale,
                    height: isHighlighted ? 10 * scale : 7 * scale,
                    borderRadius: '50%',
                    backgroundColor: color,
                    boxShadow: isHighlighted
                      ? `0 0 ${12 * scale}px ${color}, 0 0 ${24 * scale}px ${color}66`
                      : `0 0 ${4 * scale}px ${color}66`,
                  }}
                />
              </div>

              <div style={{ flex: 1, minWidth: 0 }}>
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'baseline',
                    gap: 6 * scale,
                    marginBottom: 2 * scale,
                  }}
                >
                  <span
                    style={{
                      fontSize: 13 * scale,
                      fontWeight: 700,
                      color: '#ddeeff',
                      fontFamily: '"Helvetica Neue", monospace',
                      letterSpacing: 0.5,
                    }}
                  >
                    {ev.time}
                  </span>
                  <span
                    style={{
                      fontSize: 12 * scale,
                      fontWeight: isHighlighted ? 700 : 600,
                      color: isHighlighted ? '#ffffff' : '#c8d6e5',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {ev.title}
                  </span>
                </div>
                  <div
                    style={{
                      fontSize: 10 * scale,
                      color: '#6b7a8d',
                      fontWeight: 400,
                      lineHeight: 1.3,
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                  {ev.description}
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};
