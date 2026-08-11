package copy

import (
	"fmt"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
)

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

	if netTotal != 0 {
		superPct := totalSuper / netTotal * 100
		bigPct := totalBig / netTotal * 100
		dir := ""
		if netTotal > 0 {
			dir = "净流入"
		} else {
			dir = "净流出"
		}
		lines = append(lines, fmt.Sprintf("资金结构上，超大单%s%.0f亿(占比%.0f%%)，大单%s%.0f亿(占比%.0f%%)，说明今日以%s为主。",
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
		lines = append(lines, fmt.Sprintf("明日关注：%s能否延续流入，以及科技板块(%s)是否有情绪修复机会。",
			inflows[0].Name,
			func() string {
				if len(outflows) > 0 {
					return outflows[0].Name
				}
				return "超跌方向"
			}()))
	} else {
		lines = append(lines, "明日关注市场能否止跌企稳。")
	}

	return strings.Join(lines, "\n")
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

func GenerateCopywritingAI(sectors []fetcher.Sector, dateStr, session, prevPrediction string) (string, error) {
	brief, err := BuildNarrativeBrief(sectors, dateStr, session, prevPrediction)
	if err != nil {
		return "", fmt.Errorf("build narrative brief: %w", err)
	}

	inflows, _, _, _, _ := splitSectorFlows(sectors)

	prompt := fmt.Sprintf(PromptFromBrief, brief.Format())
	text, err := chatCompletion(prompt, 0.7, 900)
	if err != nil {
		return "", err
	}
	text = normalizeHallucinatedSectorNames(text, sectors, inflows)
	text = normalizeTaggedCopy(text)

	if issues := ValidateCopy(text, brief, inflows); len(issues) > 0 {
		fixPrompt := fmt.Sprintf(PromptFixCopy,
			strings.Join(issues, "\n"),
			text,
			brief.Format(),
		)
		if fixed, fixErr := chatCompletion(fixPrompt, 0.5, 900); fixErr == nil && fixed != "" {
			text = normalizeTaggedCopy(fixed)
		}
	}

	anchorPool := append([]string{}, hookGreetingPhrases...)
	anchorPool = append(anchorPool, transitionPhrases...)
	anchorPool = append(anchorPool, closingInteractionPhrases...)
	anchorPool = append(anchorPool, closingOperationPhrases...)
	var anchorLines []string
	for _, a := range anchorPool {
		a = strings.TrimSpace(a)
		if a != "" {
			anchorLines = append(anchorLines, "  - "+a)
		}
	}
	anchorBlock := strings.Join(anchorLines, "\n")

	humanPrompt := fmt.Sprintf(PromptHumanizeCopy, anchorBlock, brief.Format(), text)
	if humanized, hErr := chatCompletion(humanPrompt, 0.82, 900); hErr == nil && humanized != "" {
		normalizedHuman := normalizeHallucinatedSectorNames(humanized, sectors, inflows)
		normalizedHuman = normalizeTaggedCopy(normalizedHuman)
		if postIssues := ValidateCopy(normalizedHuman, brief, inflows); len(postIssues) == 0 {
			text = normalizedHuman
		}
	}

	return text, nil
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

var businessBuzzwordReplacer = strings.NewReplacer(
	"闭环", "完整流程",
	"抓手", "切入点",
	"颗粒度", "细致程度",
	"对齐", "同步",
	"拉齐", "同步",
	"赋能", "帮助",
	"赛道", "领域",
	"弯道超车", "后发追上",
	"占领心智", "形成印象",
	"心智", "印象",
)

var writtenToOralReplacer = strings.NewReplacer(
	"综上所述", "说白了",
	"不难看出", "你再往深看",
	"显而易见", "关键",
	"值得注意的是", "更关键的来了",
	"整体而言", "今天盘面",
	"整体来看", "今天盘面",
	"综合来看", "今天盘面",
	"客观来说", "说句实在话",
	"坦率地讲", "说句实在话",
	"整体来看", "今天盘面",
	"达到了", "到了",
	"取得了", "干到了",
	"占比达到了", "占到了",
	"的一个", "的",
	"非常明显的", "明显的",
	"进行了", "做了",
	"实现了", "做到了",
)

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
				content = writtenToOralReplacer.Replace(content)
				content = businessBuzzwordReplacer.Replace(content)
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
