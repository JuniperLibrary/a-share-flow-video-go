package tick

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// TickDataDrivenGenerate 基于 tick 时序数据，数据驱动地生成事件分析。
// 利用真实时间序列特征检测：资金突变、板块轮动、极值点、交叉点等。
func TickDataDrivenGenerate(points []TickPoint, session string) ([]analyzer.MarketEvent, []analyzer.TimelineEvent, []analyzer.TickerItem) {
	timeOrder := uniqueTickTimes(points)
	sectorSeries := buildSectorTimeSeries(points, timeOrder)

	cumulative := make(map[string][]float64)
	for name, deltas := range sectorSeries {
		cum := make([]float64, len(deltas))
		sum := 0.0
		for i, d := range deltas {
			sum += d
			cum[i] = sum
		}
		cumulative[name] = cum
	}

	type SpikeEvent struct {
		Time     string
		Minutes  int
		Sector   string
		Delta    float64
		IsInflow bool
	}
	var spikes []SpikeEvent
	for name, deltas := range sectorSeries {
		for i, d := range deltas {
			if absF(d) > 5 {
				spikes = append(spikes, SpikeEvent{
					Time:     timeOrder[i],
					Minutes:  minutesOf(timeOrder[i]),
					Sector:   name,
					Delta:    d,
					IsInflow: d > 0,
				})
			}
		}
	}
	sort.Slice(spikes, func(i, j int) bool {
		return absF(spikes[i].Delta) > absF(spikes[j].Delta)
	})

	type PeakEvent struct {
		Time    string
		Minutes int
		Sector  string
		Value   float64
		IsMax   bool
	}
	var peaks []PeakEvent
	for name, cum := range cumulative {
		maxVal := 0.0
		minVal := 0.0
		maxIdx := 0
		minIdx := 0
		for i, v := range cum {
			if v > maxVal {
				maxVal = v
				maxIdx = i
			}
			if v < minVal {
				minVal = v
				minIdx = i
			}
		}
		if maxVal > 3 {
			peaks = append(peaks, PeakEvent{Time: timeOrder[maxIdx], Minutes: minutesOf(timeOrder[maxIdx]), Sector: name, Value: maxVal, IsMax: true})
		}
		if absF(minVal) > 3 {
			peaks = append(peaks, PeakEvent{Time: timeOrder[minIdx], Minutes: minutesOf(timeOrder[minIdx]), Sector: name, Value: minVal, IsMax: false})
		}
	}

	type RankChange struct {
		Time       string
		Minutes    int
		Sector     string
		FromRank   int
		ToRank     int
		PrevNet    float64
		CurrNet    float64
	}
	var rankChanges []RankChange
	for i := 1; i < len(timeOrder); i++ {
		prevNets := make(map[string]float64)
		for name, cum := range cumulative {
			if i-1 < len(cum) {
				prevNets[name] = cum[i-1]
			}
		}
		currNets := make(map[string]float64)
		for name, cum := range cumulative {
			if i < len(cum) {
				currNets[name] = cum[i]
			}
		}
		prevRank := rankByAbs(prevNets)
		currRank := rankByAbs(currNets)
		for name := range currNets {
			pr, ok1 := prevRank[name]
			cr, ok2 := currRank[name]
			if ok1 && ok2 && pr != cr && absInt(pr-cr) >= 3 {
				rankChanges = append(rankChanges, RankChange{
					Time:     timeOrder[i],
					Minutes:  minutesOf(timeOrder[i]),
					Sector:   name,
					FromRank: pr,
					ToRank:   cr,
					PrevNet:  prevNets[name],
					CurrNet:  currNets[name],
				})
			}
		}
	}
	sort.Slice(rankChanges, func(i, j int) bool {
		return absF(rankChanges[i].CurrNet-rankChanges[i].PrevNet) > absF(rankChanges[j].CurrNet-rankChanges[j].PrevNet)
	})

	var timeline []analyzer.TimelineEvent

	if len(timeOrder) > 0 {
		firstTime := timeOrder[0]
		firstNets := make(map[string]float64)
		for name, cum := range cumulative {
			if len(cum) > 0 {
				firstNets[name] = cum[0]
			}
		}
		topFirst := topByAbs(firstNets, 1)
		if len(topFirst) > 0 {
			s := topFirst[0]
			timeline = append(timeline, analyzer.TimelineEvent{
				Time:        firstTime,
				TimeMinutes: minutesOf(firstTime),
				Sector:      s.Name,
				Title:       fmt.Sprintf("%s开盘异动", s.Name),
				Description: fmt.Sprintf("开盘资金%s%.1f亿", ifElse(s.Net > 0, "净流入", "净流出"), absF(s.Net)),
				Sentiment:   ifElse(s.Net > 0, "positive", "negative"),
			})
		}
	}

	for _, sp := range spikes {
		if len(timeline) >= 10 {
			break
		}
		duplicate := false
		for _, ev := range timeline {
			if absInt(ev.TimeMinutes-sp.Minutes) < 15 && ev.Sector == sp.Sector {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		timeline = append(timeline, analyzer.TimelineEvent{
			Time:        sp.Time,
			TimeMinutes: sp.Minutes,
			Sector:      sp.Sector,
			Title:       fmt.Sprintf("%s资金%s", sp.Sector, ifElse(sp.IsInflow, "加速涌入", "加速流出")),
			Description: fmt.Sprintf("单时段%s%.1f亿", ifElse(sp.IsInflow, "净流入", "净流出"), absF(sp.Delta)),
			Sentiment:   ifElse(sp.IsInflow, "positive", "negative"),
		})
	}

	for _, rc := range rankChanges {
		if len(timeline) >= 12 {
			break
		}
		duplicate := false
		for _, ev := range timeline {
			if absInt(ev.TimeMinutes-rc.Minutes) < 10 && ev.Sector == rc.Sector {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		dir := ifElse(rc.ToRank < rc.FromRank, "排名上升", "排名下降")
		timeline = append(timeline, analyzer.TimelineEvent{
			Time:        rc.Time,
			TimeMinutes: rc.Minutes,
			Sector:      rc.Sector,
			Title:       fmt.Sprintf("%s%s至第%d", rc.Sector, dir, rc.ToRank),
			Description: fmt.Sprintf("资金从%.1f亿变化至%.1f亿", rc.PrevNet, rc.CurrNet),
			Sentiment:   ifElse(rc.ToRank < rc.FromRank, "positive", "negative"),
		})
	}

	for _, pk := range peaks {
		if len(timeline) >= 14 {
			break
		}
		duplicate := false
		for _, ev := range timeline {
			if absInt(ev.TimeMinutes-pk.Minutes) < 10 && ev.Sector == pk.Sector {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		timeline = append(timeline, analyzer.TimelineEvent{
			Time:        pk.Time,
			TimeMinutes: pk.Minutes,
			Sector:      pk.Sector,
			Title:       fmt.Sprintf("%s%s", pk.Sector, ifElse(pk.IsMax, "净流入峰值", "净流出峰值")),
			Description: fmt.Sprintf("累计%s%.1f亿", ifElse(pk.IsMax, "净流入", "净流出"), absF(pk.Value)),
			Sentiment:   ifElse(pk.IsMax, "positive", "negative"),
		})
	}

	if len(timeOrder) > 0 {
		lastTime := timeOrder[len(timeOrder)-1]
		totalNet := 0.0
		for _, cum := range cumulative {
			if len(cum) > 0 {
				totalNet += cum[len(cum)-1]
			}
		}
		timeline = append(timeline, analyzer.TimelineEvent{
			Time:        lastTime,
			TimeMinutes: minutesOf(lastTime),
			Sector:      "市场",
			Title:       "收盘总结",
			Description: fmt.Sprintf("全天主力净流向%+.1f亿", totalNet),
			Sentiment:   mapSentiment(totalNet),
		})
	}

	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].TimeMinutes < timeline[j].TimeMinutes
	})

	var ticker []analyzer.TickerItem
	ticker = append(ticker, analyzer.TickerItem{Time: timeOrder[0], Text: "A股开盘，主力资金流向实时监控"})

	for _, sp := range spikes {
		if len(ticker) >= 10 {
			break
		}
		ticker = append(ticker, analyzer.TickerItem{
			Time: sp.Time,
			Text: fmt.Sprintf("%s%s%.1f亿，%s", sp.Sector, ifElse(sp.IsInflow, "主力净流入", "主力净流出"), absF(sp.Delta), ifElse(sp.IsInflow, "多头强势", "空头主导")),
		})
	}

	for _, rc := range rankChanges {
		if len(ticker) >= 10 {
			break
		}
		ticker = append(ticker, analyzer.TickerItem{
			Time: rc.Time,
			Text: fmt.Sprintf("%s排名变化：第%d→第%d", rc.Sector, rc.FromRank, rc.ToRank),
		})
	}

	var events []analyzer.MarketEvent
	totalFrames := 900

	if len(timeOrder) > 0 {
		firstNets := make(map[string]float64)
		for name, cum := range cumulative {
			if len(cum) > 0 {
				firstNets[name] = cum[0]
			}
		}
		topFirst := topByAbs(firstNets, 1)
		sentimentStr := "偏多"
		inflowCount := 0
		for _, cum := range cumulative {
			if len(cum) > 0 && cum[0] > 0 {
				inflowCount++
			}
		}
		if len(cumulative)-inflowCount > inflowCount {
			sentimentStr = "偏空"
		}
		events = append(events, analyzer.MarketEvent{
			EventType: "market", Frame: totalFrames * 3 / 100,
			Text:    "A股开盘",
			Subtext: fmt.Sprintf("资金%s，%d板块主力流入", sentimentStr, inflowCount),
			Importance: 2,
		})
		if len(topFirst) > 0 {
			events = append(events, analyzer.MarketEvent{
				EventType: "concentration", Frame: totalFrames * 12 / 100,
				Text:    fmt.Sprintf("%s开盘领涨", topFirst[0].Name),
				Subtext: fmt.Sprintf("净流入%.1f亿，多头集结", topFirst[0].Net),
				Importance: 3,
			})
		}
	}

	for _, sp := range spikes {
		if len(events) >= 8 {
			break
		}
		frame := timeMinutesToFrame(sp.Minutes, totalFrames)
		events = append(events, analyzer.MarketEvent{
			EventType: ifElse(sp.IsInflow, "sentiment", "aberration"),
			Frame:     frame,
			Text:      fmt.Sprintf("%s资金%s", sp.Sector, ifElse(sp.IsInflow, "加速涌入", "加速流出")),
			Subtext:   fmt.Sprintf("单时段%s%.1f亿", ifElse(sp.IsInflow, "净流入", "净流出"), absF(sp.Delta)),
			Importance: ifElseInt(absF(sp.Delta) > 10, 3, 2),
		})
	}

	for _, rc := range rankChanges {
		if len(events) >= 10 {
			break
		}
		frame := timeMinutesToFrame(rc.Minutes, totalFrames)
		events = append(events, analyzer.MarketEvent{
			EventType: "rotation",
			Frame:     frame,
			Text:      fmt.Sprintf("%s板块轮动", rc.Sector),
			Subtext:   fmt.Sprintf("排名从第%d变化至第%d", rc.FromRank, rc.ToRank),
			Importance: 2,
		})
	}

	conclusion := buildConclusionFromCumulative(cumulative) + " 你最看好哪个方向？"
	events = append(events, analyzer.MarketEvent{
		EventType: "market", Frame: totalFrames * 95 / 100,
		Text:    "资金流总结",
		Subtext: conclusion,
		Importance: 3,
	})

	sort.Slice(events, func(i, j int) bool {
		return events[i].Frame < events[j].Frame
	})

	logger.Info("时序驱动生成",
		zap.Int("events", len(events)),
		zap.Int("timeline", len(timeline)),
		zap.Int("ticker", len(ticker)))
	return analyzer.FilterBySession(events, timeline, ticker, session)
}

func uniqueTickTimes(points []TickPoint) []string {
	seen := make(map[string]bool)
	var times []string
	for _, p := range points {
		if !seen[p.Time] {
			seen[p.Time] = true
			times = append(times, p.Time)
		}
	}
	sort.Slice(times, func(i, j int) bool {
		return minutesOf(times[i]) < minutesOf(times[j])
	})
	return times
}

func buildSectorTimeSeries(points []TickPoint, timeOrder []string) map[string][]float64 {
	sectorPrev := make(map[string]float64)
	sectorDeltas := make(map[string][]float64)

	for _, p := range points {
		prev := sectorPrev[p.Name]
		delta := p.Net - prev
		sectorDeltas[p.Name] = append(sectorDeltas[p.Name], delta)
		sectorPrev[p.Name] = p.Net
	}

	for _, times := range sectorDeltas {
		if len(times) < len(timeOrder) {
			padded := make([]float64, len(timeOrder))
			copy(padded, times)
			times = padded
		}
	}

	return sectorDeltas
}

func minutesOf(t string) int {
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

func timeMinutesToFrame(minutes, totalFrames int) int {
	if minutes < 0 {
		return 0
	}
	// 上午: 0-120分钟, 下午补90分钟午休偏移
	fraction := float64(minutes) / 330.0
	frame := int(fraction * float64(totalFrames))
	if frame >= totalFrames {
		frame = totalFrames - 1
	}
	return frame
}

func mapSentiment(net float64) string {
	if net > 0 {
		return "positive"
	} else if net < 0 {
		return "negative"
	}
	return "neutral"
}

// AITickGenerate 基于 tick 时序数据，AI 分析生成事件。
// 采用游资复盘视角，分析资金攻击路径、扩散方向、主线判断等。
func AITickGenerate(points []TickPoint, dateStr string, session string, aiCfg config.AIConfig) ([]analyzer.MarketEvent, []analyzer.TimelineEvent, []analyzer.TickerItem, error) {
	dateDisplay := parseDateDisplay(dateStr)

	timeOrder := uniqueTickTimes(points)
	sectorSeries := buildSectorTimeSeries(points, timeOrder)

	cumulative := make(map[string][]float64)
	for name, deltas := range sectorSeries {
		cum := make([]float64, len(deltas))
		sum := 0.0
		for i, d := range deltas {
			sum += d
			cum[i] = sum
		}
		cumulative[name] = cum
	}

	var sectorLines []string
	for name, cum := range cumulative {
		var pts []string
		for i, v := range cum {
			if i < len(timeOrder) {
				pts = append(pts, fmt.Sprintf("%s:%+.1f", timeOrder[i], v))
			}
		}
		sectorLines = append(sectorLines, fmt.Sprintf("- %s: [%s]", name, strings.Join(pts, ", ")))
	}
	sort.Strings(sectorLines)

	dataSummary := fmt.Sprintf(`## %s A股板块主力资金流向时序数据

### 时间序列（每个板块在各时间点的累计净流入，单位：亿元）
%s

### 时间规则
- 上午: 09:30 - 11:30
- 午休: 11:30 - 13:00（闭盘，不产生事件）
- 下午: 13:00 - 15:00`, dateDisplay, strings.Join(sectorLines, "\n"))

	prompt := fmt.Sprintf(PromptTickAnalysis, dataSummary)

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
			TimelineEvents []analyzer.TimelineEvent `json:"timelineEvents"`
			TickerItems    []analyzer.TickerItem    `json:"tickerItems"`
			Events         []analyzer.MarketEvent   `json:"events"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			logger.Warn("JSON 解析失败", zap.Error(err))
			return nil, nil, nil, nil
		}

		logger.Info("游资视角AI分析",
			zap.Int("events", len(data.Events)),
			zap.Int("timeline", len(data.TimelineEvents)),
			zap.Int("ticker", len(data.TickerItems)))
		return data.Events, data.TimelineEvents, data.TickerItems, nil
	}

	return nil, nil, nil, lastErr
}

// AnalyzeTickContent Tick 事件分析统一入口：优先尝试 AI 游资复盘生成，失败时自动降级到数据驱动生成。
func AnalyzeTickContent(points []TickPoint, dateStr string, session string) ([]analyzer.MarketEvent, []analyzer.TimelineEvent, []analyzer.TickerItem) {
	aiCfg := config.GetAIConfig()
	if aiCfg.APIKey == "" {
		logger.Info("未设置 AI_API_KEY，使用数据驱动生成")
		return TickDataDrivenGenerate(points, session)
	}

	events, timeline, ticker, err := AITickGenerate(points, dateStr, session, aiCfg)
	if err != nil {
		logger.Warn("API 请求失败，使用数据驱动生成", zap.Error(err))
		return TickDataDrivenGenerate(points, session)
	}
	if events == nil && timeline == nil && ticker == nil {
		logger.Warn("大模型分析失败，使用数据驱动生成")
		return TickDataDrivenGenerate(points, session)
	}
	return analyzer.FilterBySession(events, timeline, ticker, session)
}

func extractJSON(content string) string {
	start := strings.Index(content, "{")
	if start == -1 {
		return ""
	}
	end := strings.LastIndex(content, "}")
	if end == -1 || end < start {
		return ""
	}
	return content[start : end+1]
}

func parseDateDisplay(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return fmt.Sprintf("%d月%d日", t.Month(), t.Day())
}

type sectorNet struct {
	Name string
	Net  float64
}

func rankByAbs(nets map[string]float64) map[string]int {
	type kv struct {
		k string
		v float64
	}
	var sorted []kv
	for k, v := range nets {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return absF(sorted[i].v) > absF(sorted[j].v)
	})
	rank := make(map[string]int)
	for i, kv := range sorted {
		rank[kv.k] = i + 1
	}
	return rank
}

func topByAbs(nets map[string]float64, n int) []sectorNet {
	var list []sectorNet
	for name, net := range nets {
		list = append(list, sectorNet{Name: name, Net: net})
	}
	sort.Slice(list, func(i, j int) bool {
		return absF(list[i].Net) > absF(list[j].Net)
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}

func buildConclusionFromCumulative(cumulative map[string][]float64) string {
	type item struct {
		name string
		val  float64
	}
	var inflows, outflows []item
	for name, cum := range cumulative {
		if len(cum) == 0 {
			continue
		}
		v := cum[len(cum)-1]
		if v > 0 {
			inflows = append(inflows, item{name, v})
		} else {
			outflows = append(outflows, item{name, v})
		}
	}
	sort.Slice(inflows, func(i, j int) bool { return inflows[i].val > inflows[j].val })
	sort.Slice(outflows, func(i, j int) bool { return outflows[i].val < outflows[j].val })

	var parts []string
	for i, v := range inflows {
		if i >= 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s+%.0f亿", v.name, v.val))
	}
	if len(parts) > 0 {
		return "流入TOP3: " + strings.Join(parts, " ")
	}
	return "无明显资金流入"
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func ifElse[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

func ifElseInt(cond bool, a, b int) int {
	if cond {
		return a
	}
	return b
}

func roundTo2(v float64) float64 {
	return math.Round(v*100) / 100
}
