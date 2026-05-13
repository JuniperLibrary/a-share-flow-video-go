import React from 'react';
import { interpolate } from 'remotion';

interface BackgroundProps {
  frame: number;
  totalFrames: number;
  sentiment?: 'bullish' | 'bearish' | 'neutral';
  width?: number;
  height?: number;
  format?: 'mobile' | 'tv';
}

const GridOverlay: React.FC<{ width: number; height: number }> = ({ width, height }) => {
  const lines: React.ReactNode[] = [];
  const spacing = 80;

  for (let y = 0; y <= height; y += spacing) {
    lines.push(
      <line
        key={`h${y}`}
        x1={0}
        y1={y}
        x2={width}
        y2={y}
        stroke="#2a3550"
        strokeWidth={0.5}
        opacity={y === 0 || y >= height - 1 ? 0.15 : 0.06}
      />,
    );
  }

  for (let x = 0; x <= width; x += spacing) {
    lines.push(
      <line
        key={`v${x}`}
        x1={x}
        y1={0}
        x2={x}
        y2={height}
        stroke="#2a3550"
        strokeWidth={0.5}
        opacity={x === 0 || x >= width - 1 ? 0.15 : 0.06}
      />,
    );
  }

  return (
    <svg width={width} height={height} style={{ position: 'absolute', top: 0, left: 0, zIndex: 1 }}>
      {lines}
    </svg>
  );
};

const ScanLine: React.FC<{ frame: number; width: number; height: number }> = ({ frame, width, height }) => {
  const scanY = (frame * 4) % (height + 80);
  const scanOpacity = 0.04 + Math.sin(frame * 0.1) * 0.02;

  return (
    <svg width={width} height={height} style={{ position: 'absolute', top: 0, left: 0, zIndex: 3, pointerEvents: 'none' }}>
      <defs>
        <linearGradient id="scanGrad" x1="0%" y1="0%" x2="0%" y2="100%">
          <stop offset="0%" stopColor="#4a80d0" stopOpacity={0} />
          <stop offset="40%" stopColor="#4a80d0" stopOpacity={scanOpacity} />
          <stop offset="60%" stopColor="#4a80d0" stopOpacity={scanOpacity} />
          <stop offset="100%" stopColor="#4a80d0" stopOpacity={0} />
        </linearGradient>
      </defs>
      <rect x={0} y={scanY - 60} width={width} height={120} fill="url(#scanGrad)" />
    </svg>
  );
};

const RadarScan: React.FC<BackgroundProps> = ({ frame, totalFrames, width = 1080, height = 1920 }) => {
  const cycleFrames = totalFrames * 1.5;
  const progress = (frame % cycleFrames) / cycleFrames;
  const sweepAngle = interpolate(progress, [0, 1], [5, 100], { extrapolateRight: 'clamp' });

  const cx = width / 2;
  const cy = height;
  const radius = Math.max(width, height) * 1.2;
  const startAngleDeg = -180;
  const endAngleDeg = startAngleDeg + sweepAngle;
  const rad = (deg: number) => (deg * Math.PI) / 180;

  const x1 = cx + radius * Math.cos(rad(startAngleDeg));
  const y1 = cy + radius * Math.sin(rad(startAngleDeg));
  const xe = cx + radius * Math.cos(rad(endAngleDeg));
  const ye = cy + radius * Math.sin(rad(endAngleDeg));
  const largeArc = sweepAngle > 180 ? 1 : 0;

  return (
    <svg width={width} height={height} style={{ position: 'absolute', top: 0, left: 0, zIndex: 1 }}>
      <defs>
        <linearGradient id="radarGrad" x1="0" y1="1" x2="0" y2="0">
          <stop offset="0%" stopColor="#4a80d0" stopOpacity={0.06} />
          <stop offset="100%" stopColor="#4a80d0" stopOpacity={0} />
        </linearGradient>
      </defs>
      <path
        d={`M ${x1} ${y1} A ${radius} ${radius} 0 ${largeArc} 1 ${xe} ${ye} L ${cx} ${cy} Z`}
        fill="url(#radarGrad)"
        opacity={0.5}
      />
      <path
        d={`M ${x1} ${y1} A ${radius} ${radius} 0 ${largeArc} 1 ${xe} ${ye}`}
        fill="none"
        stroke="#4a80d0"
        strokeWidth={1.5}
        opacity={0.25}
      />
      <line x1={cx} y1={cy} x2={xe} y2={ye} stroke="#4a80d0" strokeWidth={1} opacity={0.15} />
    </svg>
  );
};

const CornerBrackets: React.FC<{ width: number; height: number }> = ({ width, height }) => {
  const s = 45;
  const m = 20;
  return (
    <svg width={width} height={height} style={{ position: 'absolute', top: 0, left: 0, zIndex: 4, pointerEvents: 'none' }}>
      <line x1={m} y1={m} x2={m + s} y2={m} stroke="#3a5570" strokeWidth={1} opacity={0.5} />
      <line x1={m} y1={m} x2={m} y2={m + s} stroke="#3a5570" strokeWidth={1} opacity={0.5} />
      <line x1={width - m} y1={m} x2={width - m - s} y2={m} stroke="#3a5570" strokeWidth={1} opacity={0.5} />
      <line x1={width - m} y1={m} x2={width - m} y2={m + s} stroke="#3a5570" strokeWidth={1} opacity={0.5} />
      <line x1={m} y1={height - m} x2={m + s} y2={height - m} stroke="#3a5570" strokeWidth={1} opacity={0.5} />
      <line x1={m} y1={height - m} x2={m} y2={height - m - s} stroke="#3a5570" strokeWidth={1} opacity={0.5} />
      <line x1={width - m} y1={height - m} x2={width - m - s} y2={height - m} stroke="#3a5570" strokeWidth={1} opacity={0.5} />
      <line x1={width - m} y1={height - m} x2={width - m} y2={height - m - s} stroke="#3a5570" strokeWidth={1} opacity={0.5} />
    </svg>
  );
};

const Vignette: React.FC<{ sentiment?: string; width: number; height: number }> = ({ sentiment, width, height }) => {
  const baseColor = sentiment === 'bearish' ? 'rgba(100,30,30,0.08)' : sentiment === 'bullish' ? 'rgba(30,100,50,0.05)' : 'rgba(20,60,120,0.06)';

  return (
    <div
      style={{
        position: 'absolute',
        top: 0,
        left: 0,
        width,
        height,
        background: `radial-gradient(ellipse 60% 70% at 50% 40%, ${baseColor} 0%, transparent 70%)`,
        zIndex: 2,
        pointerEvents: 'none',
      }}
    />
  );
};

export const Background: React.FC<BackgroundProps> = ({ frame, totalFrames, sentiment = 'neutral', width = 1080, height = 1920 }) => {
  const bgGradient = sentiment === 'bearish'
    ? 'linear-gradient(180deg, #080c14 0%, #0a1018 30%, #0e1520 60%, #080c14 100%)'
    : sentiment === 'bullish'
    ? 'linear-gradient(180deg, #081018 0%, #0a1420 30%, #0e1826 60%, #080f18 100%)'
    : 'linear-gradient(180deg, #081018 0%, #0B1220 30%, #101826 60%, #0A0F18 100%)';

  return (
    <div style={{ position: 'absolute', top: 0, left: 0, width, height, overflow: 'hidden', background: bgGradient }}>
      <GridOverlay width={width} height={height} />
      <RadarScan frame={frame} totalFrames={totalFrames} width={width} height={height} />
      <ScanLine frame={frame} width={width} height={height} />
      <Vignette sentiment={sentiment} width={width} height={height} />
      <CornerBrackets width={width} height={height} />
    </div>
  );
};
