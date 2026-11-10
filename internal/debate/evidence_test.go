package debate

import (
	"strings"
	"testing"
)

// 实跑里漏算式的三种典型写法,和一份合格样本。
func TestCheckEvidence(t *testing.T) {
	cases := []struct {
		name    string
		speaker Speaker
		text    string
		want    evidenceGap
	}{
		{
			name:    "合格:算式+证伪",
			speaker: Bull,
			text:    "应收 3.65 亿÷营收 1332.84 亿=0.27%,回款没问题。若下季应收翻倍,即改判。",
			want:    evidenceGap{},
		},
		{
			name:    "只并排数字无算式",
			speaker: Bear,
			text:    "存货 613.91 亿,净利 644.67 亿,跌价 10% 就是 61.4 亿。若毛利下滑,减值会放大。",
			want:    evidenceGap{NeedFormula: true},
		},
		{
			name:    "有算式但没有证伪条件",
			speaker: Sector,
			text:    "营收 1332.84 亿、+1.8%,归母 644.67 亿、-0.25%,增速差 1.8-(-0.25)=2.05 个点,增收不增利。",
			want:    evidenceGap{NeedFormula: false, NeedFalsifier: true},
		},
		{
			name:    "两项都缺",
			speaker: Risk,
			text:    "存货是最大减值敞口,占净资产 21.9%。",
			want:    evidenceGap{NeedFormula: true, NeedFalsifier: true},
		},
		{
			name:    "主持人只点名复算,不受约束",
			speaker: Moderator,
			text:    "核实利润现金桥缺口,只查出处。",
			want:    evidenceGap{},
		},
		{
			name:    "裁决者四段式自带检验卡,不要求三句式",
			speaker: Synthesizer,
			text:    "持有。最强证据是收现比 1.077。",
			want:    evidenceGap{FourSection: true},
		},
		{
			name:    "裁决者复述的算式算错仍要退回",
			speaker: Synthesizer,
			text:    "结论:持有。最强证据:经营现金流840.59÷净利644.67=0.30。",
			want:    evidenceGap{FourSection: true, BadMath: []arithIssue{{}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkEvidence(c.speaker, c.text)
			if got.NeedFormula != c.want.NeedFormula || got.NeedFalsifier != c.want.NeedFalsifier ||
				len(got.BadMath) != len(c.want.BadMath) || got.FourSection != c.want.FourSection {
				t.Errorf("checkEvidence() = %+v, want %+v", got, c.want)
			}
		})
	}
}

// 返工要求必须禁止引入新数字,否则模型会编算式来应付校验。
func TestBuildEvidenceRetryPrompt(t *testing.T) {
	got := buildEvidenceRetryPrompt("BASE", "上一轮的正文", evidenceGap{NeedFormula: true, NeedFalsifier: true})
	for _, want := range []string{"BASE", "上一轮的正文", "显式算式", "若", "不得引入上一轮没有的新数字"} {
		if !strings.Contains(got, want) {
			t.Errorf("返工 prompt 缺少 %q\n实际:\n%s", want, got)
		}
	}
}

// 裁决者的返工提示不能要求"三句式",否则会把四段式结论改成三段,
// 渲染层拿不到结论/最强证据/最致命反证/检验卡这四块。
func TestInstructionKeepsStructurePerRole(t *testing.T) {
	three := evidenceGap{NeedFormula: true}.Instruction()
	if !strings.Contains(three, "三句式") {
		t.Errorf("普通轮次返工应保持三句式:\n%s", three)
	}
	four := evidenceGap{BadMath: []arithIssue{{Expr: "1÷2=3", Expected: 0.5, Stated: 3}}, FourSection: true}.Instruction()
	if !strings.Contains(four, "四段式") {
		t.Errorf("裁决者返工应保持四段式:\n%s", four)
	}
	if strings.Contains(four, "三句式") {
		t.Errorf("裁决者返工不应出现三句式:\n%s", four)
	}
	if !strings.Contains(four, "240 字上限") {
		t.Errorf("裁决者返工应给出字数上限:\n%s", four)
	}
}

func TestEvidenceGapEmpty(t *testing.T) {
	if !(evidenceGap{}).Empty() {
		t.Error("空 gap 应为 Empty")
	}
	if (evidenceGap{NeedFormula: true}).Empty() {
		t.Error("缺算式不应为 Empty")
	}
}

// 溯源只在拿到池子时启用:池子不完整就判断"没有出处"必然误判。
func TestCheckEvidenceWithSourcesOnlyTracesVerdict(t *testing.T) {
	verdict := "结论:持有。最强证据:经营现金流840.59亿÷归母净利644.67亿=1.30。" +
		"最致命反证:2025年报实际仅80.07亿,短11.65亿。检验卡:若跌破1.0,作废。"
	sources := []string{"经营现金流840.59亿,归母净利644.67亿"}

	got := checkEvidenceWithSources(Synthesizer, verdict, sources)
	if len(got.Unverified) != 2 {
		t.Fatalf("Unverified = %v, 期望 2 项(80.07、11.65)", got.Unverified)
	}
	if got.Empty() {
		t.Error("有无出处金额时不能判为合格,否则不会触发返工")
	}
	if got := checkEvidenceWithSources(Synthesizer, verdict, nil); len(got.Unverified) != 0 {
		t.Errorf("没有溯源池却做了判断: %v", got.Unverified)
	}
	// 非裁决角色不查:他们的职责就是引入新的科目与工具结果。
	if got := checkEvidenceWithSources(Bull, verdict, sources); len(got.Unverified) != 0 {
		t.Errorf("多方被误查了溯源: %v", got.Unverified)
	}
}

// 返工提示必须点名具体金额,否则模型不知道该删哪个数。
func TestInstructionListsUnverifiedAmounts(t *testing.T) {
	got := evidenceGap{FourSection: true, Unverified: []string{"80.07亿", "11.65亿"}}.Instruction()
	for _, want := range []string{"找不到出处", "80.07亿", "11.65亿", "四段式", "240 字上限"} {
		if !strings.Contains(got, want) {
			t.Errorf("返工提示缺少 %q\n实际:\n%s", want, got)
		}
	}
}
