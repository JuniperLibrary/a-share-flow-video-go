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
}

type SectorNews struct {
	Sector string     `json:"sector"`
	News   []NewsItem `json:"news"`
}

type NewsPage struct {
	Sectors []SectorNews `json:"sectors"`
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
	for _, s := range fetcher.Top21HotSectors {
		hotSet[s] = true
	}

	sectorMap := make(map[string][]NewsItem)
	seenTitles := make(map[string]bool)
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
		for _, s := range sectors {
			if !hotSet[s] {
				continue
			}
			if seenTitles[title] {
				continue
			}
			seenTitles[title] = true
			sectorMap[s] = append(sectorMap[s], item)
			break
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

func GenerateTTSText(pages []NewsPage) []string {
	texts := make([]string, len(pages))
	for i, page := range pages {
		var parts []string
		if i == 0 {
			parts = append(parts, "资金这样走，背后主要看这些催化。")
		}
		for si, sn := range page.Sectors {
			var titles []string
			for ni, item := range sn.News {
				if ni >= 2 {
					break
				}
				title := strings.TrimSpace(item.Title)
				if title == "" {
					continue
				}
				titles = append(titles, title)
			}
			if len(titles) == 0 {
				continue
			}
			if si == 0 {
				parts = append(parts, sn.Sector+"的催化是，"+strings.Join(titles, "。"))
			} else {
				parts = append(parts, "再看"+sn.Sector+"，"+strings.Join(titles, "。"))
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
