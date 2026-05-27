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
		item := NewsItem{
			Title:      title,
			Level:      r.Level,
			Time:       r.CTime,
			ReadingNum: r.ReadingNum,
		}
		for _, s := range sectors {
			if hotSet[s] {
				sectorMap[s] = append(sectorMap[s], item)
			}
		}
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
		if len(items) > 10 {
			items = items[:10]
		}
		result = append(result, SectorNews{Sector: name, News: items})
	}

	perPage := 3
	if format != "tv" {
		perPage = 2
	}

	var pages []NewsPage
	for i := 0; i < len(result); i += perPage {
		end := i + perPage
		if end > len(result) {
			end = len(result)
		}
		pages = append(pages, NewsPage{Sectors: result[i:end]})
	}

	return pages, nil
}

func GenerateTTSText(pages []NewsPage) []string {
	texts := make([]string, len(pages))
	for i, page := range pages {
		var parts []string
		if i == 0 {
			parts = append(parts, "接下来看看今天的板块热点新闻。")
		}
		for si, sn := range page.Sectors {
			var titles []string
			for _, item := range sn.News {
				titles = append(titles, item.Title)
			}
			if si == 0 {
				parts = append(parts, sn.Sector+"板块。"+strings.Join(titles, "，"))
			} else {
				parts = append(parts, "再看"+sn.Sector+"，"+strings.Join(titles, "，"))
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
