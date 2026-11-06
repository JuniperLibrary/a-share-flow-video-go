package debate

import (
	"testing"

	"github.com/a-share-flow-video-go/internal/config"
)

// 改版约束:轮数从 63 压到 23,再按实跑时长压到 17。深度靠每轮的证据单元,
// 不靠堆轮数。如果有人为了"更热闹"把轮数加回去,这里会红,
// 提醒先算时长与 TTS 成本(17 轮 × 平均 120 字 ≈ 8 分钟成片)。
func TestDefaultPhasePlanTurnBudget(t *testing.T) {
	wantOrder := []Phase{
		PhaseOpen, PhaseOpening, PhaseCrossExam,
		PhaseRebuttal, PhaseFactCheck, PhaseSynthesis,
	}
	if len(defaultPhasePlan) != len(wantOrder) {
		t.Fatalf("阶段数量 = %d, 期望 %d", len(defaultPhasePlan), len(wantOrder))
	}

	total := 0
	for i, step := range defaultPhasePlan {
		if step.Phase != wantOrder[i] {
			t.Errorf("第 %d 阶段 = %s, 期望 %s", i, step.Phase, wantOrder[i])
		}
		if step.Rounds < 1 || len(step.Speakers) == 0 {
			t.Errorf("阶段 %s 配置非法: rounds=%d speakers=%d", step.Phase, step.Rounds, len(step.Speakers))
		}
		total += step.Rounds * len(step.Speakers)
	}

	if total != 17 {
		t.Errorf("总轮次 = %d, 期望 17(更深的版本:少轮次、每轮一个证据单元)", total)
	}
}

// 工具只在核实阶段开放,避免每轮都打外部接口。
func TestOnlyFactCheckAllowsTools(t *testing.T) {
	for _, step := range defaultPhasePlan {
		want := step.Phase == PhaseFactCheck
		if step.AllowTools != want {
			t.Errorf("阶段 %s AllowTools = %v, 期望 %v", step.Phase, step.AllowTools, want)
		}
	}
}

// 每轮预算必须容得下"正文 + citations"。500 token 时 qwen 系模型会 finish_reason=length,
// 返回不闭合 JSON,整场辩论作废。这里把实测下界钉住,防止有人为了省钱调回去。
func TestTurnTokenBudgetFitsEvidenceTurn(t *testing.T) {
	if turnMaxTokens < 1200 {
		t.Errorf("turnMaxTokens = %d, 证据型轮次至少需要 1200(实测 250-420 token + 余量)", turnMaxTokens)
	}
	if turnMaxTokensRetry <= turnMaxTokens {
		t.Errorf("turnMaxTokensRetry = %d, 必须大于基线 %d, 否则截断重试没有意义", turnMaxTokensRetry, turnMaxTokens)
	}
}

// thinking 模型会先用 max_tokens 推理,预算必须按协议区分,否则只返回 thinking 块。
func TestTokenBudgetsByProtocol(t *testing.T) {
	openai := NewOrchestrator(config.AIConfig{BaseURL: "https://api.openai.com/v1"})
	if b, r := openai.tokenBudgets(); b != turnMaxTokens || r != turnMaxTokensRetry {
		t.Errorf("openai 协议预算 = (%d,%d), 期望 (%d,%d)", b, r, turnMaxTokens, turnMaxTokensRetry)
	}

	kimi := NewOrchestrator(config.AIConfig{BaseURL: "https://api.kimi.com/coding"})
	if b, r := kimi.tokenBudgets(); b < 3000 || r <= b {
		t.Errorf("anthropic/thinking 预算 = (%d,%d), 基线至少 3000 且重试更大", b, r)
	}
}
