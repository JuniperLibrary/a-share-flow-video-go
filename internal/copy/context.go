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
	sort.Slice(inflows, func(i, j int) bool { return inflows[i].Net > inflows[j].Net })
	sort.Slice(outflows, func(i, j int) bool { return outflows[i].Net < outflows[j].Net })
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
