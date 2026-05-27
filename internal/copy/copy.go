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

	dateDisplay := formatDate(dateStr)
	sessionLabel := sessCfg.TitleSuffix

	var lines []string
	lines = append(lines, fmt.Sprintf("%s %s资金流向", dateDisplay, sessionLabel))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("今天%d个板块获得资金关注，%d个板块出现流出，合计净%s%.0f亿。", len(inflows), len(outflows), dirLabel(netTotal), absF(netTotal)))
	lines = append(lines, "")

	if len(inflows) > 0 {
		lines = append(lines, fmt.Sprintf("吸金最多的是%s，净流入%.0f亿。", inflows[0].Name, inflows[0].Net))
	}
	if len(outflows) > 0 {
		lines = append(lines, fmt.Sprintf("流出最多的是%s，净流出%.0f亿。", outflows[0].Name, outflows[0].Net))
	}
	lines = append(lines, "")

	if len(inflows) > 0 {
		var topNames []string
		for i := 0; i < len(inflows) && i < 5; i++ {
			topNames = append(topNames, inflows[i].Name)
		}
		lines = append(lines, fmt.Sprintf("资金关注最多的五个方向：%s。", strings.Join(topNames, "、")))
	}
	if len(outflows) > 0 {
		var topNames []string
		for i := 0; i < len(outflows) && i < 5; i++ {
			topNames = append(topNames, outflows[i].Name)
		}
		lines = append(lines, fmt.Sprintf("流出居前的五个板块：%s。", strings.Join(topNames, "、")))
	}
	lines = append(lines, "")

	lines = append(lines, "资金不会说谎，主线都会留下痕迹。")
	lines = append(lines, "")
	lines = append(lines, "以上数据仅供参考，不构成投资建议。股市有风险，投资需谨慎。")

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
			cleaned := cleanHistoryText(string(data))
			history = append(history, fmt.Sprintf("[%s]\n%s", prevStr, cleaned))
		}
	}
	return history
}

func cleanHistoryText(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		trimmed = strings.ReplaceAll(trimmed, "*", "")
		trimmed = strings.ReplaceAll(trimmed, "📊", "")
		trimmed = strings.ReplaceAll(trimmed, "💰", "")
		trimmed = strings.ReplaceAll(trimmed, "📈", "")
		trimmed = strings.ReplaceAll(trimmed, "📉", "")
		trimmed = strings.ReplaceAll(trimmed, "🏆", "")
		trimmed = strings.ReplaceAll(trimmed, "⚠️", "")
		trimmed = strings.ReplaceAll(trimmed, "🔥", "")
		trimmed = strings.ReplaceAll(trimmed, "❄️", "")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return strings.Join(lines, "\n")
}

func GenerateCopywritingAI(sectors []fetcher.Sector, dateStr, session string) (string, error) {
	sessCfg, ok := config.SessionConfigs[session]
	if !ok {
		sessCfg = config.SessionConfigs["full"]
	}
	sessionLabel := sessCfg.TitleSuffix
	dateDisplay := formatDate(dateStr)

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

	prompt := fmt.Sprintf(`你是一位A股市场财经知识分享者，在抖音上做数据解读类短视频。现在是%s %s，请根据以下板块资金流向数据，写一篇口播稿。

重要：这是一篇口播稿，会被语音合成系统朗读，必须像真人播报一样自然流畅。

## 文案会被拆成两段分别朗读

你写的文案会被拆成两部分，各自独立生成语音：
- 标题（第一行）：作为视频开头的开场白，单独朗读
- 正文（标题之后的所有内容）：作为视频结尾的总结语，单独朗读

所以标题和正文都要能独立成句，读起来完整自然。

## 格式铁律（违反任何一条都会导致语音合成异常）

- 禁止使用任何 emoji 表情符号
- 禁止使用 # 话题标签
- 禁止使用 ** 加粗标记或其他 markdown 格式
- 禁止使用数字编号列表（如"1. 2. 3."）
- 禁止使用项目符号（如"-""·""•"）
- 只用纯文本，用逗号和句号控制节奏

## 口播节奏要求

- 每句话不超过20个字，用逗号断句
- 多用连接词衔接："不过""同时""另外""与此同时""反观"
- 数字用整数或模糊表达："超过二十亿""接近十亿""近一百亿"，不要写"20.5亿""18.4亿"
- 不要写正负号，用文字表达方向："净流入二十亿""流出近十亿"
- 像朋友聊天一样讲数据，不像念报告

## 内容合规要求

- 禁止任何形式的投资建议、操作指导、买卖建议
- 禁止预测股价涨跌、预判明天走势
- 禁止推荐具体板块或个股让粉丝买入/卖出
- 禁止"抄底""建仓""加仓""止损""上车"等暗示操作的动作词
- 禁止暗示"跟着买就能赚钱"的引导性语言
- 禁止保证收益、承诺回报的表述
- 禁止使用"必涨""暴跌""血亏"等极端情绪化词汇

## 数据摘要
net_total=%.1f亿，流入%d个板块，流出%d个板块
流入TOP5：%s
流出TOP5：%s

## 各板块详细数据
%s

%s

## 品牌定位
- 账号 Slogan："资金不会说谎，主线都会留下痕迹"
- 内容风格：专业数据解读，不荐股，仅记录市场

## 口播稿结构

### 标题（第一行，作为开场白朗读）
一句话抓住注意力，可选风格：
- 疑问："今天谁在疯狂吸金？"
- 冲突："AI退潮了？"
- 数据："科技板块净流入超两百亿"
标题独立一行，后面空一行再写正文。控制在20字以内。

### 正文（作为结尾总结语朗读）
先抛结论再展开数据：定调整体资金格局，解读流入流出特征，指出趋势变化。
短句为主，口语化，像朋友聊天一样讲数据。
用连接词让句子之间有过渡，不要一句一句蹦。

### 收尾
品牌结语加互动提问，自然衔接，不要生硬。

## 口播风格示例

5月15日全天，科技吸金超两百亿

今天谁在疯狂吸金？答案是科技。半导体和AI应用两大方向，合计吸金超过一百亿，成为全天绝对主线。与此同时，资金从有色和银行流出，高低切换特征明显。另外CPO概念午后跟涨，算力产业链形成共振。

以上数据仅供参考，不构成投资建议。股市有风险，投资需谨慎。

资金不会说谎，主线都会留下痕迹。你最看好哪个方向？

## 写作特征

### 用语习惯
- 说"资金关注""资金流入流出""获得资金青睐""资金分布"
- 说"板块表现分化""结构性特征明显""此消彼长""主线共振"
- 不说"买""卖""抄""建仓"等动作词
- 常用词：数据显示、资金流向、板块表现、关注度、特征明显、值得观察
- 常用连接词：不过、同时、另外、与此同时、反观、而

### 写作节奏
- 每句不超过20字，用逗号断句
- 先抛结论再展开数据
- 客观中立，不带诱导性
- 像新闻播报，不像念文章

### 标题要求
- 推荐故事型标题："科技吸金超两百亿，发生了什么？"
- 也可用传统型："5月15日全天，机器人逆势吸金近二十亿"
- 必须包含日期和维度（早盘/全天）
- 严格控制在20字以内
- 标题单独一行，后面空一行再写正文

### 内容要素
1. 标题：日期加维度，故事型优先，单独一行
2. 开头钩子：疑问/冲突/数据型，独立一段
3. 一句话定调：今天的整体资金格局
4. 数据解读：资金流向分布，流入流出特征，是否有主线共振
5. 趋势感：结合历史文案，指出是否是连续趋势
6. 品牌结语：固定文案"资金不会说谎，主线都会留下痕迹"
7. 风险提示：固定文案"以上数据仅供参考，不构成投资建议。股市有风险，投资需谨慎。"
8. 互动引导：结尾提问引导评论
9. 不需要话题标签

### 禁止事项
- 禁止逐条列举板块数据
- 禁止使用"首先/其次/最后/综上所述"
- 禁止写"以上数据表明""从数据可以看出"
- 禁止使用任何 emoji
- 禁止写"值得注意的是""需要关注的是"等公文腔
- 禁止出现任何形式的投资建议或操作指导
- 禁止"建议""推荐""应该""可以买""逢低"等引导性词汇
- 禁止 markdown 格式（加粗、列表、标题符号）
- 禁止 # 话题标签
- 禁止小数点数字（如"20.5亿"），用整数或模糊表达

正文80到120字，纯文本口播稿，不做买卖建议。`,
		dateDisplay, sessionLabel,
		netTotal, len(inflows), len(outflows),
		inflowTop5, outflowTop5,
		table.String(),
		historySection,
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

func formatDate(dateStr string) string {
	t := timeParse(dateStr)
	return fmt.Sprintf("%d月%d日", t.Month(), t.Day())
}
