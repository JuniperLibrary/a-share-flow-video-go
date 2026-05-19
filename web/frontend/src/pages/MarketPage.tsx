import { useState, useEffect, useRef, useCallback } from 'react';

interface TickPoint {
  Time: string;
  Name: string;
  Net: number;
}

interface TickSnapshot {
  points: TickPoint[];
  date: string;
  running: boolean;
  count: number;
  lastTime: string;
}

interface MarketEvent {
  type: string;
  sector: string;
  time: string;
  description: string;
  netChange: number;
}

const SECTOR_COLORS: Record<string, string> = {
  '半导体': '#00d4ff',
  'AI应用': '#00ffaa',
  'CPO概念': '#00ff88',
  '有色金属': '#ffc107',
  '锂矿概念': '#66bb6a',
  '商业航天': '#ff8a80',
  '电池': '#4caf50',
  '机器人': '#00ffcc',
  '创新药': '#ba68c8',
  '白酒': '#ff9800',
  '消费电子': '#00c8ff',
  '银行': '#ffb300',
  '人工智能': '#00b4ff',
  '云计算': '#ce93d8',
  '低空经济': '#ff6b9d',
  '电网设备': '#42a5f5',
  '通信设备': '#26c6da',
  '传媒': '#ab47bc',
  '国产芯片': '#e07a5f',
  '元件': '#5cdb95',
  '通信服务': '#845ec2',
};

function getSectorColor(name: string): string {
  for (const key in SECTOR_COLORS) {
    if (name.includes(key)) return SECTOR_COLORS[key];
  }
  return '#888888';
}

export function MarketPage() {
  const [snapshots, setSnapshots] = useState<TickSnapshot[]>([]);
  const [connected, setConnected] = useState(false);
  const [events, setEvents] = useState<MarketEvent[]>([]);
  const [selectedSession, setSelectedSession] = useState<'full' | 'morning'>('full');
  const eventSourceRef = useRef<EventSource | null>(null);
  const prevNetRef = useRef<Map<string, number>>(new Map());

  const detectEvents = useCallback((newPoints: TickPoint[]) => {
    const newEvents: MarketEvent[] = [];
    const currentNet = new Map<string, number>();
    for (const p of newPoints) {
      currentNet.set(p.Name, p.Net);
    }

    for (const [name, net] of currentNet) {
      const prev = prevNetRef.current.get(name);
      if (prev !== undefined) {
        const delta = net - prev;
        if (Math.abs(delta) > 3) {
          const lastPoint = newPoints[newPoints.length - 1];
          newEvents.push({
            type: delta > 0 ? '资金涌入' : '资金流出',
            sector: name,
            time: lastPoint?.Time || '',
            description: `${name} ${delta > 0 ? '净流入' : '净流出'} ${Math.abs(delta).toFixed(2)}亿`,
            netChange: delta,
          });
        }
      }
    }

    prevNetRef.current = currentNet;
    if (newEvents.length > 0) {
      setEvents(prev => [...newEvents, ...prev].slice(0, 50));
    }
  }, []);

  useEffect(() => {
    const es = new EventSource('/api/tick/stream');
    eventSourceRef.current = es;

    es.onopen = () => setConnected(true);

    es.addEventListener('tick', (e: MessageEvent) => {
      try {
        const snap: TickSnapshot = JSON.parse(e.data);
        setSnapshots(prev => {
          const filtered = prev.filter(s =>
            selectedSession === 'morning'
              ? s.points.every(p => p.Time <= '11:30')
              : true
          );
          return [...filtered, snap];
        });
        detectEvents(snap.points);
      } catch {}
    });

    es.addEventListener('heartbeat', () => {});

    es.onerror = () => {
      setConnected(false);
      es.close();
      setTimeout(() => {
        setConnected(true);
      }, 5000);
    };

    return () => {
      es.close();
    };
  }, [selectedSession, detectEvents]);

  const latestSnapshot = snapshots.length > 0 ? snapshots[snapshots.length - 1] : null;

  const latestPoints = latestSnapshot?.points || [];
  const latestNet = new Map<string, number>();
  for (const p of latestPoints) {
    latestNet.set(p.Name, p.Net);
  }

  const sectors = Array.from(latestNet.entries())
    .map(([name, net]) => ({ name, net, color: getSectorColor(name) }))
    .sort((a, b) => Math.abs(b.net) - Math.abs(a.net));

  const allTimes = Array.from(new Set(latestPoints.map(p => p.Time))).sort();
  const timeToIdx = new Map<string, number>();
  allTimes.forEach((t, i) => timeToIdx.set(t, i));

  const curves = sectors.map(s => {
    const data: number[] = [];
    for (const t of allTimes) {
      const pt = latestPoints.find(p => p.Name === s.name && p.Time === t);
      data.push(pt ? pt.Net : (data[data.length - 1] ?? 0));
    }
    return { ...s, data, times: allTimes };
  });

  const chartW = 800;
  const chartH = 360;
  const chartLeft = 60;
  const chartRight = chartW - 20;
  const chartTop = 20;
  const chartBottom = chartH - 40;

  const yMin = Math.min(0, ...curves.flatMap(c => c.data));
  const yMax = Math.max(0, ...curves.flatMap(c => c.data));
  const yPad = (yMax - yMin) * 0.1 || 10;
  const yBounds = { min: yMin - yPad, max: yMax + yPad };

  const xScale = (i: number) => chartLeft + (allTimes.length <= 1 ? 0.5 : i / (allTimes.length - 1)) * (chartRight - chartLeft);
  const yScale = (v: number) => chartBottom - ((v - yBounds.min) / (yBounds.max - yBounds.min)) * (chartBottom - chartTop);

  const yTicks: number[] = [];
  const yStep = Math.max(1, Math.round((yBounds.max - yBounds.min) / 6));
  for (let v = Math.ceil(yBounds.min / yStep) * yStep; v <= yBounds.max; v += yStep) {
    yTicks.push(Math.round(v));
  }

  const isTrading = latestSnapshot?.running || false;

  return (
    <div>
      <div className="card" style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
          <h2 style={{ margin: 0 }}>实时行情</h2>
          <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
            <select
              value={selectedSession}
              onChange={e => setSelectedSession(e.target.value as 'full' | 'morning')}
              style={{ background: '#0d1b2a', color: '#00F0FF', border: '1px solid #1e2d45', borderRadius: 4, padding: '4px 8px', fontSize: 13 }}
            >
              <option value="full">全天</option>
              <option value="morning">早盘</option>
            </select>
            <span style={{ fontSize: 12, color: isTrading ? '#4ade80' : '#5a6577' }}>
              {isTrading ? '● 采集中' : '○ 未采集'}
            </span>
            <span style={{ fontSize: 12, color: connected ? '#4ade80' : '#f87171' }}>
              {connected ? '● SSE已连接' : '○ 断开'}
            </span>
            {latestSnapshot?.lastTime && (
              <span style={{ fontSize: 12, color: '#8892a4' }}>
                最新: {latestSnapshot.lastTime}
              </span>
            )}
          </div>
        </div>

        {curves.length === 0 ? (
          <div style={{ textAlign: 'center', color: '#5a6577', padding: 40, fontSize: 14 }}>
            等待交易数据... {isTrading ? '采集中' : '非交易时段'}
          </div>
        ) : (
          <svg viewBox={`0 0 ${chartW} ${chartH}`} style={{ width: '100%', height: 'auto', background: '#0a1628', borderRadius: 8 }}>
            {yTicks.map(v => (
              <line key={`y${v}`} x1={chartLeft} y1={yScale(v)} x2={chartRight} y2={yScale(v)} stroke="#1e2d45" strokeWidth={0.8} opacity={0.6} />
            ))}
            {yBounds.min < 0 && yBounds.max > 0 && (
              <line x1={chartLeft} y1={yScale(0)} x2={chartRight} y2={yScale(0)} stroke="#3a5570" strokeWidth={1.5} opacity={0.7} strokeDasharray="6 4" />
            )}
            {allTimes.map((t, i) => {
              if (i % Math.max(1, Math.floor(allTimes.length / 6)) !== 0 && i !== allTimes.length - 1) return null;
              return (
                <text key={`x${i}`} x={xScale(i)} y={chartBottom + 16} fill="#5a6577" fontSize={11} textAnchor="middle">
                  {t}
                </text>
              );
            })}
            {yTicks.map(v => (
              <text key={`yl${v}`} x={chartLeft - 6} y={yScale(v) + 4} fill="#4a5568" fontSize={10} textAnchor="end">
                {v >= 0 ? `+${v}` : `${v}`}
              </text>
            ))}
            {curves.map(c => {
              if (c.data.length < 2) return null;
              const pathD = c.data.map((v, i) => `${i === 0 ? 'M' : 'L'} ${xScale(i)} ${yScale(v)}`).join(' ');
              const isTop5 = sectors.findIndex(s => s.name === c.name) < 5;
              return (
                <g key={c.name}>
                  <path d={pathD} fill="none" stroke={c.color} strokeWidth={isTop5 ? 2.5 : 1.2} opacity={isTop5 ? 0.85 : 0.5} strokeLinecap="round" />
                  {isTop5 && c.data.length > 0 && (
                    <>
                      <circle cx={xScale(c.data.length - 1)} cy={yScale(c.data[c.data.length - 1])} r={4} fill={c.color} stroke="#fff" strokeWidth={1} />
                      <text x={xScale(c.data.length - 1) + 8} y={yScale(c.data[c.data.length - 1]) + 4} fill={c.color} fontSize={11} fontWeight={600}>
                        {c.name}
                      </text>
                    </>
                  )}
                </g>
              );
            })}
          </svg>
        )}

        {sectors.length > 0 && (
          <div style={{ marginTop: 12, display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            {sectors.map(s => (
              <span key={s.name} style={{ fontSize: 12, color: s.color, display: 'flex', alignItems: 'center', gap: 4 }}>
                <span style={{ width: 8, height: 8, borderRadius: '50%', background: s.color, display: 'inline-block' }} />
                {s.name} {s.net >= 0 ? '+' : ''}{s.net.toFixed(2)}亿
              </span>
            ))}
          </div>
        )}
      </div>

      <div className="card">
        <h2>AI 分析事件</h2>
        {events.length === 0 ? (
          <div style={{ textAlign: 'center', color: '#5a6577', padding: 24, fontSize: 13 }}>
            等待资金异动...（净流入/流出超过3亿时自动标记）
          </div>
        ) : (
          <div style={{ maxHeight: 300, overflowY: 'auto' }}>
            {events.map((ev, i) => (
              <div key={i} style={{ padding: '8px 0', borderBottom: '1px solid #0d1b2a', display: 'flex', gap: 12, alignItems: 'flex-start' }}>
                <span style={{ fontSize: 11, color: '#5a6577', fontFamily: 'monospace', minWidth: 40 }}>{ev.time}</span>
                <span style={{
                  fontSize: 11, padding: '2px 6px', borderRadius: 4,
                  color: ev.netChange > 0 ? '#4ade80' : '#f87171',
                  background: ev.netChange > 0 ? '#4ade8015' : '#f8717115',
                }}>
                  {ev.type}
                </span>
                <span style={{ fontSize: 13, color: '#c9d1d9' }}>{ev.description}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
