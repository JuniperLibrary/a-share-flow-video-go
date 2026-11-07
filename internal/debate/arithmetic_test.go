package debate

import (
	"encoding/json"
	"os"
	"testing"
)

// 验算器的成本不对称:漏判只是少兜一层底,误判会让每轮多烧一次模型调用
// (实测单轮 27s),深度档 17 轮全误判就是多跑七八分钟。
// 所以用两场真实产出的全部正文做回归:任何一条正确算式被退回都算失败。
func TestCheckArithmeticNoFalsePositiveOnRealCorpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/real_turns.json")
	if err != nil {
		t.Fatalf("读取真实产出样本失败: %v", err)
	}
	var corpus map[string]json.RawMessage
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("解析样本失败: %v", err)
	}
	total, withFormula := 0, 0
	for name, payload := range corpus {
		if name == "说明" {
			continue
		}
		var turns []string
		if err := json.Unmarshal(payload, &turns); err != nil {
			t.Fatalf("%s 解析失败: %v", name, err)
		}
		for i, text := range turns {
			total++
			if !evidenceFormulaRe.MatchString(text) {
				continue
			}
			withFormula++
			if issues := checkArithmetic(text); len(issues) != 0 {
				t.Errorf("%s 第 %d 轮的正确算式被误判:\n  文本: %s\n  问题: %v", name, i, text, issues)
			}
		}
	}
	if total == 0 || withFormula == 0 {
		t.Fatalf("样本为空或没有算式,回归等于没跑: total=%d withFormula=%d", total, withFormula)
	}
	t.Logf("回归样本 %d 轮,其中 %d 轮含算式", total, withFormula)
}

// 全部取自 2026-11-06 深度档真实产出(茅台 2026Q3),用于防止误判:
// 这些算式都是对的,验算器不能把它们退回重写,否则每轮都要多烧一次模型调用。
func TestCheckArithmeticAcceptsRealOutput(t *testing.T) {
	texts := []string{
		"销售收现1435.23亿÷营收1332.84亿=1.077,应收仅3.65亿",
		"合同负债20.84亿÷营收1332.84亿=1.6%,预收近乎归零",
		"隐含上年营收1332.84÷1.018=1309.3亿",
		"613.91×10%=61.39亿,61.39÷644.67=9.5%,一致",
		"收现1435.23亿÷含税营收1506.1亿=0.953",
		"含税营收1332.84×1.13=1506.1亿,缺口1506.1-1435.23=70.9亿",
		"840.59-70.9=769.69亿,769.69÷644.67=1.19",
		"644.67-643.25=1.42亿,非经常占0.2%",
		"税费595.17亿÷经营流出648.05亿=91.8%",
		"非税流出648.05-595.17=52.88亿",
		"费用101.54-非税流出52.88=48.66亿",
		// 全角/变体运算符:真实产出大量使用 − (U+2212) 而不是 ASCII 减号。
		"经营现金流840.59亿−净利644.67亿=195.92亿",
		"1506.11−1435.23=70.88亿",
		"税费实付595.17−所得税费用244.7=多付350.5亿",
		// 加减链:同一优先级的多次运算按书写顺序算。
		"Q3折旧=222.21+(14.50−8.32)−218.19=10.20亿",
		"96-32-24=40",
		// 乘除链。
		"1000÷10÷5=20",
		// 括号里带等号的写法。
		"收现1435.23÷(营收1332.84×1.13=1506.11)=0.953",
		// 括号:先归约括号内,再验外层。
		"复算(20.84+3.65)÷195.92=12.5%一致",
		"1435.23÷(1332.84×1.13)=1435.23÷1506.11=0.953",
		"(20.84+3.65)÷195.92=12.5%",
	}
	for _, text := range texts {
		if issues := checkArithmetic(text); len(issues) != 0 {
			t.Errorf("正确算式被判错:\n  文本: %s\n  问题: %v", text, issues)
		}
	}
}

func TestCheckArithmeticCatchesErrors(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"除法方向反了", "销售收现1435.23÷营收1332.84=0.93"},  // 实际 1.077
		{"百分数少乘 100", "应收3.65÷营收1332.84=0.0027%"}, // 实际 0.27%
		{"减法顺序颠倒", "经营现金流644.67-840.59=195.92亿"},  // 实际 -195.92
		{"乘法结果错", "存货613.91×10%=6.14亿"},           // 实际 61.39
		{"两位数除法写反", "769.69÷644.67=0.84"},         // 实际 1.19
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			issues := checkArithmetic(c.text)
			if len(issues) == 0 {
				t.Fatalf("错误算式没被抓到: %s", c.text)
			}
			t.Logf("%s -> %s", c.text, issues[0].String())
		})
	}
}

// 混合优先级(a+b×c)需要真正的表达式解析,我们必须放弃而不是猜:
// 按书写顺序硬算会把对的说成错的。
func TestCheckArithmeticSkipsMixedPrecedence(t *testing.T) {
	// 按正确优先级 2+3×4=14;按左到右是 20。两种都不能判错。
	if issues := checkArithmetic("2+3×4=14"); len(issues) != 0 {
		t.Errorf("混合优先级不应被验算(会误判): %v", issues)
	}
	if issues := checkArithmetic("2+3×4=999"); len(issues) > 0 {
		t.Logf("按左到右误判了: %v", issues)
		t.Error("混合优先级必须整体跳过,不能截取后两段硬算")
	}
}

// 无算式、近似号、只有数字的文本不应触发验算。
func TestCheckArithmeticIgnoresNonEquations(t *testing.T) {
	for _, text := range []string{
		"存货 613.91 亿,净利 644.67 亿,压测压力大。",
		"成本÷10.2%≈9.8倍",
		"2026 年前三季度营收增长 1.8%",
		"CFO/NI=1.30 达标",
		"计提跌价5%=30.7亿",
		"毛利改善 1.2—1.5 个点",
	} {
		if issues := checkArithmetic(text); len(issues) != 0 {
			t.Errorf("不该验算的文本产生了问题: %s -> %v", text, issues)
		}
	}
}
