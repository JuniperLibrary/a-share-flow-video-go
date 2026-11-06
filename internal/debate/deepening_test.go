package debate

import (
	"context"
	"strings"
	"testing"
)

// 模型常返回 "Synthesizer"/"Bull" 这种首字母大写形式。不规范化会导致
// verdict 提取失败、前端角色映射串味、TTS 音色选错——实跑里 Synthesizer
// 就被写成 "Synthesizer",verdict 直接丢了。
func TestParseTurnNormalizesSpeakerAndPhase(t *testing.T) {
	raw := `{"turns":[{"index":7,"phase":"Synthesis","speaker":"Synthesizer","text":"结论:持有。","emotion":"neutral"}]}`
	turn, err := parseTurn(raw, Synthesizer, PhaseSynthesis)
	if err != nil {
		t.Fatalf("parseTurn: %v", err)
	}
	if turn.Speaker != Synthesizer {
		t.Errorf("speaker = %q, 期望规范化为 %q", turn.Speaker, Synthesizer)
	}
	if turn.Phase != PhaseSynthesis {
		t.Errorf("phase = %q, 期望规范化为 %q", turn.Phase, PhaseSynthesis)
	}
}

func TestExtractVerdictFallsBackToLastTurn(t *testing.T) {
	turns := []Turn{
		{Speaker: Bull, Text: "多头"},
		{Speaker: Bear, Text: "最后一轮"},
	}
	if got := extractVerdict(turns); got != "最后一轮" {
		t.Errorf("verdict = %q, 无 Synthesizer 时应回退最后一轮", got)
	}
	if got := extractVerdict(nil); got != "" {
		t.Errorf("空 transcript 应返回空, 实际 %q", got)
	}
}

// FactCheck 阶段真实调过工具,结果必须出现在后续轮次的 prompt 里,
// 否则那一轮白跑(实跑里 7 次工具调用的结果全部被丢弃)。
func TestBuildTurnPromptInjectsToolResults(t *testing.T) {
	state := &DebateState{
		ReportText:  "营收 1332.84 亿",
		ToolResults: "- fetch_metric: {\"operatingCashFlow\": 0}\n",
	}
	got := buildTurnPrompt(state, Synthesizer, PhaseSynthesis, false)
	if !strings.Contains(got, "已核实的工具结果") || !strings.Contains(got, "operatingCashFlow") {
		t.Error("工具结果未注入后续 prompt")
	}

	// 非工具阶段必须显式禁止 toolCalls,否则模型会在 opening 轮乱调工具。
	if !strings.Contains(got, "toolCalls 必须输出空数组") {
		t.Error("非工具阶段缺少 toolCalls 约束")
	}

	allowed := buildTurnPrompt(state, Moderator, PhaseFactCheck, true)
	if strings.Contains(allowed, "toolCalls 必须输出空数组") {
		t.Error("FactCheck 阶段不应禁止 toolCalls")
	}
}

// fetch_metric 只能查估值指标。模型拿它去核实"经营现金流/销售收现"时,
// 必须明确拒绝,否则会拿到 PE/PB 当成核实结果。
func TestFetchMetricRejectsStatementMetrics(t *testing.T) {
	tool := &fetchMetricTool{}
	for _, m := range []string{"operatingCashFlow", "sales_cash_received", "contract_liability"} {
		if _, err := tool.Execute(context.Background(), map[string]any{"stock": "600519", "metric": m}); err == nil {
			t.Errorf("metric=%s 应被拒绝", m)
		}
	}
	if !fetchMetricSupported["pe"] || !fetchMetricSupported[""] {
		t.Error("白名单应包含估值指标与空 metric")
	}
}
