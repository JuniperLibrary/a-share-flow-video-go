export interface SectorData {
  name: string;
  net: number;
  color: string;
}

export interface MarketEvent {
  event_type: string;
  frame: number;
  text: string;
  subtext: string;
  importance: number;
  time?: string;
  sector?: string;
  sentiment?: 'positive' | 'negative' | 'neutral';
}

export interface TimelineEvent {
  time: string;
  timeMinutes: number;
  sector: string;
  title: string;
  description: string;
  sentiment: 'positive' | 'negative' | 'neutral';
}

export interface TickerItem {
  time: string;
  text: string;
}

export interface BloombergVideoProps {
  dateStr: string;
  displayDate: string;
  totalFrames?: number;
  sectors: SectorData[];
  events?: MarketEvent[];
  timelineEvents?: TimelineEvent[];
  tickerItems?: TickerItem[];
  format?: 'mobile' | 'tv';
  width?: number;
  height?: number;
  session?: 'morning' | 'full';
  xLim?: [number, number];
}

// Multi-day Bar Chart Race types
export interface BarSnapshot {
  date: string;
  bars: SectorData[];
}

export interface TrendInsight {
  day: string;
  title: string;
  description: string;
  sentiment: 'positive' | 'negative' | 'neutral';
  sector: string;
}

export interface RankingChange {
  from_day: string;
  to_day: string;
  from_rank: number;
  to_rank: number;
  sector: string;
  description: string;
  sentiment: 'positive' | 'negative' | 'neutral';
}

export interface SummaryText {
  title: string;
  content: string;
  key_sectors: string[];
}

export interface MultiDayTicker {
  day: string;
  text: string;
}

export interface MultiDayAnalysis {
  trendInsights: TrendInsight[];
  rankingChanges: RankingChange[];
  summaryText: SummaryText;
  tickerItems: MultiDayTicker[];
}

export interface MultiDayVideoProps {
  dates: string[];
  fullDates: string[];
  snapshots: BarSnapshot[];
  analysis: MultiDayAnalysis;
  format: 'mobile' | 'tv';
  width: number;
  height: number;
  totalFrames: number;
}