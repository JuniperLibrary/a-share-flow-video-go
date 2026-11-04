package rendercheck

import "fmt"

// 规则标识。报告与门禁都依赖这些稳定字符串。
const (
	RuleAudioMissing      = "audio_missing"
	RuleBlankFrame        = "blank_frame"
	RuleEdgeBlank         = "edge_blank"
	RuleAVDuration        = "av_duration"
	RuleSilenceGap        = "silence_gap"
	RuleSilenceRatio      = "silence_ratio"
	RuleFormatMismatch    = "format_mismatch"
	RuleNarrationSegments = "narration_segments"
)

// Violation 是一条被违反的不变量。
type Violation struct {
	Rule    string  `json:"rule"`
	Message string  `json:"message"`
	At      float64 `json:"at,omitempty"`
}

// Metrics 是可复核的客观指标，报告里原样输出。
type Metrics struct {
	DurationSec       float64 `json:"durationSec"`
	AudioSec          float64 `json:"audioSec"`
	AVDiffSec         float64 `json:"avDiffSec"`
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	FPS               float64 `json:"fps"`
	SilenceCount      int     `json:"silenceCount"`
	SilenceTotalSec   float64 `json:"silenceTotalSec"`
	SilenceRatio      float64 `json:"silenceRatio"`
	LongestSilenceSec float64 `json:"longestSilenceSec"`
	BlankCount        int     `json:"blankCount"`
	BlankTotalSec     float64 `json:"blankTotalSec"`
	FirstLuma         float64 `json:"firstLuma"`
	LastLuma          float64 `json:"lastLuma"`
}

// Observation 是媒体探测结果，与 Evaluate 解耦以便单测。
type Observation struct {
	Silences   []SilenceSegment
	Blanks     []BlankSegment
	FirstFrame FrameStat
	LastFrame  FrameStat
}

// Report 是一期视频的体检结论。
type Report struct {
	Path       string           `json:"path"`
	Format     Format           `json:"format"`
	OK         bool             `json:"ok"`
	Metrics    Metrics          `json:"metrics"`
	Violations []Violation      `json:"violations"`
	Silences   []SilenceSegment `json:"silences,omitempty"`
	Blanks     []BlankSegment   `json:"blanks,omitempty"`
}

// CheckWithSpec 按给定不变量体检一个视频。
func CheckWithSpec(path string, media MediaInfo, spec Spec) (Report, error) {
	silences, err := DetectSilenceIn(path, -35, 0.8, media.VideoSec)
	if err != nil {
		return Report{}, err
	}
	stats, err := DetectFrameStats(path, spec.SampleFPS)
	if err != nil {
		return Report{}, err
	}
	if len(stats) == 0 {
		return Report{}, fmt.Errorf("%s 未能采样到任何画面帧", path)
	}

	interval := 1 / spec.SampleFPS
	obs := Observation{
		Silences:   silences,
		Blanks:     BlankSegments(stats, spec, interval),
		FirstFrame: stats[0],
		LastFrame:  stats[len(stats)-1],
	}
	return Evaluate(media, obs, spec), nil
}

// CheckFile 探测并按文件名/分辨率推断形态，返回体检结论。
func CheckFile(path string) (Report, error) {
	media, err := ProbeFile(path)
	if err != nil {
		return Report{}, err
	}
	return CheckWithSpec(path, media, SpecFor(path, media))
}

// Evaluate 把媒体事实与不变量比对，产生可读结论。纯函数，不触发任何外部命令。
func Evaluate(media MediaInfo, obs Observation, spec Spec) Report {
	r := Report{
		Path:     media.Path,
		Format:   spec.Format,
		Silences: obs.Silences,
		Blanks:   obs.Blanks,
	}
	r.Metrics = buildMetrics(media, obs)

	add := func(rule, msg string, at float64) {
		r.Violations = append(r.Violations, Violation{Rule: rule, Message: msg, At: at})
	}

	if !media.HasAudio {
		add(RuleAudioMissing, "视频中没有音频流，口播必然缺失", 0)
	}

	for _, b := range obs.Blanks {
		if d := b.Duration(); d > spec.MaxBlankSec {
			add(RuleBlankFrame, fmt.Sprintf("黑屏 %.2fs（%.2fs → %.2fs），超过上限 %.2fs",
				d, b.Start, b.End, spec.MaxBlankSec), b.Start)
		}
	}
	if spec.IsBlank(obs.FirstFrame) {
		add(RuleEdgeBlank, fmt.Sprintf("首帧无内容（亮度 %.0f，饱和 %.1f，最高亮度 %.0f）",
			obs.FirstFrame.Luma, obs.FirstFrame.Sat, obs.FirstFrame.YMax), 0)
	}
	if spec.IsBlank(obs.LastFrame) {
		add(RuleEdgeBlank, fmt.Sprintf("末帧无内容（亮度 %.0f，饱和 %.1f，最高亮度 %.0f）",
			obs.LastFrame.Luma, obs.LastFrame.Sat, obs.LastFrame.YMax), media.VideoSec)
	}

	if media.HasAudio && media.AudioSec > 0 && media.VideoSec > 0 {
		if diff := media.AVDiff(); diff > spec.MaxAVDeltaSec {
			add(RuleAVDuration, fmt.Sprintf("音视频时长差 %.0fms，超过上限 %.0fms",
				diff*1000, spec.MaxAVDeltaSec*1000), 0)
		}
	}

	if media.HasAudio {
		// 片尾 0.5s 内的留白属于收尾淡出，不计入"口播中断"；
		// 但若片尾留白很长，仍会被 silence_ratio 抓住。
		const tailEdge = 0.5
		for _, s := range obs.Silences {
			d := s.Duration()
			if d <= spec.MaxSilenceGapSec {
				continue
			}
			if media.VideoSec > 0 && s.End >= media.VideoSec-tailEdge {
				continue
			}
			add(RuleSilenceGap, fmt.Sprintf("静音空洞 %.2fs（%.2fs → %.2fs），超过上限 %.2fs",
				d, s.Start, s.End, spec.MaxSilenceGapSec), s.Start)
		}
		if spec.MaxSilenceRatio > 0 && media.VideoSec > 0 {
			ratio := r.Metrics.SilenceRatio
			if ratio > spec.MaxSilenceRatio {
				add(RuleSilenceRatio, fmt.Sprintf("静音占比 %.1f%%（%.1fs / %.1fs），超过上限 %.0f%%，共 %d 段",
					ratio*100, r.Metrics.SilenceTotalSec, media.VideoSec,
					spec.MaxSilenceRatio*100, len(obs.Silences)), 0)
			}
		}
		if spec.NarrationSegments > 0 {
			blocks := SpeechBlockCount(obs.Silences, media.VideoSec)
			if blocks < spec.NarrationSegments {
				add(RuleNarrationSegments, fmt.Sprintf("口播分段缺失：检测到 %d 段，清单要求 %d 段",
					blocks, spec.NarrationSegments), 0)
			}
		}
	}

	if spec.ExpectWidth > 0 {
		if media.Width != spec.ExpectWidth || media.Height != spec.ExpectHeight {
			add(RuleFormatMismatch, fmt.Sprintf("分辨率 %dx%d，期望 %dx%d",
				media.Width, media.Height, spec.ExpectWidth, spec.ExpectHeight), 0)
		}
	}
	if spec.ExpectFPS > 0 && absF(media.FPS-spec.ExpectFPS) > spec.MaxFPSDeviation {
		add(RuleFormatMismatch, fmt.Sprintf("帧率 %.2f，期望 %.2f",
			media.FPS, spec.ExpectFPS), 0)
	}

	r.OK = len(r.Violations) == 0
	return r
}

func buildMetrics(media MediaInfo, obs Observation) Metrics {
	m := Metrics{
		DurationSec: media.VideoSec,
		AudioSec:    media.AudioSec,
		AVDiffSec:   media.AVDiff(),
		Width:       media.Width,
		Height:      media.Height,
		FPS:         media.FPS,
		FirstLuma:   obs.FirstFrame.Luma,
		LastLuma:    obs.LastFrame.Luma,
	}
	var longest float64
	for _, s := range obs.Silences {
		m.SilenceTotalSec += s.Duration()
		if d := s.Duration(); d > longest {
			longest = d
		}
	}
	for _, b := range obs.Blanks {
		m.BlankTotalSec += b.Duration()
	}
	m.SilenceCount = len(obs.Silences)
	m.LongestSilenceSec = longest
	m.BlankCount = len(obs.Blanks)
	if media.VideoSec > 0 {
		m.SilenceRatio = m.SilenceTotalSec / media.VideoSec
	}
	return m
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
