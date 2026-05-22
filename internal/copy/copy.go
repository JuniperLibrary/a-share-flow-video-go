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

	lines = append(lines, "")
	lines = append(lines, "⚠️ 风险提示：以上数据仅供参考，不构成投资建议。股市有风险，投资需谨慎。")
	lines = append(lines, "")
	lines = append(lines, "#A股 #资金流向 #财经知识 #投资参考")
	lines = append(lines, "#股市分析")
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

	prompt := fmt.Sprintf(`你是一位A股市场财经知识分享者，在抖音上做数据解读类短视频。现在是%s %s，请根据以下板块资金流向数据，写一篇数据解读文案。

重要：你的内容必须符合抖音平台合规要求，绝对不能包含以下内容：
- ❌ 任何形式的投资建议、操作指导、买卖建议
- ❌ 预测股价涨跌、预判明天走势
- ❌ 推荐具体板块或个股让粉丝买入/卖出
- ❌ "抄底""建仓""加仓""止损""上车"等暗示操作的动作词
- ❌ 暗示"跟着买就能赚钱"的引导性语言
- ❌ 保证收益、承诺回报的表述
- ❌ 使用"必涨""暴跌""血亏"等极端情绪化词汇

## 数据摘要
net_total=%.1f亿，流入%d个板块，流出%d个板块
流入TOP5：%s
流出TOP5：%s

## 各板块详细数据
%s

%s

## 风格示例（模仿这个味道）

05月15日全天：15板块14绿，机器人逆势吸金

今天盘面资金流向分化明显，半导体、有色流出居前，AI和CPO继续调整。**机器人**板块获得18.4亿资金关注，成为全天亮点。资金分布显示高低切换特征，前期热门板块降温，新方向开始冒头。

⚠️ 以上数据仅供参考，不构成投资建议。股市有风险，投资需谨慎。

#A股 #资金流向 #财经知识

## 写作特征

### 用语习惯
- 说"资金关注""资金流入/流出""获得资金青睐""资金分布"
- 说"板块表现分化""结构性特征明显""此消彼长"
- 不说"买""卖""抄""建仓"等动作词
- 常用词：数据显示、资金流向、板块表现、关注度、特征明显、值得观察

### 写作节奏
- 短句为主，清晰易懂
- 先抛结论再展开数据
- 客观中立，不带诱导性

### 标题要求
- 标题必须有吸引力，但不能夸大误导
- 标题必须包含日期和维度（早盘/全天）
- 好标题示例：
  - "05月15日全天：机器人逆势吸金18亿，资金高低切换明显"
  - "05月14日早盘：半导体流入82亿领跑全场"
- 严格控制在25字以内
- 标题单独一行，后面空一行再写正文

### 内容要素
1. **标题**：日期+维度，单独一行
2. **一句话定调**：今天的整体资金格局
3. **数据解读**：资金流向分布，流入流出特征
4. **趋势感**：结合历史文案，指出是否是连续趋势
5. **结语**：固定文案 "以上数据仅供参考，不构成投资建议。股市有风险，投资需谨慎。"
6. **话题标签**：#A股 #%s资金流向 #财经知识

### 禁止事项
- 禁止逐条列举板块数据
- 禁止使用"首先/其次/最后/综上所述"
- 禁止写"以上数据表明""从数据可以看出"
- 禁止使用emoji（仅限⚠️）
- 禁止写"值得注意的是""需要关注的是"等公文腔
- 禁止出现任何形式的投资建议或操作指导
- 禁止"建议""推荐""应该""可以买""逢低"等引导性词汇

全文150-250字，客观展示资金流向数据，不做买卖建议。`,
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
