// Package debate 实现"财报两方辩论"功能：
//  1. 用 LLM 基于用户粘贴的财报文本，编排乐观派 vs 谨慎派发言（轮数由 LLM 决定）
//  2. 为每轮发言生成 TTS 音频（男女声区分角色）
//  3. 调 Remotion 渲染双栏辩论舞台视频
//
// 设计原则：3 个独立函数，无内部状态，前端按需串行调用。
package debate

type Speaker string

const (
	Moderator   Speaker = "moderator"
	Bull        Speaker = "bull"
	Bear        Speaker = "bear"
	Sector      Speaker = "sector"
	Risk        Speaker = "risk"
	Synthesizer Speaker = "synthesizer"
)

type Phase string

const (
	PhaseOpen      Phase = "open"
	PhaseOpening   Phase = "opening"
	PhaseCrossExam Phase = "cross_exam"
	PhaseRebuttal  Phase = "rebuttal"
	PhaseFactCheck Phase = "fact_check"
	PhaseClosing   Phase = "closing"
	PhaseSynthesis Phase = "synthesis"
)

type Citation struct {
	Source string `json:"source"`
	Ref    string `json:"ref"`
	Quote  string `json:"quote,omitempty"`
}

type ToolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type Turn struct {
	Index     int        `json:"index"`
	Phase     Phase      `json:"phase,omitempty"`
	Speaker   Speaker    `json:"speaker"`
	Text      string     `json:"text"`
	Emotion   string     `json:"emotion,omitempty"`
	Citations []Citation `json:"citations,omitempty"`
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
}

type Script struct {
	Turns       []Turn  `json:"turns"`
	ReportHash  string  `json:"reportHash,omitempty"`
	StockCode   string  `json:"stockCode,omitempty"`
	StockName   string  `json:"stockName,omitempty"`
	SessionID   string  `json:"sessionId,omitempty"`
	PreviousRefs string  `json:"previousRefs,omitempty"`
	Verdict     string  `json:"verdict,omitempty"`
	Phase       Phase   `json:"phase,omitempty"`
}

// AudioTurn 单轮发言的音频元数据。
type AudioTurn struct {
	Index       int     `json:"index"`
	Speaker     Speaker `json:"speaker"`
	Text        string  `json:"text"`
	AudioFile   string  `json:"audioFile"`
	DurationSec float64 `json:"durationSec"`
	Frames      int     `json:"frames"`
}

// RenderProps 传递给 DebateVideo Remotion 组件的 props。
type RenderProps struct {
	TaskID           string      `json:"taskId"`
	ModeratorName    string      `json:"moderatorName"`
	BullName         string      `json:"bullName"`
	BearName         string      `json:"bearName"`
	SectorName       string      `json:"sectorName"`
	RiskName         string      `json:"riskName"`
	SynthesizerName  string      `json:"synthesizerName"`
	ReportTitle      string      `json:"reportTitle"`
	Turns            []Turn      `json:"turns"`
	AudioTurns       []AudioTurn `json:"audioTurns"`
	TotalFrames      int         `json:"totalFrames"`
	Width            int         `json:"width"`
	Height           int         `json:"height"`
	Format           string      `json:"format"`
}
