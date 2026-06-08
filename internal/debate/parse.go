package debate

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
	if t.Speaker != expectedSpeaker {
		return Turn{}, fmt.Errorf("speaker 应为 %s,实际 %s", expectedSpeaker, t.Speaker)
	}
	if strings.TrimSpace(t.Text) == "" {
		return Turn{}, fmt.Errorf("text 为空")
	}
	if t.Phase == "" {
		t.Phase = expectedPhase
	} else if t.Phase != expectedPhase {
		return Turn{}, fmt.Errorf("phase 应为 %s,实际 %s", expectedPhase, t.Phase)
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
