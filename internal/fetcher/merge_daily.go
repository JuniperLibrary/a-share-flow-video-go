package fetcher

import (
	"math"
	"sort"
	"strings"

	"github.com/a-share-flow-video-go/internal/storage"
)

var platformBucketExact = map[string]bool{
	"融资融券": true, "MSCI中国": true, "富时罗素": true, "沪股通": true, "深股通": true,
	"标准普尔": true, "2026中报预增": true, "大盘股": true, "中盘股": true, "小盘股": true,
	"沪深300": true, "HS300_": true, "上证50": true, "上证50_": true, "上证180_": true,
	"上证380_": true, "中证500": true, "中证1000": true, "中证2000": true, "深成500": true,
	"深证成指": true, "创业板指": true, "科创50": true, "央国企改革": true, "国企改革": true,
	"题材股": true, "东方财富热股": true, "最近多板": true, "高股息": true, "低价股": true,
	"高价股": true, "预亏": true, "扭亏": true, "摘帽": true, "增持": true, "回购": true,
	"股权激励": true, "员工持股": true, "举牌": true, "异动股": true, "庄股": true,
	"独角兽": true, "壳资源": true, "分拆上市": true, "债转股": true, "北交所概念": true,
	"四川板块": true, "北京板块": true, "上海板块": true, "广东板块": true, "浙江板块": true,
	"江苏板块": true, "山东板块": true, "河南板块": true, "湖北板块": true, "湖南板块": true,
	"福建板块": true, "安徽板块": true, "河北板块": true, "陕西板块": true, "重庆板块": true,
	"天津板块": true, "辽宁板块": true, "吉林板块": true, "黑龙江板块": true, "江西板块": true,
	"山西板块": true, "云南板块": true, "贵州板块": true, "广西板块": true, "新疆板块": true,
	"西藏板块": true, "青海板块": true, "甘肃板块": true, "宁夏板块": true, "内蒙古板块": true,
	"海南板块": true, "深圳板块": true, "新三板": true,
}

var platformBucketPrefix = []string{
	"昨日", "新股", "次新股", "中报", "年报", "一季报", "三季报",
	"预增", "预亏", "扭亏", "ST", "*ST",
}

var platformBucketContains = []string{
	"成份", "指数", "精选", "核心资产",
}

func isPlatformBucket(name string) bool {
	if name == "" {
		return true
	}
	if platformBucketExact[name] {
		return true
	}
	if strings.HasSuffix(name, "_") {
		return true
	}
	for _, p := range platformBucketPrefix {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	for _, p := range platformBucketContains {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}

// IsPlatformBucket reports whether a sector name is a composite market label /
// platform bucket (融资融券 / MSCI中国 / 四川板块 / 东方财富热股 …)
// instead of a tradable industry or concept.
func IsPlatformBucket(name string) bool { return isPlatformBucket(name) }

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
			if isPlatformBucket(d.Name) {
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
