package debate

import "testing"

// 行情接口对亏损/停牌返回 0,过去会被原样写进结果,模型据此说"PE 为 0 很便宜"。
// 现在 0 一律进"未取到",并且市值统一换算成亿元。
func TestBuildMetricResultDropsMissing(t *testing.T) {
	// 数值取自 2026-11-06 对 1.600519 的实测响应。
	data := map[string]any{
		"f57":  "600519",
		"f58":  "贵州茅台",
		"f43":  145300.0,        // 最新价,分
		"f60":  145200.0,        // 昨收,分
		"f116": 1816368566253.0, // 总市值,元
		"f162": 2113.0,          // PE-TTM ×100
		"f167": 670.0,           // PB ×100
		// f117(流通市值)/f168(换手率)/f170(涨跌幅) 缺失
	}

	got := buildMetricResult(data, "pe")

	if got["code"] != "600519" || got["name"] != "贵州茅台" {
		t.Errorf("基础字段丢失: %v", got)
	}
	if v, _ := got["price"].(float64); v != 1453 {
		t.Errorf("price = %v, 期望 1453(分转元)", v)
	}
	if v, _ := got["pe_ttm"].(float64); v != 21.13 {
		t.Errorf("pe_ttm = %v, 期望 21.13(接口是 ×100 定点数)", v)
	}
	if v, _ := got["pb"].(float64); v != 6.7 {
		t.Errorf("pb = %v, 期望 6.7", v)
	}
	if v, _ := got["mkt_cap_亿"].(float64); v != 18163.69 {
		t.Errorf("mkt_cap_亿 = %v, 期望 18163.69(元转亿元)", v)
	}
	missing, ok := got["未取到"].([]string)
	if !ok || len(missing) == 0 {
		t.Fatalf("缺失字段应进\"未取到\",实际 %v", got["未取到"])
	}
	for _, m := range missing {
		if m == "pe_ttm" || m == "pb" || m == "price" || m == "mkt_cap_亿" {
			t.Errorf("%s 有值却被判为未取到", m)
		}
	}
	if got["requested_metric"] != "pe" {
		t.Errorf("requested_metric = %v", got["requested_metric"])
	}
	if got["as_of"] == nil || got["口径"] == nil {
		t.Error("必须带 as_of 与口径:估值是实时快照,与报告期无关")
	}
}
