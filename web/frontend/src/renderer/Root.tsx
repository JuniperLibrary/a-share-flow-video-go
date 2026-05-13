import { Composition } from 'remotion';
import { BloombergVideo } from './BloombergVideo.tsx';

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
          typeLabel: '行业板块',
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
          typeLabel: '行业板块',
          sectors: [],
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
