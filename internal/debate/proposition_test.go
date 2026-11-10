package debate

import (
	"encoding/json"
	"os"
	"testing"
)

func TestExtractPropositionRatio(t *testing.T) {
	cases := []struct {
		name string
		text string
		a, b float64
		ok   bool
	}{
		{
			name: "标准命题",
			text: "命题:CFO/归母净利 = 840.59÷644.67 = 1.30,本期是否 ≥ 1.2。",
			a:    840.59, b: 644.67, ok: true,
		},
		{
			name: "科目名里带斜杠不影响",
			text: "命题:销售收现/营业收入 = 1435.23/1332.84 = 1.077。",
			a:    1435.23, b: 1332.84, ok: true,
		},
		{
			name: "全角除号",
			text: "命题:CFO／净利＝840.59／644.67。",
			a:    840.59, b: 644.67, ok: true,
		},
		{
			name: "没有算式",
			text: "命题:本期现金支撑的部分不低于 90%。",
			ok:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ExtractPropositionRatio(tc.text)
			if ok != tc.ok {
				t.Fatalf("ok=%v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if got.Numerator != tc.a || got.Denominator != tc.b {
				t.Fatalf("取到 %v÷%v, want %v÷%v", got.Numerator, got.Denominator, tc.a, tc.b)
			}
		})
	}
}

func TestAnsweredRatio(t *testing.T) {
	proposition := "命题:CFO/归母净利 = 840.59÷644.67 = 1.30,本期是否 ≥ 1.2。"

	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "原样复算",
			text: "复算:经营现金流 840.59 亿÷归母净利 644.67 亿=1.30,利润被现金覆盖。",
			want: true,
		},
		{
			name: "只写比值也算",
			text: "这条我承认,1.30 倍确实过了阈值。",
			want: true,
		},
		{
			name: "千分位写法",
			text: "840.59÷644.67 我算出来也是 1.30。",
			want: true,
		},
		{
			name: "答非所问",
			text: "销售收现 1435.23 亿÷营收 1332.84 亿=1.077,收入全额现金背书。",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ratio, applicable := AnsweredRatio(proposition, tc.text)
			if !applicable {
				t.Fatal("命题自带算式,应当可判")
			}
			if got != tc.want {
				t.Fatalf("AnsweredRatio=%v, want %v(命题算式 %v÷%v)", got, tc.want, ratio.Numerator, ratio.Denominator)
			}
		})
	}
}

func TestAnsweredRatioSkipsProposalWithoutFormula(t *testing.T) {
	if _, _, applicable := AnsweredRatio("命题:成色如何?", "营收 +12%。"); applicable {
		t.Fatal("命题没有算式时不该判,否则旧产出会无端变红")
	}
}

func TestCheckProposition(t *testing.T) {
	turns := []Turn{
		{Index: 0, Phase: PhaseOpen, Speaker: Moderator, Text: "命题:销售收现/营收 = 1435.23÷1332.84 = 1.077,是否 ≥ 1.0。"},
		{Index: 1, Phase: PhaseOpening, Speaker: Bull, Text: "销售收现 1435.23 亿÷营收 1332.84 亿=1.077,收入有现金背书。"},
	}
	st := CheckProposition(turns)
	if !st.HasFormula || !st.Answered {
		t.Fatalf("命题应自带算式且被复算,got %+v", st)
	}

	turns[1].Text = "存货 613.91 亿,计提 2% 跌价=12.28 亿。"
	if st := CheckProposition(turns); st.Answered {
		t.Fatalf("答非所问不该算复算,got %+v", st)
	}
}

func TestNumbersInHandlesThousands(t *testing.T) {
	got := numbersIn("营收 1,332.84 亿,净利 644.67 亿,缺口 -62.81 亿。")
	want := []float64{1332.84, 644.67, 62.81}
	if len(got) != len(want) {
		t.Fatalf("取到 %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("取到 %v, want %v", got, want)
		}
	}
}

// 真实产出回归:2026-11-10 的深度场次是命题改造后第一次跑,
// 命题自带算式、Bull 原样复算、Bear 也复算了同一条。
// 改造前的 quick 场次(2026-11-07)命题只有"不低于 1.0 倍",没有算法,
// Bull 于是答了收现比——这里两种情况都要能被判对。
func TestCheckPropositionOnRealTurns(t *testing.T) {
	raw, err := os.ReadFile("testdata/proposition_turns.json")
	if err != nil {
		t.Fatalf("读取真实产出失败: %v", err)
	}
	var corpus map[string][]Turn
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("解析真实产出失败: %v", err)
	}

	deep := CheckProposition(corpus["deep_2026-11-10"])
	if !deep.HasFormula {
		t.Fatalf("改造后的命题应带算式,got %+v", deep)
	}
	if !deep.Answered {
		t.Fatalf("Bull 第一句就是原样复算,不该判成答非所问,got %+v", deep)
	}

	quick := CheckProposition(corpus["quick_2026-11-07"])
	if quick.HasFormula {
		t.Fatalf("改造前的命题没有算式,不该判成有,got %+v", quick)
	}
	if quick.Answered {
		t.Fatalf("没有算式时不存在复算,got %+v", quick)
	}
}
