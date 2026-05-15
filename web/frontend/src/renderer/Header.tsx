import React from 'react';

interface HeaderProps {
  displayDate: string;
  frame?: number;
  totalFrames?: number;
  sentiment?: 'bullish' | 'bearish' | 'neutral';
  width?: number;
  height?: number;
  format?: 'mobile' | 'tv';
}

export const Header: React.FC<HeaderProps> = ({
  displayDate,
  frame = 0,
  totalFrames = 900,
  sentiment = 'neutral',
  width = 1080,
  height = 1920,
  format = 'mobile',
}) => {
  const isTV = format === 'tv';
  const scale = isTV ? 1.2 : 1.55;

  const progress = frame / totalFrames;
  const mins = Math.round(progress * 330);
  const hours = Math.floor(9.5 + mins / 60);
  const minutes = mins % 60;
  const seconds = Math.round((progress * 330 * 60) % 60);
  const timeStr = `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;

  const getMarketPhase = () => {
    if (mins < 15) return { text: 'OPEN', color: '#fbbf24' };
    if (mins < 60) return { text: 'AM SESSION', color: '#4ade80' };
    if (mins < 120) return { text: 'AM CLOSE', color: '#4a80d0' };
    if (mins < 150) return { text: 'BREAK', color: '#8899aa' };
    if (mins < 240) return { text: 'PM SESSION', color: '#4ade80' };
    if (mins < 315) return { text: 'CLOSING', color: '#f87171' };
    return { text: 'CLOSE', color: '#fbbf24' };
  };

  const phase = getMarketPhase();
  const sentimentColor = sentiment === 'bullish' ? '#4ade80' : sentiment === 'bearish' ? '#f87171' : '#8899aa';
  const livePulse = Math.sin(frame * 0.15) * 0.4 + 0.6;
  const blinkOpacity = Math.sin(frame * 0.3) > 0.3 ? 1 : 0.3;

  const titleColors = ['#f87171', '#fb923c', '#fbbf24', '#4ade80', '#2dd4bf'];
  const titleChars = ['板', '块', '情', '绪', '流'];

  return (
    <div style={{ position: 'absolute', top: 0, left: 0, width, zIndex: 10 }}>
      {/* Top bar */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: `${12 * scale}px ${24 * scale}px`,
          background: 'linear-gradient(180deg, rgba(10,15,24,0.95) 0%, rgba(10,15,24,0) 100%)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 * scale }}>
          <div
            style={{
              padding: `${2 * scale}px ${10 * scale}px`,
              borderRadius: 3,
              backgroundColor: `${sentimentColor}12`,
              border: `1px solid ${sentimentColor}33`,
              fontSize: 13 * scale,
              color: sentimentColor,
              fontWeight: 600,
              letterSpacing: 1,
              fontFamily: '"PingFang SC", "Helvetica Neue", sans-serif',
            }}
          >
            板块
          </div>

          <div
            style={{
              color: '#8899aa',
              fontSize: 22 * scale,
              fontWeight: 300,
              letterSpacing: 3,
              fontFamily: '"Helvetica Neue", Arial, sans-serif',
              opacity: 0.8,
            }}
          >
            {displayDate}
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 14 * scale }}>
          <div
            style={{
              color: '#4a80d0',
              fontSize: 20 * scale,
              fontWeight: 500,
              fontFamily: '"Helvetica Neue", "SF Mono", monospace',
              letterSpacing: 2,
            }}
          >
            {timeStr}
          </div>

          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6 * scale,
              padding: `${3 * scale}px ${10 * scale}px`,
              borderRadius: 4,
              backgroundColor: '#0f1922',
              border: '1px solid #1e3a5f',
            }}
          >
            <div
              style={{
                width: 6 * scale,
                height: 6 * scale,
                borderRadius: '50%',
                backgroundColor: sentimentColor,
                boxShadow: `0 0 ${6 + livePulse * 6}px ${sentimentColor}`,
                opacity: livePulse,
              }}
            />
            <span style={{ color: sentimentColor, fontSize: 11 * scale, fontWeight: 600, letterSpacing: 2 }}>
              实时
            </span>
          </div>
        </div>
      </div>

      {/* Title area */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: `${4 * scale}px 0 ${2 * scale}px 0`,
          background: 'linear-gradient(180deg, rgba(10,15,24,0.5) 0%, transparent 100%)',
          flexDirection: 'column',
          gap: 2 * scale,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          {titleChars.map((char, i) => (
            <span
              key={i}
              style={{
                fontSize: 36 * scale,
                fontWeight: 700,
                letterSpacing: 4,
                fontFamily: '"PingFang SC", "Helvetica Neue", sans-serif',
                color: titleColors[i],
                textShadow: `0 0 20px ${titleColors[i]}44, 0 2px 8px rgba(0,0,0,0.5)`,
              }}
            >
              {char}
            </span>
          ))}
          <span
            style={{
              fontSize: 15 * scale,
              fontWeight: 400,
              color: '#556677',
              marginLeft: 12 * scale,
              letterSpacing: 2,
              fontFamily: '"Helvetica Neue", Arial, sans-serif',
            }}
          >
            主力资金净流入（亿）
          </span>
        </div>
      </div>

      {/* Separator line */}
      <div style={{ height: 1, background: 'linear-gradient(90deg, transparent, #2a3550, transparent)', margin: `0 ${40 * scale}px` }} />
    </div>
  );
};
