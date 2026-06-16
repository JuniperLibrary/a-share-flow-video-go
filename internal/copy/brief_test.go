package copy

import (
	"strings"
	"testing"

	"github.com/a-share-flow-video-go/internal/fetcher"
)

func TestBuildNarrativeBrief_IncludesStructure(t *testing.T) {
	sectors := []fetcher.Sector{
		{Name: "国产芯片", Net: 253, SuperNet: 238, BigNet: 15, ChangePct: 2.5},
		{Name: "半导体", Net: 163, SuperNet: 140, BigNet: 23, ChangePct: 1.8},
		{Name: "银行", Net: -120, SuperNet: -110, BigNet: -10, ChangePct: -0.5},
	}
	brief, err := BuildNarrativeBrief(sectors, "2026-06-15", "full", "")
	if err != nil {
		t.Fatal(err)
	}
	formatted := brief.Format()
	if !strings.Contains(formatted, "国产芯片") || !strings.Contains(formatted, "超大单+238") {
		t.Fatalf("brief missing sector structure:\n%s", formatted)
	}
	if !strings.Contains(formatted, "可用叙事角度") {
		t.Fatalf("brief missing angles:\n%s", formatted)
	}
}

func TestTruncateRunes_BytesVsRunes(t *testing.T) {
	// 14 个汉字 = 42 UTF-8 bytes，但仅 14 runes；旧逻辑 len>40 会 panic
	title := strings.Repeat("测", 14)
	got := truncateRunes(title, 40)
	if got != title {
		t.Fatalf("expected no truncate for 14 runes, got %q", got)
	}
	long := strings.Repeat("测", 50)
	got = truncateRunes(long, 40)
	if len([]rune(got)) != 41 { // 40 + ellipsis rune
		t.Fatalf("expected 41 runes with ellipsis, got %d: %q", len([]rune(got)), got)
	}
}

func TestLoadNewsContext_NoPanicOnLongByteTitle(t *testing.T) {
	s := strings.Repeat("新", 20)
	_ = truncateRunes(s, 40)
}

func TestValidateCopy_RejectsBannedPhrase(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 资金只认国产芯片。
[悬念] 指数涨了。
[反转] 全是游资。
[收尾] 科技主线。
[钩子] 明天看。`
	issues := ValidateCopy(text, brief, inflows)
	if len(issues) == 0 {
		t.Fatal("expected validation issues")
	}
}
