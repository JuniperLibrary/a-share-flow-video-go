package hotnews

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/storage"
)

type NewsItem struct {
	Title      string `json:"title"`
	Brief      string `json:"brief"`
	Level      string `json:"level"`
	Time       string `json:"time"`
	ReadingNum int64  `json:"-"`
	// MatchRank 本条新闻在当前板块的优先级。越小越优先（Top21 序位 + 热度综合，多板块归因时用）。
	MatchRank int `json:"-"`
}

type SectorNews struct {
	Sector string     `json:"sector"`
	News   []NewsItem `json:"news"`
}

type NewsPage struct {
	Sectors []SectorNews `json:"sectors"`
}

// CatalysisNewsScore 单条新闻对单板块的匹配证据，用于"时间窗 + 斜率 + 热度"打分。
type CatalysisNewsScore struct {
	Title string `json:"title"`
	Brief string `json:"brief"`
	Level string `json:"level"`
	Time  string `json:"time"`
	// TimeMatch 时间窗命中：0 = 无数据 1 = 60min 内 2 = 30min 内 3 = 15min 内
	TimeMatch int `json:"timeMatch"`
	// FlowSlope 发布后 30min 板块资金斜率（亿/小时，正=流入加速，负=流出加速）
	FlowSlope float64 `json:"flowSlope"`
	// FlowDelta 发布后 30min 累计净流入（亿）
	FlowDelta float64 `json:"flowDelta"`
	// HeatScore 热度分：Level rank + ReadingNum 归一化，0-10
	HeatScore float64 `json:"heatScore"`
	// SectorRankScore 板块优先级分：Top21 越靠前分数越高，0-10
	SectorRankScore float64 `json:"sectorRankScore"`
	// TotalScore 总分 0-100，>=70 高置信，>=40 中，其余低
	TotalScore float64 `json:"totalScore"`
}

// CatalysisSectorEvidence 单板块的整体匹配证据。
type CatalysisSectorEvidence struct {
	Sector   string  `json:"sector"`
	Flow     float64 `json:"flow"`
	FlowRank int     `json:"flowRank"`
	// Confidence  high / medium / low
	Confidence string `json:"confidence"`
	// OverallScore 0-100
	OverallScore float64              `json:"overallScore"`
	NewsScores   []CatalysisNewsScore `json:"newsScores"`
	// OralScript 给 TTS 念的口语化文案，与 UI 显示的 insights 一一对应。
	OralScript []string `json:"oralScript,omitempty"`
}

var noisePrefixes = []string{
	"财联社",
	"投资日历",
	"三大指数",
	"竞价看龙头",
	"今日申购",
	"南向资金",
	"美股",
	"欧股",
	"亚太",
	"国际油价",
	"黄金价格",
	"美元指数",
}

func isNoise(title string) bool {
	for _, p := range noisePrefixes {
		if strings.HasPrefix(title, p) {
			return true
		}
	}
	return false
}

func LoadForVideo(dateStr, format string) ([]NewsPage, error) {
	db, err := storage.Get()
	if err != nil {
		return nil, err
	}

	records, _, err := db.LoadNewsByDate(dateStr, 200, 0)
	if err != nil {
		return nil, err
	}

	hotSet := make(map[string]bool, len(fetcher.Top21HotSectors))
	sectorRank := make(map[string]int, len(fetcher.Top21HotSectors))
	for i, s := range fetcher.Top21HotSectors {
		hotSet[s] = true
		sectorRank[s] = i
	}

	sectorMap := make(map[string][]NewsItem)
	// seenTitles 从"全局去重"改成"板块内去重"，一条新闻可以挂多个板块。
	sectorSeen := make(map[string]map[string]bool)
	var maxReading int64 = 1
	for _, r := range records {
		var sectors []string
		if err := json.Unmarshal([]byte(r.Sectors), &sectors); err != nil {
			continue
		}
		title := r.Title
		if title == "" && len(r.Content) > 0 {
			runes := []rune(r.Content)
			if len(runes) > 80 {
				title = string(runes[:80]) + "..."
			} else {
				title = string(runes)
			}
		}
		if title == "" {
			continue
		}
		if isNoise(title) {
			continue
		}
		if r.ReadingNum > maxReading {
			maxReading = r.ReadingNum
		}
		item := NewsItem{
			Title:      title,
			Brief:      r.Brief,
			Level:      r.Level,
			Time:       r.CTime,
			ReadingNum: r.ReadingNum,
		}
		if item.Brief == "" {
			runes := []rune(r.Content)
			if len(runes) > 80 {
				item.Brief = string(runes[:80]) + "..."
			} else {
				item.Brief = string(runes)
			}
		}
		// 多板块归因：所有 top21 命中的板块都挂。
		for _, s := range sectors {
			if !hotSet[s] {
				continue
			}
			seen, ok := sectorSeen[s]
			if !ok {
				seen = make(map[string]bool)
				sectorSeen[s] = seen
			}
			if seen[title] {
				continue
			}
			seen[title] = true
			// MatchRank 越小越优先：Top21 序位权重 + levelRank 权重 + 热度
			rank := (sectorRank[s] + 1) * 1000
			rank += levelRank(item.Level) * 400
			if item.ReadingNum > 0 {
				heat := int(float64(item.ReadingNum) / float64(maxReading) * 500)
				rank -= heat
			}
			withRank := item
			withRank.MatchRank = rank
			sectorMap[s] = append(sectorMap[s], withRank)
		}
	}

	perSectorLimit := 2
	perPage := 2
	if format == "tv" {
		perSectorLimit = 3
		perPage = 3
	}

	var result []SectorNews
	for _, name := range fetcher.Top21HotSectors {
		items, ok := sectorMap[name]
		if !ok || len(items) == 0 {
			continue
		}
		sort.Slice(items, func(i, j int) bool {
			ri, rj := items[i].MatchRank, items[j].MatchRank
			if ri != rj {
				return ri < rj
			}
			li, lj := levelRank(items[i].Level), levelRank(items[j].Level)
			if li != lj {
				return li < lj
			}
			return items[i].ReadingNum > items[j].ReadingNum
		})
		if len(items) > perSectorLimit {
			items = items[:perSectorLimit]
		}
		result = append(result, SectorNews{Sector: name, News: items})
	}

	var pages []NewsPage
	for i := 0; i < len(result); i += perPage {
		end := i + perPage
		if end > len(result) {
			end = len(result)
		}
		page := NewsPage{Sectors: result[i:end]}
		pageTitles := make(map[string]bool)
		for si := range page.Sectors {
			var deduped []NewsItem
			for _, item := range page.Sectors[si].News {
				if pageTitles[item.Title] {
					continue
				}
				pageTitles[item.Title] = true
				deduped = append(deduped, item)
			}
			page.Sectors[si].News = deduped
		}
		pages = append(pages, page)
	}
	return pages, nil
}

func GenerateTTSText(pages []NewsPage, catalysis ...interface{}) []string {
	var ca interface{}
	if len(catalysis) > 0 {
		ca = catalysis[0]
	}
	type sectorAnal struct {
		Analysis  string
		Insights  []string
		OralBlock string
	}
	var caMap map[string]*sectorAnal
	if ca != nil {
		caMap = make(map[string]*sectorAnal)
		switch v := ca.(type) {
		case map[string]interface{}:
			if secs, ok := v["sectors"].([]interface{}); ok {
				for _, s := range secs {
					sm, ok := s.(map[string]interface{})
					if !ok {
						continue
					}
					name, _ := sm["sector"].(string)
					if name == "" {
						continue
					}
					sa := &sectorAnal{}
					if a, ok := sm["analysis"].(string); ok {
						sa.Analysis = a
					}
					if ob, ok := sm["oralBlock"].(string); ok {
						sa.OralBlock = ob
					}
					if arr, ok := sm["insights"].([]interface{}); ok {
						for _, it := range arr {
							if s, ok := it.(string); ok {
								sa.Insights = append(sa.Insights, s)
							}
						}
					}
					caMap[name] = sa
				}
			}
		}
	}

	texts := make([]string, len(pages))
	for i, page := range pages {
		var parts []string
		if i == 0 {
			parts = append(parts, "资金这样走，背后主要看这些催化。")
		}
		for si, sn := range page.Sectors {
			sector := sn.Sector
			sa := caMap[sector]
			if sa != nil && sa.OralBlock != "" {
				if si == 0 {
					parts = append(parts, sector+"的催化是："+sa.OralBlock)
				} else {
					parts = append(parts, "再看"+sector+"，"+sa.OralBlock)
				}
				continue
			}
			var phrases []string
			if sa != nil && len(sa.Insights) > 0 {
				limit := 2
				if len(sa.Insights) < limit {
					limit = len(sa.Insights)
				}
				for k := 0; k < limit; k++ {
					t := strings.TrimSpace(sa.Insights[k])
					if t == "" {
						continue
					}
					phrases = append(phrases, t)
				}
			}
			if len(phrases) == 0 {
				for ni, item := range sn.News {
					if ni >= 2 {
						break
					}
					title := strings.TrimSpace(item.Title)
					if title == "" {
						continue
					}
					phrases = append(phrases, title)
				}
			}
			if len(phrases) == 0 {
				continue
			}
			if si == 0 {
				parts = append(parts, sector+"的催化是，"+strings.Join(phrases, "。"))
			} else {
				parts = append(parts, "再看"+sector+"，"+strings.Join(phrases, "。"))
			}
		}
		texts[i] = strings.Join(parts, "。") + "。"
	}
	return texts
}

func levelRank(level string) int {
	switch level {
	case "A":
		return 0
	case "B":
		return 1
	default:
		return 2
	}
}
