package copy

import (
	"fmt"
	"sort"
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

	type sectorSummary struct {
		Name     string
		Net      float64
		SuperNet float64
		BigNet   float64
		ChgPct   float64
	}
	var inflows, outflows []sectorSummary
	for _, s := range sectors {
		item := sectorSummary{s.Name, s.Net, s.SuperNet, s.BigNet, s.ChangePct}
		if s.Net > 0 {
			inflows = append(inflows, item)
		} else {
			outflows = append(outflows, item)
		}
	}
	sort.Slice(inflows, func(i, j int) bool { return inflows[i].Net > inflows[j].Net })
	sort.Slice(outflows, func(i, j int) bool { return outflows[i].Net < outflows[j].Net })

	netTotal := 0.0
	var totalSuper, totalBig float64
	for _, s := range sectors {
		netTotal += s.Net
		totalSuper += s.SuperNet
		totalBig += s.BigNet
	}

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

	if issues := ValidateCopy(text, brief, inflows); len(issues) > 0 {
		fixPrompt := fmt.Sprintf(PromptFixCopy,
			strings.Join(issues, "\n"),
			text,
			brief.Format(),
		)
		if fixed, fixErr := chatCompletion(fixPrompt, 0.5, 900); fixErr == nil && fixed != "" {
			text = fixed
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
