package rendercheck

import "testing"

// 真实成片的画面统计：深色品牌底 Y≈20、SAT≈8.6，且有高光粒子（YMAX=255）。
func brandBackgroundFrame(t float64) FrameStat {
	return FrameStat{TimeSec: t, Luma: 20.0, Sat: 8.6, YMax: 255}
}

// 真黑屏：限制范围下纯黑 Y=16，色度中性，无高光。
func pureBlackFrame(t float64) FrameStat {
	return FrameStat{TimeSec: t, Luma: 16, Sat: 0, YMax: 16}
}

func mobileMedia() MediaInfo {
	return MediaInfo{
		Path:       "output/2026-08-20/全天_tick_mobile.mp4",
		VideoSec:   130.03,
		AudioSec:   130.09,
		HasAudio:   true,
		AudioCodec: "aac",
		Width:      1080,
		Height:     1920,
		FPS:        30,
	}
}

func tvMedia() MediaInfo {
	return MediaInfo{
		Path:       "output/2026-08-14/全天_tick_tv.mp4",
		VideoSec:   191.50,
		AudioSec:   191.55,
		HasAudio:   true,
		AudioCodec: "aac",
		Width:      1920,
		Height:     1080,
		FPS:        30,
	}
}

func hasRule(r Report, rule string) bool {
	for _, v := range r.Violations {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

// 2026-08-20 mobile 实测：12 段静音共 12.7s（9.8%），应判绿。
func TestEvaluate_MobileDeliveredVideoPasses(t *testing.T) {
	obs := Observation{
		Silences:   sampleSilences(12, 1.05, 130.03),
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(130),
	}

	r := Evaluate(mobileMedia(), obs, DefaultSpec(FormatMobile))

	if !r.OK {
		t.Fatalf("expected pass, got violations: %+v", r.Violations)
	}
	if r.Metrics.SilenceRatio > 0.25 {
		t.Fatalf("silence ratio = %.3f, want <= 0.25", r.Metrics.SilenceRatio)
	}
}

// 2026-08-14 TV 实测：12 段静音共 68.5s（35.8%），含 29.5s、10.0s、19.5s 三段长空洞。
func TestEvaluate_TVDeliveredVideoFailsSilence(t *testing.T) {
	obs := Observation{
		Silences: append([]SilenceSegment{
			{Start: 44.67, End: 74.21}, // 29.54s
			{Start: 81.18, End: 91.22}, // 10.03s
		}, sampleSilences(10, 2.89, 191.50)...),
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(191),
	}

	r := Evaluate(tvMedia(), obs, DefaultSpec(FormatTV))

	if r.OK {
		t.Fatal("expected failure for tv silence, got pass")
	}
	if !hasRule(r, RuleSilenceRatio) || !hasRule(r, RuleSilenceGap) {
		t.Fatalf("expected silence_ratio + silence_gap, got %+v", r.Violations)
	}
	if got := r.Metrics.SilenceCount; got != 12 {
		t.Fatalf("silence count = %d, want 12", got)
	}
	if got := r.Metrics.LongestSilenceSec; got < 29 || got > 30 {
		t.Fatalf("longest silence = %.2fs, want ~29.5s", got)
	}
}

// 回归：深色品牌底曾被 pix_th=0.10 的 blackdetect 误判为 18s 黑屏。
func TestEvaluate_BrandBackgroundIsNotBlank(t *testing.T) {
	spec := DefaultSpec(FormatMobile)
	if spec.IsBlank(brandBackgroundFrame(0)) {
		t.Fatal("深色品牌底不能判为黑屏")
	}
	if !spec.IsBlank(pureBlackFrame(0)) {
		t.Fatal("纯黑帧必须判为黑屏")
	}
}

func TestBlankSegments_MergesConsecutiveBlackFrames(t *testing.T) {
	spec := DefaultSpec(FormatMobile)
	var stats []FrameStat
	for i := 0; i < 2; i++ {
		stats = append(stats, pureBlackFrame(float64(i)*0.5))
	}
	for i := 2; i < 10; i++ {
		stats = append(stats, brandBackgroundFrame(float64(i)*0.5))
	}

	got := BlankSegments(stats, spec, 0.5)

	if len(got) != 1 {
		t.Fatalf("segments = %+v, want 1", got)
	}
	if d := got[0].Duration(); d < 0.9 || d > 1.1 {
		t.Fatalf("segment duration = %.2f, want ~1.0", d)
	}
}

func TestBlankSegments_NoBlanksReturnsNil(t *testing.T) {
	spec := DefaultSpec(FormatMobile)
	var stats []FrameStat
	for i := 0; i < 10; i++ {
		stats = append(stats, brandBackgroundFrame(float64(i)*0.5))
	}

	if got := BlankSegments(stats, spec, 0.5); len(got) != 0 {
		t.Fatalf("expected no blanks, got %+v", got)
	}
}

func TestEvaluate_BlankFrameOverLimitFails(t *testing.T) {
	obs := Observation{
		Blanks:     []BlankSegment{{Start: 12, End: 12.8}},
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(130),
	}

	r := Evaluate(mobileMedia(), obs, DefaultSpec(FormatMobile))

	if !hasRule(r, RuleBlankFrame) {
		t.Fatalf("expected rule %s, got %+v", RuleBlankFrame, r.Violations)
	}
}

func TestEvaluate_ShortBlankTolerated(t *testing.T) {
	obs := Observation{
		Blanks:     []BlankSegment{{Start: 12, End: 12.2}},
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(130),
	}

	r := Evaluate(mobileMedia(), obs, DefaultSpec(FormatMobile))

	if hasRule(r, RuleBlankFrame) {
		t.Fatalf("0.2s blank should be tolerated, got %+v", r.Violations)
	}
}

func TestEvaluate_LongSilenceGapFails(t *testing.T) {
	obs := Observation{
		Silences:   []SilenceSegment{{Start: 10, End: 12.6}},
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(130),
	}

	r := Evaluate(mobileMedia(), obs, DefaultSpec(FormatMobile))

	if !hasRule(r, RuleSilenceGap) {
		t.Fatalf("expected rule %s, got %+v", RuleSilenceGap, r.Violations)
	}
}

func TestEvaluate_TrailingSilenceNotCountedAsGap(t *testing.T) {
	media := mobileMedia()
	obs := Observation{
		Silences:   []SilenceSegment{{Start: 128.6, End: 130.03}}, // 1.43s 片尾淡出
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(130),
	}

	r := Evaluate(media, obs, DefaultSpec(FormatMobile))

	if hasRule(r, RuleSilenceGap) {
		t.Fatalf("片尾淡出不应判为口播中断: %+v", r.Violations)
	}
}

// 片尾长静音虽然不算"中断"，但必须被静音占比抓住。
func TestEvaluate_LongTrailingSilenceFailsRatio(t *testing.T) {
	media := mobileMedia()
	obs := Observation{
		Silences:   []SilenceSegment{{Start: 90, End: 130.03}}, // 40s 片尾静音
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(130),
	}

	r := Evaluate(media, obs, DefaultSpec(FormatMobile))

	if !hasRule(r, RuleSilenceRatio) {
		t.Fatalf("40s 片尾静音必须触发占比规则: %+v", r.Violations)
	}
}

func TestEvaluate_AVDurationMismatchFails(t *testing.T) {
	media := mobileMedia()
	media.AudioSec = media.VideoSec - 0.40

	r := Evaluate(media, Observation{
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(130),
	}, DefaultSpec(FormatMobile))

	if !hasRule(r, RuleAVDuration) {
		t.Fatalf("expected rule %s, got %+v", RuleAVDuration, r.Violations)
	}
}

func TestEvaluate_MissingAudioFails(t *testing.T) {
	media := mobileMedia()
	media.HasAudio = false
	media.AudioSec = 0

	r := Evaluate(media, Observation{
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(130),
	}, DefaultSpec(FormatMobile))

	if !hasRule(r, RuleAudioMissing) {
		t.Fatalf("expected rule %s, got %+v", RuleAudioMissing, r.Violations)
	}
}

func TestEvaluate_ResolutionMismatchFails(t *testing.T) {
	media := tvMedia()
	media.Width, media.Height = 640, 480

	r := Evaluate(media, Observation{
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(191),
	}, DefaultSpec(FormatTV))

	if !hasRule(r, RuleFormatMismatch) {
		t.Fatalf("expected rule %s, got %+v", RuleFormatMismatch, r.Violations)
	}
}

func TestEvaluate_BlackEdgeFrameFails(t *testing.T) {
	r := Evaluate(mobileMedia(), Observation{
		FirstFrame: pureBlackFrame(0),
		LastFrame:  brandBackgroundFrame(130),
	}, DefaultSpec(FormatMobile))

	if !hasRule(r, RuleEdgeBlank) {
		t.Fatalf("expected rule %s, got %+v", RuleEdgeBlank, r.Violations)
	}
}

func TestEvaluate_NarrationSegmentsShortFails(t *testing.T) {
	spec := DefaultSpec(FormatMobile)
	spec.NarrationSegments = 6
	obs := Observation{
		Silences:   []SilenceSegment{{Start: 30, End: 31}, {Start: 60, End: 61}},
		FirstFrame: brandBackgroundFrame(0),
		LastFrame:  brandBackgroundFrame(130),
	}

	r := Evaluate(mobileMedia(), obs, spec)

	if !hasRule(r, RuleNarrationSegments) {
		t.Fatalf("expected rule %s, got %+v", RuleNarrationSegments, r.Violations)
	}
}

// sampleSilences 造 count 段等长静音，均匀分布在 total 内。
func sampleSilences(count int, each float64, total float64) []SilenceSegment {
	out := make([]SilenceSegment, 0, count)
	step := total / float64(count+1)
	for i := 1; i <= count; i++ {
		start := step * float64(i)
		out = append(out, SilenceSegment{Start: start, End: start + each})
	}
	return out
}
