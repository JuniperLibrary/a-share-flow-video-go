import React from 'react';
import { AbsoluteFill, useCurrentFrame, useVideoConfig, getInputProps, interpolate } from 'remotion';
import { BarRaceChart } from './BarChartRace.tsx';
import { MarketEventFeed } from './MarketEventFeed.tsx';
import { FinancialBackground } from './FinancialBackground.tsx';
import type { MultiDayVideoProps } from './types.ts';

export const MultiDayVideo: React.FC = () => {
  const frame = useCurrentFrame();
  const { durationInFrames, width, height } = useVideoConfig();
  const inputProps = (getInputProps() ?? {}) as unknown as MultiDayVideoProps;

  const dates = inputProps.dates || [];
  const snapshots = inputProps.snapshots || [];
  const analysis = inputProps.analysis || { trendInsights: [], rankingChanges: [], summaryText: { title: '', content: '', key_sectors: [] }, tickerItems: [] };
  const totalFrames = inputProps.totalFrames || durationInFrames;
  const format = inputProps.format || 'mobile';

  // Frame phases
  const showCover = frame < 60;
  const showRace = frame >= 60 && frame < 840;
  const showSummary = frame >= 840;

  // Cover animations
  const coverOpacity = frame < 30 ? interpolate(frame, [0, 30], [0, 1]) : frame > 50 ? interpolate(frame, [50, 60], [1, 0]) : 1;
  const summaryOpacity = showSummary ? interpolate(frame, [840, 860], [0, 1]) : 0;

  const dateRange = dates.length > 0 ? `${dates[0]} → ${dates[dates.length - 1]}` : '';

  // Sentiment for background
  const inflowCount = (snapshots[0]?.bars || []).filter(s => s.net > 0).length;
  const totalSectors = (snapshots[0]?.bars || []).length;
  const sentiment = inflowCount > totalSectors * 0.6 ? 'bullish' : inflowCount < totalSectors * 0.4 ? 'bearish' : 'neutral';

  return (
    <AbsoluteFill style={{ backgroundColor: '#0a0e17' }}>
      {/* Cover */}
      {showCover && (
        <AbsoluteFill style={{
          opacity: coverOpacity,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          background: 'linear-gradient(180deg, #0a0e17 0%, #0d1220 50%, #0a0e17 100%)',
        }}>
          {/* HUD corner markers */}
          <div style={{ position: 'absolute', top: 20, left: 20, width: 40, height: 40, borderTop: '2px solid rgba(0, 240, 255, 0.3)', borderLeft: '2px solid rgba(0, 240, 255, 0.3)' }} />
          <div style={{ position: 'absolute', top: 20, right: 20, width: 40, height: 40, borderTop: '2px solid rgba(0, 240, 255, 0.3)', borderRight: '2px solid rgba(0, 240, 255, 0.3)' }} />
          <div style={{ position: 'absolute', bottom: 20, left: 20, width: 40, height: 40, borderBottom: '2px solid rgba(0, 240, 255, 0.3)', borderLeft: '2px solid rgba(0, 240, 255, 0.3)' }} />
          <div style={{ position: 'absolute', bottom: 20, right: 20, width: 40, height: 40, borderBottom: '2px solid rgba(0, 240, 255, 0.3)', borderRight: '2px solid rgba(0, 240, 255, 0.3)' }} />

          {/* Title */}
          <div style={{
            fontSize: 14,
            fontWeight: 600,
            color: '#4a5568',
            letterSpacing: 4,
            textTransform: 'uppercase',
            marginBottom: 20,
            fontFamily: 'monospace',
          }}>
            A股板块资金流动态监控系统
          </div>

          <div style={{
            fontSize: 42,
            fontWeight: 800,
            color: '#00F0FF',
            marginBottom: 12,
            textShadow: '0 0 30px rgba(0, 240, 255, 0.3)',
            textAlign: 'center',
            letterSpacing: 2,
          }}>
            近{dates.length}日资金流向
          </div>

          <div style={{
            fontSize: 18,
            color: '#8892a4',
            marginBottom: 30,
            fontFamily: 'monospace',
            letterSpacing: 3,
          }}>
            BAR CHART RACE
          </div>

          {/* Date range */}
          {dateRange && (
            <div style={{
              fontSize: 16,
              color: '#5a6577',
              fontFamily: 'monospace',
              padding: '8px 24px',
              border: '1px solid rgba(74, 144, 217, 0.2)',
              borderRadius: 4,
              backgroundColor: 'rgba(10, 14, 23, 0.6)',
              letterSpacing: 2,
            }}>
              {dateRange}
            </div>
          )}

          {/* Subtitle */}
          <div style={{
            marginTop: 40,
            fontSize: 11,
            color: '#3a4557',
            letterSpacing: 2,
            textTransform: 'uppercase',
            fontFamily: 'monospace',
          }}>
            机构终端感 · 金融纪录片 · 市场战争感
          </div>
        </AbsoluteFill>
      )}

      {/* Bar Chart Race + Market Event Feed */}
      {showRace && (
        <>
          {/* Main chart area (70%) */}
          <div style={{ position: 'absolute', top: 0, left: 0, right: 0, height: height * 0.70 }}>
            <BarRaceChart
              snapshots={snapshots}
              analysis={analysis}
              frame={frame}
              totalFrames={totalFrames}
              width={width}
              height={height * 0.70}
            />
          </div>

          {/* Market Event Feed (30%) */}
          <MarketEventFeed
            analysis={analysis}
            frame={frame}
            totalFrames={totalFrames}
            width={width}
            height={height}
            currentDate={snapshots[0]?.date || ''}
          />
        </>
      )}

      {/* Summary */}
      {showSummary && (
        <AbsoluteFill style={{
          opacity: summaryOpacity,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          background: 'linear-gradient(180deg, #0a0e17 0%, #0d1220 50%, #0a0e17 100%)',
          padding: '40px 60px',
        }}>
          {/* HUD corners */}
          <div style={{ position: 'absolute', top: 20, left: 20, width: 30, height: 30, borderTop: '1px solid rgba(0, 240, 255, 0.2)', borderLeft: '1px solid rgba(0, 240, 255, 0.2)' }} />
          <div style={{ position: 'absolute', top: 20, right: 20, width: 30, height: 30, borderTop: '1px solid rgba(0, 240, 255, 0.2)', borderRight: '1px solid rgba(0, 240, 255, 0.2)' }} />
          <div style={{ position: 'absolute', bottom: 20, left: 20, width: 30, height: 30, borderBottom: '1px solid rgba(0, 240, 255, 0.2)', borderLeft: '1px solid rgba(0, 240, 255, 0.2)' }} />
          <div style={{ position: 'absolute', bottom: 20, right: 20, width: 30, height: 30, borderBottom: '1px solid rgba(0, 240, 255, 0.2)', borderRight: '1px solid rgba(0, 240, 255, 0.2)' }} />

          <div style={{
            fontSize: 12,
            fontWeight: 600,
            color: '#4a5568',
            letterSpacing: 3,
            textTransform: 'uppercase',
            marginBottom: 16,
            fontFamily: 'monospace',
          }}>
            MARKET SUMMARY
          </div>

          <div style={{
            fontSize: 32,
            fontWeight: 800,
            color: '#00F0FF',
            marginBottom: 24,
            textShadow: '0 0 20px rgba(0, 240, 255, 0.3)',
            textAlign: 'center',
          }}>
            {analysis.summaryText?.title || '数据总结'}
          </div>

          <div style={{
            fontSize: 18,
            color: '#c0c8d4',
            textAlign: 'center',
            lineHeight: 1.8,
            maxWidth: width * 0.75,
            marginBottom: 36,
          }}>
            {analysis.summaryText?.content || '暂无数据'}
          </div>

          {/* Key sectors */}
          {(analysis.summaryText?.key_sectors || []).length > 0 && (
            <div style={{
              display: 'flex',
              gap: 12,
              flexWrap: 'wrap',
              justifyContent: 'center',
              marginBottom: 40,
            }}>
              {(analysis.summaryText?.key_sectors || []).map((sector: string) => (
                <div key={sector} style={{
                  padding: '6px 16px',
                  backgroundColor: 'rgba(0, 240, 255, 0.08)',
                  border: '1px solid rgba(0, 240, 255, 0.2)',
                  borderRadius: 3,
                  fontSize: 14,
                  color: '#00F0FF',
                  fontFamily: 'monospace',
                  letterSpacing: 1,
                }}>
                  {sector}
                </div>
              ))}
            </div>
          )}

          <div style={{
            fontSize: 11,
            color: '#3a4557',
            letterSpacing: 2,
            fontFamily: 'monospace',
          }}>
            数据仅供参考，不构成投资建议
          </div>
        </AbsoluteFill>
      )}
    </AbsoluteFill>
  );
};
