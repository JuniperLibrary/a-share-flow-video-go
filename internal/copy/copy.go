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
	lines = append(lines, fmt.Sprintf("📊 %s %s资金流向速览", dateDisplay, sessionLabel))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("今日重点监控%d个板块：", len(sectors)))
	lines = append(lines, fmt.Sprintf("💰 净流入 %d 个 · 净流出 %d 个", len(inflows), len(outflows)))
	lines = append(lines, fmt.Sprintf("📈 合计净%s %.1f亿", dirLabel(netTotal), absF(netTotal)))
	lines = append(lines, "")

	if len(inflows) > 0 {
		lines = append(lines, fmt.Sprintf("🏆 净流入榜首：%s +%.1f亿", inflows[0].Name, inflows[0].Net))
	}
	if len(outflows) > 0 {
		lines = append(lines, fmt.Sprintf("⚠️ 净流出最多：%s %.1f亿", outflows[0].Name, outflows[0].Net))
	}
	lines = append(lines, "")

	lines = append(lines, "🔥 资金净流入TOP5：")
	for i := 0; i < len(inflows) && i < 5; i++ {
		lines = append(lines, fmt.Sprintf("  %d. %s  +%.1f亿", i+1, inflows[i].Name, inflows[i].Net))
	}
	lines = append(lines, "")

	lines = append(lines, "❄️ 资金净流出TOP5：")
	for i := 0; i < len(outflows) && i < 5; i++ {
		lines = append(lines, fmt.Sprintf("  %d. %s  %.1f亿", i+1, outflows[i].Name, outflows[i].Net))
	}
	lines = append(lines, "")

	if len(inflows) > 0 && len(outflows) > 0 {
		sentiment := ""
		if inflows[0].Net > absF(outflows[0].Net)*3 {
			sentiment = fmt.Sprintf("多头火力集中，%s大幅领跑", inflows[0].Name)
		} else if inflows[0].Net > absF(outflows[0].Net) {
			sentiment = fmt.Sprintf("多头占优，%s引领上攻", inflows[0].Name)
		} else if len(inflows) > len(outflows) {
			sentiment = "做多情绪回暖，但力度有限"
		} else {
			sentiment = "空头施压，谨慎观望为宜"
		}

		if totalInflow > totalOutflow*1.5 {
			lines = append(lines, fmt.Sprintf("💡 市场判断：%s，主力进攻意愿较强", sentiment))
		} else if totalOutflow > totalInflow*1.5 {
			lines = append(lines, fmt.Sprintf("💡 市场判断：%s，资金出逃意愿明显", sentiment))
		} else {
			lines = append(lines, fmt.Sprintf("💡 市场判断：%s，结构性行情延续", sentiment))
		}
	}

	lines = append(lines, "")
	lines = append(lines, "#A股 #资金流向 #投资理财 #财经分析")
	lines = append(lines, fmt.Sprintf("#%s资金流向", sessionLabel))

	return strings.Join(lines, "\n")
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

	prompt := fmt.Sprintf(`你是一位小红书A股财经博主。现在是%s %s，请根据以下%d个板块的主力资金净流入数据，写一篇小红书笔记。

数据：
net_total=%.1f亿，流入%d个，流出%d个
流入TOP5：%s
流出TOP5：%s

全文数据表：
%s

要求：
1. 标题要抓眼球，带数字或情绪
2. 分析今日板块情绪动向，点出最值得关注的板块
3. 给出一条具体操作建议
4. 语气像真实个人博主，不要AI腔
5. 结尾带话题标签：#A股 #%s情绪流
6. 全文200字左右`,
		dateDisplay, sessionLabel, len(sectors),
		netTotal, len(inflows), len(outflows),
		inflowTop5, outflowTop5,
		table.String(),
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
		"max_tokens":  800,
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
