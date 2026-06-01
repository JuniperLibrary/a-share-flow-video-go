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
		lines = append(lines, fmt.Sprintf("流出最多的是%s，净流出%.0f亿。", outflows[0].Name, absF(outflows[0].Net)))
	}

	var totalSuper, totalBig, totalMain float64
	hasStructure := false
	for _, s := range sectors {
		totalSuper += s.SuperNet
		totalBig += s.BigNet
		totalMain += s.Net
		if s.SuperNet != 0 || s.BigNet != 0 {
			hasStructure = true
		}
	}
	if hasStructure && totalMain != 0 {
		superPct := totalSuper / totalMain * 100
		bigPct := totalBig / totalMain * 100
		lines = append(lines, fmt.Sprintf("从结构看，超大单净流入%.0f亿、占比%.0f%%，大单净流入%.0f亿、占比%.0f%%。",
			totalSuper, superPct, totalBig, bigPct))
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
	table.WriteString("板块,主力资金净流入(亿),涨跌幅(%),超大单净流入(亿),超大单净占比(%),大单净流入(亿),大单净占比(%)\n")
	for _, s := range sectors {
		table.WriteString(fmt.Sprintf("%s,%.1f,%.2f,%.1f,%.2f,%.1f,%.2f\n",
			s.Name, s.Net, s.ChangePct, s.SuperNet, s.SuperRate, s.BigNet, s.BigRate))
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

	prompt := fmt.Sprintf(`你是一位A股市场数据解读类短视频创作者。现在是%s %s，请根据以下板块资金流向数据，写一篇口播稿。

## 输出格式（必须遵守）

文案会被拆成两段独立朗读（标题 + 正文），两者都要能独立成句、读起来完整自然。

标题（第一行）→ 视频开场白
空一行
正文 → 视频结尾总结语

## 格式红线（违者语音合成异常）

- 纯文本，只用逗号和句号控制节奏
- 禁止 emoji、#话题标签、**加粗**、markdown、数字编号、项目符号
- 禁止小数点数字（写"超过二十亿"不写"20.5亿"）
- 禁止正负号（写"净流入二十亿"不写"+20亿"）
- 禁止"首先/其次/最后/综上所述/以上数据表明/从数据可以看出/值得注意的是/需要关注的是"等公文腔

## 口播风格

- 每句不超过20字，用逗号断句
- 多用连接词衔接：不过、同时、另外、与此同时、反观
- 像朋友聊天一样讲数据，像新闻播报，不像念报告或念文章
- 数字用整数或模糊表达（"超过二十亿""接近十亿""近一百亿"）
- 先抛结论再展开数据，一句话定调整体资金格局

## 数据摘要
net_total=%.1f亿，流入%d个板块，流出%d个板块
流入TOP5：%s
流出TOP5：%s

## 各板块详细数据（含主力资金结构）
%s

%s

## 主力资金结构解读要求

表格新增了涨跌幅、超大单净流入、超大单净占比、大单净流入、大单净占比字段。
请在数据解读环节补充一段主力资金结构分析（1-2句话，自然融入正文，不要单独成段）：

- 如果某个流入主线的超大单净流入占比较高（超过该板块主力净流入的50%%），用"机构主导"或"主力集中布局"等术语点出
- 如果超大单和大单合计占比较高但都是小额，说明是机构+游资共同参与，可以用"多元资金共振"
- 如果超大单和大单都不显著（占比低），主要靠中单小单推动，说明是散户情绪驱动，可以用"散户主导""情绪化行情"
- 涨跌幅字段可辅助验证：净流入+涨幅为正=多头强势；净流入+涨幅为负或平=低位吸筹；净流出+跌幅=空头加速
- 不要罗列所有板块的结构数据，只挑最显著的特征讲1-2个

## 品牌定位
- 账号 Slogan："资金不会说谎，主线都会留下痕迹"
- 内容风格：专业数据解读，不荐股，仅记录市场

## 标题要求（第一行，开场白）
一句话抓住注意力，控制在20字以内，必须包含日期和维度（早盘/全天）。
推荐故事型标题："科技吸金超两百亿，发生了什么？"
也可用传统型："5月15日全天，机器人逆势吸金近二十亿"
标题单独一行，后面空一行再写正文。

## 正文结构（标题后的内容，结尾总结语）

按以下顺序组织，每段1-2句话：

1. **开头钩子**：疑问/冲突/数据型，独立一段
2. **整体定调**：一句话概括今天资金格局——是集中还是分散，是否有主线
3. **数据解读**：流入流出特征、板块表现分化情况、是否形成主线共振
4. **主力结构**：1-2句话点出机构/游资/散户主导特征（参考上方解读要求）
5. **趋势感**：结合历史文案，指出是否是连续趋势（如果有历史参考）
6. **收尾**：品牌结语 → 风险提示 → 互动提问，自然衔接

### 固定文案必须逐字使用
- 品牌结语："资金不会说谎，主线都会留下痕迹"
- 风险提示："以上数据仅供参考，不构成投资建议。股市有风险，投资需谨慎。"

## 合规红线

- 禁止投资建议、操作指导、买卖建议、预测涨跌、预判走势
- 禁止"抄底""建仓""加仓""止损""上车"等动作词
- 禁止"建议""推荐""应该""可以买""逢低"等引导性词汇
- 禁止"必涨""暴跌""血亏"等极端情绪化词汇
- 禁止暗示"跟着买就能赚钱"或保证收益

## 用语习惯

✅ 使用："资金关注""资金流入流出""获得资金青睐""板块表现分化""结构性特征""主线共振""此消彼长""机构主导""主力集中布局""多元资金共振""散户主导"
❌ 避免："买""卖""抄""建仓"等动作词
常用词：数据显示、资金流向、板块表现、关注度、特征明显
常用连接词：不过、同时、另外、与此同时、反观、而

## 口播风格示例

5月15日全天，科技吸金超两百亿

今天谁在疯狂吸金？答案是科技。半导体和AI应用两大方向，合计吸金超过一百亿，成为全天绝对主线，主力结构看超大单占比突出，机构集中布局特征明显。与此同时，资金从有色和银行流出，高低切换特征明显。另外CPO概念午后跟涨，算力产业链形成共振。

以上数据仅供参考，不构成投资建议。股市有风险，投资需谨慎。

资金不会说谎，主线都会留下痕迹。你最看好哪个方向？

正文控制在80到120字，纯文本口播稿，不做买卖建议。`,
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
