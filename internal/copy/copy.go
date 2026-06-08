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

func GenerateCopywritingAI(sectors []fetcher.Sector, dateStr, session string) (string, error) {
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

	inflowTop5 := formatPairs(inflows, 5, true)
	outflowTop5 := formatPairs(outflows, 5, false)

	prompt := fmt.Sprintf(`你是一个每天追着主力资金跑的散户，今天A股收盘了，你要用大白话发一条抖音。

## 你的任务

从数据中找出一个有趣的故事，不要报数字，要讲故事。

## 故事结构（5个场景）

1. [钩子] 制造好奇心：今天有个反常现象
2. [悬念] 提出疑问：为什么会出现这种情况？
3. [反转] 揭示真相：更离谱的是...
4. [收尾] 给出答案：解释为什么
5. [钩子] 留下悬念：明天会怎样？

## 输出格式

每行一个场景，格式：[场景名] 内容
总共5行，每行15-25字，像跟朋友聊天一样自然。

## 风格要求

- 像发朋友圈，不要公文腔
- 用口语："今天XX疯了""主力在偷偷XX""散户被套了"
- 数字只说整数："五十亿"不写"50.5亿"
- 不要用"首先、其次、最后、综上所述"
- 不要用 emoji、markdown、编号
- 不要写风险提示，不要写品牌slogan

## 今天的数据

净流入/流出：%.0f亿
流入板块：%d个，流出板块：%d个
流入TOP5：%s
流出TOP5：%s

## 示例

[钩子] 今天半导体疯了，主力直接砸进去五十亿
[悬念] 但奇怪的是，散户在疯狂追涨
[反转] 更离谱的是，银行被抛售了三十亿
[收尾] 主力在调仓，从金融转科技
[钩子] 明天还能继续追半导体吗？

现在根据今天的数据，找一个有趣的故事讲出来。`,
		netTotal, len(inflows), len(outflows),
		inflowTop5, outflowTop5,
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
