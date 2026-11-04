package rendercheck

import "testing"

// 取自真实 ffmpeg 输出，含交错出现的编码进度行。
const silenceSample = `ffmpeg version 7.1
Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'output/2026-08-14/全天_tick_tv.mp4':
  Duration: 00:03:11.50, start: 0.000000, bitrate: 1219 kb/s
[Parsed_silencedetect_0 @ 0x9b4c09e00] silence_start: 6.95675
[Parsed_silencedetect_0 @ 0x9b4c09e00] silence_end: 7.956479 | silence_duration: 0.999729
frame= 1269 fps=0.0 q=-0.0 size=N/A time=00:00:42.30 bitrate=N/A speed=  84x
[Parsed_silencedetect_0 @ 0x9b4c09e00] silence_start: 44.668021
[Parsed_silencedetect_0 @ 0x9b4c09e00] silence_end: 74.214417 | silence_duration: 29.546396
[Parsed_silencedetect_0 @ 0x9b4c09e00] silence_start: 190.0
`

const frameStatsSample = `frame:0    pts:0       pts_time:0
lavfi.signalstats.YMIN=16
lavfi.signalstats.YAVG=16
lavfi.signalstats.YMAX=16
lavfi.signalstats.SATAVG=0
frame:1    pts:512     pts_time:0.5
lavfi.signalstats.YMIN=15
lavfi.signalstats.YAVG=20.8262
lavfi.signalstats.YMAX=255
lavfi.signalstats.SATAVG=8.83052
frame:2    pts:1024    pts_time:1
lavfi.signalstats.YMIN=11
lavfi.signalstats.YAVG=18.9882
lavfi.signalstats.YMAX=255
lavfi.signalstats.SATAVG=8.57195
`

func TestParseSilenceLog_ParsesSegmentsAndClosedTail(t *testing.T) {
	got := ParseSilenceLog(silenceSample, 191.5)

	if len(got) != 3 {
		t.Fatalf("segments = %d, want 3: %+v", len(got), got)
	}
	if got[0].Start != 6.95675 || got[0].End != 7.956479 {
		t.Fatalf("first segment = %+v", got[0])
	}
	if d := got[1].Duration(); d < 29.5 || d > 29.6 {
		t.Fatalf("second segment duration = %.3f, want ~29.55", d)
	}
	// 没有 silence_end 的尾部静音用总时长补全。
	if got[2].End != 191.5 {
		t.Fatalf("tail segment end = %.2f, want 191.5", got[2].End)
	}
}

func TestParseSilenceLog_EmptyInputReturnsNil(t *testing.T) {
	if got := ParseSilenceLog("no silence here", 100); len(got) != 0 {
		t.Fatalf("expected no segments, got %+v", got)
	}
}

func TestParseFrameStats_ReadsLumaSatAndHighlight(t *testing.T) {
	got := ParseFrameStats(frameStatsSample)

	if len(got) != 3 {
		t.Fatalf("frames = %d, want 3", len(got))
	}
	if got[0].Luma != 16 || got[0].Sat != 0 || got[0].YMax != 16 {
		t.Fatalf("black frame = %+v", got[0])
	}
	if got[1].TimeSec != 0.5 || got[1].Luma < 20.8 || got[1].Sat < 8.8 {
		t.Fatalf("background frame = %+v", got[1])
	}
	if got[2].YMax != 255 {
		t.Fatalf("highlight = %.0f, want 255", got[2].YMax)
	}
}

func TestParseFrameStats_EmptyInputReturnsNil(t *testing.T) {
	if got := ParseFrameStats(""); len(got) != 0 {
		t.Fatalf("expected none, got %+v", got)
	}
}

func TestSpeechBlockCount_SplitsOnSilence(t *testing.T) {
	silences := []SilenceSegment{{Start: 10, End: 11}, {Start: 20, End: 21}}
	if got := SpeechBlockCount(silences, 100); got != 3 {
		t.Fatalf("blocks = %d, want 3", got)
	}
}

func TestSpeechBlockCount_IgnoresEdgeSilence(t *testing.T) {
	// 片头 0-2s 与片尾 98-100s 的留白不算独立口播段。
	silences := []SilenceSegment{{Start: 0, End: 2}, {Start: 10, End: 11}, {Start: 98, End: 100}}
	if got := SpeechBlockCount(silences, 100); got != 2 {
		t.Fatalf("blocks = %d, want 2", got)
	}
}
