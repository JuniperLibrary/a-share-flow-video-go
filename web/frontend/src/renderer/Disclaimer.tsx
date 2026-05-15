import React from 'react';

interface DisclaimerProps {
  frame: number;
  totalFrames: number;
  width?: number;
  height?: number;
  format?: 'mobile' | 'tv';
}

export const Disclaimer: React.FC<DisclaimerProps> = ({
  frame,
  totalFrames,
  width = 1080,
  height = 1920,
  format = 'mobile',
}) => {
  const isTV = format === 'tv';
  const scale = isTV ? 1.0 : 1.35;

  const progress = frame / totalFrames;
  // 淡入：前3秒，淡出：最后2秒
  const fadeInEnd = 3 * 30; // 假设30fps
  const fadeOutStart = totalFrames - 2 * 30;
  
  let opacity = 0;
  if (frame < fadeInEnd) {
    opacity = frame / fadeInEnd;
  } else if (frame > fadeOutStart) {
    opacity = Math.max(0, 1 - (frame - fadeOutStart) / (totalFrames - fadeOutStart));
  } else {
    opacity = 1;
  }

  // 半透明显示，不干扰主内容
  opacity *= 0.6;

  const disclaimerY = isTV ? height - 30 : height * 0.97;

  return (
    <div
      style={{
        position: 'absolute',
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: 5,
        opacity,
        textAlign: 'center',
        pointerEvents: 'none',
      }}
    >
      <div
        style={{
          fontSize: 10 * scale,
          color: '#556677',
          fontFamily: '"PingFang SC", "Helvetica Neue", sans-serif',
          letterSpacing: 0.5,
          padding: `${4 * scale}px 0`,
        }}
      >
        ⚠️ 数据仅供分析参考，不构成任何投资建议。市场有风险，投资需谨慎。
      </div>
    </div>
  );
};
