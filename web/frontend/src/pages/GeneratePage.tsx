import { useState, useEffect } from 'react';
import { api } from '../api';
import { useSSE } from '../hooks/useSSE';
import type { SSEMessage } from '../types';

interface GeneratePageProps {
  dates: string[];
  onDone: () => void;
}

type VideoType = 'single' | 'multiday';

export function GeneratePage({ dates, onDone }: GeneratePageProps) {
  const [videoType, setVideoType] = useState<VideoType>('single');
  const [genDate, setGenDate] = useState('');
  const [copyMode, setCopyMode] = useState('template');
  const [format, setFormat] = useState('mobile');
  const [days, setDays] = useState(3);
  const { logs, progress, isRunning, startStream } = useSSE();
  const [statusText, setStatusText] = useState('');
  const [statusClass, setStatusClass] = useState('');

  useEffect(() => {
    if (dates.length > 0 && !genDate) setGenDate(dates[0]);
  }, [dates]);

  async function handleGenerate() {
    if (!genDate) { alert('请选择日期'); return; }
    setStatusText('');
    setStatusClass('running');

    setStatusText('⏳ 正在生成视频...');

    try {
      await startStream(
        () => {
          if (videoType === 'multiday') {
            return api.generateMultiDay(genDate, days, copyMode, format);
          }
          return api.generate(genDate, copyMode, format);
        },
        (msg: SSEMessage) => {
          if (msg.type === 'done') {
            setStatusClass('done');
            setStatusText('✅ ' + msg.text);
            onDone();
          } else if (msg.type === 'error') {
            throw new Error(msg.text);
          }
        }
      );
    } catch (e: unknown) {
      setStatusClass('error');
      setStatusText(`❌ ${e instanceof Error ? e.message : String(e)}`);
    }
  }

  const fmtLabel = format === 'tv' ? '📺 TV (16:9)' : '📱 App (9:16)';

  return (
    <div className="card">
      <h2>生成配置</h2>

      {/* Video Type Tabs */}
      <div className="video-type-tabs">
        <button
          className={`tab-btn ${videoType === 'single' ? 'active' : ''}`}
          onClick={() => setVideoType('single')}
        >
          📊 单日资金流向
          <span className="tab-desc">单交易日板块资金曲线</span>
        </button>
        <button
          className={`tab-btn ${videoType === 'multiday' ? 'active' : ''}`}
          onClick={() => setVideoType('multiday')}
        >
          📈 多日 Bar Chart Race
          <span className="tab-desc">动态横向排名条形竞赛图</span>
        </button>
      </div>

      <div className="form-row">
        <div className="form-group">
          <label>截止日期</label>
          <select value={genDate} onChange={e => setGenDate(e.target.value)} style={{ minWidth: 130 }}>
            {(dates || []).map(d => <option key={d} value={d}>{d}</option>)}
          </select>
        </div>
        {videoType === 'multiday' && (
          <div className="form-group">
            <label>天数</label>
            <select value={days} onChange={e => setDays(Number(e.target.value))} style={{ minWidth: 80 }}>
              <option value={3}>近3日</option>
              <option value={5}>近5日</option>
              <option value={7}>近7日</option>
            </select>
          </div>
        )}
        <div className="form-group">
          <label>文案模式</label>
          <select value={copyMode} onChange={e => setCopyMode(e.target.value)} style={{ minWidth: 100 }}>
            <option value="template">模板文案</option>
            <option value="ai">AI文案</option>
          </select>
        </div>
        <div className="form-group">
          <label>输出格式</label>
          <select value={format} onChange={e => setFormat(e.target.value)} style={{ minWidth: 100 }}>
            <option value="mobile">📱 App (9:16)</option>
            <option value="tv">📺 TV (16:9)</option>
          </select>
        </div>
        <div className="form-group">
          <label>&nbsp;</label>
          <button className="btn btn-primary" onClick={handleGenerate} disabled={isRunning || !genDate}>🚀 生成</button>
        </div>
      </div>

      {statusClass && (
        <div className={`status ${statusClass}`}>
          <div>{statusText}</div>
          {progress && statusClass === 'running' && <div className="progress">{progress}</div>}
        </div>
      )}

      {logs.length > 0 && (
        <div className="log-panel" style={{ marginTop: 8 }}>
          {logs.map((l, i) => <div key={i}>{l}</div>)}
        </div>
      )}

      {statusClass === 'done' && (
        <div style={{ marginTop: 12 }}>
          <button className="btn btn-secondary" onClick={onDone}>👁 前往预览</button>
        </div>
      )}
    </div>
  );
}
