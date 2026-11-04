package copy

import (
	"strings"
	"testing"

	"github.com/a-share-flow-video-go/internal/fetcher"
)

func TestBuildStanceLine_TechInflowDay(t *testing.T) {
	inflows := []sectorFlow{
		{Name: "国产芯片", Net: 253, SuperNet: 238},
		{Name: "半导体", Net: 163, SuperNet: 140},
		{Name: "AI应用", Net: 97, SuperNet: 77},
	}
	outflows := []sectorFlow{
		{Name: "银行", Net: -120},
		{Name: "白酒", Net: -88},
	}
	got := buildStanceLine(inflows, outflows)
	if !strings.Contains(got, "科技主线再度确认") {
		t.Fatalf("科技吸金日应写主线确认, got: %s", got)
	}
	if !strings.Contains(got, "国产芯片") {
		t.Fatalf("应点出龙头板块, got: %s", got)
	}
}

func TestBuildStanceLine_TechOutflowDay(t *testing.T) {
	inflows := []sectorFlow{
		{Name: "银行", Net: 120},
		{Name: "白酒", Net: 88},
	}
	outflows := []sectorFlow{
		{Name: "半导体", Net: -163},
		{Name: "国产芯片", Net: -120},
		{Name: "创新药", Net: -41},
	}
	got := buildStanceLine(inflows, outflows)
	if !strings.Contains(got, "回调不改主线") {
		t.Fatalf("科技回调日应写回调不改主线, got: %s", got)
	}
}

// 回归：科技同时出现在流入榜与流出榜，且流出强度 ≥ 流入强度时，
// 必须写「内部分歧/承压」，禁止误判为「科技主线确认」。
func TestBuildStanceLine_DivergenceDay_NoFalseConfirmation(t *testing.T) {
	inflows := []sectorFlow{
		{Name: "机器人", Net: 7},
		{Name: "银行", Net: 6},
		{Name: "电网设备", Net: 1},
	}
	outflows := []sectorFlow{
		{Name: "AI应用", Net: -59},
		{Name: "通信设备", Net: -125},
		{Name: "人工智能", Net: -179},
	}
	got := buildStanceLine(inflows, outflows)
	if strings.Contains(got, "科技主线再度确认") {
		t.Fatalf("分歧日禁止误报主线确认, got: %s", got)
	}
	if !strings.Contains(got, "内部分歧") || !strings.Contains(got, "回调不改主线") {
		t.Fatalf("分歧日应写内部分歧+回调不改主线, got: %s", got)
	}
}

func TestBuildStanceLine_NonTechRotationDay(t *testing.T) {
	inflows := []sectorFlow{
		{Name: "白酒", Net: 88},
		{Name: "银行", Net: 120},
	}
	outflows := []sectorFlow{
		{Name: "半导体", Net: -63},
		{Name: "创新药", Net: -41},
	}
	got := buildStanceLine(inflows, outflows)
	if !strings.Contains(got, "轮动/避险") {
		t.Fatalf("非科技吸金日应写轮动/避险, got: %s", got)
	}
}

func TestBuildStanceLine_NoTechNoStance(t *testing.T) {
	inflows := []sectorFlow{{Name: "银行", Net: 120}}
	outflows := []sectorFlow{{Name: "白酒", Net: -88}}
	if got := buildStanceLine(inflows, outflows); got != "" {
		t.Fatalf("无科技数据不应输出立场线, got: %s", got)
	}
}

func TestBuildStanceFollowup_TechInflow(t *testing.T) {
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253}, {Name: "银行", Net: 6}}
	outflows := []sectorFlow{{Name: "AI应用", Net: -59}}
	got := buildStanceFollowup(inflows, outflows)
	if !strings.Contains(got, "科技主线(国产芯片)能否继续承接") {
		t.Fatalf("收尾跟进应指向科技承接, got: %s", got)
	}
}

func TestBuildStanceFollowup_TechOutflow(t *testing.T) {
	inflows := []sectorFlow{{Name: "银行", Net: 120}}
	outflows := []sectorFlow{{Name: "半导体", Net: -163}, {Name: "国产芯片", Net: -120}}
	got := buildStanceFollowup(inflows, outflows)
	if !strings.Contains(got, "科技主线(半导体)的情绪修复机会") {
		t.Fatalf("收尾跟进应指向科技修复, got: %s", got)
	}
}

// deriveAngles 与 buildStanceLine 同优先级：分歧日不写「主线确认」。
func TestDeriveAngles_DivergenceDay_NoFalseConfirmation(t *testing.T) {
	inflows := []sectorFlow{
		{Name: "机器人", Net: 7},
		{Name: "银行", Net: 6},
		{Name: "电网设备", Net: 1},
	}
	outflows := []sectorFlow{
		{Name: "AI应用", Net: -59},
		{Name: "通信设备", Net: -125},
		{Name: "人工智能", Net: -179},
	}
	angles := deriveAngles(inflows, outflows, -349, 0)
	for _, a := range angles {
		if strings.Contains(a, "科技主线确认") {
			t.Fatalf("分歧日角度禁止写主线确认: %v", angles)
		}
	}
	found := false
	for _, a := range angles {
		if strings.Contains(a, "内部分歧") || strings.Contains(a, "回调不改主线") {
			found = true
		}
	}
	if !found {
		t.Fatalf("分歧日缺少内部分歧角度: %v", angles)
	}
}

func TestValidateCopy_RejectsAntiStance(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，当日最反常的一件事，资金在砸国产芯片。
[悬念] 很多人没看懂，为什么科技这么弱。
[反转] 你再往深看一层，其实科技完了，资金全跑了。
[答案] 说白了，当日真正的主线就一个：银行避险。
[收尾] 8月10日盯银行能不能延续，别碰科技，点个赞，8月10日见。`
	issues := ValidateCopy(text, brief, inflows, nil)
	found := false
	for _, it := range issues {
		if strings.Contains(it, "立场矛盾") {
			found = true
		}
	}
	if !found {
		t.Fatalf("反立场表达未被拦截: %v", issues)
	}
}

func TestValidateCopy_RejectsNeutralAnswer(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，当日最反常的一件事，资金只认国产芯片。
[悬念] 很多人没看懂，为什么指数红着票却没动。
[反转] 你再往深看一层，半导体内部在分歧。
[答案] 8月7日国产芯片净流入253亿，半导体净流入163亿。
[收尾] 8月10日盯国产芯片承接，再看半导体的分歧能不能收敛，点个赞，8月10日见。`
	issues := ValidateCopy(text, brief, inflows, nil)
	found := false
	for _, it := range issues {
		if strings.Contains(it, "立场锚点") {
			found = true
		}
	}
	if !found {
		t.Fatalf("中立[答案]未被拦截: %v", issues)
	}
}

func TestValidateCopy_PassesStanceCopy(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，当日最反常的一件事，资金直接顶了国产芯片高开。
[悬念] 很多人没看懂，为什么指数红的，手里的票却没动。
[反转] 你再往深看一层，其实是半导体内部在高低切换。
[答案] 说白了，当日真正的主线就一个：国产芯片的承接。
[收尾] 当日资金就是国产芯片和元件做高低切，AI 应用还在失血别乱接。8月10日先盯国产芯片能不能继续承接，再看 AI 应用的流出能不能收敛。点个赞，8月10日见。`
	issues := ValidateCopy(text, brief, inflows, nil)
	if len(issues) > 0 {
		t.Fatalf("合规立场文案被误报: %v", issues)
	}
}

// 模板文案端到端：科技吸金日应输出立场线（真实 Sector 数据路径）。
func TestGenerateCopywriting_Stance_TechInflowDay(t *testing.T) {
	sectors := []fetcher.Sector{
		{Name: "国产芯片", Net: 253, SuperNet: 238, ChangePct: 2.5},
		{Name: "半导体", Net: 163, SuperNet: 140, ChangePct: 1.8},
		{Name: "AI应用", Net: 97, SuperNet: 77, ChangePct: 3.1},
		{Name: "银行", Net: -120, SuperNet: -110, ChangePct: -0.5},
		{Name: "白酒", Net: -88, SuperNet: -76, ChangePct: -0.9},
	}
	text := GenerateCopywriting(sectors, "2026-08-07", "full")
	if !strings.Contains(text, "科技主线再度确认") {
		t.Fatalf("模板文案缺少科技立场:\n%s", text)
	}
	if strings.Contains(text, "科技板块(") {
		t.Fatalf("模板文案不应再出现误标的「科技板块(非科技板块)」:\n%s", text)
	}
}