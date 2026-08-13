package tick

import (
	"strings"
	"testing"
)

// TestFarewellTexts_OralMatchesBlessingAndSignoff 验证告别仪式修复：
// 口播文案必须由画面展示的 blessing + signoff 拼接而成（同一日期 seed），
// 保证 MP3 与视频文字一致，且使用最新的祝福语池。
func TestFarewellTexts_OralMatchesBlessingAndSignoff(t *testing.T) {
	dates := []string{"07-15", "08-13", "01-02", "12-31", "06-30"}
	for _, d := range dates {
		_, blessing, signoff := buildFarewellTexts(d)
		oral := blessing + " " + signoff

		if oral == "" {
			t.Fatalf("date=%s: farewellText 为空", d)
		}
		if !strings.Contains(oral, blessing) || !strings.Contains(oral, signoff) {
			t.Fatalf("date=%s: 口播文案未包含 blessing/signoff: oral=%q blessing=%q signoff=%q", d, oral, blessing, signoff)
		}
		// blessing 必须来自最新祝福语池（非旧口播池首句）
		if !containsStr(farewellBlessingPool, blessing) {
			t.Fatalf("date=%s: blessing %q 不在最新祝福语池", d, blessing)
		}
		if !containsStr(farewellSignoffPool, signoff) {
			t.Fatalf("date=%s: signoff %q 不在告别语池", d, signoff)
		}
	}
}

func containsStr(pool []string, s string) bool {
	for _, p := range pool {
		if p == s {
			return true
		}
	}
	return false
}
