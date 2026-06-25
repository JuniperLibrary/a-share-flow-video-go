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
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

const debatePromptWithData = `你是一场"财报深度辩论"的参与者。你和对手需要围绕财报数据展开有逻辑、有深度的攻防辩论。不是念数据，而是用数据构建论点。

【重要】直接输出最终 JSON 字符串，不要分析、不要推理、不要"让我想想"。读完数据后立即动手写。

## 辩论规则

1. **8-12 轮**（偶数）。超过就是超时。
2. **每轮 25-60 字**。太短说不清逻辑，太长破坏节奏。
3. **每轮至少引用一个核心数字**，但不能只有数字——要加上你的判断和逻辑。
4. **每个论点必须包含：你的判断 + 依据的数据 + 逻辑分析**。例如"营收 3456 增 12%——这个增速在 2000 亿体量里是第一梯队，说明规模效应仍在释放"，而不是"营收 3456 增 12%"。
5. **第 2 轮起必须直接回应对方的论点**，指出其逻辑漏洞或被忽略的维度。"你说 A 数据好，但你没看到 B 维度..."，不能自说自话。
6. **最后 2 轮可以总结你的核心论点**，不需要一直扔新数据。
7. **同一数据可以反复用**——赢家是把同一个数据解读得更深的一方，不是换数据最快的一方。
8. **人格**：Bull 盯增长和竞争力，Bear 盯估值和风险。不要角色互换。允许反问句，但要论证支撑。

## 数据来源

以下是精确数据，不能编造、不能模糊化：

{{METRICS}}

完整财报原文（辅助定性判断）：

{{REPORT}}

## 语言红线

❌ "众所周知""值得关注""长期来看""从...来看"
❌ 空洞判断："这显示出...""这反映了..."
❌ 四字成语连续使用（稳中向好、动能强劲等）
❌ 对称写法：Bull 说"增长快"→ Bear 只说"增长慢"。必须在同一数据上找出不同解读维度和逻辑。
❌ 只扔数据不分析。"营收 3456 亿"不是论点，"营收 3456 亿 +12%，说明..."才是论点。

## 辩论示范

数据：营收 3456 亿 +12.3%，净利 567 亿 +8.9%，合同负债 -11%，PE 32 倍，行业平均 21 倍

bull "营收 3456 增 12%。这个体量还在加速——Q3 单季 15% 比上半年快，说明需求不仅没见顶还在走强。"
bear "营收增速 12% 看着不错，但净利只增 8.9%，利润增速跑输收入。毛利率 91.53% 没变，说明费用端在膨胀，利润质量在下滑。"
bull "利润跑输收入确实该关注，但拆开看：销售费用投了系列酒，这是战略性投入。系列酒增 24%，毛利率低于茅台酒但绝对值可观，长期是第二增长曲线。"
bear "系列酒增 24% 但规模才 193 亿，占营收 16%，拉不动大盘。更关键的是合同负债掉 11%——经销商打款意愿下降，前瞻指标亮了红灯。"

## 输出格式

严格 JSON，不要任何包裹文本：

{"turns":[{"index":0,"speaker":"bull","text":"...","emotion":"confident"}, ...]}`

const debatePrompt = `你是一场"财报深度辩论"的参与者。你和对手需要围绕财报数据展开有逻辑、有深度的攻防辩论。不是念数据，而是用数据构建论点。

【重要】直接输出最终 JSON 字符串，不要分析、不要推理、不要"让我想想"。读完数据后立即动手写。

## 辩论规则

1. **8-12 轮**（偶数）。超过就是超时。
2. **每轮 25-60 字**。太短说不清逻辑，太长破坏节奏。
3. **每轮至少引用一个核心数字**，但不能只有数字——要加上你的判断和逻辑。
4. **每个论点必须包含：你的判断 + 依据的数据 + 逻辑分析**。例如"营收 3456 增 12%——这个增速在 2000 亿体量里是第一梯队，说明规模效应仍在释放"，而不是"营收 3456 增 12%"。
5. **第 2 轮起必须直接回应对方的论点**，指出其逻辑漏洞或被忽略的维度。"你说 A 数据好，但你没看到 B 维度..."，不能自说自话。
6. **最后 2 轮可以总结你的核心论点**，不需要一直扔新数据。
7. **同一数据可以反复用**——赢家是把同一个数据解读得更深的一方，不是换数据最快的一方。
8. **人格**：Bull 盯增长和竞争力，Bear 盯估值和风险。不要角色互换。允许反问句，但要论证支撑。

## 数据来源

以下是你唯一的财报数据来源。不能编造、不能模糊化。

%s

## 语言红线

❌ "众所周知""值得关注""长期来看""从...来看"
❌ 空洞判断："这显示出...""这反映了..."
❌ 四字成语连续使用（稳中向好、动能强劲等）
❌ 对称写法：Bull 说"增长快"→ Bear 只说"增长慢"。必须在同一数据上找出不同解读维度和逻辑。
❌ 只扔数据不分析。"营收 3456 亿"不是论点，"营收 3456 亿 +12%，说明..."才是论点。

## 辩论示范

数据：营收 3456 亿 +12.3%，净利 567 亿 +8.9%，合同负债 -11%，PE 32 倍，行业平均 21 倍

bull "营收 3456 增 12%。这个体量还在加速——Q3 单季 15% 比上半年快，说明需求不仅没见顶还在走强。"
bear "营收增速 12% 看着不错，但净利只增 8.9%，利润增速跑输收入。毛利率 91.53% 没变，说明费用端在膨胀，利润质量在下滑。"
bull "利润跑输收入确实该关注，但拆开看：销售费用投了系列酒，这是战略性投入。系列酒增 24%，毛利率低于茅台酒但绝对值可观，长期是第二增长曲线。"
bear "系列酒增 24% 但规模才 193 亿，占营收 16%，拉不动大盘。更关键的是合同负债掉 11%——经销商打款意愿下降，前瞻指标亮了红灯。"

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

	logger.Info("辩论(legacy)开始",
		zap.Int("reportLen", len(reportText)),
		zap.Bool("hasStructuredData", len(structuredData) > 0 && structuredData[0] != nil),
	)

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
		logger.Error("辩论(legacy) LLM 调用失败", zap.Error(err))
		return Script{}, fmt.Errorf("LLM 编排失败: %w", err)
	}

	script, parseErr := parseScript(raw)
	if parseErr != nil {
		logger.Warn("辩论(legacy) 解析 LLM 输出失败",
			zap.Int("rawLen", len(raw)),
			zap.String("rawPreview", truncate(raw, 200)),
			zap.Error(parseErr),
		)
		return Script{}, parseErr
	}

	script.ReportHash = hashReport(reportText)
	logger.Info("辩论(legacy)完成", zap.Int("turns", len(script.Turns)))
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

	client := &http.Client{Timeout: 180 * time.Second}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, _ := http.NewRequest("POST", aiCfg.BaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+aiCfg.APIKey)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < 2 && strings.Contains(err.Error(), "context deadline exceeded") {
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
				continue
			}
			return "", fmt.Errorf("LLM 请求失败: %w", err)
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
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
	return "", fmt.Errorf("LLM 请求失败: %w", lastErr)
}

func parseScript(raw string) (Script, error) {
	var wrapper struct {
		Turns []Turn `json:"turns"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return Script{}, fmt.Errorf("JSON 解析失败: %w\n原始响应: %s", err, truncate(raw, 200))
	}
	if len(wrapper.Turns) < 8 || len(wrapper.Turns) > 12 || len(wrapper.Turns)%2 != 0 {
		return Script{}, fmt.Errorf("轮数需在 8-12 之间且为偶数，实际 %d 轮", len(wrapper.Turns))
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
