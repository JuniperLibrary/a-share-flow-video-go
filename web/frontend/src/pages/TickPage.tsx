import { useState, useEffect } from 'react';

interface TickStatus {
  running: boolean;
  date: string;
  tickCount: number;
  errCount: number;
  lastTick: string;
  enabled: boolean;
}

export function TickPage() {
  const [status, setStatus] = useState<TickStatus | null>(null);
  const [msg, setMsg] = useState('');

  useEffect(() => {
    fetchStatus();
    const iv = setInterval(fetchStatus, 10000);
    return () => clearInterval(iv);
  }, []);

  async function fetchStatus() {
    try {
      const res = await fetch('/api/tick/status');
      const data = await res.json();
      setStatus(data);
    } catch {}
  }

  async function handleStart() {
    setMsg('');
    try {
      const res = await fetch('/api/tick/start', { method: 'POST' });
      const data = await res.json();
      if (data.error) {
        setMsg('❌ ' + data.error);
      } else {
        setMsg('✅ ' + data.message);
      }
    } catch (e: unknown) {
      setMsg('❌ ' + String(e));
    }
    fetchStatus();
  }

  async function handleStop() {
    setMsg('');
    try {
      await fetch('/api/tick/stop', { method: 'POST' });
      setMsg('✅ 采集已停止');
    } catch (e: unknown) {
      setMsg('❌ ' + String(e));
    }
    fetchStatus();
  }

  async function handleToggleEnable(on: boolean) {
    try {
      await fetch('/api/tick/enable', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: on }),
      });
    } catch {}
    fetchStatus();
  }

  if (!status) return <div style={{ textAlign: 'center', color: '#8892a4', padding: 40 }}>加载中...</div>;

  return (
    <div className="card">
      <h2>Tick 采集</h2>
      <p style={{ fontSize: 13, color: '#8892a4', marginBottom: 16 }}>
        交易时段内每10分钟自动采集一次板块资金流向数据，生成真实曲线视频
      </p>

      <div style={{ marginBottom: 16 }}>
        <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
          <input
            type="checkbox"
            checked={status.enabled}
            onChange={e => handleToggleEnable(e.target.checked)}
            style={{ accentColor: '#4ade80', transform: 'scale(1.2)' }}
          />
          <span style={{ fontSize: 14, color: status.enabled ? '#4ade80' : '#8892a4' }}>
            定时采集 {status.enabled ? '已启用' : '已禁用'}
          </span>
        </label>
        <div style={{ fontSize: 12, color: '#5a6577', marginTop: 4 }}>
          早盘 09:28 / 全天 12:58 自动启动
        </div>
      </div>

      <div style={{ display: 'flex', gap: 12, marginBottom: 16 }}>
        <button
          className="btn btn-sm"
          onClick={handleStart}
          disabled={status.running}
          style={{ borderColor: '#4ade80', color: '#4ade80', opacity: status.running ? 0.5 : 1 }}
        >
          ▶ 立即启动
        </button>
        <button
          className="btn btn-sm"
          onClick={handleStop}
          disabled={!status.running}
          style={{ borderColor: '#f87171', color: '#f87171', opacity: !status.running ? 0.5 : 1 }}
        >
          ⏹ 停止
        </button>
      </div>

      {msg && <div style={{ fontSize: 13, marginBottom: 12, color: msg.includes('❌') ? '#f87171' : '#4ade80' }}>{msg}</div>}

      <div style={{ fontSize: 13, lineHeight: 2, color: '#8892a4' }}>
        <div>状态：<span style={{ color: status.running ? '#00ff88' : '#8892a4' }}>
          {status.running ? '采集中' : '未运行'}
        </span></div>
        {status.date && <div>日期：{status.date}</div>}
        <div>已采集：<span style={{ color: '#00F0FF' }}>{status.tickCount}</span> 个点</div>
        {status.lastTick && <div>最新时间点：{status.lastTick}</div>}
        {status.errCount > 0 && <div style={{ color: '#f87171' }}>失败次数：{status.errCount}</div>}
      </div>
    </div>
  );
}
