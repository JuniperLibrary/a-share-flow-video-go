import { useState, useEffect, useCallback } from 'react';
import { DataPage } from './pages/DataPage';
import { GeneratePage } from './pages/GeneratePage';
import { PreviewPage } from './pages/PreviewPage';
import { ConfigPage } from './pages/ConfigPage';
import { SchedulerPage } from './pages/SchedulerPage';
import { api } from './api';
import type { Sector, DateItem } from './types';

type Page = 'data' | 'generate' | 'preview' | 'config' | 'scheduler';

export default function App() {
  const [page, setPage] = useState<Page>('data');
  const [dates, setDates] = useState<string[]>([]);
  const [sectorData, setSectorData] = useState<Sector[]>([]);

  useEffect(() => { loadDates(); }, []);

  async function loadDates() {
    try {
      const data = await api.getDates();
      setDates(data.dates.map(d => d.date));
    } catch { void 0; }
  }

  const handleSectorData = useCallback((data: Sector[]) => {
    setSectorData(data);
  }, []);

  const handleDone = useCallback(() => {
    loadDates();
    setPage('preview');
  }, []);

  const navItems: { key: Page; label: string; icon: string }[] = [
    { key: 'data', label: '数据', icon: '📋' },
    { key: 'generate', label: '生成', icon: '🎬' },
    { key: 'preview', label: '预览', icon: '👁' },
    { key: 'config', label: '配置', icon: '⚙' },
    { key: 'scheduler', label: '定时', icon: '⏰' },
  ];

  return (
    <div className="container">
      <div className="header">
        <h1>📊 <span>A股</span>情绪流动可视化</h1>
        <div className="sub">板块情绪监控 · 智能视频生成 · 一键发布</div>
      </div>

      <div className="nav">
        {navItems.map(item => (
          <a key={item.key} className={page === item.key ? 'active' : ''} onClick={() => setPage(item.key)}>
            {item.icon} {item.label}
          </a>
        ))}
      </div>

      <div className={page === 'data' ? 'page active' : 'page'}>
        <DataPage onSectorData={handleSectorData} sectorData={sectorData} />
      </div>
      <div className={page === 'generate' ? 'page active' : 'page'}>
        <GeneratePage dates={dates} onDone={handleDone} />
      </div>
      <div className={page === 'preview' ? 'page active' : 'page'}>
        <PreviewPage />
      </div>
      <div className={page === 'config' ? 'page active' : 'page'}>
        <ConfigPage />
      </div>
      <div className={page === 'scheduler' ? 'page active' : 'page'}>
        <SchedulerPage />
      </div>
    </div>
  );
}
