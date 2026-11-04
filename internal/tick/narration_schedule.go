package tick

import (
	"math"

	"github.com/a-share-flow-video-go/internal/config"
)

// NarrationClip 是一段口播音频在时间轴上的可用信息。
//
// Frames 是文件本身的帧数，Lead/Tail 是文件首尾自带的静音。
// 排布的度量必须是“可听语音”而不是“文件时长”——否则 TTS 自带的
// 0.1s 开头 + 0.6s 结尾静音会被当成停顿叠加进设计停顿里。
type NarrationClip struct {
	Frames int
	Lead   int
	Tail   int
}

// Audible 是可听语音的帧数。
func (c NarrationClip) Audible() int {
	n := c.Frames - c.Lead - c.Tail
	if n < 0 {
		return 0
	}
	return n
}

// AudibleEnd 是第 from 帧开始时，可听语音结束的位置。
// 与 Audible() 保持一致：可听时长为 0 时，结束点不早于语音起点。
func (c NarrationClip) AudibleEnd(from int) int {
	return from + c.Lead + c.Audible()
}

// ChartTimeline 是主图动画里口播排布的结果。
// Starts 与音频一一对应，单位是相对主图起点的帧（指文件起点，含开头静音）。
type ChartTimeline struct {
	ChartFrames int
	Starts      []int
	End         int
	// MaxGap 相邻两段“可听语音”之间最大的空白帧数。
	MaxGap int
	// TailGap 最后一段可听语音结束到动画结束的空白帧数，负数表示放不下。
	TailGap int
	// Sequential 为 true 表示锚点已改为顺播（不再与图表进度点一一对应）。
	Sequential bool
}

// scheduleNarration 是排布的核心：speechStart(i) 给出第 i 段“可听语音”的起始帧。
// 文件起点 = 语音起点 - 开头静音；若与上一段语音冲突，则顺延到上一段结束之后。
func scheduleNarration(speechStart func(i int) int, clips []NarrationClip, chartFrames int) ChartTimeline {
	t := ChartTimeline{ChartFrames: chartFrames}
	if len(clips) == 0 {
		t.TailGap = chartFrames
		return t
	}

	audibleEnd := 0
	starts := make([]int, 0, len(clips))
	for i, c := range clips {
		speech := speechStart(i)
		if speech < 0 {
			speech = 0
		}
		from := speech - c.Lead
		if from < 0 {
			from = 0
		}
		if from < audibleEnd {
			from = audibleEnd
		}
		// 第一段之前只剩“文件起点延迟”；段间量的是可听语音之间的真实停顿。
		gap := from
		if i > 0 {
			gap = from + c.Lead - audibleEnd
		}
		if gap > t.MaxGap {
			t.MaxGap = gap
		}
		starts = append(starts, from)
		audibleEnd = c.AudibleEnd(from)
	}
	t.Starts = starts
	t.End = audibleEnd
	t.TailGap = chartFrames - audibleEnd
	return t
}

// ScheduleChartNarration 把口播按锚点比例排进给定长度的动画。
//
// 锚点比例相对动画长度，用于在压缩/延长时保持“口播落点=图表进度点”。
// 落点对齐的是可听语音的第一帧，因此文件起点要提前开头静音。
func ScheduleChartNarration(anchorRatios []float64, clips []NarrationClip, chartFrames int) ChartTimeline {
	n := len(clips)
	if len(anchorRatios) < n {
		n = len(anchorRatios)
	}
	return scheduleNarration(func(i int) int {
		return int(math.Round(anchorRatios[i] * float64(chartFrames)))
	}, clips[:n], chartFrames)
}

// scheduleAbsoluteNarration 用不随动画长度变化的绝对锚点排布。
//
// 口播比动画长时必须用这套：按比例延长会让锚点随动画一起后移，
// 动画越长锚点越靠后，永远追不上，最后把尾段口播挤出画面。
func scheduleAbsoluteNarration(anchorFrames []int, clips []NarrationClip) ChartTimeline {
	n := len(clips)
	if len(anchorFrames) < n {
		n = len(anchorFrames)
	}
	return scheduleNarration(func(i int) int {
		return anchorFrames[i]
	}, clips[:n], 0)
}

// ScheduleSequentialNarration 口播顺播：每段可听语音之间只留 gapFrames。
// 当按进度分布的锚点无法在空白预算内排完时使用，保证没有大段无声空转。
func ScheduleSequentialNarration(clips []NarrationClip, gapFrames, tailFrames int) ChartTimeline {
	t := ChartTimeline{Sequential: true}
	if len(clips) == 0 {
		return t
	}
	if gapFrames < 0 {
		gapFrames = 0
	}

	audibleEnd := 0
	starts := make([]int, 0, len(clips))
	for i, c := range clips {
		from := 0
		if i > 0 {
			// 上一段可听结束 + 设计停顿 - 本段开头静音 = 本段文件起点。
			from = audibleEnd + gapFrames - c.Lead
			if from < 0 {
				from = 0
			}
			if gap := from + c.Lead - audibleEnd; gap > t.MaxGap {
				t.MaxGap = gap
			}
		}
		starts = append(starts, from)
		audibleEnd = c.AudibleEnd(from)
	}
	t.Starts = starts
	t.End = audibleEnd
	t.ChartFrames = audibleEnd + tailFrames
	if len(starts) > 0 && t.ChartFrames <= starts[len(starts)-1] {
		t.ChartFrames = starts[len(starts)-1] + 1
	}
	t.TailGap = t.ChartFrames - audibleEnd
	return t
}

// fitsChartGap 判断某个时间轴是否满足空白预算。
// 片尾允许比片中更长的收尾留白。
func fitsChartGap(t ChartTimeline, maxGapFrames, maxTailFrames int) bool {
	if t.TailGap < 0 {
		return false
	}
	return t.MaxGap <= maxGapFrames && t.TailGap <= maxTailFrames
}

// totalAudibleFrames 是全部可听语音的帧数，动画不能短于它。
func totalAudibleFrames(clips []NarrationClip) int {
	sum := 0
	for _, c := range clips {
		sum += c.Audible()
	}
	return sum
}

// FitChartFrames 求合适的动画帧数，使片内空白不超过 maxGapFrames、片尾留白不超过 maxTailFrames。
//
// anchorFrames 是按 baseChartFrames 长度算出的绝对锚点（图表进度点）。
// 三种情况分别处理：
//   - 口播比动画短：等比压缩锚点，保住“口播落点=图表进度点”
//   - 口播比动画长：绝对锚点延长，只加长尾巴
//   - 锚点过于稀疏：退化为顺播，宁可放弃进度对齐也不留无声空转
func FitChartFrames(anchorFrames []int, baseChartFrames int, clips []NarrationClip,
	minFrames, maxFrames, maxGapFrames, maxTailFrames int) ChartTimeline {
	if len(clips) == 0 {
		return ChartTimeline{ChartFrames: maxFrames, TailGap: maxFrames}
	}
	if minFrames <= 0 {
		minFrames = 1
	}
	if maxFrames < minFrames {
		maxFrames = minFrames
	}
	// 硬下限：动画至少要装得下全部可听语音，否则口播会被切掉。
	if total := totalAudibleFrames(clips); minFrames < total {
		minFrames = total
	}

	ratios := AnchorRatios(anchorFrames, baseChartFrames)
	proportional := func(frames int) ChartTimeline {
		return ScheduleChartNarration(ratios, clips, frames)
	}

	// 口播在最大长度下仍放不下：改用绝对锚点延长，只加长尾巴。
	// 注意不能用比例锚点延长：锚点会随动画后移，延长永远追不上口播。
	if proportional(maxFrames).TailGap < 0 {
		ext := scheduleAbsoluteNarration(anchorFrames, clips)
		tail := config.ChartNarrationExtendTailFrames
		ext.ChartFrames = ext.End + tail
		ext.TailGap = tail
		return ext
	}

	// 下限可能装不下全部语音（此时片尾为负），先抬到刚好装得下的最短长度。
	// 装不下不是“锚点太稀疏”，不能直接退顺播。
	lo, hi := minFrames, maxFrames
	if proportional(lo).TailGap < 0 {
		for lo < hi {
			mid := (lo + hi) / 2
			if proportional(mid).TailGap >= 0 {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
	}

	if !fitsChartGap(proportional(lo), maxGapFrames, maxTailFrames) {
		// 最短的可行动画都留出超额空白：锚点本身太稀疏，改顺播。
		return ScheduleSequentialNarration(clips,
			config.ChartNarrationSequentialGapFrames, config.ChartNarrationSequentialTailFrames)
	}

	// 二分：空白随动画变长而变大，找满足预算的最长动画。
	hi = maxFrames
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if fitsChartGap(proportional(mid), maxGapFrames, maxTailFrames) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return proportional(lo)
}

// AnchorRatios 把绝对锚点帧换算成相对动画长度的比例，供压缩/延长时等比还原。
func AnchorRatios(startFrames []int, chartFrames int) []float64 {
	if chartFrames <= 0 {
		return nil
	}
	ratios := make([]float64, len(startFrames))
	for i, f := range startFrames {
		ratios[i] = float64(f) / float64(chartFrames)
	}
	return ratios
}
