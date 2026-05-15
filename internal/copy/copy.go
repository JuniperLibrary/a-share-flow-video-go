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

	var inflows, outflows []struct {
		Name string
		Net  float64
	}
	for _, s := range sectors {
		if s.Net > 0 {
			inflows = append(inflows, struct {
				Name string
				Net  float64
			}{s.Name, s.Net})
		} else {
			outflows = append(outflows, struct {
				Name string
				Net  float64
			}{s.Name, s.Net})
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

	var inflows, outflows []struct {
		Name string
		Net  float64
	}
	for _, s := range sectors {
		if s.Net > 0 {
			inflows = append(inflows, struct {
				Name string
				Net  float64
			}{s.Name, s.Net})
		} else {
			outflows = append(outflows, struct {
				Name string
				Net  float64
			}{s.Name, s.Net})
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

	prompt := fmt.Sprintf(`你是一位在A股摸爬滚打十年的交易老手，在小红书/抖音上给粉丝做复盘。现在是%s %s，请根据以下板块资金流向数据，写一篇复盘笔记。

## 数据摘要
net_total=%.1f亿，流入%d个板块，流出%d个板块
流入TOP5：%s
流出TOP5：%s

## 各板块详细数据
%s

%s

## 风格示例（模仿这个味道）

05月15日全天：15板块14绿，机器人成唯一独苗

今天盘面太冷，资金避险情绪拉满。半导体、有色带头砸盘，AI和CPO被按在地上摩擦。但主力没躺平，**机器人**逆势吸金18.4亿，成了全场唯一独苗！资金明显从高波科技撤出，在做高低切换。

💡操作建议：管住手！流出的板块别急着抄底。既然主力明牌抱团机器人，可轻仓试错前排核心，跌破5日线果断止损。弱市里，保住本金最重要！

⚠️ 风险提示：以上分析仅供参考，不构成投资建议。股市有风险，投资需谨慎。

#A股 #资金流向 #风险提示

## 交易老手的写作特征

### 用语习惯（用这些词，不用官方术语）
- 数据不说"净流入/净流出"，说"吸金/砸盘/出逃/失血/抱团/抢筹"
- 不说"板块轮动"，说"资金在做高低切换""从XX撤到XX"
- 不说"市场情绪分化"，说"冰火两重天""一边吃肉一边挨打"
- 常用词：主力、资金、盘面、砸盘、护盘、洗盘、出货、建仓、抄底、止损、管住手、独苗、明牌、抱团、躺平、挨打、吃肉

### 写作节奏
- 短句为主，像说话，不像写文章
- 先抛结论再展开："今天盘面太冷"→"半导体带头砸"→"但机器人逆势吸金"
- 有情绪起伏：看到机会兴奋，看到风险警惕
- 用**加粗**强调关键板块，不用emoji堆砌

### 标题要求
- 标题必须有吸引力，像小红书爆款标题，能让人停下来看
- 标题必须包含日期和维度（早盘/全天），但不能写成"XX日复盘"这种无聊格式
- 好标题示例：
  - "05月15日全天：15板块14绿！机器人成唯一独苗"
  - "05月14日早盘：半导体狂吸82亿，AI全线爆发"
  - "05月13日全天：资金大逃亡！主力狂抛1163亿"
- 公式：日期+维度 + 最极端的盘面特征/最反常的信号
- 严格控制在20字以内
- 标题单独一行，后面空一行再写正文

### 内容要素
1. **标题**：日期+维度，单独一行
2. **一句话定调**：今天整体什么感觉（冷/热/冰火两重天/极端分化）
3. **讲一个故事**：资金从哪跑到哪，谁在挨打谁在吃肉，为什么
4. **趋势感**：结合历史文案，指出这是否是连续几天的趋势
5. **预判明天**：给出一句对明天的看法（"明天大概率..."）
6. **操作建议**：用"💡操作建议："开头，具体到动作（管住手/轻仓/止损/加仓）
7. **风险提示**：固定文案
8. **话题标签**：#A股 #%s资金流向 #风险提示

### 禁止事项
- 禁止逐条列举板块数据
- 禁止使用"首先/其次/最后/综上所述"
- 禁止写"以上数据表明""从数据可以看出"
- 禁止使用emoji（除了💡和⚠️）
- 禁止写"值得注意的是""需要关注的是"等公文腔

全文200-300字，像一条真实的交易老手朋友圈。`,
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

func formatPairs(pairs []struct {
	Name string
	Net  float64
}, n int, positive bool) string {
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
