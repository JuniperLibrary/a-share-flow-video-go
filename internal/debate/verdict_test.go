package debate

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// 2026-11-10 的两场真实验收:deep 的裁决段把反证写成"若该提未提,年报利润
// 虚增约 3%"(有后果),quick 的那段则是"扣非 99.78%、无粉饰"(其实是支持
// 结论的证据,且没有后果)。后者就是这条检查要抓的。
func TestCheckVerdictShapeOnRealVerdicts(t *testing.T) {
	raw, err := os.ReadFile("testdata/verdict_samples.json")
	if err != nil {
		t.Fatalf("读取裁决样本失败: %v", err)
	}
	var samples map[string]string
	if err := json.Unmarshal(raw, &samples); err != nil {
		t.Fatalf("解析裁决样本失败: %v", err)
	}

	if issues := CheckVerdictShape(samples["deep_2026-11-10"]); len(issues) != 0 {
		t.Errorf("合格的裁决段被误判: %v", issues)
	}
	issues := CheckVerdictShape(samples["quick_2026-11-10"])
	if len(issues) == 0 {
		t.Fatal("反证段只有事实、没有后果,应当被标记")
	}
	joined := strings.Join(issues, ";")
	if !strings.Contains(joined, "后果") {
		t.Errorf("提示应点名缺后果,got %v", issues)
	}
}

func TestCheckVerdictShapeCatchesMissingSegment(t *testing.T) {
	issues := CheckVerdictShape("结论:持有,因为现金流覆盖利润。最强证据:840.59÷644.67=1.30。")
	found := false
	for _, s := range issues {
		if strings.Contains(s, "最致命反证") {
			found = true
		}
	}
	if !found {
		t.Fatalf("缺第 3 段必须报出来,got %v", issues)
	}
}

func TestCheckVerdictShapeAcceptsAlternativeCues(t *testing.T) {
	for _, cue := range []string{"若", "一旦", "意味着", "否则", "如果"} {
		v := "结论:持有,因为现金流覆盖利润。最强证据:840.59÷644.67=1.30。最致命反证:Risk 的存货跌价" +
			cue + "计提不足,利润虚增 18 亿。检验:年报计提率若低于 3%,结论作废。"
		if issues := CheckVerdictShape(v); len(issues) != 0 {
			t.Errorf("含 %q 的后果句被误判: %v", cue, issues)
		}
	}
}
