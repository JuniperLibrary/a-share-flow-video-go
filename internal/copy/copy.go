package copy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/oral"
)

const (
	defaultCopyTotalDeadlineSec = 180
	copyDraftMaxTokens          = 700
	copyFixMaxTokens            = 520
	copyHumanizeMaxTokens       = 520
	copyMinFixBudget            = 25 * time.Second
	copyMinHumanizeBudget       = 18 * time.Second
)

type AIGenerationMeta struct {
	BriefBytes       int
	BriefDuration    time.Duration
	DraftDuration    time.Duration
	FixDuration      time.Duration
	HumanizeDuration time.Duration
	TotalDuration    time.Duration
	FixTriggered     bool
	FixApplied       bool
	HumanizeApplied  bool
	BlockingIssues   []string
	Notes            []string
}

func GenerateCopywriting(sectors []fetcher.Sector, dateStr, session string) string {
	sessCfg, ok := config.SessionConfigs[session]
	if !ok {
		sessCfg = config.SessionConfigs["full"]
	}

	inflows, outflows, netTotal, totalSuper, totalBig := splitSectorFlows(sectors)

	dateDisplay := formatDate(dateStr)
	sessionLabel := sessCfg.TitleSuffix

	var lines []string

	if len(outflows) > 0 && len(inflows) > 0 {
		lines = append(lines, fmt.Sprintf("%s %s，资金分化明显。%s净流出%.0f亿居首，%s逆势净流入%.0f亿。",
			dateDisplay, sessionLabel,
			outflows[0].Name, absF(outflows[0].Net),
			inflows[0].Name, inflows[0].Net))
	} else if len(outflows) > 0 {
		lines = append(lines, fmt.Sprintf("%s %s，全线承压。%s净流出%.0f亿居首，无一板块获资金关注。",
			dateDisplay, sessionLabel, outflows[0].Name, absF(outflows[0].Net)))
	} else {
		lines = append(lines, fmt.Sprintf("%s %s，全线净流入。", dateDisplay, sessionLabel))
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("整体来看，%d个板块净流入、%d个板块净流出，合计净流向%.0f亿。",
		len(inflows), len(outflows), netTotal))

	if stance := buildStanceLine(inflows, outflows); stance != "" {
		lines = append(lines, stance)
	}

	if netTotal != 0 {
		superPct := totalSuper / netTotal * 100
		bigPct := totalBig / netTotal * 100
		dir := ""
		if netTotal > 0 {
			dir = "净流入"
		} else {
			dir = "净流出"
		}
		lines = append(lines, fmt.Sprintf("资金结构上，超大单%s%.0f亿(占比%.0f%%)，大单%s%.0f亿(占比%.0f%%)，说明当日以%s为主。",
			dir, absF(totalSuper), absF(superPct),
			dir, absF(totalBig), absF(bigPct),
			mapStructureType(superPct, bigPct, netTotal)))
	}

	if len(inflows) > 0 {
		var topInDetails []string
		for i := 0; i < len(inflows) && i < 3; i++ {
			v := inflows[i]
			topInDetails = append(topInDetails, fmt.Sprintf("%s+%.0f亿(涨幅%.1f%%)", v.Name, v.Net, v.ChgPct))
		}
		lines = append(lines, fmt.Sprintf("流入侧：%s。", strings.Join(topInDetails, "、")))
	}
	if len(outflows) > 0 {
		var topOutDetails []string
		for i := 0; i < len(outflows) && i < 3; i++ {
			v := outflows[i]
			topOutDetails = append(topOutDetails, fmt.Sprintf("%s%.0f亿(跌幅%.1f%%)", v.Name, absF(v.Net), absF(v.ChgPct)))
		}
		lines = append(lines, fmt.Sprintf("流出侧：%s。", strings.Join(topOutDetails, "、")))
	}

	lines = append(lines, "")
	if len(inflows) > 0 {
		lines = append(lines, fmt.Sprintf("下一交易日关注：%s能否延续流入，以及%s。",
			inflows[0].Name, buildStanceFollowup(inflows, outflows)))
	} else {
		lines = append(lines, "下一交易日关注市场能否止跌企稳。")
	}

	return strings.Join(lines, "\n")
}

// buildStanceFollowup 模板文案收尾的科技视角跟进方向：
// 科技在流入榜 → 写科技主线承接；科技在流出榜 → 写科技主线情绪修复；否则写大盘放量。
func buildStanceFollowup(inflows, outflows []sectorFlow) string {
	for i, v := range inflows {
		if i >= 3 {
			break
		}
		if techSectorNames[v.Name] {
			return fmt.Sprintf("科技主线(%s)能否继续承接", v.Name)
		}
	}
	for i, v := range outflows {
		if i >= 3 {
			break
		}
		if techSectorNames[v.Name] {
			return fmt.Sprintf("科技主线(%s)的情绪修复机会", v.Name)
		}
	}
	return "市场整体能否放量"
}

func mapStructureType(superPct, bigPct float64, netTotal float64) string {
	if netTotal > 0 {
		if superPct > 60 {
			return "机构主导的集中流入"
		} else if bigPct > 60 {
			return "大户主导的分散流入"
		}
		return "机构和大户共同参与"
	}
	if absF(superPct) > 60 {
		return "机构主导的集中出逃"
	} else if absF(bigPct) > 60 {
		return "大户主导的分散出逃"
	}
	return "机构和大户同步减仓"
}

// buildStanceLine 模板文案的立场注入：按「重仓大A科技板块的财经博主」视角
// 解读当日资金——科技吸金写主线确认，科技失血写回调不改主线，
// 科技同时吸金又失血写内部分歧/高低切，非科技吸金写轮动/避险。
// 定性依据 techNetOut vs techNetIn 相对强度，避免小量吸金被误判为「主线确认」。
func buildStanceLine(inflows, outflows []sectorFlow) string {
	techInTop := 0
	techNetIn := 0.0
	for i, v := range inflows {
		if i >= 3 {
			break
		}
		if techSectorNames[v.Name] {
			techInTop++
			techNetIn += v.Net
		}
	}

	techOutTop := 0
	techNetOut := 0.0
	var techOutLeader string
	for i, v := range outflows {
		if i >= 3 {
			break
		}
		if techSectorNames[v.Name] {
			techOutTop++
			techNetOut += absF(v.Net)
			if techOutLeader == "" {
				techOutLeader = v.Name
			}
		}
	}

	if techOutTop >= 2 && techNetOut >= techNetIn {
		var names []string
		for i, v := range inflows {
			if i >= 3 {
				break
			}
			if techSectorNames[v.Name] {
				names = append(names, v.Name)
			}
		}
		if len(names) > 0 {
			return fmt.Sprintf("科技主线内部分歧加大：%s失血居前，但%s仍在承接——回调不改主线，重点盯高低切。",
				techOutLeader, strings.Join(names, "、"))
		}
		return fmt.Sprintf("科技方向出现分歧，%s失血居前，但回调不改主线，重点盯科技内部的高低切。", techOutLeader)
	}

	if techInTop >= 2 {
		var names []string
		for i, v := range inflows {
			if i >= 3 {
				break
			}
			if techSectorNames[v.Name] {
				names = append(names, v.Name)
			}
		}
		return fmt.Sprintf("科技主线再度确认：%s承接坚决，是当日资金最认可的方向。",
			strings.Join(names, "、"))
	}

	if techOutTop >= 2 {
		return fmt.Sprintf("科技方向出现分歧，%s失血居前，但回调不改主线，重点盯科技内部的高低切。", techOutLeader)
	}

	if len(inflows) > 0 && !techSectorNames[inflows[0].Name] && (techInTop >= 1 || techOutTop >= 1) {
		return fmt.Sprintf("当日吸金集中在%s，更多是轮动/避险性质；科技主线仍需盯承接强度。", inflows[0].Name)
	}

	return ""
}

func GenerateCopywritingAI(sectors []fetcher.Sector, dateStr, session, prevPrediction string) (string, error) {
	text, _, err := GenerateCopywritingAIWithMeta(sectors, dateStr, session, prevPrediction)
	return text, err
}

func GenerateCopywritingAIWithMeta(sectors []fetcher.Sector, dateStr, session, prevPrediction string) (string, *AIGenerationMeta, error) {
	aiCfg := config.GetAIConfigFor("copy")
	totalDeadline := time.Duration(defaultCopyTotalDeadlineSec) * time.Second
	if aiCfg.CopyTotalDeadlineSec > 0 {
		totalDeadline = time.Duration(aiCfg.CopyTotalDeadlineSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), totalDeadline)
	defer cancel()
	meta := &AIGenerationMeta{}
	startAll := time.Now()

	briefStart := time.Now()
	brief, err := BuildNarrativeBrief(sectors, dateStr, session, prevPrediction)
	if err != nil {
		return "", meta, fmt.Errorf("build narrative brief: %w", err)
	}
	brief.CompactForLLM()
	meta.BriefDuration = time.Since(briefStart)

	inflows, outflows, _, _, _ := splitSectorFlows(sectors)

	dateCtx := fetcher.TradingDayContext(dateStr)
	briefText := brief.Format()
	meta.BriefBytes = len([]byte(briefText))
	prompt := fmt.Sprintf(PromptFromBrief, dateCtx, briefText)
	draftStart := time.Now()
	text, err := chatCompletionWithContext(ctx, prompt, 0.7, copyDraftMaxTokens)
	meta.DraftDuration = time.Since(draftStart)
	if err != nil {
		meta.TotalDuration = time.Since(startAll)
		return "", meta, err
	}
	text = normalizeHallucinatedSectorNames(text, sectors, inflows)
	text = normalizeTaggedCopy(text)

	if issues := BlockingCopyIssues(ValidateCopy(text, brief, inflows, outflows)); len(issues) > 0 {
		meta.BlockingIssues = append([]string(nil), issues...)
		meta.FixTriggered = true
		fixPrompt := fmt.Sprintf(PromptFixCopy,
			strings.Join(issues, "\n"),
			text,
			briefText,
			dateCtx,
		)
		if remainingBudget(ctx) >= copyMinFixBudget {
			fixStart := time.Now()
			fixed, fixErr := chatCompletionWithContext(ctx, fixPrompt, 0.45, copyFixMaxTokens)
			meta.FixDuration = time.Since(fixStart)
			if fixErr == nil && fixed != "" {
				text = normalizeTaggedCopy(fixed)
				meta.FixApplied = true
			}
		} else {
			meta.Notes = append(meta.Notes, "剩余预算不足，跳过 FixCopy")
		}
	}

	anchorPool := append([]string{}, hookGreetingPhrases...)
	anchorPool = append(anchorPool, transitionPhrases...)
	anchorPool = append(anchorPool, closingInteractionPhrases...)
	anchorPool = append(anchorPool, closingOperationPhrases...)
	anchorPool = append(anchorPool, stanceAnchorPhrases...)
	var anchorLines []string
	for _, a := range anchorPool {
		a = strings.TrimSpace(a)
		if a != "" {
			anchorLines = append(anchorLines, "  - "+a)
		}
	}
	anchorBlock := strings.Join(anchorLines, "\n")

	if aiCfg.CopyHumanizeEnabled {
		if remainingBudget(ctx) >= copyMinHumanizeBudget {
			humanPrompt := fmt.Sprintf(PromptHumanizeCopy, anchorBlock, briefText, text, dateCtx)
			humanizeStart := time.Now()
			humanized, hErr := chatCompletionWithContext(ctx, humanPrompt, 0.78, copyHumanizeMaxTokens)
			meta.HumanizeDuration = time.Since(humanizeStart)
			if hErr == nil && humanized != "" {
				normalizedHuman := normalizeHallucinatedSectorNames(humanized, sectors, inflows)
				normalizedHuman = normalizeTaggedCopy(normalizedHuman)
				if postIssues := ValidateCopy(normalizedHuman, brief, inflows, outflows); len(postIssues) == 0 {
					text = normalizedHuman
					meta.HumanizeApplied = true
				}
			}
		} else {
			meta.Notes = append(meta.Notes, "剩余预算不足，跳过 Humanize")
		}
	} else {
		meta.Notes = append(meta.Notes, "配置关闭 Humanize")
	}

	meta.TotalDuration = time.Since(startAll)
	return text, meta, nil
}

func remainingBudget(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		return time.Until(deadline)
	}
	return 24 * time.Hour
}

func normalizeHallucinatedSectorNames(text string, sectors []fetcher.Sector, inflows []sectorFlow) string {
	names := make(map[string]bool)
	for _, s := range sectors {
		if s.Name != "" {
			names[s.Name] = true
		}
	}
	if strings.Contains(text, "新能源") && !names["新能源"] {
		repl := ""
		if names["电池"] {
			repl = "电池"
		} else if len(inflows) > 0 && inflows[0].Name != "" {
			repl = inflows[0].Name
		}
		if repl != "" {
			text = strings.ReplaceAll(text, "新能源", repl)
		}
	}
	return text
}

func normalizeTaggedCopy(text string) string {
	tags := []string{"[钩子]", "[悬念]", "[反转]", "[答案]", "[收尾]"}
	lines := strings.Split(text, "\n")
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, tag) {
				continue
			}
			if idx := strings.Index(trimmed, "]"); idx >= 0 {
				content := strings.TrimSpace(trimmed[idx+1:])
				content = oral.WrittenToOral.Replace(content)
				content = oral.Buzzwords.Replace(content)
				content = strings.Join(strings.Fields(content), " ")
				if content != "" {
					normalized = append(normalized, fmt.Sprintf("%s %s", tag, content))
					break
				}
			}
		}
	}
	if len(normalized) == len(tags) {
		return strings.Join(normalized, "\n")
	}
	return strings.TrimSpace(text)
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func timeParse(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Now()
	}
	return t
}

func formatDate(dateStr string) string {
	t := timeParse(dateStr)
	return fmt.Sprintf("%d月%d日", t.Month(), t.Day())
}
