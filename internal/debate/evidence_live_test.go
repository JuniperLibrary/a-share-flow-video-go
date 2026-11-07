package debate

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/a-share-flow-video-go/internal/config"
)

// 这条例外走真实模型:后置校验的价值全在"模型第一次没写算式时,返工能不能补上",
// 这一点无法用假客户端证明。默认跳过,只有显式设置环境变量时才跑:
//
//	DEBATE_LIVE_KEY=sk-... DEBATE_LIVE_BASE=https://api.kimi.com/coding \
//	DEBATE_LIVE_MODEL=k3 LIVE_AI_TEST=1 go test ./internal/debate/ -run TestEnforceEvidenceLive -v
func TestEnforceEvidenceLive(t *testing.T) {
	if os.Getenv("LIVE_AI_TEST") != "1" {
		t.Skip("需要 LIVE_AI_TEST=1 与 DEBATE_LIVE_KEY")
	}
	key := os.Getenv("DEBATE_LIVE_KEY")
	if key == "" {
		t.Skip("需要 DEBATE_LIVE_KEY")
	}
	base := os.Getenv("DEBATE_LIVE_BASE")
	if base == "" {
		base = "https://api.deepseek.com"
	}
	model := os.Getenv("DEBATE_LIVE_MODEL")
	if model == "" {
		model = "deepseek-chat"
	}

	cfg := config.AIConfig{
		APIKey:           key,
		BaseURL:          base,
		Model:            model,
		TimeoutSec:       180,
		MaxRetries:       2,
		RetryBaseDelayMs: 1000,
	}
	o := NewOrchestrator(cfg)

	state := &DebateState{
		ReportText: "贵州茅台 2026 三季报:营业收入 1332.84 亿(+1.8%),归母净利润 644.67 亿(-0.2%)," +
			"存货 613.91 亿,合同负债 20.84 亿,经营活动现金流净额 840.59 亿。",
		StockName:    "贵州茅台",
		StockCode:    "600519",
		ReportPeriod: "2026-09-30",
	}
	prompt := buildTurnPrompt(state, Risk, PhaseRebuttal, false)

	// 故意给一条"有科目、有判断、没算式、没证伪条件"的正文,模拟实跑里最常见的漏项。
	deficient := Turn{
		Text:    "存货 613.91 亿,跌价准备一旦计提就会吃掉利润,这是最大的减值敞口。",
		Speaker: Risk,
		Phase:   PhaseRebuttal,
	}
	if gap := checkEvidence(Risk, deficient.Text); gap.Empty() {
		t.Fatal("测试样本本身就合规,改样本再跑")
	}

	got := o.enforceEvidence(context.Background(), prompt, deficient, Risk, PhaseRebuttal)
	t.Logf("返工前: %s", deficient.Text)
	t.Logf("返工后: %s", got.Text)

	if gap := checkEvidence(Risk, got.Text); !gap.Empty() {
		t.Errorf("返工后仍缺项: %+v\n正文: %s", gap, got.Text)
	}
	if got.Text == deficient.Text {
		t.Error("正文没变化,返工没有真正执行")
	}
	if len(got.Citations) == 0 {
		t.Error("返工丢了 citations:引用是证据链,不能为空")
	}
}

// 裁决者走的是另一条返工提示(四段式),必须单独验证:
// 用错提示会把结论结构改坏,渲染层就取不到检验卡。
func TestEnforceEvidenceSynthesizerLive(t *testing.T) {
	if os.Getenv("LIVE_AI_TEST") != "1" {
		t.Skip("需要 LIVE_AI_TEST=1 与 DEBATE_LIVE_KEY")
	}
	key := os.Getenv("DEBATE_LIVE_KEY")
	if key == "" {
		t.Skip("需要 DEBATE_LIVE_KEY")
	}
	base := os.Getenv("DEBATE_LIVE_BASE")
	if base == "" {
		base = "https://api.deepseek.com"
	}
	model := os.Getenv("DEBATE_LIVE_MODEL")
	if model == "" {
		model = "deepseek-chat"
	}

	cfg := config.AIConfig{
		APIKey:           key,
		BaseURL:          base,
		Model:            model,
		TimeoutSec:       180,
		MaxRetries:       2,
		RetryBaseDelayMs: 1000,
	}
	o := NewOrchestrator(cfg)

	state := &DebateState{
		ReportText: "贵州茅台 2026 三季报:营业收入 1332.84 亿(+1.8%),归母净利润 644.67 亿(-0.2%)," +
			"经营活动现金流净额 840.59 亿,合同负债 20.84 亿。",
		StockName:    "贵州茅台",
		StockCode:    "600519",
		ReportPeriod: "2026-09-30",
	}
	prompt := buildTurnPrompt(state, Synthesizer, PhaseSynthesis, false)

	// 复述别人的算式时把 1.19 抄成 0.19,这是真实存在的一类错。
	deficient := Turn{
		Text:    "结论:持有。最强证据:剔除预收后 769.69÷644.67=0.19,现金覆盖仍然充足。最致命反证:渠道打款枯竭。检验卡:年报 CFO/NI 若低于 1.0,结论作废。",
		Speaker: Synthesizer,
		Phase:   PhaseSynthesis,
	}
	if gap := checkEvidence(Synthesizer, deficient.Text); len(gap.BadMath) == 0 {
		t.Fatal("样本里的算式错误没被识别,测试前提不成立")
	}

	got := o.enforceEvidence(context.Background(), prompt, deficient, Synthesizer, PhaseSynthesis)
	t.Logf("返工前: %s", deficient.Text)
	t.Logf("返工后: %s", got.Text)

	if issues := checkArithmetic(got.Text); len(issues) > 0 {
		t.Errorf("返工后仍有算式错误: %v\n正文: %s", issues, got.Text)
	}
	if got.Text == deficient.Text {
		t.Error("正文没变化,返工没有真正执行")
	}
	for _, want := range []string{"结论", "最强证据", "最致命反证", "检验"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("返工后丢了四段式中的\"%s\": %s", want, got.Text)
		}
	}
	// 返工只该改数字,不该把论据换掉:原文讨论的是"剔除预收后的 769.69",
	// 换成"840.59÷644.67 充足"就是另一条论据了,结论的说服力随之改变。
	if !strings.Contains(got.Text, "769.69") {
		t.Errorf("返工把论据换掉了,原文的 769.69 不见了: %s", got.Text)
	}
}
