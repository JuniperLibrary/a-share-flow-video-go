package debate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	Turns             []Turn
}

type PhaseStep struct {
	Phase      Phase
	Speakers   []Speaker
	AllowTools bool
	Rounds     int
}

var defaultPhasePlan = []PhaseStep{
	{Phase: PhaseOpen, Speakers: []Speaker{Moderator}, AllowTools: false, Rounds: 1},
	{Phase: PhaseOpening, Speakers: []Speaker{Bull, Bear, Sector, Risk}, AllowTools: false, Rounds: 4},
	{Phase: PhaseCrossExam, Speakers: []Speaker{Bull, Bear, Sector, Risk}, AllowTools: false, Rounds: 4},
	{Phase: PhaseRebuttal, Speakers: []Speaker{Bull, Bear, Sector, Risk}, AllowTools: false, Rounds: 4},
	{Phase: PhaseFactCheck, Speakers: []Speaker{Moderator, Bull, Bear}, AllowTools: true, Rounds: 3},
	{Phase: PhaseClosing, Speakers: []Speaker{Bull, Bear}, AllowTools: false, Rounds: 2},
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

	systemPrompt := prompts.SystemPromptFor(string(speaker))
	ctx2 := prompts.TurnContext{
		ReportText:        state.ReportText,
		Transcript:        formatTranscript(state.Turns),
		StructuredMetrics: state.StructuredMetrics,
		StockName:         state.StockName,
		StockCode:         state.StockCode,
		ReportPeriod:      state.ReportPeriod,
		PreviousRefs:      state.PreviousRefs,
	}
	userPrompt := prompts.BuildUserPrompt(string(speaker), string(phase), ctx2)
	combined := systemPrompt + "\n\n" + userPrompt

	llmStart := time.Now()
	raw, err := callLLM(o.cfg, combined, 0.8, 500)
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
		return Turn{}, err
	}
	turn.Index = len(state.Turns)
	return turn, nil
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
		out, _ := json.Marshal(result)
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
	return ""
}
