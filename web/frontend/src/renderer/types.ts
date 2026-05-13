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
  sectorType: string;
  typeLabel: string;
  sectors: SectorData[];
  events?: MarketEvent[];
  timelineEvents?: TimelineEvent[];
  tickerItems?: TickerItem[];
  format?: 'mobile' | 'tv';
  width?: number;
  height?: number;
}