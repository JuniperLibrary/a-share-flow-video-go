package prompts

import (
	"strings"
	"testing"
)

// 这一组测试守的是"题目"本身:辩论的深度来自硬规则,而不是轮数。
// 任何人把证据纪律、勾稽清单或角色分工改没了,这里会红。
func TestSystemPromptsCarryEvidenceRules(t *testing.T) {
	roles := map[string]struct {
		prompt string
		owns   string
	}{
		"moderator":   {moderatorSystemPrompt, "命题"},
		"bull":        {bullSystemPrompt, "认领 C2/C3/C4"},
		"bear":        {bearSystemPrompt, "认领 C1/C6/C7"},
		"sector":      {sectorSystemPrompt, "认领 C9/C10"},
		"risk":        {riskSystemPrompt, "认领 C5/C8"},
		"synthesizer": {synthesizerSystemPrompt, "检验卡"},
	}

	shared := []string{"四元组", "证伪", "原始科目", "勾稽检查项", "节奏硬规则", "输出格式"}
	for name, tc := range roles {
		for _, want := range shared {
			if !strings.Contains(tc.prompt, want) {
				t.Errorf("%s 缺少共享规则 %q", name, want)
			}
		}
		if !strings.Contains(tc.prompt, tc.owns) {
			t.Errorf("%s 缺少职责标记 %q", name, tc.owns)
		}
	}
}

func TestRolePromptsDoNotOverlap(t *testing.T) {
	// 每个角色只认领自己的检查项,避免六个人打同一张牌。
	ownership := map[string]string{
		bullSystemPrompt:   "C2/C3/C4",
		bearSystemPrompt:   "C1/C6/C7",
		sectorSystemPrompt: "C9/C10",
		riskSystemPrompt:   "C5/C8",
	}
	for prompt, own := range ownership {
		for _, check := range []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7", "C8", "C9", "C10"} {
			if !strings.Contains(own, check) && strings.Contains(prompt, "认领 "+check) {
				t.Errorf("角色 %s 不应认领 %s", own, check)
			}
		}
	}
}

func TestCheckListIsComplete(t *testing.T) {
	for _, id := range []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7", "C8", "C9", "C10"} {
		if !strings.Contains(checkList, id+" ") {
			t.Errorf("勾稽清单缺少 %s", id)
		}
	}
}

func TestCitationFormatIsMandated(t *testing.T) {
	// 引用必须能被人翻到:写清科目、数值、期间;否则"证据"无法复现。
	for _, want := range []string{"原始科目", "期间", "算式"} {
		if !strings.Contains(turnJSONSchema, want) {
			t.Errorf("输出格式缺少 %q 要求", want)
		}
	}
	if !strings.Contains(commonStyleRules, "60-120") {
		t.Error("节奏规则缺少证据轮长度约束")
	}
}

// 实跑暴露的两个成片问题:正文超长(平均 200 字)会被 TTS 拖成十几分钟;
// 正文带 C 编号会被直接念出来。两条都必须是硬规则。
func TestPromptEnforcesRuntimeConstraints(t *testing.T) {
	if !strings.Contains(commonStyleRules, "硬上限") {
		t.Error("节奏规则缺少字数硬上限")
	}
	if !strings.Contains(commonStyleRules, "正文禁止出现 C1-C10 编号") {
		t.Error("节奏规则缺少编号禁令")
	}
	if !strings.Contains(commonStyleRules, "citations 不计入字数") {
		t.Error("缺少 citations 不计字数,模型会把复算全塞正文")
	}
}
