package tick

import (
	"math"
	"sort"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// FetchTickSectors 返回 tick 采集的板块名单 = 默认 Top21 热门板块 ∪ 自选板块。
func FetchTickSectors() ([]fetcher.Sector, error) {
	// 1. 默认板块：始终拉取 Top21 热门板块
	defaultSectors, defaultErr := fetcher.FetchTop21HotSectors()
	if defaultErr != nil {
		logger.Warn("拉取默认热门板块失败", zap.Error(defaultErr))
	}

	// 2. 自选板块：watchlist 中启用的板块
	wl := config.LoadSectorWatchlist()
	enabled := make([]config.SectorWatchItem, 0, len(wl.Items))
	for _, it := range wl.Items {
		if it.Enabled {
			enabled = append(enabled, it)
		}
	}

	var watchSectors []fetcher.Sector
	if len(enabled) > 0 {
		var err error
		watchSectors, err = fetchWatchSectors(enabled)
		if err != nil {
			return nil, err
		}
	}

	// 3. 按 BKCode/Name 去重合并，默认板块优先
	results := mergeSectorsByCodeOrName(defaultSectors, watchSectors)

	sort.Slice(results, func(i, j int) bool {
		ai := math.Abs(results[i].Net)
		aj := math.Abs(results[j].Net)
		if ai != aj {
			return ai > aj
		}
		return results[i].Name < results[j].Name
	})
	return results, nil
}

// fetchWatchSectors 按分类抓取并匹配自选板块（BKCode 优先，名称兜底）。
func fetchWatchSectors(items []config.SectorWatchItem) ([]fetcher.Sector, error) {
	byCategory := make(map[string][]config.SectorWatchItem)
	for _, it := range items {
		cat := it.Category
		if cat == "" {
			cat = "industry"
		}
		byCategory[cat] = append(byCategory[cat], it)
	}

	fetched := make(map[string]fetcher.Sector)
	nameFetched := make(map[string]fetcher.Sector)
	for cat := range byCategory {
		sectors, err := fetcher.FetchSectorsByCategory(cat)
		if err != nil {
			return nil, err
		}
		for _, s := range sectors {
			if s.BKCode != "" {
				fetched[s.BKCode] = s
			}
			if s.Name != "" {
				nameFetched[cat+"|"+s.Name] = s
			}
		}
	}

	results := make([]fetcher.Sector, 0, len(items))
	added := make(map[string]bool)
	for _, it := range items {
		if it.BKCode != "" {
			if s, ok := fetched[it.BKCode]; ok {
				if !added[s.BKCode] {
					results = append(results, s)
					added[s.BKCode] = true
				}
				continue
			}
		}
		if it.Name != "" {
			cat := it.Category
			if cat == "" {
				cat = "industry"
			}
			if s, ok := nameFetched[cat+"|"+it.Name]; ok {
				key := s.BKCode
				if key == "" {
					key = cat + "|" + s.Name
				}
				if !added[key] {
					results = append(results, s)
					added[key] = true
				}
			}
		}
	}
	return results, nil
}

// mergeSectorsByCodeOrName 按 BKCode（缺失时用 Name）去重合并，defaults 优先。
func mergeSectorsByCodeOrName(defaults, watch []fetcher.Sector) []fetcher.Sector {
	seen := make(map[string]bool)
	out := make([]fetcher.Sector, 0, len(defaults)+len(watch))
	add := func(s fetcher.Sector) {
		key := s.BKCode
		if key == "" {
			key = s.Name
		}
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}
	for _, s := range defaults {
		add(s)
	}
	for _, s := range watch {
		add(s)
	}
	return out
}
