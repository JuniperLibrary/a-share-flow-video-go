package tick

import (
	"strings"
	"testing"
)

func TestComputeChartNarrationScheduleEnd_RespectsLateAnchors(t *testing.T) {
	startFrames := []int{0, 160, 280}
	audioFrames := []int{80, 80, 150}

	got := computeChartNarrationScheduleEnd(startFrames, audioFrames)
	want := 430
	if got != want {
		t.Fatalf("computeChartNarrationScheduleEnd() = %d, want %d", got, want)
	}
}

func TestComputeChartNarrationScheduleEnd_QueuesWhenSegmentsOverlap(t *testing.T) {
	startFrames := []int{0, 40, 90}
	audioFrames := []int{80, 80, 60}

	got := computeChartNarrationScheduleEnd(startFrames, audioFrames)
	want := 220
	if got != want {
		t.Fatalf("computeChartNarrationScheduleEnd() = %d, want %d", got, want)
	}
}

func TestPolishOutroText_Fix4DetailPatched(t *testing.T) {
	raw := "明天  你就盯  两件事：先优先看国产芯片放量承接，再 看 AI 应用流出减仓观望，再回避  高位接力飞刀。先盯承接。再看流出收敛。再回避接力飞刀。明天开盘见。"
	got := polishOutroText(raw)

	if strings.Contains(got, "先优先") {
		t.Fatalf("still contains duplicate priority: %q", got)
	}
	if strings.Contains(got, "  ") {
		t.Fatalf("still contains internal double spaces: %q", got)
	}
	if strings.Contains(got, "盯  ") {
		t.Fatalf("still contains space before action phrase: %q", got)
	}
	if !strings.Contains(got, "放量，承接") && !strings.Contains(got, "减仓，观望") {
		t.Fatalf("expected double-action comma-separated, got: %q", got)
	}
	if strings.Contains(got, "放量承接") && strings.Contains(got, "减仓观望") {
		t.Fatalf("double-action phrases still hard-joined without comma: %q", got)
	}
	_ = raw
}
