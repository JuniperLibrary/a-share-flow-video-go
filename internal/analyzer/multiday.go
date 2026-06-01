package analyzer

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
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// MultiDayAnalysis Bar Chart Race 视频所需的全部分析结果。
type MultiDayAnalysis struct {
	TrendInsights  []TrendInsight   `json:"trendInsights"`
	RankingChanges []RankingChange  `json:"rankingChanges"`
	SummaryText    SummaryText      `json:"summaryText"`
	TickerItems    []MultiDayTicker `json:"tickerItems"`
}

// TrendInsight 趋势洞察，用于封面和过渡动画文字。
type TrendInsight struct {
	Day         string `json:"day"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Sentiment   string `json:"sentiment"`
	Sector      string `json:"sector"`
}

// RankingChange 排名变化解读。
type RankingChange struct {
	FromRank    int    `json:"from_rank"`
	ToRank      int    `json:"to_rank"`
	Sector      string `json:"sector"`
	Description string `json:"description"`
	Sentiment   string `json:"sentiment"`
}

// SummaryText 视频结尾总结。
type SummaryText struct {
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	KeySectors []string `json:"key_sectors"`
}

// MultiDayTicker 底部滚动资讯（带日期）。
type MultiDayTicker struct {
	Day  string `json:"day"`
	Text string `json:"text"`
}

// MultiDayAnalyze 统一入口：优先 AI，失败降级数据驱动。
// copyMode: "ai" 优先 AI（失败降级模板），"template" 直接使用模板。
func MultiDayAnalyze(dayData map[string][]fetcher.Sector, dates []string, copyMode string) MultiDayAnalysis {
	logger.Info("多日分析开始",
		zap.String("copyMode", copyMode),
		zap.Int("days", len(dates)))

	if copyMode == "template" {
		logger.Info("多日分析：用户选择模板文案", zap.String("copyMode", copyMode))
		return DataDrivenMultiDay(dayData, dates)
	}

	aiCfg := config.GetAIConfig()
	if aiCfg.APIKey == "" {
		logger.Warn("多日分析：用户选择 AI 文案但未配置 AI_API_KEY，降级使用模板")
		return DataDrivenMultiDay(dayData, dates)
	}

	logger.Info("多日分析：尝试 AI 生成")
	result, err := AIGenerateMultiDay(dayData, dates, aiCfg)
	if err != nil {
		logger.Warn("多日分析：AI 请求失败，降级使用模板生成", zap.Error(err))
		return DataDrivenMultiDay(dayData, dates)
	}
	if result.TrendInsights == nil && result.SummaryText.Title == "" {
		logger.Warn("多日分析：AI 返回空结果，降级使用模板生成")
		return DataDrivenMultiDay(dayData, dates)
	}
	logger.Info("多日分析：AI 分析成功")
	return result
}

// AIGenerateMultiDay AI 生成多日 Bar Chart Race 内容。
func AIGenerateMultiDay(dayData map[string][]fetcher.Sector, dates []string, aiCfg config.AIConfig) (MultiDayAnalysis, error) {
	dataSummary := buildMultiDaySummary(dayData, dates)

	prompt := fmt.Sprintf(`你是一位A股市场资深分析师，专注于多日板块资金趋势的深度解读。你善于从跨日数据中识别主力资金的持续性行为和板块轮动规律。

## 任务

根据下方近%d日板块资金流向数据，生成四类内容：trendInsights、rankingChanges、summaryText、tickerItems。

## 视频形式

本视频采用「动态横向排名条形图」(Bar Chart Race) 形式：
- 画面中央为横向条形图，按板块主力资金净流入绝对值排名
- 条形从左到右随日期推进平滑移动，排名自动变化
- 流入板块显示为青色，流出板块显示为粉色
- 视频分为三段：第1日快照 → 过渡动画 → 第2日快照 → 过渡动画 → 第3日快照

## 数据摘要
%s

## 输出格式

严格按以下 JSON 格式输出，不要有任何额外文本：

{
  "trendInsights": [...],
  "rankingChanges": [...],
  "summaryText": {...},
  "tickerItems": [...]
}

## trendInsights（趋势洞察，5-8条）

用于视频封面和过渡动画期间的文字展示。

| 字段 | 要求 |
|------|------|
| day | 日期 "YYYY-MM-DD" |
| title | 趋势标题，10字以内，如"半导体连续三日获主力加仓" |
| description | 详细描述，30字以内，含板块名和具体数值，使用"主力资金涌入""资金出逃""板块轮动""情绪分化""放量突破""资金接力"等术语 |
| sentiment | "positive" / "negative" / "neutral" |
| sector | 核心板块名，必须是数据中实际存在的 |

分布：第1日2条，第2日2条，第3日2条。

## rankingChanges（排名变化解读，3-5条）

用于条形图排名变化时的高亮提示。只选择排名变化≥2位的板块。

| 字段 | 要求 |
|------|------|
| from_day | 前一日日期 |
| to_day | 当日日期 |
| from_rank | 前一日排名（1-based） |
| to_rank | 当日排名（1-based） |
| sector | 板块名 |
| description | 变化解读，15字以内，如"排名跃升3位，主力加速建仓" |
| sentiment | "positive" / "negative" / "neutral" |

## summaryText（总结文案，1条）

用于视频结尾的数据总结页。

| 字段 | 要求 |
|------|------|
| title | 总结标题，12字以内，如"近三日主力资金流向总结" |
| content | 总结正文，80字以内，概括整体趋势、累计净流入TOP3、累计净流出TOP3、市场情绪判断 |
| key_sectors | 关键板块列表，2-4个，对趋势影响最大的板块名 |

## tickerItems（底部滚动资讯，8-10条）

| 字段 | 要求 |
|------|------|
| day | 日期 "YYYY-MM-DD" |
| text | 资讯内容，15字以内，含板块名和数值，使用专业术语 |

## 排序约束

- trendInsights 的 day 从早到晚
- rankingChanges 按时间顺序排列
- tickerItems 的 day 从早到晚
- 板块名必须与数据摘要中完全一致
- 数值单位统一为"亿"，保留1位小数`, len(dates), dataSummary)

	body := map[string]any{
		"model":       aiCfg.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": 0.7,
		"max_tokens":  3000,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", aiCfg.BaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+aiCfg.APIKey)

	client := &http.Client{Timeout: 180 * time.Second}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt == 0 {
				time.Sleep(time.Second)
				continue
			}
			return MultiDayAnalysis{}, lastErr
		}

		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(b, &result); err != nil {
			return MultiDayAnalysis{}, fmt.Errorf("parse API response: %w", err)
		}
		if len(result.Choices) == 0 {
			return MultiDayAnalysis{}, fmt.Errorf("empty choices from API")
		}

		content := result.Choices[0].Message.Content
		jsonStr := extractJSON(content)
		if jsonStr == "" {
			logger.Warn("多日分析：大模型返回格式异常")
			return MultiDayAnalysis{}, nil
		}

		var data MultiDayAnalysis
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			logger.Warn("多日分析：JSON 解析失败", zap.Error(err))
			return MultiDayAnalysis{}, nil
		}

		logger.Info("多日分析：大模型生成",
			zap.Int("trends", len(data.TrendInsights)),
			zap.Int("changes", len(data.RankingChanges)),
			zap.Int("ticker", len(data.TickerItems)))
		return data, nil
	}

	return MultiDayAnalysis{}, lastErr
}

// DataDrivenMultiDay 数据驱动降级方案。
func DataDrivenMultiDay(dayData map[string][]fetcher.Sector, dates []string) MultiDayAnalysis {
	multiData := fetcher.BuildMultiDaySectorData(dayData, dates)

	var insights []TrendInsight
	var rankingChanges []RankingChange
	var tickers []MultiDayTicker

	// 趋势洞察：从连续流入/流出的板块生成
	for _, md := range multiData {
		if md.Trend == "连续流入" || md.Trend == "加速流入" {
			insights = append(insights, TrendInsight{
				Day:         md.Dates[len(md.Dates)-1],
				Title:       fmt.Sprintf("%s%s", md.Name, md.Trend),
				Description: fmt.Sprintf("%s累计净流入%.1f亿，主力资金持续涌入", md.Name, md.Total),
				Sentiment:   "positive",
				Sector:      md.Name,
			})
		} else if md.Trend == "连续流出" || md.Trend == "加速流出" {
			insights = append(insights, TrendInsight{
				Day:         md.Dates[len(md.Dates)-1],
				Title:       fmt.Sprintf("%s%s", md.Name, md.Trend),
				Description: fmt.Sprintf("%s累计净流出%.1f亿，资金持续撤离", md.Name, absF(md.Total)),
				Sentiment:   "negative",
				Sector:      md.Name,
			})
		}
		if len(insights) >= 8 {
			break
		}
	}

	// 排名变化：对比第1日和最后1日
	if len(dates) >= 2 {
		firstSectors := dayData[dates[0]]
		lastSectors := dayData[dates[len(dates)-1]]

		firstRank := make(map[string]int)
		for i, s := range firstSectors {
			firstRank[s.Name] = i + 1
		}

		for i, s := range lastSectors {
			if i >= 10 {
				break
			}
			if fr, ok := firstRank[s.Name]; ok {
				change := fr - (i + 1)
				if absInt(change) >= 2 {
					dir := "上升"
					sentiment := "positive"
					if change < 0 {
						dir = "下降"
						sentiment = "negative"
					}
					rankingChanges = append(rankingChanges, RankingChange{
						FromRank:    fr,
						ToRank:      i + 1,
						Sector:      s.Name,
						Description: fmt.Sprintf("排名%s%d位，主力%s", dir, absInt(change), map[string]string{"positive": "积极布局", "negative": "加速减仓"}[sentiment]),
						Sentiment:   sentiment,
					})
				}
			}
		}
	}

	// 资讯
	for _, date := range dates {
		sectors := dayData[date]
		for _, s := range sectors {
			if len(tickers) >= 10 {
				break
			}
			if s.Net > 0 {
				tickers = append(tickers, MultiDayTicker{
					Day:  date,
					Text: fmt.Sprintf("%s主力净流入%.1f亿，多头强势", s.Name, s.Net),
				})
			} else {
				tickers = append(tickers, MultiDayTicker{
					Day:  date,
					Text: fmt.Sprintf("%s主力净流出%.1f亿，空头主导", s.Name, absF(s.Net)),
				})
			}
		}
	}

	// 总结
	var topIn, topOut []fetcher.MultiDaySectorData
	for _, md := range multiData {
		if md.Total > 0 && len(topIn) < 3 {
			topIn = append(topIn, md)
		} else if md.Total < 0 && len(topOut) < 3 {
			topOut = append(topOut, md)
		}
	}

	var summaryParts []string
	summaryParts = append(summaryParts, fmt.Sprintf("近%d日主力资金流向", len(dates)))
	if len(topIn) > 0 {
		names := make([]string, len(topIn))
		for i, s := range topIn {
			names[i] = s.Name
		}
		summaryParts = append(summaryParts, fmt.Sprintf("累计净流入TOP3: %s", strings.Join(names, "、")))
	}
	if len(topOut) > 0 {
		names := make([]string, len(topOut))
		for i, s := range topOut {
			names[i] = s.Name
		}
		summaryParts = append(summaryParts, fmt.Sprintf("累计净流出TOP3: %s", strings.Join(names, "、")))
	}

	keySectors := make([]string, 0, 4)
	for _, s := range topIn {
		keySectors = append(keySectors, s.Name)
	}
	for _, s := range topOut {
		if len(keySectors) < 4 {
			keySectors = append(keySectors, s.Name)
		}
	}

	summary := MultiDayAnalysis{
		TrendInsights:  insights,
		RankingChanges: rankingChanges,
		TickerItems:    tickers,
		SummaryText: SummaryText{
			Title:      fmt.Sprintf("近%d日资金流向总结", len(dates)),
			Content:    strings.Join(summaryParts, "，"),
			KeySectors: keySectors,
		},
	}

	logger.Info("多日分析：数据驱动生成",
			zap.Int("trends", len(insights)),
			zap.Int("changes", len(rankingChanges)),
			zap.Int("ticker", len(tickers)))
	return summary
}

func buildMultiDaySummary(dayData map[string][]fetcher.Sector, dates []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## 近%d日A股板块资金流向数据 (%s → %s)\n\n",
		len(dates), dates[0], dates[len(dates)-1]))

	for i, date := range dates {
		sectors := dayData[date]
		dayLabel := map[int]string{1: "第1日", 2: "第2日", 3: "第3日", 4: "第4日", 5: "第5日"}[i+1]
		if dayLabel == "" {
			dayLabel = fmt.Sprintf("第%d日", i+1)
		}

		var inflow, outflow []fetcher.Sector
		for _, s := range sectors {
			if s.Net > 0 {
				inflow = append(inflow, s)
			} else {
				outflow = append(outflow, s)
			}
		}
		totalNet := 0.0
		for _, s := range sectors {
			totalNet += s.Net
		}

		sb.WriteString(fmt.Sprintf("### %s (%s)\n", dayLabel, date))
		sb.WriteString(fmt.Sprintf("- 板块总数: %d个 (流入%d/流出%d)\n", len(sectors), len(inflow), len(outflow)))
		sb.WriteString(fmt.Sprintf("- 合计净流入: %+.1f亿\n\n", totalNet))

		sort.Slice(inflow, func(i, j int) bool { return inflow[i].Net > inflow[j].Net })
		sb.WriteString("  流入TOP5:\n")
		for j, s := range inflow {
			if j >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("  - %s: %+.1f亿\n", s.Name, s.Net))
		}

		sort.Slice(outflow, func(i, j int) bool { return outflow[i].Net < outflow[j].Net })
		sb.WriteString("  流出TOP5:\n")
		for j, s := range outflow {
			if j >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("  - %s: %+.1f亿\n", s.Name, s.Net))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### 跨日趋势提示\n")
	allSectors := make(map[string][]float64)
	for _, date := range dates {
		for _, s := range dayData[date] {
			allSectors[s.Name] = append(allSectors[s.Name], s.Net)
		}
	}
	for name, nets := range allSectors {
		if len(nets) == len(dates) {
			trend := classifyTrendSimple(nets)
			if trend != "波动" {
				sb.WriteString(fmt.Sprintf("- %s: %s (%s)\n", name, trend, formatNets(nets)))
			}
		}
	}

	return sb.String()
}

func classifyTrendSimple(nets []float64) string {
	allPositive := true
	allNegative := true
	for _, n := range nets {
		if n <= 0 {
			allPositive = false
		}
		if n >= 0 {
			allNegative = false
		}
	}
	if allPositive {
		return "连续流入"
	}
	if allNegative {
		return "连续流出"
	}
	return "波动"
}

func formatNets(nets []float64) string {
	parts := make([]string, len(nets))
	for i, n := range nets {
		parts[i] = fmt.Sprintf("%+.1f亿", n)
	}
	return strings.Join(parts, " → ")
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
