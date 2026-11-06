package debate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/ai"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/debate/prompts"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

type Orchestrator struct {
	cfg    config.AIConfig
	memory DebateMemory
	tools  *ToolRegistry
}

func NewOrchestrator(cfg config.AIConfig) *Orchestrator {
	return &Orchestrator{
		cfg:    cfg,
		memory: NewDebateMemoryNoop(),
		tools:  NewToolRegistry(),
	}
}

type OrchestratorOption func(*Orchestrator)

func WithMemory(m DebateMemory) OrchestratorOption {
	return func(o *Orchestrator) { o.memory = m }
}

func NewOrchestratorWithOptions(cfg config.AIConfig, opts ...OrchestratorOption) *Orchestrator {
	o := NewOrchestrator(cfg)
	for _, opt := range opts {
		opt(o)
	}
	return o
}

type DebateState struct {
	ReportText        string
	ReportHash        string
	StockCode         string
	StockName         string
	ReportPeriod      string
	SessionID         string
	PreviousRefs      string
	StructuredMetrics string
	// ToolResults 累积 FactCheck 阶段真实调用工具的结果。
	// 必须单独存:PreviousRefs 只进内存与历史,不会进 prompt。
	ToolResults string
	Turns       []Turn
}

type PhaseStep struct {
	Phase      Phase
	Speakers   []Speaker
	AllowTools bool
	Rounds     int
}

// 每轮 token 预算。证据型轮次(60-120 字 + citations)实测 250-420 token,
// 500 会截断 JSON(max_tokens=500 时 finish_reason=length),故基线取 1500;
// 截断重试时放宽到 3000。
//
// Anthropic thinking 模型(Kimi k3 等)会先用 max_tokens 做推理,实测单轮
// thinking 就要 1500+,预算必须单独抬高,否则只返回 thinking 块、没有正文。
const (
	turnMaxTokens              = 1500
	turnMaxTokensRetry         = 3000
	turnMaxTokensThinking      = 4000
	turnMaxTokensThinkingRetry = 8000
)

// tokenBudgets 按协议返回 (基线, 重试) 预算。
func (o *Orchestrator) tokenBudgets() (int, int) {
	if ai.UsesAnthropicProtocol(o.cfg.BaseURL) {
		return turnMaxTokensThinking, turnMaxTokensThinkingRetry
	}
	return turnMaxTokens, turnMaxTokensRetry
}

// defaultPhasePlan 决定辩论的结构。
//
// 2026-11 改版:从"更大的版本"改成"更深的版本"。
// 旧计划 opening/cross_exam/rebuttal 各 4 轮,共 63 轮,每轮 20-60 字,
// 大部分是立场重申;现在每个角色每轮必须交付一个证据单元(科目->算式->判断->证伪),
// 轮数压到 23 轮,同样的时长预算换成"算得更深"而不是"说得更多"。
var defaultPhasePlan = []PhaseStep{
	{Phase: PhaseOpen, Speakers: []Speaker{Moderator}, AllowTools: false, Rounds: 1},
	{Phase: PhaseOpening, Speakers: []Speaker{Bull, Bear, Sector, Risk}, AllowTools: false, Rounds: 1},
	{Phase: PhaseCrossExam, Speakers: []Speaker{Bull, Bear, Sector, Risk}, AllowTools: false, Rounds: 2},
	{Phase: PhaseRebuttal, Speakers: []Speaker{Bull, Bear, Sector, Risk}, AllowTools: false, Rounds: 1},
	{Phase: PhaseFactCheck, Speakers: []Speaker{Moderator, Bull, Bear}, AllowTools: true, Rounds: 1},
	{Phase: PhaseClosing, Speakers: []Speaker{Bull, Bear}, AllowTools: false, Rounds: 1},
	{Phase: PhaseSynthesis, Speakers: []Speaker{Synthesizer}, AllowTools: false, Rounds: 1},
}

func (o *Orchestrator) Run(ctx context.Context, state DebateState) (Script, error) {
	if strings.TrimSpace(state.ReportText) == "" {
		return Script{}, fmt.Errorf("财报文本不能为空")
	}
	if state.SessionID == "" {
		state.SessionID = fmt.Sprintf("sess_%d", time.Now().UnixNano())
	}
	if state.StockName == "" {
		state.StockName = state.StockCode
	}

	logger.Info("辩论开始",
		zap.String("sessionId", state.SessionID),
		zap.String("stockCode", state.StockCode),
		zap.String("stockName", state.StockName),
		zap.Int("reportLen", len(state.ReportText)),
		zap.String("period", state.ReportPeriod),
	)

	if state.PreviousRefs == "" && state.StockCode != "" {
		prev, err := o.memory.GetRecentSessions(ctx, state.StockCode, 1)
		if err == nil && len(prev) > 0 && prev[0].Verdict != "" {
			state.PreviousRefs = fmt.Sprintf("[历史 verdict %s] %s", prev[0].Date, prev[0].Verdict)
			logger.Info("加载历史 verdict", zap.String("sessionId", state.SessionID), zap.String("prevVerdict", prev[0].Verdict))
		}
	}

	totalTurns := 0
	for _, step := range defaultPhasePlan {
		logger.Info("辩论阶段开始",
			zap.String("sessionId", state.SessionID),
			zap.String("phase", string(step.Phase)),
			zap.Int("speakers", len(step.Speakers)),
			zap.Int("rounds", step.Rounds),
			zap.Bool("allowTools", step.AllowTools),
		)

		for round := 0; round < step.Rounds; round++ {
			for _, speaker := range step.Speakers {
				start := time.Now()
				turn, err := o.runTurn(ctx, &state, speaker, step.Phase, step.AllowTools)
				if err != nil {
					logger.Error("辩论轮次失败",
						zap.String("sessionId", state.SessionID),
						zap.String("phase", string(step.Phase)),
						zap.String("speaker", string(speaker)),
						zap.Int("round", round+1),
						zap.Error(err),
					)
					return Script{}, fmt.Errorf("phase=%s speaker=%s 失败: %w", step.Phase, speaker, err)
				}
				// 非 FactCheck 阶段的工具调用不执行也不保留:模型常无视阶段约束照样吐
				// toolCalls,留着会让下游误以为核实过。
				if !step.AllowTools && len(turn.ToolCalls) > 0 {
					logger.Warn("非核实阶段忽略工具调用",
						zap.String("sessionId", state.SessionID),
						zap.String("phase", string(step.Phase)),
						zap.Int("dropped", len(turn.ToolCalls)),
					)
					turn.ToolCalls = nil
				}

				state.Turns = append(state.Turns, turn)
				state.PreviousRefs = updatePreviousRefs(state.PreviousRefs, turn)
				totalTurns++

				logger.Info("辩论轮次完成",
					zap.String("sessionId", state.SessionID),
					zap.Int("index", turn.Index),
					zap.String("phase", string(step.Phase)),
					zap.String("speaker", string(speaker)),
					zap.Int("round", round+1),
					zap.Int("textLen", len(turn.Text)),
					zap.Int("citations", len(turn.Citations)),
					zap.Int("toolCalls", len(turn.ToolCalls)),
					zap.Duration("latency", time.Since(start)),
				)

				if step.AllowTools && len(turn.ToolCalls) > 0 {
					results := o.executeTools(ctx, turn.ToolCalls)
					state.PreviousRefs = state.PreviousRefs + "\n[tool results]\n" + results
					state.ToolResults = state.ToolResults + results
					logger.Info("工具调用完成",
						zap.String("sessionId", state.SessionID),
						zap.Int("tools", len(turn.ToolCalls)),
					)
				}
			}
		}
	}

	verdict := extractVerdict(state.Turns)
	script := Script{
		Turns:        state.Turns,
		ReportHash:   state.ReportHash,
		StockCode:    state.StockCode,
		StockName:    state.StockName,
		SessionID:    state.SessionID,
		PreviousRefs: state.PreviousRefs,
		Verdict:      verdict,
	}

	logger.Info("辩论完成",
		zap.String("sessionId", state.SessionID),
		zap.Int("totalTurns", totalTurns),
		zap.Int("verdictLen", len(verdict)),
	)

	if err := o.memory.SaveSession(ctx, SessionEntry{
		SessionID:  state.SessionID,
		StockCode:  state.StockCode,
		StockName:  state.StockName,
		Date:       time.Now().Format("2006-01-02"),
		Verdict:    verdict,
		TurnsCount: len(state.Turns),
		Script:     script,
	}); err != nil {
		logger.Warn("保存辩论记录失败", zap.String("sessionId", state.SessionID), zap.Error(err))
	}

	return script, nil
}

func (o *Orchestrator) RunLegacy(ctx context.Context, reportText string, structuredData ...map[string]any) (Script, error) {
	logger.Info("辩论(legacy 模式)",
		zap.Int("reportLen", len(reportText)),
		zap.Int("structuredCount", len(structuredData)),
	)
	return GenerateScript(reportText, o.cfg, structuredData...)
}

func (o *Orchestrator) runTurn(ctx context.Context, state *DebateState, speaker Speaker, phase Phase, allowTools bool) (Turn, error) {
	logger.Debug("构建 prompt",
		zap.String("sessionId", state.SessionID),
		zap.String("speaker", string(speaker)),
		zap.String("phase", string(phase)),
		zap.Int("transcriptLen", len(state.Turns)),
	)

	combined := buildTurnPrompt(state, speaker, phase, allowTools)

	// 证据型轮次要求 60-120 字正文 + citations(科目/算式/期间),500 token 会把 JSON 截断,
	// 表现为 "unexpected end of JSON input"。实测(2026-11, qwen-plus/qwen-max 直连):
	// max_tokens=500 时 finish_reason=length、JSON 不闭合;1500 时 stop、约 250-420 token。
	budget, retryBudget := o.tokenBudgets()
	llmStart := time.Now()
	raw, err := callLLM(o.cfg, combined, 0.8, budget)
	if errors.Is(err, ai.ErrNoTextBlock) {
		// thinking 把预算吃光:同预算重试没意义,直接用重试预算再来一次。
		logger.Warn("thinking 占满预算,提高 max_tokens 重试",
			zap.String("sessionId", state.SessionID),
			zap.String("speaker", string(speaker)),
			zap.Int("budget", budget),
			zap.Int("retryBudget", retryBudget),
		)
		raw, err = callLLM(o.cfg, combined, 0.8, retryBudget)
	}
	llmLatency := time.Since(llmStart)
	if err != nil {
		logger.Error("LLM 调用失败",
			zap.String("sessionId", state.SessionID),
			zap.String("speaker", string(speaker)),
			zap.String("phase", string(phase)),
			zap.Duration("latency", llmLatency),
			zap.Error(err),
		)
		return Turn{}, err
	}

	logger.Debug("LLM 返回原始结果",
		zap.String("sessionId", state.SessionID),
		zap.Duration("latency", llmLatency),
		zap.Int("rawLen", len(raw)),
	)

	turn, err := parseTurn(raw, speaker, phase)
	if err != nil {
		logger.Warn("解析 LLM 输出失败",
			zap.String("sessionId", state.SessionID),
			zap.String("speaker", string(speaker)),
			zap.String("phase", string(phase)),
			zap.Int("rawLen", len(raw)),
			zap.String("rawPreview", truncateString(raw, 200)),
			zap.Error(err),
		)
		// 最常见原因是输出被 max_tokens 截断。单轮失败会让整场作废,
		// 所以用更大预算 + 更低温度重试一次,仍失败才放弃。
		raw2, err2 := callLLM(o.cfg, combined, 0.4, retryBudget)
		if err2 == nil {
			if turn2, parseErr2 := parseTurn(raw2, speaker, phase); parseErr2 == nil {
				logger.Info("解析失败后重试成功",
					zap.String("sessionId", state.SessionID),
					zap.String("speaker", string(speaker)),
					zap.Int("rawLen2", len(raw2)),
				)
				turn2.Index = len(state.Turns)
				return turn2, nil
			}
		}
		return Turn{}, err
	}
	turn.Index = len(state.Turns)
	return turn, nil
}

// buildTurnPrompt 组装单轮 prompt。抽成独立函数是为了能单测:
// 工具结果必须进后续轮次、非工具阶段必须显式禁止 toolCalls,这两条都是回归点。
func buildTurnPrompt(state *DebateState, speaker Speaker, phase Phase, allowTools bool) string {
	systemPrompt := prompts.SystemPromptFor(string(speaker))
	userPrompt := prompts.BuildUserPrompt(string(speaker), string(phase), prompts.TurnContext{
		ReportText:        state.ReportText,
		Transcript:        formatTranscript(state.Turns),
		StructuredMetrics: state.StructuredMetrics,
		StockName:         state.StockName,
		StockCode:         state.StockCode,
		ReportPeriod:      state.ReportPeriod,
		PreviousRefs:      state.PreviousRefs,
	})
	combined := systemPrompt + "\n\n" + userPrompt

	// 已核实的工具结果必须进后续 prompt,否则 FactCheck 白跑一轮。
	if strings.TrimSpace(state.ToolResults) != "" {
		combined += "\n\n## 已核实的工具结果(事实,优先于推测)\n" + state.ToolResults
	}
	if !allowTools {
		combined += "\n\n## 本阶段工具约束\n本阶段不允许调用工具,toolCalls 必须输出空数组 []。要复算就用已给的科目与工具结果。"
	}
	return combined
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func (o *Orchestrator) executeTools(ctx context.Context, calls []ToolCall) string {
	var b strings.Builder
	for _, call := range calls {
		tool, ok := o.tools.Get(call.Name)
		if !ok {
			fmt.Fprintf(&b, "- %s: 未知工具\n", call.Name)
			continue
		}
		result, err := tool.Execute(ctx, call.Args)
		if err != nil {
			fmt.Fprintf(&b, "- %s: 错误 %v\n", call.Name, err)
			continue
		}
		out, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintf(&b, "- %s: 序列化结果失败 %v\n", call.Name, err)
			continue
		}
		fmt.Fprintf(&b, "- %s: %s\n", call.Name, string(out))
	}
	return b.String()
}

func formatTranscript(turns []Turn) string {
	if len(turns) == 0 {
		return "(无 — 这是开场)"
	}
	var b strings.Builder
	for _, t := range turns {
		fmt.Fprintf(&b, "[%d] %s (%s): %s\n", t.Index, t.Speaker, t.Phase, t.Text)
	}
	return b.String()
}

func updatePreviousRefs(prev string, t Turn) string {
	line := fmt.Sprintf("[%s/%s] %s", t.Speaker, t.Phase, t.Text)
	if prev == "" {
		return line
	}
	return prev + "\n" + line
}

func extractVerdict(turns []Turn) string {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Speaker == Synthesizer {
			return turns[i].Text
		}
	}
	// 兜底:没有 Synthesizer 轮(解析异常/提前中断)时,用最后一轮正文,
	// 至少不让 verdict 空着——前端与历史记录都依赖这个字段。
	if len(turns) > 0 {
		return turns[len(turns)-1].Text
	}
	return ""
}
