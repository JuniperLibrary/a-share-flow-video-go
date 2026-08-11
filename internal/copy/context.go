package copy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/a-share-flow-video-go/internal/fetcher"
)

type sectorFlow struct {
	Name     string
	Net      float64
	SuperNet float64
	BigNet   float64
	ChgPct   float64
}

func splitSectorFlows(sectors []fetcher.Sector) (inflows, outflows []sectorFlow, netTotal, totalSuper, totalBig float64) {
	inflowStrength := fetcher.SectorStrengthScore(sectors)
	nameToSector := make(map[string]fetcher.Sector, len(sectors))
	for _, s := range sectors {
		nameToSector[s.Name] = s
	}
	for _, s := range sectors {
		item := sectorFlow{s.Name, s.Net, s.SuperNet, s.BigNet, s.ChangePct}
		netTotal += s.Net
		totalSuper += s.SuperNet
		totalBig += s.BigNet
		if s.Net > 0 {
			inflows = append(inflows, item)
		} else if s.Net < 0 {
			outflows = append(outflows, item)
		}
	}

	outflowSectors := make([]fetcher.Sector, 0, len(outflows))
	for _, f := range outflows {
		if s, ok := nameToSector[f.Name]; ok {
			outflowSectors = append(outflowSectors, s)
		}
	}
	outflowStrength := make(map[string]float64, len(outflowSectors))
	if n := len(outflowSectors); n > 0 {
		byChg := make([]int, n)
		byAbsNet := make([]int, n)
		byTurnover := make([]int, n)
		for i := range outflowSectors {
			byChg[i] = i
			byAbsNet[i] = i
			byTurnover[i] = i
		}
		sort.SliceStable(byChg, func(i, j int) bool {
			return outflowSectors[byChg[i]].ChangePct < outflowSectors[byChg[j]].ChangePct
		})
		sort.SliceStable(byAbsNet, func(i, j int) bool {
			ai := outflowSectors[byAbsNet[i]]
			aj := outflowSectors[byAbsNet[j]]
			absI := ai.Net
			absJ := aj.Net
			if absI < 0 {
				absI = -absI
			}
			if absJ < 0 {
				absJ = -absJ
			}
			return absI > absJ
		})
		sort.SliceStable(byTurnover, func(i, j int) bool {
			return outflowSectors[byTurnover[i]].Turnover > outflowSectors[byTurnover[j]].Turnover
		})
		chgRank := make([]int, n)
		netRank := make([]int, n)
		turnRank := make([]int, n)
		for rank := 0; rank < n; rank++ {
			chgRank[byChg[rank]] = rank
			netRank[byAbsNet[rank]] = rank
			turnRank[byTurnover[rank]] = rank
		}
		rankScore := func(r, total int) float64 {
			if total <= 1 {
				return 0
			}
			return float64(r) / float64(total-1)
		}
		for i, s := range outflowSectors {
			sc := 0.55*rankScore(chgRank[i], n) + 0.30*rankScore(netRank[i], n) + 0.15*rankScore(turnRank[i], n)
			outflowStrength[s.Name] = sc
		}
	}

	lessByStrength := func(a, b sectorFlow, expectInflow bool) bool {
		strength := inflowStrength
		if !expectInflow {
			strength = outflowStrength
		}
		sa, oka := strength[a.Name]
		sb, okb := strength[b.Name]
		if oka && okb {
			if sa != sb {
				return sa < sb
			}
		} else {
			_ = nameToSector
		}
		absA := a.Net
		absB := b.Net
		if absA < 0 {
			absA = -absA
		}
		if absB < 0 {
			absB = -absB
		}
		if absA != absB {
			return absA > absB
		}
		if expectInflow {
			if a.Net != b.Net {
				return a.Net > b.Net
			}
			return a.ChgPct > b.ChgPct
		}
		if a.Net != b.Net {
			return a.Net < b.Net
		}
		return a.ChgPct < b.ChgPct
	}

	sort.SliceStable(inflows, func(i, j int) bool { return lessByStrength(inflows[i], inflows[j], true) })
	sort.SliceStable(outflows, func(i, j int) bool { return lessByStrength(outflows[i], outflows[j], false) })
	return inflows, outflows, netTotal, totalSuper, totalBig
}

func formatTopFlows(items []sectorFlow, inflow bool, limit int) string {
	if len(items) == 0 {
		return "无"
	}
	var lines []string
	for i, v := range items {
		if i >= limit {
			break
		}
		if inflow {
			lines = append(lines, fmt.Sprintf("- %s: 主力%+.0f亿（超大单%+.0f亿 + 大单%+.0f亿），涨幅%.1f%%",
				v.Name, v.Net, v.SuperNet, v.BigNet, v.ChgPct))
		} else {
			lines = append(lines, fmt.Sprintf("- %s: 主力%.0f亿（超大单%.0f亿 + 大单%.0f亿），跌幅%.1f%%",
				v.Name, v.Net, v.SuperNet, v.BigNet, absF(v.ChgPct)))
		}
	}
	return strings.Join(lines, "\n")
}

func buildStructureBlock(netTotal, totalSuper, totalBig float64, inflows []sectorFlow, sectors []fetcher.Sector) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("全市场加总：主力净流向%+.0f亿，超大单合计%+.0f亿，大单合计%+.0f亿",
		netTotal, totalSuper, totalBig))

	if len(inflows) > 0 {
		lines = append(lines, "流入龙头（[反转] 引用此处超大单数值）：")
		lines = append(lines, formatTopFlows(inflows, true, 3))
	}

	maxTopSuper := 0.0
	for i := 0; i < len(inflows) && i < 3; i++ {
		if absF(inflows[i].SuperNet) > absF(maxTopSuper) {
			maxTopSuper = inflows[i].SuperNet
		}
	}

	if absF(maxTopSuper) >= 20 && absF(totalSuper) < absF(maxTopSuper)*0.5 {
		lines = append(lines, fmt.Sprintf(
			"结构提示：全市场超大单加总 %+.0f 亿（板块对冲），龙头超大单绝对值 %.0f 亿——[反转] 写龙头，勿写机构缺席。",
			totalSuper, absF(maxTopSuper)))
	}

	allOrderStructZero := true
	for _, s := range sectors {
		if s.SuperNet != 0 || s.BigNet != 0 {
			allOrderStructZero = false
			break
		}
	}
	if allOrderStructZero && absF(netTotal) > 50 {
		lines = append(lines, "字段提示：超大单/大单缺失，[反转] 仅写主力方向，勿推断游资/机构。")
	}

	return strings.Join(lines, "\n")
}

var techSectorNames = map[string]bool{
	"半导体": true, "国产芯片": true, "CPO概念": true, "AI应用": true,
	"人工智能": true, "云计算": true, "机器人": true, "元件": true,
}
