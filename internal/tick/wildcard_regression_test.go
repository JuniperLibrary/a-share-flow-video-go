package tick

import (
	"testing"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/storage"
)

// TestApplyDailyToSectorTicks_NoWildcardSectors 验证 tick 视频修复：
// 资金曲线严格按真实 tick 采集到的板块生成，禁止从当日 sectors 补充额外板块。
//
// 回归场景（旧实现 bug）：tick 板块数 <26 时，applyDailyToSectorTicks 会把当日
// sectors_all 中 tick 没有的活跃板块用线性假数据补充进资金曲线，导致视频
// "多出几个板块"，且数据并非当日 sectors 真实数据。
func TestApplyDailyToSectorTicks_NoWildcardSectors(t *testing.T) {
	config.SetProjectRoot(t.TempDir())
	storage.Reset()
	t.Cleanup(func() { storage.Reset() })

	db, err := storage.Get()
	if err != nil {
		t.Fatalf("storage.Get: %v", err)
	}

	const dateStr = "2099-01-01"

	// 当日 sectors_all：包含真实 tick 板块 + 多个"活跃但未采集"的板块
	daily := []storage.SectorAll{
		{Date: dateStr, Name: "AI应用", Net: 30, ChangePct: 4.2},
		{Date: dateStr, Name: "CPO概念", Net: 20, ChangePct: 3.0},
		{Date: dateStr, Name: "有色金属", Net: 12, ChangePct: 5.1}, // 活跃，旧实现会补入
		{Date: dateStr, Name: "国产芯片", Net: 15, ChangePct: 4.5}, // 活跃，旧实现会补入
		{Date: dateStr, Name: "低空经济", Net: 8, ChangePct: 3.2},  // 活跃，旧实现会补入
	}
	if err := db.SaveSectorsAll(daily); err != nil {
		t.Fatalf("SaveSectorsAll: %v", err)
	}

	// 真实 tick 只采集到 2 个板块
	ticks := []SectorTick{
		{Name: "AI应用", Data: []float64{1, 2, 3}},
		{Name: "CPO概念", Data: []float64{4, 5, 6}},
	}

	got := applyDailyToSectorTicks(ticks, nil, dateStr)

	if len(got) != len(ticks) {
		t.Fatalf("applyDailyToSectorTicks 板块数 = %d, 期望 = %d (不应补充额外板块)",
			len(got), len(ticks))
	}

	names := make(map[string]bool, len(got))
	for _, s := range got {
		names[s.Name] = true
	}
	for _, extra := range []string{"有色金属", "国产芯片", "低空经济"} {
		if names[extra] {
			t.Fatalf("不应包含当日 sectors 补入的板块 %q：got=%v", extra, names)
		}
	}
	for _, s := range ticks {
		if !names[s.Name] {
			t.Fatalf("真实 tick 板块丢失：%s，got=%v", s.Name, names)
		}
	}

	// 当日字段覆盖仍应生效（AI应用 ChangePct 被 daily 4.2 覆盖）
	for i := range got {
		if got[i].Name == "AI应用" && got[i].ChangePct != 4.2 {
			t.Fatalf("AI应用 ChangePct 未被当日 sectors 覆盖：got=%v", got[i].ChangePct)
		}
	}
}
