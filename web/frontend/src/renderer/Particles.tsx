import React, { useMemo } from 'react';

interface ParticlesProps {
  frame: number;
  width?: number;
  height?: number;
}

interface Particle {
  x: number;
  startY: number;
  size: number;
  speed: number;
  opacity: number;
  delay: number;
}

function createRNG(seed: number) {
  let s = seed | 0;
  return () => {
    s = (s * 1664525 + 1013904223) | 0;
    return (s >>> 0) / 4294967296;
  };
}

function generateParticles(count: number, width: number, height: number): Particle[] {
  const rng = createRNG(420);
  const particles: Particle[] = [];
  for (let i = 0; i < count; i++) {
    particles.push({
      x: rng() * width,
      startY: rng() * height,
      size: rng() * 1.2 + 0.3,
      speed: rng() * 0.2 + 0.05,
      opacity: rng() * 0.15 + 0.02,
      delay: rng() * 400,
    });
  }
  return particles;
}

const PARTICLE_COUNT = 80;
const CYCLE_FRAMES = 600;

export const Particles: React.FC<ParticlesProps> = ({ frame, width = 1080, height = 1920 }) => {
  const particles = useMemo(() => generateParticles(PARTICLE_COUNT, width, height), [width, height]);

  return (
    <svg
      width={width}
      height={height}
      style={{
        position: 'absolute',
        top: 0,
        left: 0,
        zIndex: 2,
        pointerEvents: 'none',
      }}
    >
      {particles.map((p, i) => {
        const offset = (frame + p.delay) % CYCLE_FRAMES;
        const progress = offset / CYCLE_FRAMES;
        const y = (p.startY + progress * height) % height;

        return (
          <circle
            key={i}
            cx={p.x}
            cy={y}
            r={p.size}
            fill="#6a8ab0"
            opacity={p.opacity}
          />
        );
      })}
    </svg>
  );
};
