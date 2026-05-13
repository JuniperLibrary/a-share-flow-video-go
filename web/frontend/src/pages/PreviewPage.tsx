import { useState, useEffect } from 'react';
import { api } from '../api';
import { useCopyButton } from '../utils';

export function PreviewPage() {
  const [dates, setDates] = useState<string[]>([]);
  const [selectedDate, setSelectedDate] = useState('');
  const [videos, setVideos] = useState<Record<string, string>>({});
  const [copyData, setCopyData] = useState<{ template: Record<string, string>; ai: Record<string, string> }>({ template: {}, ai: {} });
  const [activeSession, setActiveSession] = useState('');
  const { copied, handleCopy } = useCopyButton();

  useEffect(() => {
    api.getDates().then(d => {
      const list = d.dates.map(x => x.date);
      setDates(list);
      if (list.length > 0) setSelectedDate(list[0]);
    }).catch(() => void 0);
  }, []);

  useEffect(() => {
    if (!selectedDate) return;
    api.getFiles(selectedDate).then(data => {
      setVideos(data.videos || {});
      setCopyData(data.copy || { template: {}, ai: {} });
      const sessions = new Set([
        ...Object.keys(data.videos || {}),
        ...Object.keys(data.copy?.template || {}),
        ...Object.keys(data.copy?.ai || {}),
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

  if (!selectedDate) {
    return <div style={{ textAlign: 'center', color: '#8892a4', padding: 40 }}>选择一个日期查看生成结果</div>;
  }

  if (sessions.length === 0) {
    return <div style={{ textAlign: 'center', color: '#8892a4', padding: 40 }}>该日期暂无生成结果</div>;
  }

  return (
    <div>
      <div className="card">
        <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <h2 style={{ marginBottom: 0 }}>视频预览</h2>
          <select value={selectedDate} onChange={e => setSelectedDate(e.target.value)} style={{ minWidth: 130 }}>
            {dates.map(d => <option key={d} value={d}>{d}</option>)}
          </select>
        </div>
      </div>

      <div className="card">
        <div className="tab-bar">
          {sessions.map((s, i) => (
            <div key={s} className={`tab-btn ${s === activeSession ? 'active' : ''}`} onClick={() => setActiveSession(s)}>{s}</div>
          ))}
        </div>

        {activeSession && (
          <div>
            <div style={{ display: 'flex', gap: 20, flexWrap: 'wrap' }}>
              {videos[activeSession] && (
                <div className="video-card">
                  <video src={`/output/${selectedDate}/${videos[activeSession]}`} controls />
                  <div className="vlabel">{activeSession.includes('_tv') ? '📺 TV (16:9)' : '📱 App (9:16)'}</div>
                  <div className="vmeta">{selectedDate}</div>
                </div>
              )}
              <div style={{ flex: 1, minWidth: 280 }}>
                {copyData.template[activeSession] && (
                  <div className="copy-section" style={{ borderTop: 'none', paddingTop: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                      <h3 style={{ margin: 0, fontSize: 14 }}>📝 模板文案</h3>
                      <button className="btn-icon" onClick={() => handleCopy(copyData.template[activeSession])}>
                        {copied ? '✅ 已复制' : '📋 复制'}
                      </button>
                      <button className="btn-icon" onClick={async () => {
                        const sk = sessionKeyMap[activeSession] || activeSession;
                        try {
                          const res = await api.optimizeCopy(selectedDate, sk);
                          if ('error' in res) { alert('AI优化失败: ' + res.error); return; }
                          setCopyData(prev => ({ ...prev, ai: { ...prev.ai, [activeSession]: res.text } }));
                        } catch (e: unknown) { alert('请求失败: ' + (e instanceof Error ? e.message : String(e))); }
                      }}>🤖 AI优化</button>
                    </div>
                    <div className="copy-preview">{copyData.template[activeSession]}</div>
                  </div>
                )}
                {copyData.ai[activeSession] && (
                  <div className="copy-section" style={{ borderTop: 'none', paddingTop: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                      <h3 style={{ margin: 0, fontSize: 14 }}>🤖 AI文案</h3>
                      <button className="btn-icon" onClick={() => handleCopy(copyData.ai[activeSession])}>
                        {copied ? '✅ 已复制' : '📋 复制'}
                      </button>
                    </div>
                    <div className="copy-preview">{copyData.ai[activeSession]}</div>
                  </div>
                )}
                {!copyData.template[activeSession] && !copyData.ai[activeSession] && (
                  <div style={{ color: '#8892a4' }}>暂无文案</div>
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
