package oral

import (
	"testing"
)

func TestWrittenToOral_ReplacesWrittenPhrases(t *testing.T) {
	cases := map[string]string{
		"综上所述，今天的主线只有一条。": "说白了，今天的主线只有一条。",
		"不难看出资金在切换。":      "你再往深看资金在切换。",
		"整体来看，盘面偏强。":      "盘面看，盘面偏强。",
		"净流入达到了 50 亿。":    "净流入到了 50 亿。",
	}
	for in, want := range cases {
		if got := WrittenToOral.Replace(in); got != want {
			t.Errorf("Replace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWrittenToOral_ConvertsFullWidthSemicolon(t *testing.T) {
	if got := WrittenToOral.Replace("资金流入；情绪回暖"); got != "资金流入，情绪回暖" {
		t.Fatalf("全角分号未转逗号，得到 %q", got)
	}
}

func TestBuzzwords_ReplacesBusinessJargon(t *testing.T) {
	cases := map[string]string{
		"形成业务闭环": "形成业务完整流程",
		"找到增长抓手": "找到增长切入点",
	}
	for in, want := range cases {
		if got := Buzzwords.Replace(in); got != want {
			t.Errorf("Replace(%q) = %q, want %q", in, got, want)
		}
	}
}

// 重复键会让先注册的规则静默失效，copy.go 曾把「整体来看」写了两遍。
func TestRules_NoDuplicateKeys(t *testing.T) {
	for name, pairs := range map[string][]string{
		"writtenToOralPairs": writtenToOralPairs,
		"buzzwordPairs":      buzzwordPairs,
	} {
		seen := make(map[string]bool, len(pairs)/2)
		for i := 0; i < len(pairs); i += 2 {
			if seen[pairs[i]] {
				t.Errorf("%s 存在重复键 %q", name, pairs[i])
			}
			seen[pairs[i]] = true
		}
	}
}

// 键值对不成对会让 strings.NewReplacer 直接 panic，这里提前拦住。
func TestRules_PairsAreBalanced(t *testing.T) {
	for name, pairs := range map[string][]string{
		"writtenToOralPairs": writtenToOralPairs,
		"buzzwordPairs":      buzzwordPairs,
	} {
		if len(pairs)%2 != 0 {
			t.Errorf("%s 键值对不成对，长度 %d", name, len(pairs))
		}
	}
}
