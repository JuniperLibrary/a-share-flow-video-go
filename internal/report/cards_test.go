package report

import "testing"

func TestBuildCardsFromData(t *testing.T) {
	r := &DailyReport{
		Date:          "2026-06-15",
		NetTotal:      120.5,
		InflowCount:   12,
		OutflowCount:  8,
		StructureDesc: "机构主导的集中流入",
		SuperNetTotal: 80,
		BigNetTotal:   40.5,
		TopInflows: []SectorSummary{
			{Name: "半导体", Net: 45.2, ChangePct: 2.1, LeadStockName: "中芯国际", LeadStockChangePct: 5.2},
			{Name: "人工智能", Net: 30.0, ChangePct: 1.5},
		},
		TopOutflows: []SectorSummary{
			{Name: "银行", Net: -20.3, ChangePct: -0.8},
		},
		NewsBriefs: []NewsBrief{
			{Title: "半导体板块资金持续流入", Level: "A", Time: "14:30", Brief: "测试摘要", Sectors: []string{"半导体"}},
			{Title: "AI应用端分化", Level: "B", Time: "15:00"},
		},
	}

	cards := BuildCardsFromData(r)
	if len(cards) < 2 {
		t.Fatalf("expected at least 2 cards, got %d", len(cards))
	}
	if cards[0].Index != 1 || cards[0].Total != len(cards) {
		t.Fatalf("unexpected first card meta: %+v", cards[0])
	}
	if cards[0].Title == "" || cards[0].Subtitle == "" {
		t.Fatalf("overview card missing title/subtitle: %+v", cards[0])
	}
	if len(cards[0].Metrics) == 0 {
		t.Fatal("overview card should include metrics")
	}
}

func TestParseNewsSectors(t *testing.T) {
	got := parseNewsSectors(`["半导体","人工智能"]`)
	if len(got) != 2 || got[0] != "半导体" {
		t.Fatalf("unexpected sectors: %v", got)
	}
	if parseNewsSectors("") != nil {
		t.Fatal("expected nil for empty sectors")
	}
}
