package debate

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const testReportPath = "testdata/report_600519_2026Q3.txt"

func readTestReport(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(testReportPath)
	if err != nil {
		t.Fatalf("读取财报样本失败: %v", err)
	}
	return string(raw)
}

// 2026-11-06 的真实缺陷:Synthesizer 用「2025 年报实际仅 80.07 亿」定案,
// 而财报原文、transcript、工具结果里都没有这个数。
// 它比算错更危险——算式错至少还有个算式可查,凭空造数连查的地方都没有。
func TestUnverifiedAmountsFlagsFabricatedVerdictAmount(t *testing.T) {
	report := readTestReport(t)
	transcript := "经营现金流840.59亿−净利644.67亿=195.92亿,合同负债20.84亿;期初合同负债需91.72亿"
	verdict := "结论:持有。最强证据:840.59亿÷644.67亿=1.30。最致命反证:期初合同负债需91.72亿," +
		"2025年报实际仅80.07亿,短11.65亿。检验卡:收现比若跌破1.0,本次结论作废。"

	got := unverifiedAmounts(verdict, report, transcript)
	want := []string{"80.07亿", "11.65亿"}
	if len(got) != len(want) {
		t.Fatalf("未溯源金额 = %v, 期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 个未溯源金额 = %q, 期望 %q", i, got[i], want[i])
		}
	}
}

// 检验卡写的是将来要看的阈值,本来就是新数字,不能要求它有出处。
func TestUnverifiedAmountsExemptsCheckCard(t *testing.T) {
	verdict := "结论:持有。最强证据:840.59亿÷644.67亿=1.30。" +
		"检验卡:年报应收票据及应付款项融资若合计跳增超999.99亿,本次结论作废。"
	if got := unverifiedAmounts(verdict, "经营现金流840.59亿,归母净利644.67亿"); len(got) != 0 {
		t.Errorf("检验卡里的阈值被误判为无出处: %v", got)
	}
}

// 正文常把 195.92 写成 196,把 173.27 写成 173:1% 以内的舍入不是编造。
func TestUnverifiedAmountsToleratesRounding(t *testing.T) {
	verdict := "结论:持有。最强证据:经营现金流840.59亿−净利644.67亿=196亿。"
	if got := unverifiedAmounts(verdict, "经营现金流840.59亿,归母净利644.67亿"); len(got) != 0 {
		t.Errorf("四舍五入被误判: %v", got)
	}
}

// 显式算式的右侧结果是算出来的,不该再要求它在原文里出现过;
// 但单边写法「缺口=80.07亿」不算自证,仍要标出来。
func TestUnverifiedAmountsAcceptsDerivedButNotAsserted(t *testing.T) {
	sources := "经营现金流840.59亿,净利644.67亿"
	derived := "结论:持有。840.59−644.67=195.92亿。"
	if got := unverifiedAmounts(derived, sources); len(got) != 0 {
		t.Errorf("算式推导结果被误判: %v", got)
	}
	asserted := "结论:持有。缺口=80.07亿。"
	if got := unverifiedAmounts(asserted, sources); len(got) != 1 {
		t.Errorf("单边断言未被标出: %v", got)
	}
}

// 不带金额单位的比值不算金额,不去追踪:裁决段大量使用 1.30、48.4% 这类换算结果。
func TestUnverifiedAmountsIgnoresRatios(t *testing.T) {
	verdict := "结论:持有。最强证据:840.59÷644.67=1.30,净利率48.4%,占比27%。"
	if got := unverifiedAmounts(verdict, "经营现金流840.59亿,归母净利644.67亿"); len(got) != 0 {
		t.Errorf("比值被误当成金额: %v", got)
	}
}

// 全角减号的金额(-80.07 亿)同样要能解析,否则模型换个符号就绕过检查。
func TestUnverifiedAmountsHandlesUnicodeMinus(t *testing.T) {
	got := unverifiedAmounts("结论:持有。调整后缺口−80.07亿。", "经营现金流840.59亿")
	if len(got) != 1 {
		t.Errorf("全角减号金额未被标出: %v", got)
	}
}

// 没有溯源池时一律不判:池子不完整就检查必然误判,
// 而误判的代价是每轮多烧一次模型调用。
func TestUnverifiedAmountsNeedsSources(t *testing.T) {
	if got := unverifiedAmounts("结论:持有。2025年报实际仅80.07亿。"); len(got) != 0 {
		t.Errorf("没有溯源池却给出了结论: %v", got)
	}
}

// 真实产出回归:deep2 的裁决段只有 80.07 与 11.65 无出处,
// 其余(840.59/644.67/3.65/195.92/91.72)都能在原文或 transcript 里找到。
func TestUnverifiedAmountsRealVerdict(t *testing.T) {
	raw, err := os.ReadFile("testdata/real_turns.json")
	if err != nil {
		t.Fatalf("读取语料失败: %v", err)
	}
	var corpus map[string]json.RawMessage
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("解析语料失败: %v", err)
	}
	var turns []string
	if err := json.Unmarshal(corpus["deep_v3_2026-11-06"], &turns); err != nil {
		t.Fatalf("解析 v3 轮次失败: %v", err)
	}
	if len(turns) < 2 {
		t.Fatalf("v3 轮次太少: %d", len(turns))
	}

	verdict := turns[len(turns)-1]
	verdict = verdict[strings.Index(verdict, ": ")+2:]
	transcript := strings.Join(turns[:len(turns)-1], "\n")

	got := unverifiedAmounts(verdict, readTestReport(t), transcript)
	want := []string{"80.07亿", "11.65亿"}
	if len(got) != len(want) {
		t.Fatalf("真实裁决段未溯源金额 = %v, 期望 %v\n裁决段: %s", got, want, verdict)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 个未溯源金额 = %q, 期望 %q", i, got[i], want[i])
		}
	}
}

// 千分位不是新数字:1332.84 亿与 1,332.84 亿是同一个数,
// 不认逗号会把它误判成 332.84 再当成编造,返工就白烧一次调用。
func TestUnverifiedAmountsHandlesThousandsSeparator(t *testing.T) {
	got := unverifiedAmounts("结论:持有。营收1,332.84亿,净利644.67亿。", "营业收入 1332.84 亿,归母净利润 644.67 亿")
	if len(got) != 0 {
		t.Errorf("千分位被误判为无出处: %v", got)
	}
}
