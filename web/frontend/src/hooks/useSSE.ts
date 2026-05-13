import { useState, useCallback, useRef } from 'react';
import type { SSEMessage } from '../types';

interface UseSSEReturn {
  logs: string[];
  progress: string;
  isRunning: boolean;
  startStream: (fetchFn: () => Promise<Response>, onData?: (msg: SSEMessage) => void) => Promise<void>;
}

export function useSSE(): UseSSEReturn {
  const [logs, setLogs] = useState<string[]>([]);
  const [progress, setProgress] = useState('');
  const [isRunning, setIsRunning] = useState(false);
  const abortRef = useRef(false);

  const startStream = useCallback(async (fetchFn: () => Promise<Response>, onData?: (msg: SSEMessage) => void) => {
    setLogs([]);
    setProgress('');
    setIsRunning(true);
    abortRef.current = false;

    try {
      const res = await fetchFn();
      if (!res.body) throw new Error('No response body');

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buf = '';

      while (true) {
        if (abortRef.current) break;
        const { done, value } = await reader.read();
        if (done) break;

        buf += decoder.decode(value, { stream: true });
        const lines = buf.split('\n');
        buf = lines.pop() || '';

        for (const line of lines) {
          if (!line.trim()) continue;
          try {
            const msg: SSEMessage = JSON.parse(line);
            if (msg.type === 'log' || msg.type === 'progress') {
              setLogs(prev => [...prev, msg.text]);
              if (msg.type === 'progress') setProgress(msg.text);
            } else if (msg.type === 'data') {
              setLogs(prev => [...prev, '数据加载完成']);
            } else if (msg.type === 'done') {
              setProgress(msg.text);
            } else if (msg.type === 'error') {
              throw new Error(msg.text);
            }
            onData?.(msg);
          } catch { void 0; }
        }
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      setLogs(prev => [...prev, `❌ ${msg}`]);
      throw e;
    } finally {
      setIsRunning(false);
    }
  }, []);

  return { logs, progress, isRunning, startStream };
}
