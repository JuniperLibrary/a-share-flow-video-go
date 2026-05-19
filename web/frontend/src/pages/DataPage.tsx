import { useState, useEffect } from 'react';
import { api } from '../api';
import type { Sector, DateItem } from '../types';
import { fmtNet, trendInfo, trendClass, netColor } from '../utils';

interface DataPageProps {
  onSectorData: (data: Sector[]) => void;
  sectorData: Sector[];
}

export function DataPage({ onSectorData, sectorData }: DataPageProps) {
  const [fetchDate, setFetchDate] = useState(new Date().toISOString().slice(0, 10));
  const [dates, setDates] = useState<DateItem[]>([]);
  const [selectedDate, setSelectedDate] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [exportStatus, setExportStatus] = useState('');
  const [hot15Status, setHot15Status] = useState('');

  useEffect(() => { loadDates(); }, []);

  async function loadDates() {
    try {
      const data = await api.getDates();
      setDates(data.dates || []);
    } catch { void 0; }
  }

  async function selectDate(date: string) {
    setSelectedDate(date);
    setFetchDate(date);
    try {
      const data = await api.getData(date);
      if (data.sectors) onSectorData(data.sectors);
    } catch { void 0; }
  }

  function downloadCSV(filename: string, headers: string[], rows: string[][]) {
    const bom = '\uFEFF';
    const csv = [headers.join(','), ...rows.map(r => r.map(c => `"${c}"`).join(','))].join('\n');
    const blob = new Blob([bom + csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
    URL.revokeObjectURL(url);
  }

  async function handleExportAll() {
    if (!fetchDate) { alert('请选择日期'); return; }
    setExportStatus('⏳ 正在启动全量下载任务...');
    try {
      const startRes = await api.exportAll(fetchDate);
      const taskId = startRes.task_id;
      setExportStatus(`⏳ ${startRes.progress || '任务已启动'}`);

      const poll = setInterval(async () => {
        try {
          const status = await api.exportAllStatus(taskId);
          setExportStatus(`⏳ ${status.progress}`);
          if (status.status === 'done') {
            clearInterval(poll);
            setExportStatus(`✅ 全量数据准备完成，正在下载...`);
            window.open(api.exportAllFile(taskId), '_blank');
            setExportStatus(`✅ 已触发下载`);
          } else if (status.status === 'error') {
            clearInterval(poll);
            setExportStatus(`❌ 下载失败: ${status.error}`);
          }
        } catch { clearInterval(poll); }
      }, 1000);
    } catch (e: unknown) {
      setExportStatus(`❌ ${e instanceof Error ? e.message : String(e)}`);
    }
  }

  async function handleExportHot15() {
    if (!fetchDate) { alert('请选择日期'); return; }
    setHot15Status('⏳ 正在获取热门板块数据...');
    try {
      const data = await api.exportHotSectors(fetchDate);
      const date = data.date || fetchDate;

      const rows = (data.sectors || [])
        .sort((a, b) => Math.abs(b.net) - Math.abs(a.net))
        .map(s => [s.name, s.net.toFixed(2), date, s.net > 0 ? '↑ 净流入' : '↓ 净流出']);
      downloadCSV(`热门板块_${date}.csv`, ['板块名称', '主力资金净流入(亿)', '时间', '趋势'], rows);

      setHot15Status(`✅ 已下载 ${rows.length} 个热门板块`);
    } catch (e: unknown) {
      setHot15Status(`❌ ${e instanceof Error ? e.message : String(e)}`);
    }
  }

  const safeSectorData = sectorData || [];
  const filtered = search
    ? safeSectorData.filter(s => s.name.includes(search))
    : safeSectorData;
  const sorted = [...filtered].sort((a, b) => Math.abs(b.net) - Math.abs(a.net));

  const inflowCount = safeSectorData.filter(s => s.net > 0).length;
  const outflowCount = safeSectorData.length - inflowCount;

  return (
    <div>
      <div className="card">
        <h2>下载CSV</h2>
        <div style={{ display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
          <button className="btn btn-sm" onClick={handleExportHot15} style={{ borderColor: '#4ade80', color: '#4ade80' }}>🔥 下载热门板块</button>
          <button className="btn btn-sm" onClick={handleExportAll} style={{ borderColor: '#4a90d9', color: '#4a90d9' }}>📥 下载全量CSV</button>
          <span style={{ fontSize: 13, color: '#8892a4' }}>日期: {fetchDate}</span>
        </div>
        {exportStatus && <div style={{ marginTop: 8, fontSize: 13, color: '#8892a4' }}>{exportStatus}</div>}
        {hot15Status && <div style={{ marginTop: 4, fontSize: 13, color: '#8892a4' }}>{hot15Status}</div>}
      </div>

      <div className="card">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 8 }}>
          <h2>板块数据</h2>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <span style={{ fontSize: 13, color: '#8892a4' }}>
              {sectorData.length > 0 ? `净流入 ${inflowCount} · 净流出 ${outflowCount}` : ''}
            </span>
            <input type="text" placeholder="搜索板块..." style={{ width: 140, padding: '6px 10px', fontSize: 13 }} value={search} onChange={e => setSearch(e.target.value)} />
          </div>
        </div>
        <div className="table-wrap" style={{ marginTop: 12 }}>
          <table>
            <thead><tr><th>板块</th><th style={{ color: '#4a90d9' }}>主力资金净流入(亿)</th><th>趋势</th></tr></thead>
            <tbody>
              {sorted.length === 0 ? (
                <tr><td colSpan={3} style={{ textAlign: 'center', color: '#8892a4' }}>请先拉取数据</td></tr>
              ) : sorted.map((s, i) => (
                <tr key={i}>
                  <td><strong>{s.name}</strong></td>
                  <td style={{ color: netColor(s.net), fontWeight: 600 }}>{fmtNet(s.net)}</td>
                  <td className={trendClass(s.net)}>{trendInfo(s.net)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div className="card">
        <h2>已有数据日期</h2>
        <div className="date-grid">
          {dates.length === 0 ? (
            <div style={{ color: '#8892a4', fontSize: 13, padding: 8 }}>暂无数据</div>
          ) : (dates || []).map(d => (
            <div key={d.date} className={`date-item ${selectedDate === d.date ? 'selected' : ''}`} onClick={() => selectDate(d.date)}>
              <div className="d">{d.date}</div>
              <div className="m">{d.sector_count}板块 · {(d.文案_count || 0) + (d.ai_count || 0)}文案 · {(d.videos || []).length}视频</div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
