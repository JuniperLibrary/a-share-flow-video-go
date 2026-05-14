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
)

type TimelineEvent struct {
	Time        string `json:"time"`
	TimeMinutes int    `json:"timeMinutes"`
	Sector      string `json:"sector"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Sentiment   string `json:"sentiment"`
}

type TickerItem struct {
	Time string `json:"time"`
	Text string `json:"text"`
}

type MarketEvent struct {
	EventType  string `json:"event_type"`
	Frame      int    `json:"frame"`
	Text       string `json:"text"`
	Subtext    string `json:"subtext"`
	Importance int    `json:"importance"`
}

// A股交易时段：上午 09:30-11:30(0-120分钟)，下午 13:00-15:00(210-330分钟)
// timeMinutes 以 09:30 为起点 0，每 60 分钟 = 60 单位
var tradingRanges = [][2]int{{0, 120}, {210, 330}}

func isValidTimeMinutes(m int) bool {
	for _, r := range tradingRanges {
		if m >= r[0] && m <= r[1] {
			return true
		}
	}
	return false
}

// clampTimeline 将 AI 生成的时间线事件校正到合法交易时段内。
// 处理三种越界情况：(1) 开盘前 → 拉到 09:30 (2) 午休 11:30-13:00 → 推到 13:00 (3) 收盘后 → 拉到 15:00
// 校正后按 timeMinutes 升序重排。
func clampTimeline(events []TimelineEvent) []TimelineEvent {
	fixed := make([]TimelineEvent, 0, len(events))
	for _, ev := range events {
		m := ev.TimeMinutes
		if !isValidTimeMinutes(m) {
			if m < 0 {
				m = 0
			} else if m < 120 {
				m = min(m, 120)
			} else if m < 210 {
				m = 210
			} else {
				m = min(m, 330)
			}
			ev.TimeMinutes = m
			if m <= 120 {
				ev.Time = fmt.Sprintf("%02d:%02d", 9+m/60, m%60)
			} else {
				ev.Time = fmt.Sprintf("%02d:%02d", 13+(m-210)/60, (m-210)%60)
			}
		}
		fixed = append(fixed, ev)
	}
	sort.Slice(fixed, func(i, j int) bool {
		return fixed[i].TimeMinutes < fixed[j].TimeMinutes
	})
	return fixed
}

// DataDrivenGenerate 基于板块资金流数据，数据驱动地生成视频所需的三类内容：
// 1. MarketEvent: 底部弹窗事件（开盘/领涨/承压/收盘）
// 2. TimelineEvent: 时间线事件（开盘活跃/持续走强/午后强势/尾盘动向等）
// 3. TickerItem: 底部滚动资讯
// 当 AI API 不可用时作为降级方案使用。
func DataDrivenGenerate(sectors []fetcher.Sector) ([]MarketEvent, []TimelineEvent, []TickerItem) {
	all := make([]fetcher.Sector, 0, len(sectors))
	all = append(all, sectors...)

	sort.Slice(all, func(i, j int) bool {
		return absF(all[i].Net) > absF(all[j].Net)
	})

	var topIn, topOut []fetcher.Sector
	for _, s := range all {
		if s.Net > 0 && len(topIn) < 4 {
			topIn = append(topIn, s)
		} else if s.Net < 0 && len(topOut) < 4 {
			topOut = append(topOut, s)
		}
	}

	inflowCount := 0
	for _, s := range all {
		if s.Net > 0 {
			inflowCount++
		}
	}
	outflowCount := len(all) - inflowCount
	totalNet := 0.0
	for _, s := range all {
		totalNet += s.Net
	}

	var timeline []TimelineEvent

	if len(topIn) > 0 {
		timeline = append(timeline, TimelineEvent{
			Time: "09:35", TimeMinutes: 5,
			Sector:      topIn[0].Name,
			Title:       fmt.Sprintf("%s开盘活跃", topIn[0].Name),
			Description: fmt.Sprintf("资金净流入%.1f亿", topIn[0].Net),
			Sentiment:   "positive",
		})
	} else {
		timeline = append(timeline, TimelineEvent{
			Time: "09:35", TimeMinutes: 5,
			Sector: "市场", Title: "A股开盘异动",
			Description: "主力资金持续流入", Sentiment: "positive",
		})
	}

	if len(topIn) > 1 {
		timeline = append(timeline, TimelineEvent{
			Time: "10:00", TimeMinutes: 30,
			Sector:      topIn[1].Name,
			Title:       fmt.Sprintf("%s资金涌入", topIn[1].Name),
			Description: fmt.Sprintf("净流入%.1f亿", topIn[1].Net),
			Sentiment:   "positive",
		})
	}

	if len(topOut) > 0 {
		timeline = append(timeline, TimelineEvent{
			Time: "10:20", TimeMinutes: 50,
			Sector:      topOut[0].Name,
			Title:       fmt.Sprintf("%s资金流出", topOut[0].Name),
			Description: fmt.Sprintf("净流出%.1f亿", absF(topOut[0].Net)),
			Sentiment:   "negative",
		})
	}

	if len(topIn) > 2 {
		timeline = append(timeline, TimelineEvent{
			Time: "10:45", TimeMinutes: 75,
			Sector:      topIn[2].Name,
			Title:       fmt.Sprintf("%s持续走强", topIn[2].Name),
			Description: fmt.Sprintf("净流入%.1f亿", topIn[2].Net),
			Sentiment:   "positive",
		})
	}

	if len(topIn) > 0 {
		s := topIn[0]
		timeline = append(timeline, TimelineEvent{
			Time: "11:10", TimeMinutes: 100,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s早盘领涨", s.Name),
			Description: fmt.Sprintf("净流入突破%.0f亿", absF(s.Net)),
			Sentiment:   "positive",
		})
	}

	if len(topOut) > 1 {
		timeline = append(timeline, TimelineEvent{
			Time: "11:25", TimeMinutes: 115,
			Sector:      topOut[1].Name,
			Title:       fmt.Sprintf("%s午前承压", topOut[1].Name),
			Description: fmt.Sprintf("净流出%.1f亿", absF(topOut[1].Net)),
			Sentiment:   "negative",
		})
	}

	if len(topIn) > 0 {
		s := topIn[0]
		timeline = append(timeline, TimelineEvent{
			Time: "13:10", TimeMinutes: 220,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s午后强势", s.Name),
			Description: fmt.Sprintf("资金持续涌入，净流入%.1f亿", s.Net),
			Sentiment:   "positive",
		})
	}

	if len(topIn) > 1 {
		timeline = append(timeline, TimelineEvent{
			Time: "13:40", TimeMinutes: 250,
			Sector:      topIn[1].Name,
			Title:       fmt.Sprintf("%s午后发力", topIn[1].Name),
			Description: fmt.Sprintf("净流入%.1f亿", topIn[1].Net),
			Sentiment:   "positive",
		})
	}

	if len(topOut) > 0 {
		s := topOut[0]
		timeline = append(timeline, TimelineEvent{
			Time: "14:05", TimeMinutes: 275,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s加速流出", s.Name),
			Description: fmt.Sprintf("净流出%.1f亿，资金离场", absF(s.Net)),
			Sentiment:   "negative",
		})
	}

	sentimentDesc := "偏多"
	if outflowCount > inflowCount {
		sentimentDesc = "偏空"
	} else if outflowCount == inflowCount {
		sentimentDesc = "均衡"
	}
	timeline = append(timeline, TimelineEvent{
		Time: "14:30", TimeMinutes: 300,
		Sector:      "市场",
		Title:       fmt.Sprintf("市场情绪%s", sentimentDesc),
		Description: fmt.Sprintf("净流入%d vs 流出%d板块", inflowCount, outflowCount),
		Sentiment:   "neutral",
	})

	timeline = append(timeline, TimelineEvent{
		Time: "14:50", TimeMinutes: 320,
		Sector: "市场", Title: "尾盘资金动向",
		Description: fmt.Sprintf("全天合计%+.1f亿", totalNet),
		Sentiment:   mapSentiment(totalNet),
	})

	var ticker []TickerItem
	ticker = append(ticker, TickerItem{Time: "09:30", Text: "A股开盘，板块资金流向实时更新"})
	for _, s := range topIn {
		if len(ticker) >= 10 {
			break
		}
		ticker = append(ticker, TickerItem{Time: "10:00", Text: fmt.Sprintf("%s净流入%.1f亿", s.Name, s.Net)})
	}
	for _, s := range topOut {
		if len(ticker) >= 10 {
			break
		}
		ticker = append(ticker, TickerItem{Time: "11:00", Text: fmt.Sprintf("%s净流出%.1f亿", s.Name, absF(s.Net))})
	}
	ticker = append(ticker, TickerItem{Time: "13:30", Text: fmt.Sprintf("市场整体%+.1f亿", totalNet)})
	ticker = append(ticker, TickerItem{Time: "14:55", Text: "尾盘资金加速流动"})

	var events []MarketEvent
	events = append(events, MarketEvent{
		EventType: "market", Frame: 3,
		Text:       "A股开盘",
		Subtext:    fmt.Sprintf("资金%s，%d板块流入", sentimentDesc, inflowCount),
		Importance: 2,
	})
	if len(topIn) > 0 {
		events = append(events, MarketEvent{
			EventType: "concentration", Frame: 15,
			Text:       fmt.Sprintf("%s领涨", topIn[0].Name),
			Subtext:    fmt.Sprintf("净流入%.1f亿", topIn[0].Net),
			Importance: 3,
		})
	}
	if len(topIn) > 1 {
		events = append(events, MarketEvent{
			EventType: "sentiment", Frame: 30,
			Text:       fmt.Sprintf("%s资金涌入", topIn[1].Name),
			Subtext:    fmt.Sprintf("净流入%.1f亿", topIn[1].Net),
			Importance: 2,
		})
	}
	if len(topOut) > 0 {
		events = append(events, MarketEvent{
			EventType: "aberration", Frame: 45,
			Text:       fmt.Sprintf("%s承压", topOut[0].Name),
			Subtext:    fmt.Sprintf("净流出%.1f亿", absF(topOut[0].Net)),
			Importance: 2,
		})
	}
	if len(topIn) > 2 {
		events = append(events, MarketEvent{
			EventType: "rotation", Frame: 60,
			Text:       fmt.Sprintf("%s持续走强", topIn[2].Name),
			Subtext:    fmt.Sprintf("资金净流入%.1f亿", topIn[2].Net),
			Importance: 2,
		})
	}
	if len(topOut) > 1 {
		events = append(events, MarketEvent{
			EventType: "aberration", Frame: 72,
			Text:       fmt.Sprintf("%s资金离场", topOut[1].Name),
			Subtext:    fmt.Sprintf("净流出%.1f亿", absF(topOut[1].Net)),
			Importance: 1,
		})
	}
	if len(topIn) > 0 {
		events = append(events, MarketEvent{
			EventType: "sentiment", Frame: 82,
			Text:       fmt.Sprintf("%s午后强势", topIn[0].Name),
			Subtext:    fmt.Sprintf("净流入%.1f亿，资金加速", topIn[0].Net),
			Importance: 3,
		})
	}
	events = append(events, MarketEvent{
		EventType: "market", Frame: 95,
		Text:       "收盘",
		Subtext:    fmt.Sprintf("全天%+.1f亿", totalNet),
		Importance: 2,
	})

	fmt.Printf("  [事件分析] 数据驱动生成: %d个事件 + %d条时间线 + %d条资讯\n", len(events), len(timeline), len(ticker))
	return events, timeline, ticker
}

func AIGenerate(sectors []fetcher.Sector, dateStr string, aiCfg config.AIConfig) ([]MarketEvent, []TimelineEvent, []TickerItem, error) {
	dateDisplay := parseDateDisplay(dateStr)

	sectorsIn := filterAndSort(sectors, true)
	sectorsOut := filterAndSort(sectors, false)

	totalIn := sumNet(sectorsIn)
	totalOut := absF(sumNet(sectorsOut))
	totalNet := totalIn - totalOut

	dataSummary := fmt.Sprintf(`## %s A股板块资金流向数据

### 整体概况
- 板块总数: %d个 (流入%d/流出%d)
- 合计净流入: %+.1f亿

### TOP5 流入
%s

### TOP5 流出
%s`,
		dateDisplay,
		len(sectors), len(sectorsIn), len(sectorsOut),
		totalNet,
		formatTop5(sectorsIn),
		formatTop5(sectorsOut),
	)

	prompt := fmt.Sprintf(`你是一位A股市场资深分析师。请根据以下板块资金流向数据，分析并生成三类内容。

## 数据摘要
%s

## A股交易时间规则
- 上午: 09:30 - 11:30
- 午休: 11:30 - 13:00（闭盘，不产生事件）
- 下午: 13:00 - 15:00
- 所有事件的 time 字段必须在以上交易时段内，禁止出现 11:31-12:59 的时间

## 输出要求
请严格按以下 JSON 格式输出一个对象，包含三个字段：

### 1. timelineEvents（市场事件时间线，10-12个）
- time: 时间 "HH:MM"（必须在 09:30-11:30 或 13:00-15:00 范围内）
- timeMinutes: 从09:30起的分钟数（如09:35=5, 10:15=45, 13:15=225, 14:10=310）
- sector: 相关板块名（必须是数据中实际存在的板块）
- title: 事件标题（10字以内，包含板块名）
- description: 事件描述（20字以内，使用专业术语如"主力资金涌入""资金出逃""板块轮动""情绪分化""量能萎缩""放量突破"，并包含具体数值）
- sentiment: "positive" / "negative" / "neutral"
- 时间分布建议：09:30-10:00 至少2个，10:00-11:00 至少2个，11:00-11:30 至少1个，13:00-14:00 至少2个，14:00-15:00 至少2个

### 2. tickerItems（底部滚动资讯，10-12条）
- time: 时间 "HH:MM"（必须在交易时段内）
- text: 资讯内容（15字以内，包含板块名和数值）

### 3. events（底部弹窗事件，8-10个）
- event_type: "market"/"sentiment"/"rotation"/"aberration"
- frame: 时间位置百分比（0-100）
- text: 主标题（10字以内）
- subtext: 副标题（20字以内）
- importance: 重要程度 1/2/3
- 时间分布建议：frame 5-15 开盘，20-40 早盘，45-65 午盘前，70-85 午盘后，90-98 收盘

## 注意事项
- timelineEvents 的 timeMinutes 必须按升序排列
- tickerItems 的 time 从早到晚排列
- events 的 frame 从低到高分布（开盘5%%，盘中30-60%%，收盘90%%+）
- 所有内容必须基于实际数据，不要编造数据
- 板块名和数值必须与数据摘要一致
- 必须输出有效的 JSON 对象，不要有其他内容

输出格式：
{
  "timelineEvents": [...],
  "tickerItems": [...],
  "events": [...]
}`, dataSummary)

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

	client := &http.Client{Timeout: 120 * time.Second}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt == 0 {
				time.Sleep(time.Second)
				continue
			}
			return nil, nil, nil, lastErr
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
			return nil, nil, nil, fmt.Errorf("parse API response: %w", err)
		}
		if len(result.Choices) == 0 {
			return nil, nil, nil, fmt.Errorf("empty choices from API")
		}

		content := result.Choices[0].Message.Content
		jsonStr := extractJSON(content)
		if jsonStr == "" {
			fmt.Println("  [事件分析] 大模型返回格式异常")
			return nil, nil, nil, nil
		}

		var data struct {
			TimelineEvents []TimelineEvent `json:"timelineEvents"`
			TickerItems    []TickerItem    `json:"tickerItems"`
			Events         []MarketEvent   `json:"events"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			fmt.Printf("  [事件分析] JSON 解析失败: %v\n", err)
			return nil, nil, nil, nil
		}

		timeline := clampTimeline(data.TimelineEvents)
		fmt.Printf("  [事件分析] 大模型生成: %d个事件 + %d条时间线 + %d条资讯\n",
			len(data.Events), len(timeline), len(data.TickerItems))
		return data.Events, timeline, data.TickerItems, nil
	}

	return nil, nil, nil, lastErr
}

// AnalyzeAllContent 统一入口：优先尝试 AI 生成，失败时自动降级到数据驱动生成。
// 返回 (events, timeline, ticker) 三元组，保证调用方始终有可用数据。
func AnalyzeAllContent(sectors []fetcher.Sector, dateStr string) ([]MarketEvent, []TimelineEvent, []TickerItem) {
	aiCfg := config.GetAIConfig()
	if aiCfg.APIKey == "" {
		fmt.Println("  [事件分析] 未设置 AI_API_KEY，使用数据驱动生成")
		return DataDrivenGenerate(sectors)
	}

	events, timeline, ticker, err := AIGenerate(sectors, dateStr, aiCfg)
	if err != nil {
		fmt.Printf("  [事件分析] API 请求失败: %v，使用数据驱动生成\n", err)
		return DataDrivenGenerate(sectors)
	}
	if events == nil && timeline == nil && ticker == nil {
		fmt.Println("  [事件分析] 大模型分析失败，使用数据驱动生成")
		return DataDrivenGenerate(sectors)
	}
	return events, timeline, ticker
}

func GetFallbackEvents(totalFrames int) []MarketEvent {
	return []MarketEvent{
		{EventType: "market", Frame: totalFrames * 3 / 100, Text: "A股开盘", Subtext: "主力资金持续流入", Importance: 2},
		{EventType: "sentiment", Frame: totalFrames * 18 / 100, Text: "市场情绪偏多", Subtext: "资金积极布局", Importance: 2},
		{EventType: "concentration", Frame: totalFrames * 32 / 100, Text: "板块资金涌入", Subtext: "净流入加速", Importance: 3},
		{EventType: "rotation", Frame: totalFrames * 48 / 100, Text: "板块分化明显", Subtext: "资金轮动加剧", Importance: 2},
		{EventType: "aberration", Frame: totalFrames * 62 / 100, Text: "部分板块承压", Subtext: "资金流出观望", Importance: 2},
		{EventType: "sentiment", Frame: totalFrames * 75 / 100, Text: "午后情绪回暖", Subtext: "资金回流", Importance: 2},
		{EventType: "rotation", Frame: totalFrames * 85 / 100, Text: "尾盘资金动向", Subtext: "板块轮动加速", Importance: 1},
		{EventType: "market", Frame: totalFrames * 95 / 100, Text: "收盘", Subtext: "今日板块情绪分化明显", Importance: 2},
	}
}

func filterAndSort(sectors []fetcher.Sector, inflow bool) []fetcher.Sector {
	var result []fetcher.Sector
	for _, s := range sectors {
		if (inflow && s.Net > 0) || (!inflow && s.Net < 0) {
			result = append(result, s)
		}
	}
	if inflow {
		sort.Slice(result, func(i, j int) bool { return result[i].Net > result[j].Net })
	} else {
		sort.Slice(result, func(i, j int) bool { return result[i].Net < result[j].Net })
	}
	return result
}

func sumNet(sectors []fetcher.Sector) float64 {
	total := 0.0
	for _, s := range sectors {
		total += s.Net
	}
	return total
}

func formatTop5(sectors []fetcher.Sector) string {
	if len(sectors) == 0 {
		return "无"
	}
	n := 5
	if len(sectors) < n {
		n = len(sectors)
	}
	lines := make([]string, n)
	for i := 0; i < n; i++ {
		lines[i] = fmt.Sprintf("- %s: %+.1f亿", sectors[i].Name, sectors[i].Net)
	}
	return strings.Join(lines, "\n")
}

func parseDateDisplay(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("01月02日")
}

func mapSentiment(net float64) string {
	if net > 0 {
		return "positive"
	}
	return "negative"
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func extractJSON(content string) string {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return content[start : end+1]
}
