import { useState, useEffect } from 'react';
import { api } from '../api';

export function ConfigPage() {
  const [apiKey, setApiKey] = useState('');
  const [apiBase, setApiBase] = useState('https://api.openai.com/v1');
  const [model, setModel] = useState('gpt-4o-mini');
  const [hasKey, setHasKey] = useState(false);
  const [status, setStatus] = useState('');

  useEffect(() => { loadConfig(); }, []);

  async function loadConfig() {
    try {
      const cfg = await api.getConfig();
      setHasKey(cfg.has_api_key);
      setApiBase(cfg.api_base);
      setModel(cfg.model);
      setApiKey(cfg.has_api_key ? '••••••' : '');
    } catch { void 0; }
  }

  async function handleSave() {
    try {
      const key = apiKey === '••••••' ? '' : apiKey;
      await api.saveConfig(key, apiBase, model);
      setStatus('✅ 已保存（.env）');
      loadConfig();
    } catch (e: unknown) {
      setStatus(`❌ 保存失败: ${e instanceof Error ? e.message : String(e)}`);
    }
  }

  return (
    <div>
      <div className="card">
        <h2>API 配置</h2>
        <div className="form-row">
          <div className="form-group" style={{ flex: 2 }}>
            <label>API Key (OpenAI 协议)</label>
            <input type="password" placeholder={hasKey ? '已设置 (••••••)' : 'sk-...'} value={apiKey} onChange={e => setApiKey(e.target.value)} style={{ width: '100%' }} />
          </div>
        </div>
        <div className="form-row" style={{ marginTop: 12 }}>
          <div className="form-group" style={{ flex: 1 }}>
            <label>接口地址</label>
            <input type="text" value={apiBase} onChange={e => setApiBase(e.target.value)} />
          </div>
          <div className="form-group" style={{ flex: 1 }}>
            <label>模型</label>
            <input type="text" value={model} onChange={e => setModel(e.target.value)} />
          </div>
        </div>
        <div style={{ marginTop: 16 }}>
          <button className="btn btn-primary" onClick={handleSave}>💾 保存配置</button>
          {status && <span style={{ marginLeft: 12, fontSize: 13, color: '#8892a4' }}>{status}</span>}
        </div>
      </div>

      <div className="card">
        <h2>当前设置</h2>
        <div style={{ fontSize: 13, lineHeight: 1.8, color: '#8892a4' }}>
          <div>🤖 AI状态：{hasKey ? '✅ 已配置' : '❌ 未配置'}</div>
          <div>🔗 API地址：{apiBase}</div>
          <div>🧠 模型：{model}</div>
          <div>📊 行业板块：净流入 TOP10 + 净流出 TOP10</div>
        </div>
      </div>
    </div>
  );
}
