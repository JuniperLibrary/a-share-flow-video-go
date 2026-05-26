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
	"github.com/a-share-flow-video-go/internal/tickfetcher"
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

// TickDataDrivenGenerate 基于 tick 时序数据，数据驱动地生成事件分析。
// 利用真实时间序列特征检测：资金突变、板块轮动、极值点、交叉点等。
func TickDataDrivenGenerate(points []tickfetcher.TickPoint, session string) ([]MarketEvent, []TimelineEvent, []TickerItem) {
	// 1. 按时间排序，构建 per-sector 时间序列
	timeOrder := uniqueTickTimes(points)
	sectorSeries := buildSectorTimeSeries(points, timeOrder)

	// 2. 计算每个板块的累计值和变化率
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

	// 3. 检测资金突变（delta 变化最大的时间点）
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
			if absF(d) > 5 { // 单次变化超过5亿视为突变
				spikes = append(spikes, SpikeEvent{
					Time:     timeOrder[i],
					Minutes:  timeToMinutes(timeOrder[i]),
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

	// 4. 检测极值点（累计值最大的时间点）
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
			peaks = append(peaks, PeakEvent{Time: timeOrder[maxIdx], Minutes: timeToMinutes(timeOrder[maxIdx]), Sector: name, Value: maxVal, IsMax: true})
		}
		if absF(minVal) > 3 {
			peaks = append(peaks, PeakEvent{Time: timeOrder[minIdx], Minutes: timeToMinutes(timeOrder[minIdx]), Sector: name, Value: minVal, IsMax: false})
		}
	}

	// 5. 检测板块轮动（排名变化）
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
		// 计算上一时刻排名
		prevNets := make(map[string]float64)
		for name, cum := range cumulative {
			if i-1 < len(cum) {
				prevNets[name] = cum[i-1]
			}
		}
		// 计算当前时刻排名
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
					Minutes:  timeToMinutes(timeOrder[i]),
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

	// 6. 生成 TimelineEvent
	var timeline []TimelineEvent

	// 开盘
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
			timeline = append(timeline, TimelineEvent{
				Time:        firstTime,
				TimeMinutes: timeToMinutes(firstTime),
				Sector:      s.Name,
				Title:       fmt.Sprintf("%s开盘异动", s.Name),
				Description: fmt.Sprintf("开盘资金%s%.1f亿", ifElse(s.Net > 0, "净流入", "净流出"), absF(s.Net)),
				Sentiment:   ifElse(s.Net > 0, "positive", "negative"),
			})
		}
	}

	// 资金突变事件
	for _, sp := range spikes {
		if len(timeline) >= 10 {
			break
		}
		// 检查是否已有相近时间点的事件
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
		timeline = append(timeline, TimelineEvent{
			Time:        sp.Time,
			TimeMinutes: sp.Minutes,
			Sector:      sp.Sector,
			Title:       fmt.Sprintf("%s资金%s", sp.Sector, ifElse(sp.IsInflow, "加速涌入", "加速流出")),
			Description: fmt.Sprintf("单时段%s%.1f亿", ifElse(sp.IsInflow, "净流入", "净流出"), absF(sp.Delta)),
			Sentiment:   ifElse(sp.IsInflow, "positive", "negative"),
		})
	}

	// 板块轮动事件
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
		timeline = append(timeline, TimelineEvent{
			Time:        rc.Time,
			TimeMinutes: rc.Minutes,
			Sector:      rc.Sector,
			Title:       fmt.Sprintf("%s%s至第%d", rc.Sector, dir, rc.ToRank),
			Description: fmt.Sprintf("资金从%.1f亿变化至%.1f亿", rc.PrevNet, rc.CurrNet),
			Sentiment:   ifElse(rc.ToRank < rc.FromRank, "positive", "negative"),
		})
	}

	// 极值点事件
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
		timeline = append(timeline, TimelineEvent{
			Time:        pk.Time,
			TimeMinutes: pk.Minutes,
			Sector:      pk.Sector,
			Title:       fmt.Sprintf("%s%s", pk.Sector, ifElse(pk.IsMax, "净流入峰值", "净流出峰值")),
			Description: fmt.Sprintf("累计%s%.1f亿", ifElse(pk.IsMax, "净流入", "净流出"), absF(pk.Value)),
			Sentiment:   ifElse(pk.IsMax, "positive", "negative"),
		})
	}

	// 收盘总结
	if len(timeOrder) > 0 {
		lastTime := timeOrder[len(timeOrder)-1]
		totalNet := 0.0
		for _, cum := range cumulative {
			if len(cum) > 0 {
				totalNet += cum[len(cum)-1]
			}
		}
		timeline = append(timeline, TimelineEvent{
			Time:        lastTime,
			TimeMinutes: timeToMinutes(lastTime),
			Sector:      "市场",
			Title:       "收盘总结",
			Description: fmt.Sprintf("全天主力净流向%+.1f亿", totalNet),
			Sentiment:   mapSentiment(totalNet),
		})
	}

	// 按时间排序
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].TimeMinutes < timeline[j].TimeMinutes
	})

	// 7. 生成 TickerItem
	var ticker []TickerItem
	ticker = append(ticker, TickerItem{Time: timeOrder[0], Text: "A股开盘，主力资金流向实时监控"})

	// 从 spikes 生成 ticker
	for _, sp := range spikes {
		if len(ticker) >= 10 {
			break
		}
		ticker = append(ticker, TickerItem{
			Time: sp.Time,
			Text: fmt.Sprintf("%s%s%.1f亿，%s", sp.Sector, ifElse(sp.IsInflow, "主力净流入", "主力净流出"), absF(sp.Delta), ifElse(sp.IsInflow, "多头强势", "空头主导")),
		})
	}

	// 从 rankChanges 生成 ticker
	for _, rc := range rankChanges {
		if len(ticker) >= 10 {
			break
		}
		ticker = append(ticker, TickerItem{
			Time: rc.Time,
			Text: fmt.Sprintf("%s排名变化：第%d→第%d", rc.Sector, rc.FromRank, rc.ToRank),
		})
	}

	// 8. 生成 MarketEvent
	var events []MarketEvent
	totalFrames := 900

	// 开盘事件
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
		events = append(events, MarketEvent{
			EventType: "market", Frame: totalFrames * 3 / 100,
			Text:    "A股开盘",
			Subtext: fmt.Sprintf("资金%s，%d板块主力流入", sentimentStr, inflowCount),
			Importance: 2,
		})
		if len(topFirst) > 0 {
			events = append(events, MarketEvent{
				EventType: "concentration", Frame: totalFrames * 12 / 100,
				Text:    fmt.Sprintf("%s开盘领涨", topFirst[0].Name),
				Subtext: fmt.Sprintf("净流入%.1f亿，多头集结", topFirst[0].Net),
				Importance: 3,
			})
		}
	}

	// 从 spikes 生成事件
	for _, sp := range spikes {
		if len(events) >= 8 {
			break
		}
		frame := timeMinutesToFrame(sp.Minutes, totalFrames, session)
		events = append(events, MarketEvent{
			EventType: ifElse(sp.IsInflow, "sentiment", "aberration"),
			Frame:     frame,
			Text:      fmt.Sprintf("%s资金%s", sp.Sector, ifElse(sp.IsInflow, "加速涌入", "加速流出")),
			Subtext:   fmt.Sprintf("单时段%s%.1f亿", ifElse(sp.IsInflow, "净流入", "净流出"), absF(sp.Delta)),
			Importance: ifElseInt(absF(sp.Delta) > 10, 3, 2),
		})
	}

	// 从 rankChanges 生成事件
	for _, rc := range rankChanges {
		if len(events) >= 10 {
			break
		}
		frame := timeMinutesToFrame(rc.Minutes, totalFrames, session)
		events = append(events, MarketEvent{
			EventType: "rotation",
			Frame:     frame,
			Text:      fmt.Sprintf("%s板块轮动", rc.Sector),
			Subtext:   fmt.Sprintf("排名从第%d变化至第%d", rc.FromRank, rc.ToRank),
			Importance: 2,
		})
	}

	// 收盘事件（含结论页）
	conclusion := buildConclusionFromCumulative(cumulative) + " 你最看好哪个方向？"
	events = append(events, MarketEvent{
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
	return filterBySession(events, timeline, ticker, session)
}

func uniqueTickTimes(points []tickfetcher.TickPoint) []string {
	seen := make(map[string]bool)
	var times []string
	for _, p := range points {
		if !seen[p.Time] {
			seen[p.Time] = true
			times = append(times, p.Time)
		}
	}
	sort.Slice(times, func(i, j int) bool {
		return timeToMinutes(times[i]) < timeToMinutes(times[j])
	})
	return times
}

func buildSectorTimeSeries(points []tickfetcher.TickPoint, timeOrder []string) map[string][]float64 {
	// 构建 delta 序列
	sectorPrev := make(map[string]float64)
	sectorDeltas := make(map[string][]float64)

	for _, p := range points {
		prev := sectorPrev[p.Name]
		delta := p.Net - prev
		sectorDeltas[p.Name] = append(sectorDeltas[p.Name], delta)
		sectorPrev[p.Name] = p.Net
	}

	// 补齐缺失的时间点
	for name := range sectorDeltas {
		deltas := sectorDeltas[name]
		if len(deltas) < len(timeOrder) {
			padded := make([]float64, len(timeOrder))
			copy(padded, deltas)
			sectorDeltas[name] = padded
		}
	}

	return sectorDeltas
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

func rankByAbs(nets map[string]float64) map[string]int {
	type pair struct {
		Name string
		Net  float64
	}
	var pairs []pair
	for name, net := range nets {
		pairs = append(pairs, pair{name, net})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return absF(pairs[i].Net) > absF(pairs[j].Net)
	})
	rank := make(map[string]int)
	for i, p := range pairs {
		rank[p.Name] = i + 1
	}
	return rank
}

type sectorNet struct {
	Name string
	Net  float64
}

func topByAbs(nets map[string]float64, n int) []sectorNet {
	var pairs []sectorNet
	for name, net := range nets {
		pairs = append(pairs, sectorNet{name, net})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return absF(pairs[i].Net) > absF(pairs[j].Net)
	})
	if len(pairs) > n {
		pairs = pairs[:n]
	}
	return pairs
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
	return filterBySession(events, timeline, ticker, session)
}

// AITickGenerate 基于 tick 时序数据，AI 分析生成事件。
// 采用游资复盘视角，分析资金攻击路径、扩散方向、主线判断等。
func AITickGenerate(points []tickfetcher.TickPoint, dateStr string, session string, aiCfg config.AIConfig) ([]MarketEvent, []TimelineEvent, []TickerItem, error) {
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

	prompt := fmt.Sprintf(`你是一名顶级A股主线研究员和游资资金流分析师。

下面给你的是不同时间段的板块主力资金净流入时序数据。

## 你的任务

从资金流变化中分析并生成视频所需的三类内容（timelineEvents、tickerItems、events）。

## 分析视角（重要）

不要机械复述数据，要像游资复盘一样思考：
- 资金最先攻击哪个方向？为什么？
- 后续资金扩散路径是什么？是产业链联动还是情绪套利？
- 是否形成主线共振？核心龙头是谁？
- 是否出现高低切、低位补涨、资金回流？
- 市场风险偏好是提升还是下降？
- 主力真正想做什么？

## 输出风格对比

❌ 错误（财经新闻口吻）：
- title: "AI应用资金流入"
- description: "AI应用板块净流入增加3.2亿"

✅ 正确（游资复盘风格）：
- title: "AI应用早盘抢筹"
- description: "主力率先攻击AI应用方向，净流入+3.2亿"

❌ 错误：
- title: "半导体板块表现良好"
- description: "半导体板块资金持续流入"

✅ 正确：
- title: "半导体产业链共振"
- description: "CPO与半导体同步获资金，算力主线强化"

## 数据摘要
%s

## 输出要求

请严格按以下 JSON 格式输出一个对象，包含三个字段：

### 视频5段式结构（重要）
整个视频按以下模板组织 events 的 frame 分布：

| 段落 | frame 范围 | 内容 | events 数量 |
|------|-----------|------|------------|
| ① 钩子 | 0-5 | 前3秒钩子（疑问/冲突/数据型） | 1个 event，event_type="market" |
| ② 资金流动态图 | 6-40 | 板块资金流曲线展示 | 3-4个 events，早盘资金动态 |
| ③ 排行榜变化 | 41-70 | TOP10排名变化 | 2-3个 events，板块轮动/排名变化 |
| ④ AI总结 | 71-85 | 核心结论，今日主线判断 | 1-2个 events，主线/情绪总结 |
| ⑤ 结论页+互动 | 86-100 | 流入TOP3/流出TOP3 + 互动引导 | 1-2个 events，event_type="market" 结论页 |

### 1. timelineEvents（市场事件时间线，10-12个）
- time: 时间 "HH:MM"（必须在 09:30-11:30 或 13:00-15:00 范围内）
- timeMinutes: 从09:30起的分钟数（如09:35=5, 10:15=45, 13:15=225, 14:10=310）
- sector: 相关板块名（必须是数据中实际存在的板块）
- title: 事件标题（8-10字，游资复盘风格）
- description: 事件描述（15-20字，使用专业交易术语如"主力抢筹""产业链共振""高低切换""补涨逻辑""主线强化""资金分歧"，并包含具体数值）
- sentiment: "positive" / "negative" / "neutral"
- 时间分布建议：09:30-10:00 至少2个，10:00-11:00 至少2个，11:00-11:30 至少1个，13:00-14:00 至少2个，14:00-15:00 至少2个

### 2. tickerItems（底部滚动资讯，10-12条）
- time: 时间 "HH:MM"（必须在交易时段内）
- text: 资讯内容（12-15字，游资复盘风格）

### 3. events（底部弹窗事件，8-10个）
- event_type: "market"/"sentiment"/"rotation"/"aberration"
- frame: 时间位置百分比（0-100），必须按照上面的5段式结构分布
- text: 主标题（8-10字，游资风格）
- subtext: 副标题（15-20字，包含板块名和数值）
- importance: 重要程度 1/2/3
- **必须包含一个结论页 event（frame 90-98）**：event_type="market"，text为"资金流总结"，subtext包含今日流入TOP3板块名和净流入金额，并在末尾追加" 你最看好哪个方向？"

## 注意事项
- timelineEvents 的 timeMinutes 必须按升序排列
- tickerItems 的 time 从早到晚排列
- events 的 frame 从低到高分布，严格按5段式结构
- 所有内容必须基于实际数据，不要编造数据
- 板块名和数值必须与数据摘要一致
- 输出风格必须像游资复盘、私募策略会，而不是财经新闻
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
		logger.Info("游资视角AI分析",
			zap.Int("events", len(data.Events)),
			zap.Int("timeline", len(timeline)),
			zap.Int("ticker", len(data.TickerItems)))
		return data.Events, timeline, data.TickerItems, nil
	}

	return nil, nil, nil, lastErr
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

## 账号品牌
- 账号名称：主线共振Lab
- Slogan：**资金不会说谎，主线都会留下痕迹**
- 风格：每日板块资金图谱可视化，不荐股，仅记录市场

## 数据摘要
%s

## A股交易时间规则
- 上午: 09:30 - 11:30
- 午休: 11:30 - 13:00（闭盘，不产生事件）
- 下午: 13:00 - 15:00
- 所有事件的 time 字段必须在以上交易时段内，禁止出现 11:31-12:59 的时间

## 输出要求
请严格按以下 JSON 格式输出一个对象，包含三个字段：

### 视频5段式结构（重要）
整个视频按以下模板组织 events 的 frame 分布：

| 段落 | frame 范围 | 内容 | events 数量 |
|------|-----------|------|------------|
| ① 钩子 | 0-5 | 前3秒钩子（疑问/冲突/数据型） | 1个 event，event_type="market" |
| ② 资金流动态图 | 6-40 | 板块资金流曲线展示 | 3-4个 events，早盘资金动态 |
| ③ 排行榜变化 | 41-70 | TOP10排名变化 | 2-3个 events，板块轮动/排名变化 |
| ④ AI总结 | 71-85 | 核心结论，今日主线判断 | 1-2个 events，主线/情绪总结 |
| ⑤ 结论页+互动 | 86-100 | 流入TOP3/流出TOP3 + 互动引导 | 1-2个 events，event_type="market" 结论页 |

### 1. timelineEvents（市场事件时间线，10-12个）
- time: 时间 "HH:MM"（必须在 09:30-11:30 或 13:00-15:00 范围内）
- timeMinutes: 从09:30起的分钟数（如09:35=5, 10:15=45, 13:15=225, 14:10=310）
- sector: 相关板块名（必须是数据中实际存在的板块）
- title: 事件标题（10字以内，包含板块名）
- description: 事件描述（20字以内，使用专业术语如"主力资金涌入""资金出逃""板块轮动""情绪分化""量能萎缩""放量突破""主线共振"，并包含具体数值）
- sentiment: "positive" / "negative" / "neutral"
- 时间分布建议：09:30-10:00 至少2个，10:00-11:00 至少2个，11:00-11:30 至少1个，13:00-14:00 至少2个，14:00-15:00 至少2个

### 2. tickerItems（底部滚动资讯，10-12条）
- time: 时间 "HH:MM"（必须在交易时段内）
- text: 资讯内容（15字以内，包含板块名和数值）

### 3. events（底部弹窗事件，8-10个）
- event_type: "market"/"sentiment"/"rotation"/"aberration"
- frame: 时间位置百分比（0-100），必须按照上面的5段式结构分布
- text: 主标题（10字以内）
- subtext: 副标题（20字以内）
- importance: 重要程度 1/2/3
- **必须包含一个结论页 event（frame 90-98）**：event_type="market"，text为"资金流总结"，subtext包含今日流入TOP3板块名和净流入金额，并在末尾追加" 你最看好哪个方向？"

## 注意事项
- timelineEvents 的 timeMinutes 必须按升序排列
- tickerItems 的 time 从早到晚排列
- events 的 frame 从低到高分布，严格按5段式结构
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
	aiCfg := config.GetAIConfig()
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
	return filterBySession(events, timeline, ticker, session)
}

// AnalyzeTickContent Tick 事件分析统一入口：优先尝试 AI 游资复盘生成，失败时自动降级到数据驱动生成。
func AnalyzeTickContent(points []tickfetcher.TickPoint, dateStr string, session string) ([]MarketEvent, []TimelineEvent, []TickerItem) {
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
	return filterBySession(events, timeline, ticker, session)
}

func filterBySession(events []MarketEvent, timeline []TimelineEvent, ticker []TickerItem, session string) ([]MarketEvent, []TimelineEvent, []TickerItem) {
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

// buildConclusionText 生成"流入TOP3 / 流出TOP3"文本，用于结论页 event 的 subtext
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
	return fmt.Sprintf("流入TOP3：%s | 流出TOP3：%s", inStr, outStr)
}

// buildConclusionFromCumulative 从 tick 累计数据生成结论页文本
func buildConclusionFromCumulative(cumulative map[string][]float64) string {
	type entry struct {
		name string
		net  float64
	}
	var entries []entry
	for name, vals := range cumulative {
		if len(vals) > 0 {
			entries = append(entries, entry{name, vals[len(vals)-1]})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].net > entries[j].net })

	var inTop, outTop []string
	for _, e := range entries {
		if e.net > 0 && len(inTop) < 3 {
			inTop = append(inTop, fmt.Sprintf("%s%.0f亿", e.name, e.net))
		} else if e.net < 0 && len(outTop) < 3 {
			outTop = append(outTop, fmt.Sprintf("%s%.0f亿", e.name, absF(e.net)))
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
	return fmt.Sprintf("流入TOP3：%s | 流出TOP3：%s", inStr, outStr)
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
