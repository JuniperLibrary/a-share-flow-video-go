package fetcher

import (
	"math"
	"sort"

	"github.com/a-share-flow-video-go/internal/storage"
)

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func rankScore(rank, total int) float64 {
	if total <= 1 {
		return 0
	}
	return float64(rank) / float64(total-1)
}

func SectorStrengthScore(sectors []Sector) map[string]float64 {
	n := len(sectors)
	if n == 0 {
		return nil
	}

	byPct := make([]int, n)
	byAbsNet := make([]int, n)
	byTurnover := make([]int, n)
	for i := range sectors {
		byPct[i] = i
		byAbsNet[i] = i
		byTurnover[i] = i
	}

	sort.SliceStable(byPct, func(i, j int) bool { return sectors[byPct[i]].ChangePct > sectors[byPct[j]].ChangePct })
	sort.SliceStable(byAbsNet, func(i, j int) bool {
		return abs64(sectors[byAbsNet[i]].Net) > abs64(sectors[byAbsNet[j]].Net)
	})
	sort.SliceStable(byTurnover, func(i, j int) bool {
		return sectors[byTurnover[i]].Turnover > sectors[byTurnover[j]].Turnover
	})

	pctRank := make([]int, n)
	absNetRank := make([]int, n)
	turnoverRank := make([]int, n)
	for rank := 0; rank < n; rank++ {
		pctRank[byPct[rank]] = rank
		absNetRank[byAbsNet[rank]] = rank
		turnoverRank[byTurnover[rank]] = rank
	}

	out := make(map[string]float64, n)
	for i := 0; i < n; i++ {
		score := 0.55*rankScore(pctRank[i], n) + 0.30*rankScore(absNetRank[i], n) + 0.15*rankScore(turnoverRank[i], n)
		out[sectors[i].Name] = score
	}
	return out
}

func MergeSectorsWithDaily(tickSectors []Sector, daily []storage.SectorAll, wildcardLimit int) []Sector {
	byName := make(map[string]storage.SectorAll, len(daily))
	for _, d := range daily {
		byName[d.Name] = d
	}

	tickNameSet := make(map[string]bool, len(tickSectors))
	result := make([]Sector, 0, len(tickSectors)+wildcardLimit)

	for _, ts := range tickSectors {
		s := ts
		tickNameSet[s.Name] = true
		if d, ok := byName[s.Name]; ok {
			if d.ChangePct != 0 {
				s.ChangePct = d.ChangePct
			}
			if d.Net != 0 {
				s.Net = d.Net
			}
			if d.Rate != 0 {
				s.Rate = d.Rate
			}
			if d.SuperNet != 0 {
				s.SuperNet = d.SuperNet
			}
			if d.SuperRate != 0 {
				s.SuperRate = d.SuperRate
			}
			if d.BigNet != 0 {
				s.BigNet = d.BigNet
			}
			if d.BigRate != 0 {
				s.BigRate = d.BigRate
			}
			if d.Volume != 0 {
				s.Volume = d.Volume
			}
			if d.Turnover != 0 {
				s.Turnover = d.Turnover
			}
			if d.TurnoverRate != 0 {
				s.TurnoverRate = d.TurnoverRate
			}
			if d.LeadStockName != "" {
				s.LeadStockName = d.LeadStockName
			}
			if d.LeadStockChangePct != 0 {
				s.LeadStockChangePct = d.LeadStockChangePct
			}
			if d.TotalMarketCap != 0 {
				s.TotalMarketCap = d.TotalMarketCap
			}
			if d.CirculatingMarketCap != 0 {
				s.CirculatingMarketCap = d.CirculatingMarketCap
			}
			if d.Code != "" {
				s.BKCode = d.Code
			}
			if d.Category != "" {
				s.Category = d.Category
			}
		}
		result = append(result, s)
	}

	if wildcardLimit > 0 && len(daily) > 0 {
		var candidates []Sector
		for _, d := range daily {
			if tickNameSet[d.Name] {
				continue
			}
			if math.Abs(d.Net) < 3 && math.Abs(d.ChangePct) < 2.5 {
				continue
			}
			candidates = append(candidates, Sector{
				Name:                 d.Name,
				Net:                  d.Net,
				Rate:                 d.Rate,
				ChangePct:            d.ChangePct,
				SuperNet:             d.SuperNet,
				SuperRate:            d.SuperRate,
				BigNet:               d.BigNet,
				BigRate:              d.BigRate,
				Volume:               d.Volume,
				Turnover:             d.Turnover,
				BKCode:               d.Code,
				TurnoverRate:         d.TurnoverRate,
				LeadStockName:        d.LeadStockName,
				LeadStockChangePct:   d.LeadStockChangePct,
				TotalMarketCap:       d.TotalMarketCap,
				CirculatingMarketCap: d.CirculatingMarketCap,
				Category:             d.Category,
			})
		}
		if len(candidates) > 0 {
			scores := SectorStrengthScore(candidates)
			sort.SliceStable(candidates, func(i, j int) bool {
				return scores[candidates[i].Name] < scores[candidates[j].Name]
			})
			if len(candidates) > wildcardLimit {
				candidates = candidates[:wildcardLimit]
			}
			result = append(result, candidates...)
		}
	}

	return result
}

func SortInflowsByStrength(flows []struct {
	Name     string
	Net      float64
	SuperNet float64
	BigNet   float64
	ChgPct   float64
}, sectorsMap map[string]Sector, strength map[string]float64) {
	sort.SliceStable(flows, func(i, j int) bool {
		si, oki := strength[flows[i].Name]
		sj, okj := strength[flows[j].Name]
		if oki && okj && math.Abs(si-sj) > 1e-9 {
			return si < sj
		}
		if flows[i].Net != flows[j].Net {
			return flows[i].Net > flows[j].Net
		}
		return flows[i].ChgPct > flows[j].ChgPct
	})
	_ = sectorsMap
}
