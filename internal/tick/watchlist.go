package tick

import (
	"math"
	"sort"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
)

func FetchTickSectors() ([]fetcher.Sector, error) {
	wl := config.LoadSectorWatchlist()
	enabled := make([]config.SectorWatchItem, 0, len(wl.Items))
	for _, it := range wl.Items {
		if it.Enabled {
			enabled = append(enabled, it)
		}
	}
	if len(enabled) == 0 {
		return fetcher.FetchTop21HotSectors()
	}

	byCategory := make(map[string][]config.SectorWatchItem)
	for _, it := range enabled {
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

	results := make([]fetcher.Sector, 0, len(enabled))
	added := make(map[string]bool)
	for _, it := range enabled {
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
