package tick

import "testing"

const (
	testMaxGap  = 75  // 2.5s
	testMaxTail = 150 // 5s
)

// 实测的 TTS 首尾静音：开头约 0.14s、结尾约 0.63s（@30fps → 4 / 19 帧）。
const (
	testLead = 4
	testTail = 19
)

// clip 构造一段“audibleFrames 帧语音 + TTS 自带首尾静音”的口播。
func clip(audibleFrames int) NarrationClip {
	return NarrationClip{Frames: audibleFrames + testLead + testTail, Lead: testLead, Tail: testTail}
}

func clipsOf(audibleFrames int, n int) []NarrationClip {
	out := make([]NarrationClip, n)
	for i := range out {
		out[i] = clip(audibleFrames)
	}
	return out
}

// TV 版真实锚点（2026-08-14 实测）：5 段口播落在图表 0% / 40% / 70% / 73% / 73%，
// 后三段挤在同一位置，90s 动画下留下 29.5 秒无声——与成片实测 29.55s 一致。
func tvAnchors() []int {
	return []int{0, 1081, 1890, 1969, 1969}
}

// 诊断复现：90s 动画 + 真实锚点，必然出现 20s 以上的无声空转。
func TestScheduleChartNarration_ReproducesTVDeadAir(t *testing.T) {
	ratios := AnchorRatios(tvAnchors(), 2700)

	got := ScheduleChartNarration(ratios, clipsOf(200, 5), 2700)

	if got.MaxGap < 600 {
		t.Fatalf("maxGap = %d 帧（%.1fs），预期在 90s 动画下出现 20s 以上空转",
			got.MaxGap, float64(got.MaxGap)/30)
	}
}

// 修复目标：稀疏锚点改用顺播，片内停顿等于设计值，片尾等于设计值。
func TestFitChartFrames_SparseAnchorsFallBackToSequential(t *testing.T) {
	got := FitChartFrames(tvAnchors(), 2700, clipsOf(200, 5), 915, 2715, testMaxGap, testMaxTail)

	if !got.Sequential {
		t.Fatalf("稀疏锚点应回退顺播，实际仍是比例锚点：%+v", got)
	}
	if got.MaxGap != 15 {
		t.Fatalf("maxGap = %d 帧（%.2fs），want 15 帧（0.5s）", got.MaxGap, float64(got.MaxGap)/30)
	}
	if got.TailGap != 30 {
		t.Fatalf("tailGap = %d 帧，want 30 帧（1.0s）", got.TailGap)
	}
	// 动画长度 = 首段开头静音 + 5 段可听语音 + 4 个 0.5s 停顿 + 1s 片尾。
	if want := testLead + 5*200 + 4*15 + 30; got.ChartFrames != want {
		t.Fatalf("chart frames = %d，want %d", got.ChartFrames, want)
	}
	if got.ChartFrames >= 2715*2/3 {
		t.Fatalf("chart frames = %d，压缩幅度不足", got.ChartFrames)
	}
}

// 这条锁住真正的模型错误：按“文件时长”排布会让 TTS 自带的结尾静音叠加成额外停顿。
func TestScheduleSequentialNarration_TailSilenceIsNotCounted(t *testing.T) {
	clips := clipsOf(200, 2)

	got := ScheduleSequentialNarration(clips, 15, 30)

	// 文件级排布会得到 19(尾静音) + 15(停顿) + 4(头静音) = 38 帧（1.27s），
	// 再加上旧代码每段补的 6 帧补白就是 44 帧（1.47s）——正是成片里量到的 1.5s。
	if got.MaxGap != 15 {
		t.Fatalf("可听语音间隔 = %d 帧（%.2fs），want 15 帧（0.5s）",
			got.MaxGap, float64(got.MaxGap)/30)
	}
	if want := clips[0].AudibleEnd(0) + 15 - clips[1].Lead; got.Starts[1] != want {
		t.Fatalf("第二段文件起点 = %d，want %d", got.Starts[1], want)
	}
	// 文件起点间隔必须小于文件时长，否则说明结尾静音又被算进停顿了。
	if spacing := got.Starts[1] - got.Starts[0]; spacing >= clips[0].Frames {
		t.Fatalf("文件起点间隔 %d 不小于文件时长 %d，结尾静音被计入停顿", spacing, clips[0].Frames)
	}
}

// 首尾静音缺失时（量不到）退化为按文件时长排布，不能算出负的可听时长。
func TestNarrationClip_AudibleClampsAtZero(t *testing.T) {
	c := NarrationClip{Frames: 10, Lead: 8, Tail: 8}

	if got := c.Audible(); got != 0 {
		t.Fatalf("Audible() = %d，want 0", got)
	}
	// 语音起点在 100+Lead，可听时长为 0 时结束点就是起点本身。
	if got := c.AudibleEnd(100); got != 108 {
		t.Fatalf("AudibleEnd() = %d，want 108（= 起点 + 开头静音）", got)
	}
}

// 锚点本身排得开时，保持比例锚点以对齐图表进度，且不做无谓压缩。
func TestFitChartFrames_DenseAnchorsKeepProportional(t *testing.T) {
	anchors := []int{0, 297, 594}

	got := FitChartFrames(anchors, 900, clipsOf(290, 3), 600, 900, testMaxGap, testMaxTail)

	if got.Sequential {
		t.Fatal("锚点足够密时不应放弃进度对齐")
	}
	if got.ChartFrames != 900 {
		t.Fatalf("chart frames = %d，预期保持 900", got.ChartFrames)
	}
	if got.MaxGap > testMaxGap {
		t.Fatalf("maxGap = %d 帧，超过预算", got.MaxGap)
	}
	if got.TailGap > testMaxTail {
		t.Fatalf("tailGap = %d 帧，超过预算", got.TailGap)
	}
}

// 下限装不下全部语音时，应该抬到“刚好装得下”的长度，而不是直接放弃进度对齐。
func TestFitChartFrames_RaisesFloorInsteadOfGivingUpAlignment(t *testing.T) {
	anchors := []int{0, 315, 630}

	got := FitChartFrames(anchors, 900, clipsOf(240, 3), 600, 900, testMaxGap, testMaxTail)

	if got.Sequential {
		t.Fatal("只是下限太短，不该退化成顺播")
	}
	if got.ChartFrames <= 600 {
		t.Fatalf("chart frames = %d，未抬到装得下全部语音的长度", got.ChartFrames)
	}
	if got.TailGap < 0 {
		t.Fatalf("tailGap = %d，尾段口播被切", got.TailGap)
	}
	if got.MaxGap > testMaxGap {
		t.Fatalf("maxGap = %d 帧，超过预算", got.MaxGap)
	}
}

// mobile 真实形态（2026-08-14）：3 段口播、锚点 0/40%/70%，20s 动画装不下。
// 延长必须用绝对锚点，否则锚点随动画一起后移，尾段会被挤出画面。
func TestFitChartFrames_ExtendsWithAbsoluteAnchors(t *testing.T) {
	anchors := []int{0, 240, 420} // 600 帧图表上的 0 / 40% / 70%

	got := FitChartFrames(anchors, 600, clipsOf(200, 3), 615, 615, testMaxGap, testMaxTail)

	if got.Sequential {
		t.Fatal("锚点不稀疏，只是装不下，不该退顺播")
	}
	if got.TailGap < 0 {
		t.Fatalf("tailGap = %d，尾段口播被切", got.TailGap)
	}
	// 绝对锚点排布：0 → 204，240 → 440，420 被顺延到 440 → 644，加 6 帧收尾。
	if want := 650; got.ChartFrames != want {
		t.Fatalf("chart frames = %d，want %d（比例延长会发散到几千帧）", got.ChartFrames, want)
	}
	if got.ChartFrames > 615*2 {
		t.Fatalf("chart frames = %d，延长发散", got.ChartFrames)
	}
	for i := range got.Starts {
		if end := got.Starts[i] + testLead + 200; end > got.ChartFrames {
			t.Fatalf("第 %d 段语音结束于 %d，超出动画 %d", i, end, got.ChartFrames)
		}
	}
}

// 口播远长于动画时同样走绝对锚点延长，不能把尾巴吃掉。
func TestFitChartFrames_ExtendsWhenNarrationOverflows(t *testing.T) {
	anchors := []int{0, 1357}

	got := FitChartFrames(anchors, 2715, clipsOf(2000, 2), 915, 2715, testMaxGap, testMaxTail)

	if got.End > got.ChartFrames {
		t.Fatalf("口播结束 %d 超出动画 %d", got.End, got.ChartFrames)
	}
	if got.ChartFrames < 4000 {
		t.Fatalf("chart frames = %d，预期延长到容纳两段口播", got.ChartFrames)
	}
}

// 比例模式下，落点要对齐“可听语音的第一帧”，而不是文件起点。
func TestScheduleChartNarration_AnchorPointsAtAudibleStart(t *testing.T) {
	clips := clipsOf(240, 2)

	got := ScheduleChartNarration([]float64{0, 0.5}, clips, 1200)

	if want := 600 - clips[1].Lead; got.Starts[1] != want {
		t.Fatalf("第二段文件起点 = %d，want %d（进度点需提前开头静音）", got.Starts[1], want)
	}
}

func TestScheduleSequentialNarration_NoClips(t *testing.T) {
	got := ScheduleSequentialNarration(nil, 15, 30)
	if got.ChartFrames != 0 || len(got.Starts) != 0 {
		t.Fatalf("unexpected timeline: %+v", got)
	}
}

func TestScheduleChartNarration_OverlappingAnchorsQueueUp(t *testing.T) {
	// 锚点比上一段可听语音还靠前时，顺延到上一段语音结束之后，不允许重叠。
	clips := clipsOf(300, 3)
	got := ScheduleChartNarration([]float64{0, 0.01, 0.02}, clips, 900)

	for i := 1; i < len(got.Starts); i++ {
		prevSpeechEnd := clips[i-1].AudibleEnd(got.Starts[i-1])
		if got.Starts[i] < prevSpeechEnd {
			t.Fatalf("第 %d 段起点 %d 早于上一段语音结束 %d", i, got.Starts[i], prevSpeechEnd)
		}
	}
	// 顺延时能压出的最小停顿就是下一段的开头静音，不可能更低。
	if got.MaxGap > testLead {
		t.Fatalf("maxGap = %d 帧，want ≤ %d（开头静音长度）", got.MaxGap, testLead)
	}
}

func TestScheduleChartNarration_NoClipsKeepsChartLength(t *testing.T) {
	got := ScheduleChartNarration(nil, nil, 2700)

	if got.ChartFrames != 2700 || got.End != 0 || got.TailGap != 2700 {
		t.Fatalf("unexpected timeline: %+v", got)
	}
}

func TestAnchorRatios_NormalizesToUnit(t *testing.T) {
	got := AnchorRatios([]int{0, 600, 1200}, 2400)
	want := []float64{0, 0.25, 0.5}

	for i := range want {
		if diff := got[i] - want[i]; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("ratio[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// 端到端形态的体检：真实锚点 + 真实首尾静音，整个时间轴的空白必须落在预算内。
func TestFitChartFrames_RealTVShapeHasNoDeadAir(t *testing.T) {
	// 2026-08-14 实测可听语音长度（帧）：6.34s / 7.02s / 6.78s / 4.61s / 4.67s。
	audibles := []int{190, 211, 203, 138, 140}
	clips := make([]NarrationClip, len(audibles))
	for i, a := range audibles {
		clips[i] = clip(a)
	}

	got := FitChartFrames(tvAnchors(), 2700, clips, 915, 2715, testMaxGap, testMaxTail)

	if got.MaxGap > 15 {
		t.Fatalf("片内最大空白 = %d 帧（%.2fs），超过设计停顿 0.5s",
			got.MaxGap, float64(got.MaxGap)/30)
	}
	if got.TailGap > 30 {
		t.Fatalf("片尾空白 = %d 帧（%.2fs），超过设计值 1.0s",
			got.TailGap, float64(got.TailGap)/30)
	}
	t.Logf("最终时间轴：动画 %.1fs，片内空白 %.2fs，片尾空白 %.2fs，顺播=%v",
		float64(got.ChartFrames)/30, float64(got.MaxGap)/30,
		float64(got.TailGap)/30, got.Sequential)
}
