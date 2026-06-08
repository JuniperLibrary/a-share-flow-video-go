package debate

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/debate/prompts"
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

	if state.PreviousRefs == "" && state.StockCode != "" {
		prev, err := o.memory.GetRecentSessions(ctx, state.StockCode, 1)
		if err == nil && len(prev) > 0 && prev[0].Verdict != "" {
			state.PreviousRefs = fmt.Sprintf("[历史 verdict %s] %s", prev[0].Date, prev[0].Verdict)
		}
	}

	for _, step := range defaultPhasePlan {
		for round := 0; round < step.Rounds; round++ {
			for _, speaker := range step.Speakers {
				turn, err := o.runTurn(ctx, &state, speaker, step.Phase, step.AllowTools)
				if err != nil {
					return Script{}, fmt.Errorf("phase=%s speaker=%s 失败: %w", step.Phase, speaker, err)
				}
				state.Turns = append(state.Turns, turn)
				state.PreviousRefs = updatePreviousRefs(state.PreviousRefs, turn)

				if step.AllowTools && len(turn.ToolCalls) > 0 {
					results := o.executeTools(ctx, turn.ToolCalls)
					state.PreviousRefs = state.PreviousRefs + "\n[tool results]\n" + results
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

	if err := o.memory.SaveSession(ctx, SessionEntry{
		SessionID:  state.SessionID,
		StockCode:  state.StockCode,
		StockName:  state.StockName,
		Date:       time.Now().Format("2006-01-02"),
		Verdict:    verdict,
		TurnsCount: len(state.Turns),
		Script:     script,
	}); err != nil {
		log.Printf("[debate/orchestrator] SaveSession 失败: %v", err)
	}

	return script, nil
}

func (o *Orchestrator) RunLegacy(ctx context.Context, reportText string, structuredData ...map[string]any) (Script, error) {
	return GenerateScript(reportText, o.cfg, structuredData...)
}

func (o *Orchestrator) runTurn(ctx context.Context, state *DebateState, speaker Speaker, phase Phase, allowTools bool) (Turn, error) {
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

	raw, err := callLLM(o.cfg, combined, 0.8, 500)
	if err != nil {
		return Turn{}, err
	}

	turn, err := parseTurn(raw, speaker, phase)
	if err != nil {
		return Turn{}, err
	}
	turn.Index = len(state.Turns)
	return turn, nil
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
