package rendercheck

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// SilenceSegment 是一段被判定为静音的时间区间。
type SilenceSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Duration 返回该段时长（秒）。
func (s SilenceSegment) Duration() float64 { return s.End - s.Start }

// BlankSegment 是一段被判定为「画面无内容」的时间区间。
type BlankSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Duration 返回该段时长（秒）。
func (b BlankSegment) Duration() float64 { return b.End - b.Start }

// FrameStat 是单帧的客观统计量。
type FrameStat struct {
	TimeSec float64 `json:"timeSec"`
	Luma    float64 `json:"luma"`
	Sat     float64 `json:"sat"`
	YMax    float64 `json:"yMax"`
}

var (
	silenceStartRe = regexp.MustCompile(`silence_start:\s*(-?[0-9.]+)`)
	silenceEndRe   = regexp.MustCompile(`silence_end:\s*([0-9.]+)`)
	ptsTimeRe      = regexp.MustCompile(`pts_time:([0-9.]+)`)
)

// ParseSilenceLog 解析 ffmpeg silencedetect 的 stderr 输出。
// totalSec 用于补全一直持续到片尾、没有 silence_end 的静音段。
func ParseSilenceLog(log string, totalSec float64) []SilenceSegment {
	var (
		segments []SilenceSegment
		open     = -1.0
	)
	for _, line := range strings.Split(log, "\n") {
		if m := silenceStartRe.FindStringSubmatch(line); m != nil {
			open = parseFloat(m[1])
			if open < 0 {
				open = 0
			}
			continue
		}
		if m := silenceEndRe.FindStringSubmatch(line); m != nil {
			end := parseFloat(m[1])
			if open >= 0 && end >= open {
				segments = append(segments, SilenceSegment{Start: open, End: end})
			}
			open = -1
		}
	}
	if open >= 0 && totalSec > open {
		segments = append(segments, SilenceSegment{Start: open, End: totalSec})
	}
	return segments
}

// ParseFrameStats 解析 `signalstats,metadata=print` 的输出。
func ParseFrameStats(log string) []FrameStat {
	var (
		out []FrameStat
		cur *FrameStat
	)
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(log, "\n") {
		if m := ptsTimeRe.FindStringSubmatch(line); m != nil {
			flush()
			cur = &FrameStat{TimeSec: parseFloat(m[1])}
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case strings.Contains(line, "signalstats.YAVG="):
			cur.Luma = valueAfterEq(line)
		case strings.Contains(line, "signalstats.SATAVG="):
			cur.Sat = valueAfterEq(line)
		case strings.Contains(line, "signalstats.YMAX="):
			cur.YMax = valueAfterEq(line)
		}
	}
	flush()
	return out
}

// BlankSegments 把连续的黑屏帧合并成时间段。interval 为采样间隔（秒）。
func BlankSegments(stats []FrameStat, spec Spec, interval float64) []BlankSegment {
	var (
		segs  []BlankSegment
		start = -1.0
	)
	for i, s := range stats {
		if spec.IsBlank(s) {
			if start < 0 {
				start = s.TimeSec
			}
			continue
		}
		if start >= 0 {
			segs = append(segs, BlankSegment{Start: start, End: stats[i-1].TimeSec + interval})
			start = -1
		}
	}
	if start >= 0 {
		segs = append(segs, BlankSegment{Start: start, End: stats[len(stats)-1].TimeSec + interval})
	}
	return segs
}

// DetectSilenceIn 在已知视频总时长时解析静音段，可补全片尾静音。
func DetectSilenceIn(path string, noiseDB, minDurSec, totalSec float64) ([]SilenceSegment, error) {
	filter := fmt.Sprintf("silencedetect=noise=%.0fdB:d=%.2f", noiseDB, minDurSec)
	cmd := exec.Command("ffmpeg", "-hide_banner", "-nostats",
		"-i", path, "-af", filter, "-f", "null", "-")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg silencedetect %s: %w", path, err)
	}
	return ParseSilenceLog(string(out), totalSec), nil
}

// DetectFrameStats 按 fps 采样整片，返回逐帧统计量。
func DetectFrameStats(path string, fps float64) ([]FrameStat, error) {
	filter := fmt.Sprintf("fps=%.2f,signalstats,metadata=print:file=-", fps)
	out, err := exec.Command("ffmpeg", "-hide_banner", "-nostats",
		"-i", path, "-vf", filter, "-an", "-f", "null", "-").Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg signalstats %s: %w", path, err)
	}
	return ParseFrameStats(string(out)), nil
}

// SpeechBlockCount 依据静音段推算口播块数：静音之间以及首尾的语音区块。
// 距首尾不足 edge 秒的静音不计为分隔，避免把片头留白算成一段口播。
func SpeechBlockCount(silences []SilenceSegment, totalSec float64) int {
	const edge = 0.2
	blocks := 0
	cursor := 0.0
	for _, s := range silences {
		if s.Start > cursor+edge {
			blocks++
		}
		if s.End > cursor {
			cursor = s.End
		}
	}
	if totalSec > cursor+edge {
		blocks++
	}
	return blocks
}

func valueAfterEq(line string) float64 {
	_, v, ok := strings.Cut(line, "=")
	if !ok {
		return 0
	}
	return parseFloat(v)
}
