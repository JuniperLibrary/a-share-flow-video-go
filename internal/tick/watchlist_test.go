package tick

import (
	"testing"

	"github.com/a-share-flow-video-go/internal/fetcher"
)

// TestMergeSectorsByCodeOrName_DefaultPlusWatchlist 验证：默认板块与自选板块合并时
// 两者都保留，交集去重，自选里有但默认没有的板块也一并纳入。
func TestMergeSectorsByCodeOrName_DefaultPlusWatchlist(t *testing.T) {
	defaults := []fetcher.Sector{
		{Name: "半导体", BKCode: "BK1036", Net: 30},
		{Name: "AI应用", BKCode: "BK1629", Net: 25},
		{Name: "CPO概念", BKCode: "BK1128", Net: 20},
	}
	watch := []fetcher.Sector{
		{Name: "AI应用", BKCode: "BK1629", Net: 25},  // 交集，应去重（默认优先）
		{Name: "CPO概念", BKCode: "BK1128", Net: 20}, // 交集，应去重
		{Name: "低空经济", BKCode: "BK1166", Net: 18},  // 自选独有，应保留
		{Name: "证券", BKCode: "BK0473", Net: 15},    // 自选独有，应保留
	}

	got := mergeSectorsByCodeOrName(defaults, watch)

	if len(got) != 5 {
		t.Fatalf("合并后板块数 = %d, 期望 5（默认3 + 自选独有2），got=%+v", len(got), got)
	}

	names := make(map[string]bool, len(got))
	for _, s := range got {
		names[s.Name] = true
	}
	for _, want := range []string{"半导体", "AI应用", "CPO概念", "低空经济", "证券"} {
		if !names[want] {
			t.Fatalf("合并结果缺少板块 %q：got=%v", want, names)
		}
	}
}

// TestMergeSectorsByCodeOrName_DedupByName 验证 BKCode 为空时按 Name 去重。
func TestMergeSectorsByCodeOrName_DedupByName(t *testing.T) {
	defaults := []fetcher.Sector{{Name: "半导体", Net: 30}}
	watch := []fetcher.Sector{{Name: "半导体", Net: 31}} // 无 BKCode，按 Name 去重

	got := mergeSectorsByCodeOrName(defaults, watch)
	if len(got) != 1 {
		t.Fatalf("按名称去重失败：len=%d got=%+v", len(got), got)
	}
	if got[0].Net != 30 {
		t.Fatalf("默认优先：expected Net=30 (default), got=%v", got[0].Net)
	}
}
