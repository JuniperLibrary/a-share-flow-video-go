package tts

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"
)

// AudioSilence 是一个音频文件首尾的静音量（秒）。
//
// 实测 edge-tts 的输出普遍带 0.1–0.2s 开头静音和 0.6s 以上结尾静音。
// 时间轴如果按“文件时长”排布，这段静音会与其他设计停顿叠加，
// 让观众听到的停顿远长于设计值——这是排布模型的错误，不是参数没调好。
type AudioSilence struct {
	LeadSec float64
	TailSec float64
}

var silenceEventRe = regexp.MustCompile(`silence_(start|end):\s*([0-9.]+)`)

// silenceDetectLevel 与 rendercheck 的静音判定阈值保持一致，
// 否则“排布时认为有声、体检时认为无声”，两边永远对不上。
const silenceDetectLevel = "-35dB"

// MeasureAudioSilence 用 ffmpeg silencedetect 量出音频首尾静音。
func MeasureAudioSilence(ctx context.Context, audioPath string) (AudioSilence, error) {
	dur, err := GetAudioDurationContext(ctx, audioPath)
	if err != nil {
		return AudioSilence{}, fmt.Errorf("测量首尾静音前取时长失败: %w", err)
	}
	output, err := runCommand(ctx, 30*time.Second, "ffmpeg",
		"-hide_banner", "-nostats",
		"-i", audioPath,
		"-af", "silencedetect=noise="+silenceDetectLevel+":d=0.1",
		"-f", "null", "-")
	if err != nil {
		return AudioSilence{}, fmt.Errorf("检测 %s 首尾静音失败: %w", audioPath, err)
	}
	return parseAudioSilence(string(output), dur), nil
}

// parseAudioSilence 从 silencedetect 日志还原首尾静音。
//
// 只有紧贴文件首尾的静音才算进首尾：句中停顿属于正常语流，不能当成可回收的空白。
func parseAudioSilence(log string, dur float64) AudioSilence {
	type seg struct{ start, end float64 }

	var segs []seg
	var open *seg
	for _, m := range silenceEventRe.FindAllStringSubmatch(log, -1) {
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		switch m[1] {
		case "start":
			// 连续出现两个 start 说明上一个静音段没有闭合，以最后一次为准。
			open = &seg{start: v}
		case "end":
			if open == nil {
				continue
			}
			open.end = v
			segs = append(segs, *open)
			open = nil
		}
	}
	// 静音一直到文件末尾时没有 silence_end，用总时长补上。
	if open != nil {
		segs = append(segs, seg{start: open.start, end: dur})
	}
	if len(segs) == 0 || dur <= 0 {
		return AudioSilence{}
	}

	var out AudioSilence
	if segs[0].start <= 0.02 {
		out.LeadSec = clampSec(segs[0].end, dur)
	}
	if last := segs[len(segs)-1]; last.end >= dur-0.05 {
		out.TailSec = clampSec(dur-last.start, dur)
	}
	// 整段都是静音时首尾会互相吃掉，宁可当成“没有可回收静音”，
	// 也不要算出负的可听时长。
	if out.LeadSec+out.TailSec > dur {
		out.LeadSec = 0
		out.TailSec = clampSec(dur, dur)
	}
	return out
}

func clampSec(v, dur float64) float64 {
	if v < 0 {
		return 0
	}
	if v > dur {
		return dur
	}
	return v
}

// SilenceToFrames 把首尾静音秒数换算成帧，向上取整保证不会切到语音。
func SilenceToFrames(s AudioSilence, fps int) (lead, tail int) {
	if fps <= 0 {
		return 0, 0
	}
	lead = int(math.Floor(s.LeadSec * float64(fps)))
	tail = int(math.Floor(s.TailSec * float64(fps)))
	return lead, tail
}
