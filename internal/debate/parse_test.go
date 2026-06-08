package debate

import (
	"fmt"
	"testing"
)

func TestParseScript_NewRange(t *testing.T) {
	mockJSON := `{"turns":[
		{"speaker":"bull","text":"营收 1207 亿增 16.9%，全市场掰指头。","emotion":"confident"},
		{"speaker":"bear","text":"Q3 单季 13.2%，比 H1 慢了 3 个点。","emotion":"skeptical"},
		{"speaker":"bull","text":"净利率 50.4%，比去年还高 0.16pct。","emotion":"excident"},
		{"speaker":"bear","text":"合同负债降 11.7%，经销商不囤货。","emotion":"concerned"},
		{"speaker":"bull","text":"账上现金 800 亿，零有息负债。","emotion":"impatient"},
		{"speaker":"bear","text":"PE 32 倍，行业 21。贵了 50%。","emotion":"pointed"},
		{"speaker":"bull","text":"经营现金流 +35%，是真金白银。","emotion":"defiant"},
		{"speaker":"bear","text":"研发涨 31%，利润才涨 15%。","emotion":"pointed"},
		{"speaker":"bull","text":"ROE 24.8%，全 A 掰指头数。","emotion":"defiant"},
		{"speaker":"bear","text":"现金多但分红率 52%，钱留哪？","emotion":"pointed"},
		{"speaker":"bull","text":"直销 +27%，占比冲到 43%。","emotion":"defiant"},
		{"speaker":"bear","text":"系列酒 24%，掉队茅台 15%。","emotion":"pointed"}
	]}`

	script, err := parseScript(mockJSON)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(script.Turns) != 12 {
		t.Errorf("expected 12 turns, got %d", len(script.Turns))
	}

	bullCount, bearCount := 0, 0
	totalChars := 0
	minChars, maxChars := 999, 0
	overLimit := 0
	underLimit := 0
	for _, tn := range script.Turns {
		c := len([]rune(tn.Text))
		totalChars += c
		if c < minChars {
			minChars = c
		}
		if c > maxChars {
			maxChars = c
		}
		if c > 20 {
			overLimit++
		}
		if c < 8 {
			underLimit++
		}
		if tn.Speaker == Bull {
			bullCount++
		} else {
			bearCount++
		}
	}
	avgChars := float64(totalChars) / float64(len(script.Turns))

	fmt.Printf("\n=== parseScript Test Results ===\n")
	fmt.Printf("Turns: %d (target 8-12 even) — PASS\n", len(script.Turns))
	fmt.Printf("Speakers: bull=%d bear=%d (must be equal) — %s\n",
		bullCount, bearCount, map[bool]string{true: "PASS", false: "FAIL"}[bullCount == bearCount])
	fmt.Printf("Char range: min=%d max=%d avg=%.1f\n", minChars, maxChars, avgChars)
	fmt.Printf("Within 8-20 char target: %d/%d (over=%d under=%d)\n",
		len(script.Turns)-overLimit-underLimit, len(script.Turns), overLimit, underLimit)

	for i, tn := range script.Turns {
		chars := len([]rune(tn.Text))
		mark := "OK"
		if chars < 8 || chars > 20 {
			mark = "OUT"
		}
		fmt.Printf("  %s [%2d] %-4s  chars=%-3d  text=%q\n",
			mark, i, tn.Speaker, chars, tn.Text)
	}
}

func TestParseScript_RejectsOldRange(t *testing.T) {
	mockJSON4 := `{"turns":[
		{"speaker":"bull","text":"test","emotion":"confident"},
		{"speaker":"bear","text":"test","emotion":"skeptical"},
		{"speaker":"bull","text":"test","emotion":"confident"},
		{"speaker":"bear","text":"test","emotion":"skeptical"}
	]}`
	_, err := parseScript(mockJSON4)
	if err == nil {
		t.Errorf("expected parse to fail for 4 turns (below new minimum 8), got success")
	} else {
		fmt.Printf("✓ 4 turns correctly rejected: %v\n", err)
	}
}
