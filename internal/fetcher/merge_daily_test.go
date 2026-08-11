package fetcher

import (
	"testing"

	"github.com/a-share-flow-video-go/internal/storage"
)

func TestMergeSectorsWithDaily_JoinOverride(t *testing.T) {
	tick := []Sector{
		{Name: "CPO概念", Net: 79, ChangePct: 2.1, Turnover: 500, TurnoverRate: 3.2, LeadStockName: "中际旭创", LeadStockChangePct: 5.1},
		{Name: "半导体设备", Net: 2, ChangePct: 1.8, Turnover: 400, TurnoverRate: 2.0, LeadStockName: "北方华创", LeadStockChangePct: 3.0},
	}
	daily := []storage.SectorAll{
		{Name: "CPO概念", ChangePct: 1.2, Turnover: 520, TurnoverRate: 3.5, LeadStockName: "天孚通信", LeadStockChangePct: 4.2, Net: 21.4, Rate: 0.38},
		{Name: "半导体设备", ChangePct: 3.75, Turnover: 650, TurnoverRate: 4.1, LeadStockName: "中微公司", LeadStockChangePct: 6.5, Net: 36.9, Rate: 1.24},
		{Name: "黄金", ChangePct: 5.37, Turnover: 450, TurnoverRate: 2.8, LeadStockName: "山东黄金", LeadStockChangePct: 9.98, Net: 10.5, Rate: 2.1},
	}
	merged := MergeSectorsWithDaily(tick, daily, 5)
	if len(merged) < 3 {
		t.Fatalf("wildcard 黄金没被补入，merged=%+v", merged)
	}
	var foundGold, foundSemi bool
	var gold, semi *Sector
	for i := range merged {
		if merged[i].Name == "黄金" {
			foundGold = true
			gold = &merged[i]
		}
		if merged[i].Name == "半导体设备" {
			foundSemi = true
			semi = &merged[i]
		}
	}
	if !foundGold {
		t.Fatal("黄金外卡未补入，应因 ChangePct 5.37 > 2.5 触发")
	}
	if gold.LeadStockName != "山东黄金" {
		t.Errorf("外卡板块领涨股没填 daily 值：%+v", gold)
	}
	if !foundSemi {
		t.Fatal("白名单 半导体设备 丢失")
	}
	if semi.ChangePct != 3.75 {
		t.Errorf("白名单板块 ChangePct 没被 daily 3.75 覆盖：got=%v", semi.ChangePct)
	}
	if semi.Net != 36.9 {
		t.Errorf("白名单板块 Net 没被 daily 36.9 覆盖：got=%v", semi.Net)
	}
	if semi.LeadStockName != "中微公司" {
		t.Errorf("白名单板块领涨股没被 daily 覆盖：got=%v", semi.LeadStockName)
	}
}

func TestSectorStrengthScore_PreferenceHighPct(t *testing.T) {
	sectors := []Sector{
		{Name: "涨停小净流", Net: 3.1, ChangePct: 9.97, Turnover: 60},
		{Name: "高净流低涨幅", Net: 9.0, ChangePct: 0.8, Turnover: 120},
	}
	scores := SectorStrengthScore(sectors)
	sa, sb := scores["涨停小净流"], scores["高净流低涨幅"]
	if !(sa < sb) {
		t.Errorf("强度分应该让 '涨停+3亿' 排前 '+9亿+0.8%%'，但 sa=%v sb=%v", sa, sb)
	}
}

func TestMergeSectorsWithDaily_WildcardThreshold(t *testing.T) {
	tick := []Sector{
		{Name: "白酒", Net: 1, ChangePct: 0.1},
	}
	daily := []storage.SectorAll{
		{Name: "微涨小净流", Net: 1, ChangePct: 1.0},
		{Name: "大涨板块", Net: 1, ChangePct: 3.0},
		{Name: "大净流板块", Net: 4.5, ChangePct: 0.5},
	}
	merged := MergeSectorsWithDaily(tick, daily, 5)
	var names []string
	for _, s := range merged {
		names = append(names, s.Name)
	}
	has := func(n string) bool {
		for _, x := range names {
			if x == n {
				return true
			}
		}
		return false
	}
	if has("微涨小净流") {
		t.Errorf("微涨小净流不满足阈值不应补入，names=%v", names)
	}
	if !has("大涨板块") {
		t.Errorf("大涨板块满足 |ChangePct|>=2.5 应补入，names=%v", names)
	}
	if !has("大净流板块") {
		t.Errorf("大净流板块满足 |Net|>=3 应补入，names=%v", names)
	}
}
