import { useState, useEffect, useRef } from 'react';
import { api } from '../api';
import type { SchedulerStatus } from '../types';

export function SchedulerPage() {
  const [status, setStatus] = useState<SchedulerStatus | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => { loadStatus(); }, []);

  async function loadStatus() {
    try {
      const s = await api.getSchedulerStatus();
      setStatus(s);
      if (s.is_running && !timerRef.current) {
        timerRef.current = setInterval(() => {
          loadStatus().then(() => {
            if (!s.is_running && timerRef.current) {
              clearInterval(timerRef.current);
              timerRef.current = null;
            }
          });
        }, 2000);
      } else if (!s.is_running && timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
    } catch { void 0; }
  }

  async function toggleScheduler() {
    if (!status) return;
    try {
      await api.updateScheduler({ enabled: !status.enabled });
      loadStatus();
    } catch { void 0; }
  }

  async function updateSchedulerTime() {
    if (!status) return;
    try {
      await api.updateScheduler({ run_time: status.run_time, morning_run_time: status.morning_run_time });
      loadStatus();
    } catch { void 0; }
  }

  async function runNow() {
    try {
      await api.runSchedulerNow();
      loadStatus();
    } catch (e: unknown) { alert(e instanceof Error ? e.message : String(e)); }
  }

  if (!status) return <div style={{ textAlign: 'center', color: '#8892a4', padding: 40 }}>加载中...</div>;

  const fmtDate = (s: string) => {
    if (!s) return '—';
    try { return new Date(s).toLocaleString('zh-CN'); } catch { return '—'; }
  };

  return (
    <div>
      <div className="card">
        <h2>⏰ 定时任务</h2>
        <p style={{ fontSize: 13, color: '#8892a4', marginBottom: 16 }}>每天自动获取数据、生成视频和文案</p>
        <div className="form-row">
          <div className="form-group">
            <label>自动执行</label>
            <label className="toggle">
              <input type="checkbox" checked={status.enabled} onChange={toggleScheduler} />
              <span className="toggle-slider" />
            </label>
          </div>
          <div className="form-group">
            <label>早盘时间</label>
            <input type="time" value={status.morning_run_time || '11:35'} onChange={e => {
              setStatus({ ...status, morning_run_time: e.target.value });
              updateSchedulerTime();
            }} style={{ width: 130 }} />
          </div>
          <div className="form-group">
            <label>全天时间</label>
            <input type="time" value={status.run_time} onChange={e => {
              setStatus({ ...status, run_time: e.target.value });
              updateSchedulerTime();
            }} style={{ width: 130 }} />
          </div>
          <div className="form-group">
            <label>&nbsp;</label>
            <button className="btn btn-secondary" onClick={runNow} disabled={status.is_running}>
              {status.is_running ? '⏳ 运行中' : '▶ 立即执行'}
            </button>
          </div>
        </div>
      </div>

      <div className="card">
        <h2>执行状态</h2>
        <div style={{ fontSize: 13, lineHeight: 2, color: '#8892a4' }}>
          <div>状态：<span style={{ color: status.is_running ? '#00ff88' : (status.enabled ? '#00ff88' : '#8892a4') }}>
            {status.is_running ? '🔄 运行中' : (status.enabled ? '✅ 已启用' : '⏹ 停止')}
          </span></div>
          <div>早盘上次执行：<span>{fmtDate(status.last_morning_run)}</span></div>
          <div>全天上次执行：<span>{fmtDate(status.last_run)}</span></div>
          <div>上次结果：<span style={{ color: status.last_status === 'success' ? '#00ff88' : '#8892a4' }}>
            {status.last_status === 'success' ? '✅ 成功' : (status.last_status || '—')}
          </span></div>
          <div>早盘下次执行：<span>{fmtDate(status.next_morning_run)}</span></div>
          <div>全天下次执行：<span>{fmtDate(status.next_run)}</span></div>
        </div>
      </div>
    </div>
  );
}
