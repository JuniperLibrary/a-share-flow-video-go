import { useState, useEffect } from 'react';
import { api } from '../api';

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
  const [dates, setDates] = useState<string[]>([]);
  const [genDate, setGenDate] = useState('');
  const [session, setSession] = useState('full');
  const [format, setFormat] = useState('mobile');
  const [copyMode, setCopyMode] = useState('template');
  const [genLoading, setGenLoading] = useState(false);
  const [genMsg, setGenMsg] = useState('');
  const [intervalMin, setIntervalMin] = useState(5);
  const [tickData, setTickData] = useState<{ Time: string; Name: string; Net: number }[]>([]);
  const [dataSession, setDataSession] = useState('full');

  useEffect(() => {
    fetchStatus();
    loadDates();
    loadInterval();
    loadTickData();
    const iv = setInterval(fetchStatus, 10000);
    const iv2 = setInterval(loadTickData, 30000);
    return () => { clearInterval(iv); clearInterval(iv2); };
  }, []);

  useEffect(() => {
    if (dates.length > 0 && !genDate) setGenDate(dates[0]);
  }, [dates]);

  async function fetchStatus() {
    try {
      const res = await fetch('/api/tick/status');
      const data = await res.json();
      setStatus(data);
    } catch {}
  }

  async function loadDates() {
    try {
      const res = await fetch('/api/dates');
      const data = await res.json();
      const list = (data.dates || []).map((d: { date: string }) => d.date);
      setDates(list);
    } catch {}
  }

  async function loadInterval() {
    try {
      const data = await api.getTickInterval();
      setIntervalMin(data.intervalMinutes);
    } catch {}
  }

  async function loadTickData() {
    const date = status?.date || new Date().toISOString().slice(0, 10);
    try {
      const data = await api.getTickData(date, dataSession);
      setTickData(data.points || []);
    } catch {
      setTickData([]);
    }
  }

  async function handleIntervalChange(mins: number) {
    try {
      await api.setTickInterval(mins);
      setIntervalMin(mins);
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

  async function handleGenerateTick() {
    if (!genDate) { alert('请选择日期'); return; }
    setGenLoading(true);
    setGenMsg('');
    try {
      const res = await api.generateTick(genDate, session, format, copyMode);
      const data = await res.json();
      if (data.error) {
        setGenMsg('❌ ' + data.error);
      } else {
        setGenMsg('✅ 视频已生成: ' + data.output);
      }
    } catch (e: unknown) {
      setGenMsg('❌ ' + String(e));
    }
    setGenLoading(false);
  }

  if (!status) return <div style={{ textAlign: 'center', color: '#8892a4', padding: 40 }}>加载中...</div>;

  const fmtLabel = format === 'tv' ? '📺 TV (16:9)' : '📱 App (9:16)';

  return (
    <div>
      {/* Tick 采集 */}
      <div className="card" style={{ marginBottom: 16 }}>
        <h2>Tick 采集</h2>
        <p style={{ fontSize: 13, color: '#8892a4', marginBottom: 16 }}>
          交易时段内每{intervalMin}分钟自动采集板块资金流向数据，生成真实曲线视频
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

        <div style={{ marginBottom: 16 }}>
          <label style={{ fontSize: 13, color: '#8892a4', display: 'flex', alignItems: 'center', gap: 8 }}>
            采集频率
            <select
              value={intervalMin}
              onChange={e => handleIntervalChange(Number(e.target.value))}
              style={{ background: '#0d1b2a', color: '#00F0FF', border: '1px solid #1e2d45', borderRadius: 4, padding: '2px 8px', fontSize: 13 }}
            >
              <option value={5}>5 分钟</option>
              <option value={10}>10 分钟</option>
              <option value={15}>15 分钟</option>
              <option value={20}>20 分钟</option>
              <option value={30}>30 分钟</option>
            </select>
            <span style={{ fontSize: 12, color: '#5a6577' }}>
              （全天约 {Math.floor(240 / intervalMin)} 个点，早盘约 {Math.floor(120 / intervalMin)} 个点）
            </span>
          </label>
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

      {/* 采集数据 */}
      <div className="card" style={{ marginBottom: 16 }}>
        <h2>采集数据</h2>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
          <select value={dataSession} onChange={e => { setDataSession(e.target.value); }} style={{ background: '#0d1b2a', color: '#00F0FF', border: '1px solid #1e2d45', borderRadius: 4, padding: '4px 8px', fontSize: 13 }}>
            <option value="full">全天</option>
            <option value="morning">早盘</option>
          </select>
          <button className="btn btn-sm" onClick={loadTickData} style={{ borderColor: '#5a90d0', color: '#5a90d0' }}>🔄 刷新</button>
          <span style={{ fontSize: 12, color: '#5a6577' }}>共 {tickData.length} 条记录</span>
        </div>

        {tickData.length === 0 ? (
          <div style={{ textAlign: 'center', color: '#5a6577', padding: 24, fontSize: 13 }}>暂无采集数据</div>
        ) : (
          <div style={{ maxHeight: 400, overflowY: 'auto', fontSize: 12 }}>
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #1e2d45' }}>
                  <th style={{ textAlign: 'left', padding: '6px 8px', color: '#5a6577', fontWeight: 500, position: 'sticky', top: 0, background: '#0d1b2a' }}>时间</th>
                  <th style={{ textAlign: 'left', padding: '6px 8px', color: '#5a6577', fontWeight: 500, position: 'sticky', top: 0, background: '#0d1b2a' }}>板块</th>
                  <th style={{ textAlign: 'right', padding: '6px 8px', color: '#5a6577', fontWeight: 500, position: 'sticky', top: 0, background: '#0d1b2a' }}>净流入(亿)</th>
                </tr>
              </thead>
              <tbody>
                {tickData.map((p, i) => (
                  <tr key={i} style={{ borderBottom: '1px solid #0d1b2a' }}>
                    <td style={{ padding: '4px 8px', color: '#8892a4', fontFamily: 'monospace' }}>{p.Time}</td>
                    <td style={{ padding: '4px 8px', color: '#c9d1d9' }}>{p.Name}</td>
                    <td style={{ padding: '4px 8px', textAlign: 'right', color: p.Net >= 0 ? '#4ade80' : '#f87171', fontFamily: 'monospace' }}>
                      {p.Net >= 0 ? '+' : ''}{p.Net.toFixed(2)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* 生成 Tick 视频 */}
      <div className="card">
        <h2>生成 Tick 视频</h2>
        <p style={{ fontSize: 13, color: '#8892a4', marginBottom: 16 }}>
          基于已采集的 Tick 真实数据（SQLite），渲染资金流动曲线视频
        </p>

        <div className="form-row">
          <div className="form-group">
            <label>日期</label>
            <select value={genDate} onChange={e => setGenDate(e.target.value)} style={{ minWidth: 130 }}>
              {dates.map(d => <option key={d} value={d}>{d}</option>)}
            </select>
          </div>
          <div className="form-group">
            <label>时段</label>
            <select value={session} onChange={e => setSession(e.target.value)} style={{ minWidth: 100 }}>
              <option value="full">全天</option>
              <option value="morning">早盘</option>
            </select>
          </div>
          <div className="form-group">
            <label>文案模式</label>
            <select value={copyMode} onChange={e => setCopyMode(e.target.value)} style={{ minWidth: 100 }}>
              <option value="template">模板文案</option>
              <option value="ai">AI文案</option>
            </select>
          </div>
          <div className="form-group">
            <label>输出格式</label>
            <select value={format} onChange={e => setFormat(e.target.value)} style={{ minWidth: 120 }}>
              <option value="mobile">📱 App (9:16)</option>
              <option value="tv">📺 TV (16:9)</option>
            </select>
          </div>
          <div className="form-group">
            <label>&nbsp;</label>
            <button className="btn btn-primary" onClick={handleGenerateTick} disabled={genLoading || !genDate}>
              {genLoading ? '⏳ 生成中...' : '🎬 生成 Tick 视频'}
            </button>
          </div>
        </div>

        {genMsg && (
          <div style={{ fontSize: 13, marginTop: 12, color: genMsg.includes('❌') ? '#f87171' : '#4ade80' }}>
            {genMsg}
          </div>
        )}
      </div>
    </div>
  );
}
