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
			name:    "裁决者四段式自带检验卡,不受约束",
			speaker: Synthesizer,
			text:    "持有。最强证据是收现比 1.077。",
			want:    evidenceGap{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := checkEvidence(c.speaker, c.text); got != c.want {
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

func TestEvidenceGapEmpty(t *testing.T) {
	if !(evidenceGap{}).Empty() {
		t.Error("空 gap 应为 Empty")
	}
	if (evidenceGap{NeedFormula: true}).Empty() {
		t.Error("缺算式不应为 Empty")
	}
}
