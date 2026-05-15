export interface Sector {
  name: string;
  net: number;
  color: string;
  source: string;
  rank_group: string;
  is_auto_fill: boolean;
}

export interface DateItem {
  date: string;
  videos: string[];
  sector_count: number;
  文案_count: number;
  ai_count: number;
}

export interface ConfigData {
  has_api_key: boolean;
  api_base: string;
  model: string;
  sessions: Record<string, string>;
}

export interface SchedulerStatus {
  enabled: boolean;
  run_time: string;
  morning_run_time: string;
  last_run: string;
  last_morning_run: string;
  last_status: string;
  next_run: string;
  next_morning_run: string;
  is_running: boolean;
}

export interface SSEMessage {
  type: string;
  text: string;
}
