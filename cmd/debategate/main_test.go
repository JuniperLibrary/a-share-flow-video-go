package main

import (
	"testing"
	"time"

	"github.com/a-share-flow-video-go/internal/debate"
)

// 门禁的判据必须和辩论内部一致:这里若漏计一轮缺项,
// 退出码就是绿的,门禁等于失效。
func TestBuildReportCountsGaps(t *testing.T) {
	script := debate.Script{
		Turns: []debate.Turn{
			// 主持人不受证据纪律约束:没有算式也不算缺项。
			{Index: 0, Phase: debate.PhaseOpen, Speaker: debate.Moderator, Text: "命题:现金流覆盖率不低于 1.1。"},
			// 合格轮。
			{Index: 1, Phase: debate.PhaseOpening, Speaker: debate.Bull,
				Text: "销售收现1435.23亿÷营收1332.84亿=1.077,增长有现金支撑。若收现比跌破1.0,此论作废。"},
			// 缺证伪条件。
			{Index: 2, Phase: debate.PhaseOpening, Speaker: debate.Bear,
				Text: "合同负债20.84÷营收1332.84=1.6%,预收近乎归零。"},
			// 算式算错:769.69÷644.67 实际 1.19。
			{Index: 3, Phase: debate.PhaseRebuttal, Speaker: debate.Bull,
				Text: "769.69÷644.67=0.84,覆盖不足。若下季回升,此论作废。"},
			// 裁决者不受约束。
			{Index: 4, Phase: debate.PhaseSynthesis, Speaker: debate.Synthesizer, Text: "结论:持有。"},
		},
		Verdict: "结论:持有。",
	}

	got := buildReport("600519", "贵州茅台", "2026-09-30", "deep", 90*time.Second, script)

	if got.Turns != 5 {
		t.Errorf("Turns = %d, 期望 5", got.Turns)
	}
	if got.MissingCount != 1 {
		t.Errorf("MissingCount = %d, 期望 1(Bear 缺证伪条件)", got.MissingCount)
	}
	if got.MathErrCount != 1 {
		t.Errorf("MathErrCount = %d, 期望 1(769.69÷644.67 写成 0.84)", got.MathErrCount)
	}
	if got.TurnDetails[3].MathIssues == nil {
		t.Error("算错的轮次必须带上具体算式,否则看日志的人不知道该改哪个数")
	}
	if got.ElapsedSec != 90 {
		t.Errorf("ElapsedSec = %d, 期望 90", got.ElapsedSec)
	}
	if got.VerdictChars != len([]rune(script.Verdict)) {
		t.Errorf("VerdictChars = %d", got.VerdictChars)
	}
}

// 全合格时必须全部计零,避免"哪天误判了还显示通过"。
func TestBuildReportCleanRun(t *testing.T) {
	script := debate.Script{
		Turns: []debate.Turn{
			{Index: 0, Speaker: debate.Moderator, Text: "命题:覆盖率不低于 1.1。"},
			{Index: 1, Speaker: debate.Bull, Text: "840.59-70.9=769.69亿。若跌破1.0,此论作废。"},
		},
	}
	got := buildReport("600519", "贵州茅台", "2026-09-30", "quick", time.Minute, script)
	if got.MissingCount != 0 || got.MathErrCount != 0 {
		t.Errorf("合格产出被误判: 缺项 %d, 算式错 %d", got.MissingCount, got.MathErrCount)
	}
}

// 裁决段的无出处金额必须计入失败:造数比算错更危险,
// 而它不会表现为"缺项"或"算式错",漏计就等于门禁放行。
func TestBuildReportCountsUnverifiedVerdictAmounts(t *testing.T) {
	script := debate.Script{
		Turns: []debate.Turn{
			{Index: 0, Speaker: debate.Bull, Text: "840.59−644.67=195.92亿。若跌破1.0,此论作废。"},
			{Index: 1, Speaker: debate.Synthesizer, Text: "结论:持有。2025年报实际仅80.07亿。检验卡:若跌破1.0,作废。",
				UnverifiedAmounts: []string{"80.07亿"}},
		},
	}
	got := buildReport("600519", "贵州茅台", "2026-09-30", "deep", 90*time.Second, script)
	if got.UnverifiedCount != 1 {
		t.Errorf("UnverifiedCount = %d, 期望 1", got.UnverifiedCount)
	}
	if got.MissingCount != 0 || got.MathErrCount != 0 {
		t.Errorf("无出处金额被算进了别的判据: 缺项 %d, 算式错 %d", got.MissingCount, got.MathErrCount)
	}
	last := got.TurnDetails[len(got.TurnDetails)-1]
	if len(last.Unverified) != 1 || last.Unverified[0] != "80.07亿" {
		t.Errorf("报告没有带出具体金额: %v", last.Unverified)
	}
}
