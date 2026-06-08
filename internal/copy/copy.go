package copy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func GenerateCopywritingAI(sectors []fetcher.Sector, dateStr, session string) (string, error) {
	var inflows, outflows []struct {
		Name     string
		Net      float64
		SuperNet float64
		BigNet   float64
		ChgPct   float64
	}
	for _, s := range sectors {
		item := struct {
			Name     string
			Net      float64
			SuperNet float64
			BigNet   float64
			ChgPct   float64
		}{s.Name, s.Net, s.SuperNet, s.BigNet, s.ChangePct}
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

	var inflowTop3, outflowTop3 string
	for i, v := range inflows {
		if i >= 3 {
			break
		}
		if inflowTop3 != "" {
			inflowTop3 += "\n"
		}
		inflowTop3 += fmt.Sprintf("- %s: +%.0f亿(超大单%+.0f / 大单%+.0f)，涨幅%.1f%%", v.Name, v.Net, v.SuperNet, v.BigNet, v.ChgPct)
	}
	for i, v := range outflows {
		if i >= 3 {
			break
		}
		if outflowTop3 != "" {
			outflowTop3 += "\n"
		}
		outflowTop3 += fmt.Sprintf("- %s: %.0f亿(超大单%.0f / 大单%.0f)，跌幅%.1f%%", v.Name, v.Net, v.SuperNet, v.BigNet, v.ChgPct)
	}
	if inflowTop3 == "" {
		inflowTop3 = "无"
	}
	if outflowTop3 == "" {
		outflowTop3 = "无"
	}

	var structureLine string
	if netTotal != 0 {
		superPct := totalSuper / netTotal * 100
		bigPct := totalBig / netTotal * 100
		if superPct > 0 {
			structureLine = fmt.Sprintf("其中超大单净流入%.0f亿(占比%.0f%%)，大单净流入%.0f亿(占比%.0f%%)",
				totalSuper, superPct, totalBig, bigPct)
		} else {
			structureLine = fmt.Sprintf("其中超大单净流出%.0f亿(占比%.0f%%)，大单净流出%.0f亿(占比%.0f%%)",
				absF(totalSuper), absF(superPct), absF(totalBig), absF(bigPct))
		}
	}

	var outlookHint string
	if len(outflows) > 0 && len(inflows) > 0 {
		topOut := outflows[0]
		topIn := inflows[0]
		outlookHint = fmt.Sprintf("今日流出最重的是%s(%+.1f亿，跌幅%.1f%%)，逆势流入的是%s(%+.1f亿)。", topOut.Name, topOut.Net, topOut.ChgPct, topIn.Name, topIn.Net)
	} else if len(outflows) > 0 {
		topOut := outflows[0]
		outlookHint = fmt.Sprintf("今日流出最重的是%s(%+.1f亿，跌幅%.1f%%)，无明显资金逆势流入。", topOut.Name, topOut.Net, topOut.ChgPct)
	} else if len(inflows) > 0 {
		outlookHint = "今日全线资金净流入。"
	}
	outlookHint += " 基于今日资金结构，给出明日方向预判。"

	prompt := fmt.Sprintf(PromptCopywriting,
		netTotal, len(inflows), len(outflows),
		structureLine,
		outflowTop3,
		inflowTop3,
		outlookHint,
	)

	aiCfg := config.GetAIConfig()
	if aiCfg.APIKey == "" {
		return "", fmt.Errorf("AI模式需要设置 OPENAI_API_KEY 环境变量")
	}

	body := map[string]any{
		"model":       aiCfg.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": 0.8,
		"max_tokens":  1200,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", aiCfg.BaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+aiCfg.APIKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API 请求失败: HTTP %d %s", resp.StatusCode, string(b))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty choices from API")
	}
	return result.Choices[0].Message.Content, nil
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
