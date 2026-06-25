package debate

import (
	"encoding/json"
	"fmt"
	"strings"
)

// speakerVariants 允许的 speaker 别名映射
var speakerVariants = map[string]Speaker{
	"moderator":          Moderator,
	"moderator_summary":  Moderator,
	"bull":                Bull,
	"bull_specialist":     Bull,
	"bear":                Bear,
	"bear_specialist":     Bear,
	"sector":              Sector,
	"sector_specialist":   Sector,
	"risk":                Risk,
	"risk_analyst":        Risk,
	"risk_specialist":     Risk,
	"synthesizer":         Synthesizer,
	"synthesis_specialist": Synthesizer,
}

func parseTurn(raw string, expectedSpeaker Speaker, expectedPhase Phase) (Turn, error) {
	var wrapper struct {
		Turns []Turn `json:"turns"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return Turn{}, fmt.Errorf("JSON 解析失败: %w", err)
	}
	if len(wrapper.Turns) != 1 {
		return Turn{}, fmt.Errorf("期望单 turn,实际 %d 个", len(wrapper.Turns))
	}
	t := wrapper.Turns[0]

	// speaker 兼容性检查：允许大小写不敏感和常见别名
	speakerStr := strings.TrimSpace(string(t.Speaker))
	if !strings.EqualFold(speakerStr, string(expectedSpeaker)) {
		// 检查是否是已知的别名变体
		normalized := speakerVariants[strings.ToLower(speakerStr)]
		if normalized != expectedSpeaker {
			return Turn{}, fmt.Errorf("speaker 应为 %s,实际 %s", expectedSpeaker, t.Speaker)
		}
	}
	if strings.TrimSpace(t.Text) == "" {
		return Turn{}, fmt.Errorf("text 为空")
	}
	if t.Phase == "" {
		t.Phase = expectedPhase
	} else if !strings.EqualFold(string(t.Phase), string(expectedPhase)) {
		// LLM 返回的 phase 与预期不符，但使用 LLM 返回的值（信任 LLM 的阶段判断）
		// 仅记录 debug 日志，不作为错误处理
	}
	if t.Emotion == "" {
		t.Emotion = "neutral"
	}
	return t, nil
}

func parseSpeakerCount(turns []Turn) map[Speaker]int {
	m := make(map[Speaker]int)
	for _, t := range turns {
		m[t.Speaker]++
	}
	return m
}

func isValidPhaseSpeaker(phase Phase, speaker Speaker) bool {
	_, ok := phaseSpeakers[phase]
	if !ok {
		return false
	}
	for _, s := range phaseSpeakers[phase] {
		if s == speaker {
			return true
		}
	}
	return false
}

var phaseSpeakers = map[Phase][]Speaker{
	PhaseOpen:      {Moderator},
	PhaseOpening:   {Bull, Bear, Sector, Risk},
	PhaseCrossExam: {Bull, Bear, Sector, Risk},
	PhaseRebuttal:  {Bull, Bear, Sector, Risk},
	PhaseFactCheck: {Moderator, Bull, Bear},
	PhaseClosing:   {Bull, Bear},
	PhaseSynthesis: {Synthesizer},
}
