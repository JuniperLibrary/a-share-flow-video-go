import { useState, useEffect } from 'react';
import { api } from '../api';
import { useCopyButton } from '../utils';

export function PreviewPage() {
  const [dates, setDates] = useState<string[]>([]);
  const [selectedDate, setSelectedDate] = useState('');
  const [videos, setVideos] = useState<Record<string, string>>({});
  const [copyData, setCopyData] = useState<{ template: Record<string, string>; ai: Record<string, string> }>({ template: {}, ai: {} });
  const [activeSession, setActiveSession] = useState('');
  const [activeCopyTab, setActiveCopyTab] = useState<'template' | 'ai'>('template');
  const { copied, handleCopy } = useCopyButton();

  useEffect(() => {
    api.getDates().then(d => {
      const list = d.dates?.map(x => x.date) || [];
      setDates(list);
      if (list.length > 0) setSelectedDate(list[0]);
    }).catch(() => void 0);
  }, []);

  useEffect(() => {
    if (!selectedDate) return;
    api.getFiles(selectedDate).then(data => {
      setVideos(data.videos || {});
      setCopyData(data.文案 || { template: {}, ai: {} });
      const sessions = new Set([
        ...Object.keys(data.videos || {}),
        ...Object.keys(data.文案?.template || {}),
        ...Object.keys(data.文案?.ai || {}),
      ]);
      const arr = [...sessions];
      setActiveSession(arr.length > 0 ? arr[0] : '');
    }).catch(() => {
      setVideos({});
      setCopyData({ template: {}, ai: {} });
      setActiveSession('');
    });
  }, [selectedDate]);

  const sessions = [...new Set([
    ...Object.keys(videos),
    ...Object.keys(copyData.template),
    ...Object.keys(copyData.ai),
  ])];

  const sessionKeyMap: Record<string, string> = {
    '全天': 'full',
    '早盘': 'morning',
    '午盘': 'afternoon',
  };

  const currentVideo = videos[activeSession];
  const currentTemplateCopy = copyData.template[activeSession];
  const currentAiCopy = copyData.ai[activeSession];
  const isTV = activeSession.includes('_tv');

  if (!selectedDate) {
    return <div style={{ textAlign: 'center', color: '#8892a4', padding: 40 }}>选择一个日期查看生成结果</div>;
  }

  if (sessions.length === 0) {
    return <div style={{ textAlign: 'center', color: '#8892a4', padding: 40 }}>该日期暂无生成结果</div>;
  }

  return (
    <div>
      {/* Date selector bar */}
      <div className="card preview-header">
        <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <span style={{ fontSize: 14, color: '#8a8580', fontWeight: 500 }}>📅</span>
          <select value={selectedDate} onChange={e => setSelectedDate(e.target.value)} className="date-select">
            {(dates || []).map(d => <option key={d} value={d}>{d}</option>)}
          </select>
          <span style={{ fontSize: 13, color: '#a5a09a' }}>共 {sessions.length} 个会话</span>
        </div>
      </div>

      {/* Session tabs */}
      <div className="card session-tabs-card">
        <div className="session-tab-bar">
          {sessions.map(s => (
            <div key={s} className={`session-tab ${s === activeSession ? 'active' : ''}`} onClick={() => setActiveSession(s)}>
              {s.includes('_tv') ? '📺' : '📱'} {s}
            </div>
          ))}
        </div>
      </div>

      {/* Main content: video player + copywriting sidebar */}
      {activeSession && (
        <div className="preview-layout">
          {/* Left: Video player */}
          <div className="video-section">
            <div className="video-player-wrapper">
              {currentVideo ? (
                <video
                  key={currentVideo}
                  src={`/output/${selectedDate}/${currentVideo}`}
                  controls
                  autoPlay
                  className="main-video"
                />
              ) : (
                <div className="video-placeholder">
                  <div style={{ fontSize: 48, marginBottom: 12 }}>🎬</div>
                  <div style={{ color: '#8892a4', fontSize: 14 }}>暂无视频</div>
                </div>
              )}
            </div>

            {/* Video info bar */}
            <div className="video-info-bar">
              <div className="video-title">
                <span className="video-badge">{isTV ? 'TV' : 'APP'}</span>
                <span>{activeSession}</span>
                <span className="video-date">{selectedDate}</span>
              </div>
              <div className="video-actions">
                {currentVideo && (
                  <a
                    href={`/output/${selectedDate}/${currentVideo}`}
                    download
                    className="btn btn-sm btn-primary"
                  >
                    ⬇ 下载视频
                  </a>
                )}
              </div>
            </div>
          </div>

          {/* Right: Copywriting sidebar */}
          <div className="copy-sidebar">
            <div className="copy-sidebar-header">
              <span style={{ fontWeight: 600, fontSize: 15 }}>📝 文案</span>
            </div>

            {/* Copy tabs */}
            <div className="copy-tab-bar">
              <div
                className={`copy-tab ${activeCopyTab === 'template' ? 'active' : ''}`}
                onClick={() => currentTemplateCopy && setActiveCopyTab('template')}
                style={{ opacity: currentTemplateCopy ? 1 : 0.4, cursor: currentTemplateCopy ? 'pointer' : 'default' }}
              >
                模板
              </div>
              <div
                className={`copy-tab ${activeCopyTab === 'ai' ? 'active' : ''}`}
                onClick={() => currentAiCopy && setActiveCopyTab('ai')}
                style={{ opacity: currentAiCopy ? 1 : 0.4, cursor: currentAiCopy ? 'pointer' : 'default' }}
              >
                AI
              </div>
            </div>

            {/* Copy content */}
            <div className="copy-content">
              {activeCopyTab === 'template' && currentTemplateCopy && (
                <div>
                  <div className="copy-actions-row">
                    <button className="btn-icon" onClick={() => handleCopy(currentTemplateCopy)}>
                      {copied ? '✅ 已复制' : '📋 复制'}
                    </button>
                    <button className="btn-icon" onClick={async () => {
                      const sk = sessionKeyMap[activeSession] || activeSession;
                      try {
                        const res = await api.optimizeCopy(selectedDate, sk);
                        if ('error' in res) { alert('AI优化失败: ' + res.error); return; }
                        setCopyData(prev => ({ ...prev, ai: { ...prev.ai, [activeSession]: res.text } }));
                        setActiveCopyTab('ai');
                      } catch (e: unknown) { alert('请求失败: ' + (e instanceof Error ? e.message : String(e))); }
                    }}>🤖 AI优化</button>
                  </div>
                  <div className="copy-preview">{currentTemplateCopy}</div>
                </div>
              )}
              {activeCopyTab === 'ai' && currentAiCopy && (
                <div>
                  <div className="copy-actions-row">
                    <button className="btn-icon" onClick={() => handleCopy(currentAiCopy)}>
                      {copied ? '✅ 已复制' : '📋 复制'}
                    </button>
                  </div>
                  <div className="copy-preview">{currentAiCopy}</div>
                </div>
              )}
              {!currentTemplateCopy && !currentAiCopy && (
                <div className="copy-empty">暂无文案</div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
