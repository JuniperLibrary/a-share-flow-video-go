package copy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

	var inflows, outflows []struct{ Name string; Net float64 }
	for _, s := range sectors {
		if s.Net > 0 {
			inflows = append(inflows, struct{ Name string; Net float64 }{s.Name, s.Net})
		} else {
			outflows = append(outflows, struct{ Name string; Net float64 }{s.Name, s.Net})
		}
	}
	sort.Slice(inflows, func(i, j int) bool { return inflows[i].Net > inflows[j].Net })
	sort.Slice(outflows, func(i, j int) bool { return outflows[i].Net < outflows[j].Net })

	totalInflow := 0.0
	for _, v := range inflows {
		totalInflow += v.Net
	}
	totalOutflow := 0.0
	for _, v := range outflows {
		totalOutflow += -v.Net
	}
	netTotal := 0.0
	for _, s := range sectors {
		netTotal += s.Net
	}

	dateDisplay := timeParse(dateStr).Format("01月02日")
	sessionLabel := sessCfg.TitleSuffix

	var lines []string
	lines = append(lines, fmt.Sprintf("📊 %s %s资金流向", dateDisplay, sessionLabel))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("💰 净流入 %d 个 · 净流出 %d 个", len(inflows), len(outflows)))
	lines = append(lines, fmt.Sprintf("📈 合计净%s %.1f亿", dirLabel(netTotal), absF(netTotal)))
	lines = append(lines, "")

	if len(inflows) > 0 {
		lines = append(lines, fmt.Sprintf("🏆 榜首：%s +%.1f亿", inflows[0].Name, inflows[0].Net))
	}
	if len(outflows) > 0 {
		lines = append(lines, fmt.Sprintf("⚠️ 流出最多：%s %.1f亿", outflows[0].Name, outflows[0].Net))
	}
	lines = append(lines, "")

	lines = append(lines, "🔥 净流入TOP5：")
	for i := 0; i < len(inflows) && i < 5; i++ {
		lines = append(lines, fmt.Sprintf("  %d. %s  +%.1f亿", i+1, inflows[i].Name, inflows[i].Net))
	}
	lines = append(lines, "")

	lines = append(lines, "❄️ 净流出TOP5：")
	for i := 0; i < len(outflows) && i < 5; i++ {
		lines = append(lines, fmt.Sprintf("  %d. %s  %.1f亿", i+1, outflows[i].Name, outflows[i].Net))
	}
	lines = append(lines, "")

	if len(inflows) > 0 && len(outflows) > 0 {
		sentiment := ""
		if inflows[0].Net > absF(outflows[0].Net)*3 {
			sentiment = fmt.Sprintf("%s大幅领跑", inflows[0].Name)
		} else if inflows[0].Net > absF(outflows[0].Net) {
			sentiment = fmt.Sprintf("%s引领上攻", inflows[0].Name)
		} else if len(inflows) > len(outflows) {
			sentiment = "做多情绪回暖"
		} else {
			sentiment = "空头施压"
		}

		if totalInflow > totalOutflow*1.5 {
			lines = append(lines, fmt.Sprintf("💡 %s，主力进攻意愿较强", sentiment))
		} else if totalOutflow > totalInflow*1.5 {
			lines = append(lines, fmt.Sprintf("💡 %s，资金出逃意愿明显", sentiment))
		} else {
			lines = append(lines, fmt.Sprintf("💡 %s，结构性行情延续", sentiment))
		}
	}

	lines = append(lines, "")
	lines = append(lines, "⚠️ 风险提示：以上数据仅供参考，不构成投资建议。股市有风险，投资需谨慎。")
	lines = append(lines, "")
	lines = append(lines, "#A股 #资金流向 #投资理财 #财经分析")
	lines = append(lines, fmt.Sprintf("#%s资金流向", sessionLabel))

	return strings.Join(lines, "\n")
}

func loadHistoryCopy(dateStr, session string) []string {
	var history []string
	dateDir := config.GetCopyDir()
	prefix := "文案_"
	sessCfg, ok := config.SessionConfigs[session]
	if !ok {
		sessCfg = config.SessionConfigs["full"]
	}
	label := sessCfg.TitleSuffix

	t := timeParse(dateStr)
	for i := 1; i <= 5; i++ {
		prev := t.AddDate(0, 0, -i)
		prevStr := prev.Format("2006-01-02")
		filePath := filepath.Join(dateDir, prevStr, fmt.Sprintf("%s%s.txt", prefix, label))
		if data, err := os.ReadFile(filePath); err == nil {
			history = append(history, fmt.Sprintf("[%s]\n%s", prevStr, string(data)))
		}
	}
	return history
}

func GenerateCopywritingAI(sectors []fetcher.Sector, dateStr, session string) (string, error) {
	sessCfg, ok := config.SessionConfigs[session]
	if !ok {
		sessCfg = config.SessionConfigs["full"]
	}
	sessionLabel := sessCfg.TitleSuffix
	dateDisplay := timeParse(dateStr).Format("01月02日")

	var inflows, outflows []struct{ Name string; Net float64 }
	for _, s := range sectors {
		if s.Net > 0 {
			inflows = append(inflows, struct{ Name string; Net float64 }{s.Name, s.Net})
		} else {
			outflows = append(outflows, struct{ Name string; Net float64 }{s.Name, s.Net})
		}
	}
	sort.Slice(inflows, func(i, j int) bool { return inflows[i].Net > inflows[j].Net })
	sort.Slice(outflows, func(i, j int) bool { return outflows[i].Net < outflows[j].Net })

	netTotal := 0.0
	for _, s := range sectors {
		netTotal += s.Net
	}

	var table strings.Builder
	table.WriteString("板块,主力资金净流入(亿)\n")
	for _, s := range sectors {
		table.WriteString(fmt.Sprintf("%s,%.1f\n", s.Name, s.Net))
	}

	inflowTop5 := formatPairs(inflows, 5, true)
	outflowTop5 := formatPairs(outflows, 5, false)

	historyCopy := loadHistoryCopy(dateStr, session)
	historySection := ""
	if len(historyCopy) > 0 {
		historySection = fmt.Sprintf(`## 近5日历史文案参考（供趋势分析）

%s

请结合历史文案，分析板块资金流向的连续性和变化趋势。`, strings.Join(historyCopy, "\n\n"))
	}

	prompt := fmt.Sprintf(`你是一位小红书/抖音A股财经博主。现在是%s %s，请根据以下板块资金流向数据，写一篇财经笔记。

## 数据摘要
net_total=%.1f亿，流入%d个板块，流出%d个板块
流入TOP5：%s
流出TOP5：%s

## 各板块详细数据
%s

%s

## 要求
1. **标题**：抓眼球，带数字或情绪，**严格控制在20字以内**
2. **板块分析**：对每个流入/流出板块逐一简要分析资金动向和原因
3. **趋势判断**：结合历史文案（如有），指出板块资金的连续性和变化趋势
4. **操作建议**：给出一条具体操作建议
5. **风险提示**：必须在文末加入"⚠️ 风险提示：以上分析仅供参考，不构成投资建议。股市有风险，投资需谨慎。"
6. 语气像真实个人博主，不要AI腔
7. 结尾带话题标签：#A股 #%s情绪流 #风险提示
8. 全文300字左右`,
		dateDisplay, sessionLabel,
		netTotal, len(inflows), len(outflows),
		inflowTop5, outflowTop5,
		table.String(),
		historySection,
		sessionLabel,
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

func formatPairs(pairs []struct{ Name string; Net float64 }, n int, positive bool) string {
	if n > len(pairs) {
		n = len(pairs)
	}
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		if positive {
			parts[i] = fmt.Sprintf("%s(+%.1f亿)", pairs[i].Name, pairs[i].Net)
		} else {
			parts[i] = fmt.Sprintf("%s(%.1f亿)", pairs[i].Name, pairs[i].Net)
		}
	}
	return strings.Join(parts, ", ")
}

func dirLabel(net float64) string {
	if net > 0 {
		return "流入"
	}
	return "流出"
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
