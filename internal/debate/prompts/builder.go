package prompts

import "strings"

func SystemPromptFor(speaker string) string {
	switch speaker {
	case "moderator":
		return moderatorSystemPrompt
	case "bull":
		return bullSystemPrompt
	case "bear":
		return bearSystemPrompt
	case "sector":
		return sectorSystemPrompt
	case "risk":
		return riskSystemPrompt
	case "synthesizer":
		return synthesizerSystemPrompt
	default:
		return bullSystemPrompt
	}
}

func BuildUserPrompt(speaker, phase string, ctx TurnContext) string {
	switch speaker {
	case "moderator":
		return BuildModeratorTurn(phase, ctx.ReportText, ctx.Transcript, ctx.StockName, ctx.ReportPeriod)
	case "bull":
		return BuildBullTurn(phase, ctx.ReportText, ctx.Transcript, ctx.StructuredMetrics)
	case "bear":
		return BuildBearTurn(phase, ctx.ReportText, ctx.Transcript, ctx.StructuredMetrics)
	case "sector":
		return BuildSectorTurn(phase, ctx.ReportText, ctx.Transcript, ctx.StructuredMetrics)
	case "risk":
		return BuildRiskTurn(phase, ctx.ReportText, ctx.Transcript, ctx.StructuredMetrics)
	case "synthesizer":
		return BuildSynthesizerTurn(ctx.ReportText, ctx.Transcript, ctx.StockName, ctx.ReportPeriod)
	default:
		return BuildBullTurn(phase, ctx.ReportText, ctx.Transcript, ctx.StructuredMetrics)
	}
}

type TurnContext struct {
	ReportText        string
	Transcript        string
	StructuredMetrics string
	StockName         string
	StockCode         string
	ReportPeriod      string
	PreviousRefs      string
}

func truncateForPrompt(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.ToValidUTF8(s[:max], "") + "...(已截断)"
}
