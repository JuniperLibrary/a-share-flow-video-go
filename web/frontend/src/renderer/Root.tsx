import { Composition } from 'remotion';
import { BloombergVideo } from './BloombergVideo.tsx';
import { BloombergVideoTick } from './BloombergVideoTick.tsx';
import { MultiDayVideo } from './MultiDayVideo.tsx';

export const Root: React.FC = () => {
  return (
    <>
      <Composition
        id="BloombergVideo"
        component={BloombergVideo}
        durationInFrames={900}
        fps={30}
        width={1080}
        height={1920}
        defaultProps={{
          dateStr: '2026-05-11',
          displayDate: '05-11',
          sectors: [],
          timelineEvents: [],
          tickerItems: [],
          events: [],
          format: 'mobile',
          width: 1080,
          height: 1920,
        }}
      />
      <Composition
        id="BloombergVideoTV"
        component={BloombergVideo}
        durationInFrames={900}
        fps={30}
        width={1920}
        height={1080}
        defaultProps={{
          dateStr: '2026-05-11',
          displayDate: '05-11',
          sectors: [],
          timelineEvents: [],
          tickerItems: [],
          events: [],
          format: 'tv',
          width: 1920,
          height: 1080,
        }}
      />
      <Composition
        id="BloombergVideo3Day"
        component={MultiDayVideo}
        durationInFrames={900}
        fps={30}
        width={1080}
        height={1920}
        defaultProps={{
          dates: [],
          fullDates: [],
          snapshots: [],
          analysis: { trendInsights: [], rankingChanges: [], summaryText: { title: '', content: '', key_sectors: [] }, tickerItems: [] },
          format: 'mobile',
          width: 1080,
          height: 1920,
          totalFrames: 900,
        }}
      />
      <Composition
        id="BloombergVideo3DayTV"
        component={MultiDayVideo}
        durationInFrames={900}
        fps={30}
        width={1920}
        height={1080}
        defaultProps={{
          dates: [],
          fullDates: [],
          snapshots: [],
          analysis: { trendInsights: [], rankingChanges: [], summaryText: { title: '', content: '', key_sectors: [] }, tickerItems: [] },
          format: 'tv',
          width: 1920,
          height: 1080,
          totalFrames: 900,
        }}
      />
      <Composition
        id="BloombergVideoTick"
        component={BloombergVideoTick}
        durationInFrames={900}
        fps={30}
        width={1080}
        height={1920}
        defaultProps={{
          dateStr: '2026-05-11',
          displayDate: '05-11',
          sectorTicks: [],
          timelineEvents: [],
          tickerItems: [],
          events: [],
          format: 'mobile',
          width: 1080,
          height: 1920,
        }}
      />
      <Composition
        id="BloombergVideoTickTV"
        component={BloombergVideoTick}
        durationInFrames={900}
        fps={30}
        width={1920}
        height={1080}
        defaultProps={{
          dateStr: '2026-05-11',
          displayDate: '05-11',
          sectorTicks: [],
          timelineEvents: [],
          tickerItems: [],
          events: [],
          format: 'tv',
          width: 1920,
          height: 1080,
        }}
      />
    </>
  );
};
