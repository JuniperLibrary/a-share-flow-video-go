import React, { useMemo } from 'react';
import { AbsoluteFill, useCurrentFrame, interpolate } from 'remotion';

interface FinancialBackgroundProps {
  frame: number;
  totalFrames: number;
  width: number;
  height: number;
  sentiment?: 'bullish' | 'bearish' | 'neutral';
}

export const FinancialBackground: React.FC<FinancialBackgroundProps> = ({
  frame,
  totalFrames,
  width,
  height,
  sentiment = 'neutral',
}) => {
  // Terminal breathing effect
  const breathe = interpolate(frame % 180, [0, 90, 180], [0, 1, 0], { extrapolateRight: 'clamp' });
  const breatheOpacity = 0.02 + breathe * 0.015;

  // Scan line position
  const scanY = interpolate(frame % 120, [0, 120], [0, height], { extrapolateRight: 'clamp' });

  // Sentiment-based tint
  const sentimentTint = useMemo(() => {
    switch (sentiment) {
      case 'bullish': return 'rgba(0, 240, 255, 0.03)';
      case 'bearish': return 'rgba(255, 107, 138, 0.03)';
      default: return 'rgba(100, 140, 180, 0.02)';
    }
  }, [sentiment]);

  // Generate grid lines
  const gridLines = useMemo(() => {
    const lines = [];
    const spacing = 40;
    for (let y = 0; y < height; y += spacing) {
      lines.push(y);
    }
    return lines;
  }, [height]);

  // Generate subtle particles
  const particles = useMemo(() => {
    const pts = [];
    for (let i = 0; i < 30; i++) {
      pts.push({
        x: ((i * 137.5 + frame * 0.1) % width),
        y: ((i * 89.3 + frame * 0.05) % height),
        opacity: 0.1 + Math.sin(frame * 0.02 + i) * 0.05,
      });
    }
    return pts;
  }, [frame, width, height]);

  return (
    <AbsoluteFill style={{ backgroundColor: '#0a0e17' }}>
      {/* Deep blue gradient base */}
      <div style={{
        position: 'absolute',
        inset: 0,
        background: 'linear-gradient(180deg, #0a0e17 0%, #0d1220 40%, #0a0e17 100%)',
      }} />

      {/* Sentiment tint overlay */}
      <div style={{
        position: 'absolute',
        inset: 0,
        backgroundColor: sentimentTint,
        transition: 'background-color 2s ease',
      }} />

      {/* HUD Grid */}
      <div style={{ position: 'absolute', inset: 0, opacity: 0.06 }}>
        {gridLines.map(y => (
          <div key={y} style={{
            position: 'absolute',
            left: 0,
            right: 0,
            top: y,
            height: 1,
            backgroundColor: '#4a90d9',
          }} />
        ))}
        {/* Vertical grid lines */}
        {Array.from({ length: Math.floor(width / 60) }).map((_, i) => (
          <div key={`v-${i}`} style={{
            position: 'absolute',
            top: 0,
            bottom: 0,
            left: i * 60,
            width: 1,
            backgroundColor: '#4a90d9',
          }} />
        ))}
      </div>

      {/* Scan line */}
      <div style={{
        position: 'absolute',
        left: 0,
        right: 0,
        top: scanY,
        height: 2,
        background: 'linear-gradient(90deg, transparent, rgba(0, 240, 255, 0.15), transparent)',
        boxShadow: '0 0 20px rgba(0, 240, 255, 0.1)',
      }} />

      {/* Terminal breathing overlay */}
      <div style={{
        position: 'absolute',
        inset: 0,
        background: `radial-gradient(ellipse at 50% 50%, rgba(0, 240, 255, ${breatheOpacity}) 0%, transparent 70%)`,
        pointerEvents: 'none',
      }} />

      {/* Subtle particles */}
      {particles.map((p, i) => (
        <div key={i} style={{
          position: 'absolute',
          left: p.x,
          top: p.y,
          width: 2,
          height: 2,
          borderRadius: '50%',
          backgroundColor: '#00F0FF',
          opacity: p.opacity,
        }} />
      ))}

      {/* Vignette */}
      <div style={{
        position: 'absolute',
        inset: 0,
        background: 'radial-gradient(ellipse at center, transparent 50%, rgba(0, 0, 0, 0.4) 100%)',
        pointerEvents: 'none',
      }} />

      {/* Corner HUD markers */}
      <div style={{ position: 'absolute', top: 8, left: 8, width: 20, height: 20, borderTop: '1px solid rgba(0, 240, 255, 0.2)', borderLeft: '1px solid rgba(0, 240, 255, 0.2)' }} />
      <div style={{ position: 'absolute', top: 8, right: 8, width: 20, height: 20, borderTop: '1px solid rgba(0, 240, 255, 0.2)', borderRight: '1px solid rgba(0, 240, 255, 0.2)' }} />
      <div style={{ position: 'absolute', bottom: 8, left: 8, width: 20, height: 20, borderBottom: '1px solid rgba(0, 240, 255, 0.2)', borderLeft: '1px solid rgba(0, 240, 255, 0.2)' }} />
      <div style={{ position: 'absolute', bottom: 8, right: 8, width: 20, height: 20, borderBottom: '1px solid rgba(0, 240, 255, 0.2)', borderRight: '1px solid rgba(0, 240, 255, 0.2)' }} />
    </AbsoluteFill>
  );
};
