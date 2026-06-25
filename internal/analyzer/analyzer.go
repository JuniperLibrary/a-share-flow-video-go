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

func timeToMinutes(t string) int {
	var h, m int
	fmt.Sscanf(t, "%d:%d", &h, &m)
	base := 9*60 + 30
	val := h*60 + m - base
	if val < 0 {
		val = 0
	}
	if h >= 13 {
		val -= 90
	}
	return val
}

func timeMinutesToFrame(minutes int, totalFrames int, session string) int {
	xLim := 330
	if session == "morning" {
		xLim = 120
	}
	progress := float64(minutes) / float64(xLim)
	return int(progress * float64(totalFrames))
}

// DataDrivenGenerate 基于板块资金流数据，数据驱动地生成视频所需的三类内容：
// 1. MarketEvent: 底部弹窗事件（开盘/领涨/承压/收盘）
// 2. TimelineEvent: 时间线事件（开盘活跃/持续走强/午后强势/尾盘动向等）
// 3. TickerItem: 底部滚动资讯
// 当 AI API 不可用时作为降级方案使用。
func DataDrivenGenerate(sectors []fetcher.Sector, session string) ([]MarketEvent, []TimelineEvent, []TickerItem) {
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

	// 09:35 开盘观察
	if len(topIn) > 0 {
		s := topIn[0]
		timeline = append(timeline, TimelineEvent{
			Time: "09:35", TimeMinutes: 5,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s竞价异动", s.Name),
			Description: fmt.Sprintf("主力资金净流入%.1f亿，买盘集中", s.Net),
			Sentiment:   "positive",
		})
	} else {
		timeline = append(timeline, TimelineEvent{
			Time: "09:35", TimeMinutes: 5,
			Sector: "市场", Title: "集合竞价异动",
			Description: "主力资金早盘积极布局", Sentiment: "positive",
		})
	}

	// 10:00 早盘资金涌入
	if len(topIn) > 1 {
		s := topIn[1]
		timeline = append(timeline, TimelineEvent{
			Time: "10:00", TimeMinutes: 30,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s放量突破", s.Name),
			Description: fmt.Sprintf("主力扫货，净流入%.1f亿", s.Net),
			Sentiment:   "positive",
		})
	}

	// 10:20 流出板块承压
	if len(topOut) > 0 {
		s := topOut[0]
		timeline = append(timeline, TimelineEvent{
			Time: "10:20", TimeMinutes: 50,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s资金出逃", s.Name),
			Description: fmt.Sprintf("主力减仓，净流出%.1f亿", absF(s.Net)),
			Sentiment:   "negative",
		})
	}

	// 10:45 持续走强
	if len(topIn) > 2 {
		s := topIn[2]
		timeline = append(timeline, TimelineEvent{
			Time: "10:45", TimeMinutes: 75,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s多头强化", s.Name),
			Description: fmt.Sprintf("资金持续加仓，净流入%.1f亿", s.Net),
			Sentiment:   "positive",
		})
	}

	// 11:10 早盘领涨
	if len(topIn) > 0 {
		s := topIn[0]
		timeline = append(timeline, TimelineEvent{
			Time: "11:10", TimeMinutes: 100,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s早盘领涨", s.Name),
			Description: fmt.Sprintf("净流入突破%.0f亿，多头主导", absF(s.Net)),
			Sentiment:   "positive",
		})
	}

	// 11:25 午前承压
	if len(topOut) > 1 {
		s := topOut[1]
		timeline = append(timeline, TimelineEvent{
			Time: "11:25", TimeMinutes: 115,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s抛压加重", s.Name),
			Description: fmt.Sprintf("空头发力，净流出%.1f亿", absF(s.Net)),
			Sentiment:   "negative",
		})
	}

	// 13:10 午后强势
	if len(topIn) > 0 {
		s := topIn[0]
		timeline = append(timeline, TimelineEvent{
			Time: "13:10", TimeMinutes: 220,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s午后抢筹", s.Name),
			Description: fmt.Sprintf("主力加速建仓，净流入%.1f亿", s.Net),
			Sentiment:   "positive",
		})
	}

	// 13:40 午后发力
	if len(topIn) > 1 {
		s := topIn[1]
		timeline = append(timeline, TimelineEvent{
			Time: "13:40", TimeMinutes: 250,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s午后拉升", s.Name),
			Description: fmt.Sprintf("资金快速流入，净流入%.1f亿", s.Net),
			Sentiment:   "positive",
		})
	}

	// 14:05 加速流出
	if len(topOut) > 0 {
		s := topOut[0]
		timeline = append(timeline, TimelineEvent{
			Time: "14:05", TimeMinutes: 275,
			Sector:      s.Name,
			Title:       fmt.Sprintf("%s尾盘减仓", s.Name),
			Description: fmt.Sprintf("净流出%.1f亿，资金加速离场", absF(s.Net)),
			Sentiment:   "negative",
		})
	}

	// 14:30 市场情绪
	sentimentDesc := "偏多"
	if outflowCount > inflowCount {
		sentimentDesc = "偏空"
	} else if outflowCount == inflowCount {
		sentimentDesc = "均衡"
	}
	timeline = append(timeline, TimelineEvent{
		Time: "14:30", TimeMinutes: 300,
		Sector:      "市场",
		Title:       fmt.Sprintf("多空博弈%s", sentimentDesc),
		Description: fmt.Sprintf("流入%d板块 vs 流出%d板块，情绪分化", inflowCount, outflowCount),
		Sentiment:   "neutral",
	})

	timeline = append(timeline, TimelineEvent{
		Time: "14:50", TimeMinutes: 320,
		Sector: "市场", Title: "尾盘资金动向",
		Description: fmt.Sprintf("全天合计%+.1f亿", totalNet),
		Sentiment:   mapSentiment(totalNet),
	})

	var ticker []TickerItem
	ticker = append(ticker, TickerItem{Time: "09:30", Text: "A股开盘，主力资金流向实时监控"})
	for _, s := range topIn {
		if len(ticker) >= 10 {
			break
		}
		ticker = append(ticker, TickerItem{Time: "10:00", Text: fmt.Sprintf("%s主力净流入%.1f亿，多头强势", s.Name, s.Net)})
	}
	for _, s := range topOut {
		if len(ticker) >= 10 {
			break
		}
		ticker = append(ticker, TickerItem{Time: "11:00", Text: fmt.Sprintf("%s主力净流出%.1f亿，空头主导", s.Name, absF(s.Net))})
	}
	ticker = append(ticker, TickerItem{Time: "13:30", Text: fmt.Sprintf("午后市场整体%+.1f亿，资金方向明确", totalNet)})
	ticker = append(ticker, TickerItem{Time: "14:55", Text: "尾盘资金加速流动，板块分化加剧"})

	var events []MarketEvent
	events = append(events, MarketEvent{
		EventType: "market", Frame: 3,
		Text:       "A股开盘",
		Subtext:    fmt.Sprintf("资金%s，%d板块主力流入", sentimentDesc, inflowCount),
		Importance: 2,
	})
	if len(topIn) > 0 {
		events = append(events, MarketEvent{
			EventType: "concentration", Frame: 15,
			Text:       fmt.Sprintf("%s主力领涨", topIn[0].Name),
			Subtext:    fmt.Sprintf("净流入%.1f亿，多头集结", topIn[0].Net),
			Importance: 3,
		})
	}
	if len(topIn) > 1 {
		events = append(events, MarketEvent{
			EventType: "sentiment", Frame: 30,
			Text:       fmt.Sprintf("%s放量突破", topIn[1].Name),
			Subtext:    fmt.Sprintf("主力扫货，净流入%.1f亿", topIn[1].Net),
			Importance: 2,
		})
	}
	if len(topOut) > 0 {
		events = append(events, MarketEvent{
			EventType: "aberration", Frame: 45,
			Text:       fmt.Sprintf("%s抛压加重", topOut[0].Name),
			Subtext:    fmt.Sprintf("主力减仓，净流出%.1f亿", absF(topOut[0].Net)),
			Importance: 2,
		})
	}
	if len(topIn) > 2 {
		events = append(events, MarketEvent{
			EventType: "rotation", Frame: 60,
			Text:       fmt.Sprintf("%s多头强化", topIn[2].Name),
			Subtext:    fmt.Sprintf("资金持续加仓，净流入%.1f亿", topIn[2].Net),
			Importance: 2,
		})
	}
	if len(topOut) > 1 {
		events = append(events, MarketEvent{
			EventType: "aberration", Frame: 72,
			Text:       fmt.Sprintf("%s空头主导", topOut[1].Name),
			Subtext:    fmt.Sprintf("净流出%.1f亿，资金撤退", absF(topOut[1].Net)),
			Importance: 1,
		})
	}
	if len(topIn) > 0 {
		events = append(events, MarketEvent{
			EventType: "sentiment", Frame: 82,
			Text:       fmt.Sprintf("%s午后抢筹", topIn[0].Name),
			Subtext:    fmt.Sprintf("主力加速建仓，净流入%.1f亿", topIn[0].Net),
			Importance: 3,
		})
	}
	conclusionText := buildConclusionText(all) + " 你最看好哪个方向？"
	events = append(events, MarketEvent{
		EventType: "market", Frame: 95,
		Text:       "资金流总结",
		Subtext:    conclusionText,
		Importance: 3,
	})

	logger.Info("数据驱动生成",
		zap.Int("events", len(events)),
		zap.Int("timeline", len(timeline)),
		zap.Int("ticker", len(ticker)))
	return FilterBySession(events, timeline, ticker, session)
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

	prompt := fmt.Sprintf(PromptDailyAnalysis, dataSummary)

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
			logger.Warn("大模型返回格式异常")
			return nil, nil, nil, nil
		}

		var data struct {
			TimelineEvents []TimelineEvent `json:"timelineEvents"`
			TickerItems    []TickerItem    `json:"tickerItems"`
			Events         []MarketEvent   `json:"events"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			logger.Warn("JSON 解析失败", zap.Error(err))
			return nil, nil, nil, nil
		}

		timeline := clampTimeline(data.TimelineEvents)
		logger.Info("大模型生成",
			zap.Int("events", len(data.Events)),
			zap.Int("timeline", len(timeline)),
			zap.Int("ticker", len(data.TickerItems)))
		return data.Events, timeline, data.TickerItems, nil
	}

	return nil, nil, nil, lastErr
}

// AnalyzeAllContent 统一入口：优先尝试 AI 生成，失败时自动降级到数据驱动生成。
// 返回 (events, timeline, ticker) 三元组，保证调用方始终有可用数据。
func AnalyzeAllContent(sectors []fetcher.Sector, dateStr string, session string) ([]MarketEvent, []TimelineEvent, []TickerItem) {
	aiCfg := config.GetAIConfigFor("analyzer")
	if aiCfg.APIKey == "" {
		logger.Info("未设置 AI_API_KEY，使用数据驱动生成")
		return DataDrivenGenerate(sectors, session)
	}

	events, timeline, ticker, err := AIGenerate(sectors, dateStr, aiCfg)
	if err != nil {
		logger.Warn("API 请求失败，使用数据驱动生成", zap.Error(err))
		return DataDrivenGenerate(sectors, session)
	}
	if events == nil && timeline == nil && ticker == nil {
		logger.Warn("大模型分析失败，使用数据驱动生成")
		return DataDrivenGenerate(sectors, session)
	}
	return FilterBySession(events, timeline, ticker, session)
}

func FilterBySession(events []MarketEvent, timeline []TimelineEvent, ticker []TickerItem, session string) ([]MarketEvent, []TimelineEvent, []TickerItem) {
	if session == "full" || session == "" {
		return events, timeline, ticker
	}

	// morning: timeMinutes 0-120
	// Filter timeline events to morning range
	var filteredTimeline []TimelineEvent
	for _, ev := range timeline {
		if ev.TimeMinutes >= 0 && ev.TimeMinutes <= 120 {
			filteredTimeline = append(filteredTimeline, ev)
		}
	}

	// Filter ticker items to morning times (before 11:30)
	var filteredTicker []TickerItem
	for _, t := range ticker {
		if t.Time < "11:30" {
			filteredTicker = append(filteredTicker, t)
		}
	}

	// Filter + remap market events: keep frame <= 40 (morning portion), remap to 0-100
	var filteredEvents []MarketEvent
	for _, ev := range events {
		if ev.Frame <= 40 {
			ev.Frame = ev.Frame * 100 / 40
			if ev.Frame > 100 {
				ev.Frame = 100
			}
			filteredEvents = append(filteredEvents, ev)
		}
	}

	return filteredEvents, filteredTimeline, filteredTicker
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

func buildConclusionText(sectors []fetcher.Sector) string {
	var sorted []fetcher.Sector
	sorted = append(sorted, sectors...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Net > sorted[j].Net })

	var inTop, outTop []string
	for _, s := range sorted {
		if s.Net > 0 && len(inTop) < 3 {
			inTop = append(inTop, fmt.Sprintf("%s%.0f亿", s.Name, s.Net))
		} else if s.Net < 0 && len(outTop) < 3 {
			outTop = append(outTop, fmt.Sprintf("%s%.0f亿", s.Name, absF(s.Net)))
		}
	}
	inStr := strings.Join(inTop, "、")
	outStr := strings.Join(outTop, "、")
	if inStr == "" {
		inStr = "无"
	}
	if outStr == "" {
		outStr = "无"
	}

	// 主力资金结构（超大单+大单）汇总，仅在字段非零时附加
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
	structureLine := ""
	if hasStructure && totalMain != 0 {
		superPct := totalSuper / totalMain * 100
		bigPct := totalBig / totalMain * 100
		structureLine = fmt.Sprintf(" | 主力结构：超大单%+.0f亿(%.0f%%) 大单%+.0f亿(%.0f%%)", totalSuper, superPct, totalBig, bigPct)
	}

	return fmt.Sprintf("流入TOP3：%s | 流出TOP3：%s%s", inStr, outStr, structureLine)
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
