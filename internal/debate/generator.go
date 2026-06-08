package debate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
)

const debatePromptWithData = `你是一个参加"60 秒数据攻防"的选手。你和对手每人每次只能说一句话、扔一个数字，像乒乓球一样来回。不要结论、不要总结、不要客套。

【重要】直接输出最终 JSON 字符串，不要分析、不要推理、不要"让我想想"。读完数据表格后立即动手写。

## 硬规则

1. **10-16 轮**（偶数，数据丰富就选多的）。超过就是超时。
2. **每轮 8-20 字**，不能再多。20 字大概读 3 秒。
3. **每轮必须有且仅有一个具体数字**。没数字的轮次等于废牌。
4. **每个 turn 只传递一个信息点**。不要在一个 turn 里塞两个论点。
5. **第 3 轮起必须引用对方上一轮的数据**。"你说 X？但 Y 说明..."——不能自说自话。
6. **最后 3 轮仍然要扔新数据**——禁止做"总结来说"式收尾。
7. **禁止连续两轮反问句**。
8. **人格**：牛方盯着趋势和弹性，熊方盯着估值和风险。不要角色互換。

## 数据来源

数据表格是你唯一能用的数据。不能编造、不能模糊化。下面是精确数据：

{{METRICS}}

完整财报原文（辅助定性判断）：

{{REPORT}}

## 语言红线

❌ "众所周知""值得关注""长期来看""从...来看"
❌ 四字成语连续使用（稳中向好、动能强劲等）
❌ 空洞判断："这显示出...""这反映了..."
❌ 对称写法：牛说增长好→熊就说增长差。必须在同一数据上找出不同解读维度。

## 节奏示范

数据：营收 3456 亿 +12.3%，净利 567 亿 +8.9%，合同负债 -11%，PE 32 倍，行业平均 21 倍

bull "营收 3456 增 12%，这个体量还在跑。"
bear "合同负债掉 11%。经销商不压货，信号。"
bull "净利率 50.5%，还在往上走。"
bear "PE 32 倍，行业 21。溢价 50% 买什么？"
bull "净利 567 亿，账上现金 800 亿。"
bear "现金多但分红率 52%。另一半去哪了？"
bull "增速 15%，PEG 才 2。不贵。"
bear "行业 PEG 中位数 1.2。你贵了 60%。"
bull "经营现金流 +35%。真金白银。"
bear "应收涨了 20%。利润是数字，现金才是命。"

## 输出格式

严格 JSON，不要任何包裹文本：

{"turns":[{"index":0,"speaker":"bull","text":"...","emotion":"confident"}, ...]}`

const debatePrompt = `你是一个参加"60 秒数据攻防"的选手。你和对手每人每次只能说一句话、扔一个数字，像乒乓球一样来回。不要结论、不要总结、不要客套。

【重要】直接输出最终 JSON 字符串，不要分析、不要推理、不要"让我想想"。读完数据后立即动手写。

## 硬规则

1. **10-16 轮**（偶数，数据丰富就选多的）。超过就是超时。
2. **每轮 8-20 字**，不能再多。20 字大概读 3 秒。
3. **每轮必须有且仅有一个具体数字**。没数字的轮次等于废牌。
4. **每个 turn 只传递一个信息点**。不要在一个 turn 里塞两个论点。
5. **第 3 轮起必须引用对方上一轮的数据**。"你说 X？但 Y 说明..."——不能自说自话。
6. **最后 3 轮仍然要扔新数据**——禁止做"总结来说"式收尾。
7. **禁止连续两轮反问句**。
8. **人格**：牛方盯着趋势和弹性，熊方盯着估值和风险。不要角色互換。

## 数据来源

下面是你唯一的财报数据来源。不能编造，不能模糊化。

%s

## 语言红线

❌ "众所周知""值得关注""长期来看""从...来看"
❌ 四字成语连续使用（稳中向好、动能强劲等）
❌ 空洞判断："这显示出...""这反映了..."
❌ 对称写法：牛说增长好→熊就说增长差。必须在同一数据上找出不同解读维度。

## 节奏示范

数据：营收 3456 亿 +12.3%，净利 567 亿 +8.9%，合同负债 -11%，PE 32 倍，行业平均 21 倍

bull "营收 3456 增 12%，这个体量还在跑。"
bear "合同负债掉 11%。经销商不压货，信号。"
bull "净利率 50.5%，还在往上走。"
bear "PE 32 倍，行业 21。溢价 50% 买什么？"
bull "净利 567 亿，账上现金 800 亿。"
bear "现金多但分红率 52%。另一半去哪了？"
bull "增速 15%，PEG 才 2。不贵。"
bear "行业 PEG 中位数 1.2。你贵了 60%。"
bull "经营现金流 +35%。真金白银。"
bear "应收涨了 20%。利润是数字，现金才是命。"

## 输出格式

严格 JSON，不要任何包裹文本：

{"turns":[{"index":0,"speaker":"bull","text":"...","emotion":"confident"}, ...]}`

func structuredPrompt(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	getF := func(key string) float64 {
		v, ok := data[key]
		if !ok || v == nil {
			return 0
		}
		switch n := v.(type) {
		case float64:
			return n
		case string:
			f, _ := strconv.ParseFloat(n, 64)
			return f
		}
		return 0
	}
	getS := func(key string) string {
		v, ok := data[key]
		if !ok || v == nil {
			return ""
		}
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}

	var b strings.Builder
	if name := getS("name"); name != "" {
		b.WriteString(fmt.Sprintf("股票: %s(%s)\n", name, getS("code")))
	}
	b.WriteString(fmt.Sprintf("报告期: %s\n", getS("reportDate")))
	b.WriteString(fmt.Sprintf("报告类型: %s\n", getS("reportType")))

	rev := getF("revenue") / 1e8
	revYoY := getF("revenueYoY")
	np := getF("netProfit") / 1e8
	npYoY := getF("netProfitYoY")
	dr := getF("deductedProfit") / 1e8
	op := getF("operatingProfit") / 1e8

	b.WriteString(fmt.Sprintf("\n核心指标|数值|同比\n"))
	b.WriteString(fmt.Sprintf("营业收入|%.2f亿|%+.1f%%\n", rev, revYoY))
	b.WriteString(fmt.Sprintf("归母净利润|%.2f亿|%+.1f%%\n", np, npYoY))
	b.WriteString(fmt.Sprintf("扣非净利润|%.2f亿\n", dr))
	b.WriteString(fmt.Sprintf("营业利润|%.2f亿\n", op))
	b.WriteString(fmt.Sprintf("毛利率|%.1f%%\n", getF("grossMargin")))
	b.WriteString(fmt.Sprintf("净利率|%.1f%%\n", getF("netMargin")))
	if roe := getF("roe"); roe > 0 {
		b.WriteString(fmt.Sprintf("ROE|%.1f%%\n", roe))
	}
	if eps := getF("eps"); eps > 0 {
		b.WriteString(fmt.Sprintf("EPS|%.2f元\n", eps))
	}
	b.WriteString(fmt.Sprintf("资产负债率|%.1f%%\n", getF("debtAssetRatio")))
	if cr := getF("currentRatio"); cr > 0 {
		b.WriteString(fmt.Sprintf("流动比率|%.1f\n", cr))
	}
	b.WriteString(fmt.Sprintf("合同负债|%.2f亿\n", getF("contractLiability")/1e8))
	b.WriteString(fmt.Sprintf("经营现金流|%.2f亿\n", getF("operatingCashFlow")/1e8))

	return b.String()
}

func GenerateScript(reportText string, aiCfg config.AIConfig, structuredData ...map[string]any) (Script, error) {
	reportText = strings.TrimSpace(reportText)
	if reportText == "" {
		return Script{}, fmt.Errorf("财报文本不能为空")
	}
	if len(reportText) > 15000 {
		reportText = reportText[:15000]
	}

	var extra string
	if len(structuredData) > 0 && structuredData[0] != nil {
		extra = structuredPrompt(structuredData[0])
	}

	var prompt string
	if extra != "" {
		prompt = strings.Replace(debatePromptWithData, "{{METRICS}}", extra, 1)
		prompt = strings.Replace(prompt, "{{REPORT}}", reportText, 1)
	} else {
		prompt = strings.Replace(debatePrompt, "%s", reportText, 1)
	}

	raw, err := callLLM(aiCfg, prompt, 0.8, 2000)
	if err != nil {
		return Script{}, fmt.Errorf("LLM 编排失败: %w", err)
	}

	script, parseErr := parseScript(raw)
	if parseErr != nil {
		return Script{}, parseErr
	}

	script.ReportHash = hashReport(reportText)
	return script, nil
}

func callLLM(aiCfg config.AIConfig, prompt string, temperature float64, maxTokens int) (string, error) {
	body := map[string]any{
		"model":       aiCfg.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", aiCfg.BaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+aiCfg.APIKey)

	client := &http.Client{Timeout: 90 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", fmt.Errorf("LLM 响应读取失败: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("LLM 响应解析失败: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM 返回空 choices")
	}

	raw := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	return stripCodeFence(raw), nil
}

func parseScript(raw string) (Script, error) {
	var wrapper struct {
		Turns []Turn `json:"turns"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return Script{}, fmt.Errorf("JSON 解析失败: %w\n原始响应: %s", err, truncate(raw, 200))
	}
	if len(wrapper.Turns) < 8 || len(wrapper.Turns) > 16 || len(wrapper.Turns)%2 != 0 {
		return Script{}, fmt.Errorf("轮数需在 8-16 之间且为偶数，实际 %d 轮", len(wrapper.Turns))
	}
	for i, t := range wrapper.Turns {
		want := Bull
		if i%2 == 1 {
			want = Bear
		}
		if t.Speaker != want {
			return Script{}, fmt.Errorf("第 %d 轮 speaker 应为 %s，实际 %s", i, want, t.Speaker)
		}
		if strings.TrimSpace(t.Text) == "" {
			return Script{}, fmt.Errorf("第 %d 轮 text 为空", i)
		}
		wrapper.Turns[i].Index = i
		if wrapper.Turns[i].Emotion == "" {
			wrapper.Turns[i].Emotion = "neutral"
		}
	}
	return Script{Turns: wrapper.Turns}, nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx > 0 {
			s = s[idx+1:]
		}
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
	}
	return strings.TrimSpace(s)
}

func hashReport(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
