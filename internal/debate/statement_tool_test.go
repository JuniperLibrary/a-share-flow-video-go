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

// 销售收现大于经营流入合计时必须有口径说明,否则模型会当成"数据矛盾"。
func TestAnnotateCashflowScope(t *testing.T) {
	periods := []map[string]any{
		{"销售商品提供劳务收到的现金": 563.93, "经营活动现金流入合计": 561.81},
		{"销售商品提供劳务收到的现金": 984.22, "经营活动现金流入合计": 1092.46},
	}
	annotateCashflowScope(periods)
	if _, ok := periods[0]["口径说明"]; !ok {
		t.Error("销售收现 > 流入合计时应补口径说明")
	}
	if _, ok := periods[1]["口径说明"]; ok {
		t.Error("口径正常时不应加说明")
	}
}

// 应交税费/应付职工薪酬/其他应付款是"经营现金流与净利润的缺口由什么解释"
// 最常见的答案。2026-11-06 的实跑里 Bear 点名要「应交税费」,
// 工具当时拿不出来,只能写"无科目对应",整条反证落空。
func TestBalanceSpecCoversCashGapAccounts(t *testing.T) {
	want := map[string]string{
		"TAX_PAYABLE":          "应交税费",
		"STAFF_SALARY_PAYABLE": "应付职工薪酬",
		"TOTAL_OTHER_PAYABLE":  "其他应付款",
		"PREPAYMENT":           "预付款项",
		"CIP":                  "在建工程",
	}
	got := map[string]string{}
	for _, f := range statementSpecs["balance"].Fields {
		got[f.Key] = f.Label
	}
	for key, label := range want {
		if got[key] != label {
			t.Errorf("balance 缺少科目 %s(%s):核实阶段的常见解释项", label, key)
		}
	}
}

// 新增科目必须和其他科一样做亿元换算与报告期截断。
func TestBalanceResultScalesNewAccounts(t *testing.T) {
	rows := []map[string]any{{
		"REPORT_DATE":          "2026-09-30 00:00:00",
		"REPORT_TYPE":          "三季报",
		"TAX_PAYABLE":          7528578502.94,
		"CIP":                  2826319454.49,
		"TOTAL_OTHER_PAYABLE":  6887840325.35,
		"STAFF_SALARY_PAYABLE": nil,
	}}
	out := buildStatementResult(statementSpecs["balance"], rows)
	if len(out) != 1 {
		t.Fatalf("got %d periods, want 1", len(out))
	}
	got := out[0]
	if v, _ := got["应交税费"].(float64); v != 75.29 {
		t.Errorf("应交税费 = %v 亿, want 75.29", v)
	}
	if v, _ := got["在建工程"].(float64); v != 28.26 {
		t.Errorf("在建工程 = %v 亿, want 28.26", v)
	}
	if v, _ := got["其他应付款"].(float64); v != 68.88 {
		t.Errorf("其他应付款 = %v 亿, want 68.88", v)
	}
	if _, ok := got["应付职工薪酬"]; ok {
		t.Error("null 科目不应出现(nil 与 0 是两回事)")
	}
}
