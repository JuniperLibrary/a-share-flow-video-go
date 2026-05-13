import type { DateItem, Sector, ConfigData, SchedulerStatus, SSEMessage } from './types';

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, options);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export const api = {
  getDates: () =>
    request<{ dates: DateItem[] }>('/api/dates'),

  getData: (date: string) =>
    request<{ sectors: Sector[]; videos: string[]; copy: Record<string, Record<string, string>> }>(`/api/data/${date}`),

  fetchSectors: (date: string, force: boolean) =>
    fetch('/api/fetch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ date, force }),
    }),

  generate: (date: string, copy_mode: string, format: string) =>
    fetch('/api/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ date, copy_mode, format }),
    }),

  getConfig: () =>
    request<ConfigData>('/api/config'),

  saveConfig: (api_key: string, api_base: string, model: string) =>
    request<{ ok: boolean }>('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ api_key, api_base, model }),
    }),

  optimizeCopy: (date: string, session: string) =>
    request<{ text: string } | { error: string }>('/api/optimize-copy', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ date, session }),
    }),

  getSchedulerStatus: () =>
    request<SchedulerStatus>('/api/scheduler'),

  updateScheduler: (body: { enabled?: boolean; run_time?: string }) =>
    request<SchedulerStatus>('/api/scheduler', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),

  runSchedulerNow: () =>
    request<{ ok: boolean; message: string }>('/api/scheduler/run-now', { method: 'POST' }),

  getFiles: (date: string) =>
    request<{ videos: Record<string, string>; copy: { template: Record<string, string>; ai: Record<string, string> } }>(`/api/files/${date}`),

  exportAll: (date: string) =>
    request<{ task_id: string; status: string; progress?: string }>(`/api/export-all/${date}`),

  exportAllStatus: (taskId: string) =>
    request<{ task_id: string; status: string; progress: string; error?: string }>(`/api/export-all/status/${taskId}`),

  exportAllFile: (taskId: string) =>
    `/api/export-all/file/${taskId}`,

  exportHot15: (date: string) =>
    request<{ date: string; sectors: Sector[] }>(`/api/export-hot15/${date}`),
};
