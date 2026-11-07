package debate

import (
	"context"
	"regexp"
	"strings"

	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// 证据四元组里最容易被模型漏掉的两项:显式算式与证伪条件。
//
// 实跑(17 轮 × 3 场,茅台 2026Q3)统计:每场恒有 2-3 轮漏写算式,
// 典型形式是把两个数字并排放着说"数字成立",却不写 = 或 ÷ 的过程;
// 也有人把证伪条件写成结论句,没有"若/除非"这种触发条件。
// prompt 只能提高概率,要做到"每轮都有"必须有一道后置校验 + 定向返工。
var (
	evidenceFormulaRe = regexp.MustCompile(`[=＝]|÷|->|→`)
	evidenceTriggerRe = regexp.MustCompile(`若|除非|一旦|如果`)
)

// evidenceGap 记录一轮正文缺哪一项。
type evidenceGap struct {
	NeedFormula   bool
	NeedFalsifier bool
	// BadMath 是算错的二元算式。它比"缺算式"更危险:算式形状是对的,
	// 只是结论错了,观众与下游都不会再验一遍。
	BadMath []arithIssue
	// FourSection 表示这一轮正文是四段式(裁决者)。
	// 返工提示不能对它要求"三句式",那会把结论结构改坏。
	FourSection bool
}

func (g evidenceGap) Empty() bool {
	return !g.NeedFormula && !g.NeedFalsifier && len(g.BadMath) == 0
}

// Instruction 生成返工要求,直接拼到原 prompt 后面。
func (g evidenceGap) Instruction() string {
	var b strings.Builder
	b.WriteString("## 返工要求(上一轮不合格)\n")
	if g.NeedFormula {
		b.WriteString("- 正文第①句没有显式算式:必须写成\"科目 数值 ÷ 数值 = 结果\"或\"数值 - 数值 = 结果\",\n")
		b.WriteString("  不能只并排放两个数字。\n")
	}
	if g.NeedFalsifier {
		b.WriteString("- 正文第③句缺少证伪条件:必须以\"若\"或\"除非\"开头,给一个可判定的触发条件。\n")
	}
	if len(g.BadMath) > 0 {
		b.WriteString("- 下面这些算式算错了。只把结果数字改成正确值,论据、指标与结论一律不动:\n")
		for i, issue := range g.BadMath {
			if i >= 3 {
				break
			}
			b.WriteString("  · " + issue.String() + "\n")
		}
	}
	if g.FourSection {
		b.WriteString("保持四段式结构与 240 字上限。四个小标题必须逐字保留:\n")
		b.WriteString("  结论: / 最强证据: / 最致命反证: / 检验卡:\n")
		b.WriteString("只改算错的数字,不得更换论据或指标,不得新增论据。\n")
	} else {
		b.WriteString("保持三句式与字数上限,不得引入上一轮没有的新数字,不得新增第四句。\n")
	}
	b.WriteString("只输出与上次相同结构的 JSON。\n")
	return b.String()
}

// checkEvidence 判断一轮正文是否满足证据纪律。
//
// 三句式与证伪条件只约束 Bull/Bear/Sector/Risk:
//   - Moderator 的职责是点名谁该复算什么,它本身不产出证据单元;
//   - Synthesizer 的四段式已经内含"检验卡"作为证伪条件,
//     再要求它写"若…"的第三句会和四段式打架。
//
// 但算式对所有角色都生效:裁决者会复述双方的算式,那句
// "Bull 复算 840.59-70.9=769.69 亿"是全片最被引用的数字,
// 抄错时如果没人验,错的结论就被当成定论播出去。
func checkEvidence(speaker Speaker, text string) evidenceGap {
	switch speaker {
	case Moderator:
		return evidenceGap{BadMath: checkArithmetic(text)}
	case Synthesizer:
		return evidenceGap{BadMath: checkArithmetic(text), FourSection: true}
	}
	return evidenceGap{
		NeedFormula:   !evidenceFormulaRe.MatchString(text),
		NeedFalsifier: !evidenceTriggerRe.MatchString(text),
		BadMath:       checkArithmetic(text),
	}
}

// TurnInspection 是一轮正文的证据体检结果。
//
// 抽成导出类型是为了让门禁命令(cmd/debategate)与前端的质量标记
// 共用同一套判据:两处各写一份正则,迟早会出现"页面显示合格、
// 门禁判不合格"的对不上。
type TurnInspection struct {
	Chars        int
	HasFormula   bool
	HasFalsifier bool
	// Checked 表示这一轮是否受证据纪律约束(主持人/裁决者不受约束)。
	Checked    bool
	MathIssues []string
}

// Missing 表示这一轮该有证据要素却缺了。
func (t TurnInspection) Missing() bool {
	return t.Checked && (!t.HasFormula || !t.HasFalsifier)
}

// InspectTurn 体检一轮正文。
func InspectTurn(turn Turn) TurnInspection {
	gap := checkEvidence(turn.Speaker, turn.Text)
	checked := turn.Speaker != Moderator && turn.Speaker != Synthesizer
	out := TurnInspection{
		Chars:        len([]rune(turn.Text)),
		HasFormula:   !gap.NeedFormula,
		HasFalsifier: !gap.NeedFalsifier,
		Checked:      checked,
	}
	for _, issue := range gap.BadMath {
		out.MathIssues = append(out.MathIssues, issue.String())
	}
	return out
}

// buildEvidenceRetryPrompt 把返工要求拼到原 prompt 上,并回贴上一轮正文。
// 抽成纯函数便于单测:这条 prompt 一旦丢了"不得引入新数字",
// 模型会用编造的算式应付校验,比原来更糟。
func buildEvidenceRetryPrompt(basePrompt, prevText string, gap evidenceGap) string {
	return basePrompt + "\n\n## 你上一轮的正文\n" + prevText + "\n\n" + gap.Instruction()
}

// enforceEvidence 对不合格的一轮做一次定向返工。
//
// 只在确实缺项时才多调一次模型(实测每场 2-3 轮,约 1 分钟),
// 返工结果必须自己也合规才替换,否则保留原文——
// 宁可少一条算式,也不要让返工把已经算对的内容改坏。
func (o *Orchestrator) enforceEvidence(ctx context.Context, basePrompt string, turn Turn, speaker Speaker, phase Phase) Turn {
	gap := checkEvidence(speaker, turn.Text)
	if gap.Empty() {
		return turn
	}

	logger.Warn("轮次缺少证据要素,触发返工",
		zap.String("speaker", string(speaker)),
		zap.String("phase", string(phase)),
		zap.Bool("needFormula", gap.NeedFormula),
		zap.Bool("needFalsifier", gap.NeedFalsifier),
		zap.Int("badMath", len(gap.BadMath)),
	)

	_, retryBudget := o.tokenBudgets()
	raw, err := callLLM(o.cfg, buildEvidenceRetryPrompt(basePrompt, turn.Text, gap), 0.3, retryBudget)
	if err != nil {
		logger.Warn("返工调用失败,保留原文", zap.Error(err))
		return turn
	}
	rebuilt, err := parseTurn(raw, speaker, phase)
	if err != nil {
		logger.Warn("返工结果解析失败,保留原文", zap.Error(err))
		return turn
	}
	if !checkEvidence(speaker, rebuilt.Text).Empty() {
		logger.Warn("返工结果仍不合规,保留原文",
			zap.String("speaker", string(speaker)),
			zap.Int("textLen", len([]rune(rebuilt.Text))),
		)
		return turn
	}

	// 返工只针对正文,引用丢了就从原文补回来,别让证据链断掉。
	if len(rebuilt.Citations) == 0 {
		rebuilt.Citations = turn.Citations
	}
	rebuilt.Index = turn.Index
	logger.Info("返工成功",
		zap.String("speaker", string(speaker)),
		zap.Int("before", len([]rune(turn.Text))),
		zap.Int("after", len([]rune(rebuilt.Text))),
	)
	return rebuilt
}
