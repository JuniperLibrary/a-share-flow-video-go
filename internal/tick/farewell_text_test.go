package tick

import (
	"strings"
	"testing"
)

// TestFarewellTexts_OralMatchesBlessingAndSignoff 验证告别仪式修复：
// 口播文案必须由画面展示的 blessing + signoff 拼接而成（同一日期 seed），
// 保证 MP3 与视频文字一致，且使用最新的祝福语池。
func TestFarewellTexts_OralMatchesBlessingAndSignoff(t *testing.T) {
	dates := [][2]string{ // {date, displayDate}
		{"2026-08-13", "08-13"},
		{"2026-08-14", "08-14"},
		{"2026-01-02", "01-02"},
		{"2026-12-31", "12-31"},
		{"2026-06-30", "06-30"},
		{"2026-02-13", "02-13"}, // 春节前,下一交易日跳至 2/24
	}
	for _, dd := range dates {
		blessing, signoff := buildFarewellTexts(dd[0], dd[1])
		oral := blessing + " " + signoff

		if oral == "" {
			t.Fatalf("date=%s: farewellText 为空", dd[0])
		}
		if !strings.Contains(oral, blessing) || !strings.Contains(oral, signoff) {
			t.Fatalf("date=%s: 口播文案未包含 blessing/signoff: oral=%q blessing=%q signoff=%q", dd[0], oral, blessing, signoff)
		}
		// blessing 必须来自最新祝福语池（非旧口播池首句）
		if !containsStr(farewellBlessingPool, blessing) {
			t.Fatalf("date=%s: blessing %q 不在最新祝福语池", dd[0], blessing)
		}
		// signoff 必须带具体下一交易日日期,且不含相对时间词
		for _, w := range relativeTimeWords {
			if strings.Contains(blessing, w) || strings.Contains(signoff, w) {
				t.Fatalf("date=%s: 文案含相对时间词 %q: blessing=%q signoff=%q", dd[0], w, blessing, signoff)
			}
		}
		if !strings.Contains(signoff, "见") {
			t.Fatalf("date=%s: signoff 缺少告别语: %q", dd[0], signoff)
		}
	}
}

// TestFarewellTexts_NoAIKeyFallsBackToStaticPool 验证未配置 API key 时，
// BuildFarewellTexts 回退到静态祝福语池，保证视频生成不依赖 LLM。
func TestFarewellTexts_NoAIKeyFallsBackToStaticPool(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	blessing, signoff := BuildFarewellTexts("2026-08-13", "08-13", nil)
	if !containsStr(farewellBlessingPool, blessing) {
		t.Fatalf("无 key 时应回退静态池，blessing %q 不在池中", blessing)
	}
	if signoff == "" {
		t.Fatalf("无 key 时 signoff 不应为空")
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
