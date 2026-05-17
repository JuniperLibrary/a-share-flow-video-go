import React from 'react';
import { AbsoluteFill, interpolate, useCurrentFrame } from 'remotion';
import type { MultiDayAnalysis } from './types.ts';

interface MarketEventFeedProps {
  analysis: MultiDayAnalysis;
  frame: number;
  totalFrames: number;
  width: number;
  height: number;
  currentDate: string;
}

export const MarketEventFeed: React.FC<MarketEventFeedProps> = ({
  analysis,
  frame,
  totalFrames,
  width,
  height,
  currentDate,
}) => {
  // Combine trend insights and ranking changes into a unified feed
  const feedItems = React.useMemo(() => {
    const items: { time: string; title: string; desc: string; sentiment: string; type: string }[] = [];

    // Add trend insights
    (analysis.trendInsights || []).forEach(insight => {
      items.push({
        time: insight.day?.slice(5) || '',
        title: insight.title || '',
        desc: insight.description || '',
        sentiment: insight.sentiment || 'neutral',
        type: 'trend',
      });
    });

    // Add ranking changes
    (analysis.rankingChanges || []).forEach(change => {
      items.push({
        time: change.to_day?.slice(5) || '',
        title: `${change.sector} 排名变化`,
        desc: change.description || '',
        sentiment: change.sentiment || 'neutral',
        type: 'ranking',
      });
    });

    // Add ticker items
    (analysis.tickerItems || []).forEach(ticker => {
      items.push({
        time: ticker.day?.slice(5) || '',
        title: ticker.text || '',
        desc: '',
        sentiment: ticker.text?.includes('流入') || ticker.text?.includes('净流入') ? 'positive' : 'negative',
        type: 'ticker',
      });
    });

    return items;
  }, [analysis]);

  // Determine which items to show based on frame
  const raceStart = 60;
  const raceEnd = 839;
  const raceProgress = Math.max(0, Math.min(1, (frame - raceStart) / (raceEnd - raceStart)));
  const visibleCount = Math.floor(raceProgress * Math.min(feedItems.length, 6));
  const visibleItems = feedItems.slice(0, visibleCount);

  // Sentiment color
  const getSentimentColor = (sentiment: string) => {
    switch (sentiment) {
      case 'positive': return '#00F0FF';
      case 'negative': return '#FF6B8A';
      default: return '#8892a4';
    }
  };

  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'trend': return '◆';
      case 'ranking': return '▲';
      case 'ticker': return '●';
      default: return '•';
    }
  };

  return (
    <div style={{
      position: 'absolute',
      bottom: 0,
      left: 0,
      right: 0,
      height: height * 0.30,
      borderTop: '1px solid rgba(0, 240, 255, 0.15)',
      background: 'linear-gradient(180deg, rgba(10, 14, 23, 0.95) 0%, rgba(10, 14, 23, 0.98) 100%)',
      overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        padding: '8px 20px',
        borderBottom: '1px solid rgba(74, 144, 217, 0.1)',
      }}>
        <div style={{
          fontSize: 13,
          fontWeight: 700,
          color: '#00F0FF',
          letterSpacing: 2,
          textTransform: 'uppercase',
        }}>
          MARKET EVENT FEED
        </div>
        <div style={{ flex: 1 }} />
        <div style={{
          fontSize: 12,
          color: '#5a6577',
          fontFamily: 'monospace',
        }}>
          {currentDate}
        </div>
      </div>

      {/* Feed items */}
      <div style={{
        padding: '8px 20px',
        display: 'flex',
        flexDirection: 'column',
        gap: 6,
        overflow: 'hidden',
      }}>
        {visibleItems.map((item, idx) => {
          const itemOpacity = interpolate(frame, [raceStart + idx * 30, raceStart + idx * 30 + 20], [0, 1], { extrapolateRight: 'clamp' });
          const slideX = interpolate(frame, [raceStart + idx * 30, raceStart + idx * 30 + 20], [-20, 0], { extrapolateRight: 'clamp' });
          const color = getSentimentColor(item.sentiment);

          return (
            <div key={idx} style={{
              display: 'flex',
              alignItems: 'flex-start',
              gap: 10,
              opacity: itemOpacity,
              transform: `translateX(${slideX}px)`,
              transition: 'opacity 0.3s ease, transform 0.3s ease',
            }}>
              {/* Time */}
              <div style={{
                fontSize: 13,
                fontFamily: 'monospace',
                color: '#5a6577',
                minWidth: 36,
                paddingTop: 1,
              }}>
                {item.time}
              </div>

              {/* Icon */}
              <div style={{
                fontSize: 10,
                color: color,
                paddingTop: 3,
                textShadow: `0 0 6px ${color}40`,
              }}>
                {getTypeIcon(item.type)}
              </div>

              {/* Content */}
              <div style={{ flex: 1 }}>
                <div style={{
                  fontSize: 15,
                  fontWeight: 600,
                  color: '#e0e4e8',
                  marginBottom: 2,
                }}>
                  {item.title}
                </div>
                {item.desc && (
                  <div style={{
                    fontSize: 13,
                    color: '#7a8597',
                    lineHeight: 1.4,
                  }}>
                    {item.desc}
                  </div>
                )}
              </div>

              {/* Sentiment indicator */}
              <div style={{
                width: 3,
                height: 24,
                borderRadius: 2,
                backgroundColor: color,
                opacity: 0.6,
                boxShadow: `0 0 8px ${color}30`,
                marginTop: 2,
              }} />
            </div>
          );
        })}

        {visibleItems.length === 0 && (
          <div style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            height: '100%',
            color: '#3a4557',
            fontSize: 14,
            fontFamily: 'monospace',
            letterSpacing: 1,
          }}>
            AWAITING MARKET DATA...
          </div>
        )}
      </div>
    </div>
  );
};
