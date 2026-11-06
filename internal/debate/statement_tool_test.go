package debate

import (
	"context"
	"testing"
)

func TestStatementSecucode(t *testing.T) {
	cases := map[string]string{
		"600519":    "600519.SH",
		"000001":    "000001.SZ",
		"300750":    "300750.SZ",
		"830799":    "830799.BJ",
		"600519.SH": "600519.SH",
	}
	for in, want := range cases {
		if got := statementSecucode(in); got != want {
			t.Errorf("statementSecucode(%q) = %q, want %q", in, got, want)
		}
	}
}

// 报表数值必须换算成亿元；null 科目要跳过而不是记成 0——
// "科目为 0"和"科目缺失"在辩论里是两个完全不同的结论。
func TestBuildStatementResult(t *testing.T) {
	spec := statementSpecs["cashflow"]
	rows := []map[string]any{{
		"REPORT_DATE":     "2026-09-30 00:00:00",
		"REPORT_TYPE":     "三季报",
		"NETCASH_OPERATE": 84058979832.29,
		"SALES_SERVICES":  143522752519.23,
		"CCE_ADD":         nil,
	}}

	out := buildStatementResult(spec, rows)
	if len(out) != 1 {
		t.Fatalf("got %d periods, want 1", len(out))
	}
	got := out[0]
	if got["报告期"] != "2026-09-30" {
		t.Errorf("报告期 = %v, 应去掉时间部分", got["报告期"])
	}
	if v, _ := got["经营活动现金流净额"].(float64); v != 840.59 {
		t.Errorf("经营现金流 = %v 亿, want 840.59", v)
	}
	if v, _ := got["销售商品提供劳务收到的现金"].(float64); v != 1435.23 {
		t.Errorf("销售收现 = %v 亿, want 1435.23", v)
	}
	if _, ok := got["现金及现金等价物净增加额"]; ok {
		t.Error("null 科目不应出现在结果里")
	}
}

func TestFetchStatementRejectsUnknownStatement(t *testing.T) {
	tool := &fetchStatementTool{}
	if _, err := tool.Execute(context.Background(), map[string]any{"stock": "600519", "statement": "profit"}); err == nil {
		t.Error("未知 statement 应直接报错,而不是静默回退")
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"statement": "cashflow"}); err == nil {
		t.Error("缺 stock 应报错")
	}
}
