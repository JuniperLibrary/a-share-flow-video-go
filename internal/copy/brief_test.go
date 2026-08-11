package copy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/tts"
)

var _ = config.GetProjectRoot
var _ = fmt.Sprintf
var _ = tts.ParseCopywriting

func ttsParseCopywritingCopy(text string) []string {
	return tts.ParseCopywriting(text)
}

func TestBuildNarrativeBrief_IncludesStructure(t *testing.T) {
	sectors := []fetcher.Sector{
		{Name: "国产芯片", Net: 253, SuperNet: 238, BigNet: 15, ChangePct: 2.5},
		{Name: "半导体", Net: 163, SuperNet: 140, BigNet: 23, ChangePct: 1.8},
		{Name: "银行", Net: -120, SuperNet: -110, BigNet: -10, ChangePct: -0.5},
	}
	brief, err := BuildNarrativeBrief(sectors, "2026-06-15", "full", "")
	if err != nil {
		t.Fatal(err)
	}
	formatted := brief.Format()
	if !strings.Contains(formatted, "国产芯片") || !strings.Contains(formatted, "超大单+238") {
		t.Fatalf("brief missing sector structure:\n%s", formatted)
	}
	if !strings.Contains(formatted, "可用叙事角度") {
		t.Fatalf("brief missing angles:\n%s", formatted)
	}
}

func TestTruncateRunes_BytesVsRunes(t *testing.T) {
	// 14 个汉字 = 42 UTF-8 bytes，但仅 14 runes；旧逻辑 len>40 会 panic
	title := strings.Repeat("测", 14)
	got := truncateRunes(title, 40)
	if got != title {
		t.Fatalf("expected no truncate for 14 runes, got %q", got)
	}
	long := strings.Repeat("测", 50)
	got = truncateRunes(long, 40)
	if len([]rune(got)) != 41 { // 40 + ellipsis rune
		t.Fatalf("expected 41 runes with ellipsis, got %d: %q", len([]rune(got)), got)
	}
}

func TestLoadNewsContext_NoPanicOnLongByteTitle(t *testing.T) {
	s := strings.Repeat("新", 20)
	_ = truncateRunes(s, 40)
}

func TestValidateCopy_RejectsBannedPhrase(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，今天最反常的一件事是资金只认国产芯片。
[悬念] 很多人没看懂，为什么指数红了手里的票却没动。
[反转] 你再往深看，全是游资在里面折腾。
[答案] 说白了，今天真正的主线就一个：科技主线。
[收尾] 明天盯着承接能不能延续，不行我再提醒你，明天开盘见。`
	issues := ValidateCopy(text, brief, inflows)
	if len(issues) == 0 {
		t.Fatal("expected validation issues")
	}
}

func TestNormalizeTaggedCopy_KeepsFiveSceneOrder(t *testing.T) {
	raw := `
[悬念] 很多人没看懂，为什么今天会突然切换。
[钩子] 各位好，早盘最强的是国产芯片，资金居然直接顶高开。
[反转] 更关键的来了，午后资金开始内部分歧。
[答案] 说白了，真正的定价核心还是国产芯片承接。
[收尾] 明天就看国产芯片承接能不能延续，不行我再提醒你，明天开盘见。`
	got := normalizeTaggedCopy(raw)
	want := `[钩子] 各位好，早盘最强的是国产芯片，资金居然直接顶高开。
[悬念] 很多人没看懂，为什么今天会突然切换。
[反转] 更关键的来了，午后资金开始内部分歧。
[答案] 说白了，真正的定价核心还是国产芯片承接。
[收尾] 明天就看国产芯片承接能不能延续，不行我再提醒你，明天开盘见。`
	if got != want {
		t.Fatalf("unexpected normalize result:\n%s", got)
	}
}

func TestValidateCopy_RejectsMissingGreeting(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 今天最反常的一件事是资金盯着国产芯片。
[悬念] 很多人没看懂，为什么指数红的手里票没动。
[反转] 你再往深看一层，其实是半导体内部在分歧。
[答案] 说白了，今天真正的主线就一个：国产芯片承接。
[收尾] 明天盯着承接行不行，不行我再提醒你，明天开盘见。`
	issues := ValidateCopy(text, brief, inflows)
	found := false
	for _, it := range issues {
		if strings.Contains(it, "[钩子] 缺少博主开场招呼词") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing greeting issue, got %v", issues)
	}
}

func TestValidateCopy_RejectsMissingTransitions(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，今天最反常的一件事，资金居然只盯着国产芯片。
[悬念] 指数红的手里票没动。
[反转] 半导体内部在分歧，大资金在高低切。
[答案] 今天真正的主线是国产芯片承接。
[收尾] 明天盯着承接行不行，不行我再提醒你，明天开盘见。`
	issues := ValidateCopy(text, brief, inflows)
	hits := 0
	for _, it := range issues {
		if strings.Contains(it, "缺少人话过渡词") {
			hits++
		}
	}
	if hits < 2 {
		t.Fatalf("expected at least 2 missing transition issues, got %d in %v", hits, issues)
	}
}

func TestValidateCopy_RejectsMissingClosing(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，今天最反常的一件事，资金居然只盯着国产芯片。
[悬念] 很多人没看懂，为什么指数红的手里票没动。
[反转] 你再往深看一层，其实是半导体内部在分歧。
[答案] 说白了，今天真正的主线就一个：国产芯片承接。
[收尾] 明天看承接。`
	issues := ValidateCopy(text, brief, inflows)
	found := false
	for _, it := range issues {
		if strings.Contains(it, "[收尾] 缺少收束互动锚点") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing closing issue, got %v", issues)
	}
}

func TestValidateCopy_PassesFullBloggerTone(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，今天最反常的一件事，资金直接顶了国产芯片高开。
[悬念] 很多人没看懂，为什么指数红的手里票却没动。
[反转] 你再往深看一层，其实是半导体内部在高低切换。
[答案] 说白了，今天真正的主线就一个：国产芯片承接。
[收尾] 明天盯着承接能不能延续，不行我再提醒你，明天开盘见。`
	issues := ValidateCopy(text, brief, inflows)
	var toneIssues []string
	for _, it := range issues {
		if strings.Contains(it, "招呼词") || strings.Contains(it, "过渡词") || strings.Contains(it, "收束") {
			toneIssues = append(toneIssues, it)
		}
	}
	if len(toneIssues) > 0 {
		t.Fatalf("unexpected blogger-tone issues: %v", toneIssues)
	}
}

func TestBuildBloggerToneCopy_FromBrief(t *testing.T) {
	sectors := []fetcher.Sector{
		{Name: "国产芯片", Net: 253, SuperNet: 238, BigNet: 15, ChangePct: 2.5},
		{Name: "半导体", Net: 163, SuperNet: 140, BigNet: 23, ChangePct: 1.8},
		{Name: "AI应用", Net: 97, SuperNet: 77, BigNet: 20, ChangePct: 3.1},
		{Name: "CPO概念", Net: 54, SuperNet: 41, BigNet: 13, ChangePct: 1.2},
		{Name: "银行", Net: -120, SuperNet: -110, BigNet: -10, ChangePct: -0.5},
		{Name: "白酒", Net: -88, SuperNet: -76, BigNet: -12, ChangePct: -0.9},
		{Name: "创新药", Net: -41, SuperNet: -30, BigNet: -11, ChangePct: -1.3},
	}
	brief, err := BuildNarrativeBrief(sectors, "2026-08-07", "full", "")
	if err != nil {
		t.Fatalf("BuildNarrativeBrief failed: %v", err)
	}
	briefFormatted := brief.Format()
	if !strings.Contains(briefFormatted, "国产芯片") || !strings.Contains(briefFormatted, "银行") {
		t.Fatalf("brief missing key sectors:\n%s", briefFormatted)
	}

	inflows, _, _, _, _ := splitSectorFlows(sectors)

	hook := "各位好，今天最反常的一件事，资金只盯着国产芯片承接，其他方向基本没动。"
	suspense := "很多人没看懂，为什么指数红着，手里一半的票却还在水下。"
	twist := "更关键的来了，半导体内部已经出现分歧，大资金正在从高位往低位切。"
	answer := "说白了，今天真正的定价核心就一个：国产芯片的承接强度。"
	closing := "今天资金就是国产芯片和半导体做高低切，AI应用还在失血别乱接。先盯国产芯片能不能继续承接，再看 AI 应用流出能否收敛。别乱追，守住仓位，点个赞，明天开盘见。"

	candidate := fmt.Sprintf("[钩子] %s\n[悬念] %s\n[反转] %s\n[答案] %s\n[收尾] %s",
		hook, suspense, twist, answer, closing)

	candidate = normalizeHallucinatedSectorNames(candidate, sectors, inflows)
	candidate = normalizeTaggedCopy(candidate)

	issues := ValidateCopy(candidate, brief, inflows)
	if len(issues) > 0 {
		t.Fatalf("unexpected validation issues on blogger-tone copy:\n%s\n\nissues=%v", candidate, issues)
	}

	scenes := extractSceneContents(candidate)
	if !containsAny(scenes["钩子"], hookGreetingPhrases) {
		t.Fatalf("[钩子] missing greeting, got: %q", scenes["钩子"])
	}
	for _, k := range []string{"悬念", "反转", "答案"} {
		if !containsAny(scenes[k], transitionPhrases) {
			t.Fatalf("[%s] missing transition word, got: %q", k, scenes[k])
		}
	}
	if !containsAny(scenes["收尾"], closingInteractionPhrases) {
		t.Fatalf("[收尾] missing closing interaction, got: %q", scenes["收尾"])
	}
	for _, name := range []string{"国产芯片", "半导体"} {
		if !strings.Contains(candidate, name) {
			t.Fatalf("candidate missing sector %q:\n%s", name, candidate)
		}
	}
	t.Logf("Materialized blogger-tone copy (5 segments, validated OK):\n%s", candidate)
}

func TestValidateCopy_RejectsStructureWordsInBody(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，今天的钩子就是国产芯片承接。
[悬念] 很多人没看懂，悬念来了为什么指数红的。
[反转] 更关键的来了，反转发生在半导体内部。
[答案] 说白了，答案就是国产芯片和元件的高低切换。
[收尾] 明天盯着承接行不行，收尾就一句话，明天开盘见。`
	issues := ValidateCopy(text, brief, inflows)
	hits := 0
	for _, it := range issues {
		if strings.Contains(it, "结构锚点词") {
			hits++
		}
	}
	if hits < 5 {
		t.Fatalf("expected at least 5 structure-word issues, got %d in %v", hits, issues)
	}
}

func TestValidateCopy_RejectsBracketsInBody(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，今天最反常的 [注：AI 生成备注] 一件事，资金盯着国产芯片。
[悬念] 很多人没看懂，为什么指数红的手里票却没动（[强调] 这点很重要）。
[反转] 你再往深看一层，其实是半导体内部在 [高切低] 切换。
[答案] 说白了，今天真正的主线就一个：[国产芯片] 承接。
[收尾] 明天盯着承接行不行，[关注我] 明天开盘见。`
	issues := ValidateCopy(text, brief, inflows)
	hits := 0
	for _, it := range issues {
		if strings.Contains(it, "残留方括号") {
			hits++
		}
	}
	if hits < 4 {
		t.Fatalf("expected at least 4 bracket-leak issues, got %d in %v", hits, issues)
	}
}

func TestTTSParseCopywriting_CleansStructureWordsAndTags(t *testing.T) {
	input := `
[钩子] 各位好，今天的钩子就是 [内部标签] 国产芯片承接的一百零四亿。
[悬念] 很多人没看懂，悬念来了为什么指数红的。
[反转] 更关键的来了，反转发生在半导体内部分歧。
[答案] 说白了，答案就是国产芯片和元件对 AI 应用的高低切换。
[收尾] 明天盯着承接行不行，收尾就一句话，[评论区] 聊聊，明天开盘见。`
	scenes := ttsParseCopywritingCopy(input)
	if len(scenes) != 5 {
		t.Fatalf("expected 5 scenes, got %d: %v", len(scenes), scenes)
	}
	banned := []string{"钩子", "悬念", "反转", "答案", "收尾", "[", "]", "内部标签", "评论区"}
	for i, s := range scenes {
		for _, b := range banned {
			if strings.Contains(s, b) {
				t.Fatalf("scene %d still contains banned %q: %q", i, b, s)
			}
		}
	}
	if scenes[0] == "" || scenes[4] == "" {
		t.Fatalf("expected non-empty scenes, got: %v", scenes)
	}
}

func TestValidateCopy_RejectsClosingShort(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，今天最反常的一件事，资金直接顶了国产芯片高开。
[悬念] 很多人没看懂，为什么指数红的手里票却没动。
[反转] 你再往深看一层，其实是半导体内部在高低切换。
[答案] 说白了，今天真正的主线就一个：国产芯片承接。
[收尾] 明天看承接。`
	issues := ValidateCopy(text, brief, inflows)
	found := false
	for _, it := range issues {
		if strings.Contains(it, "信息量不够") || strings.Contains(it, "至少要包含 3 句") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected short-closing issue, got %v", issues)
	}
}

func TestValidateCopy_RejectsMissingOperations(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，今天最反常的一件事，资金直接顶了国产芯片高开。
[悬念] 很多人没看懂，为什么指数红的手里票却没动。
[反转] 你再往深看一层，其实是半导体内部在高低切换。
[答案] 说白了，今天真正的主线就一个：国产芯片承接。
[收尾] 今天行情就是国产芯片强，半导体分歧。点个赞，明天开盘见。`
	issues := ValidateCopy(text, brief, inflows)
	found := false
	for _, it := range issues {
		if strings.Contains(it, "操作建议不够具体") || strings.Contains(it, "动作锚点词") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected missing-operations issue, got %v", issues)
	}
}

func TestValidateCopy_RejectsSkillMergedBannedPhrases(t *testing.T) {
	brief := &NarrativeBrief{}
	inflows := []sectorFlow{{Name: "国产芯片", Net: 253, SuperNet: 238}}
	text := `[钩子] 各位好，总的来说今天最反常的，资金只认国产芯片这个赛道。
[悬念] 核心问题在于，为什么指数红了手里的票却没动。
[反转] 更关键的来了，这不是散户推的，是主力级的承接，至关重要。
[答案] 说白了，今天真正的主线就一个：国产芯片抓手在承接，闭环跑通了。
[收尾] 明天先讲结论，国产芯片盯承接，AI 应用看收敛，点个赞，明天见。`
	issues := ValidateCopy(text, brief, inflows)
	hits := 0
	for _, it := range issues {
		if strings.Contains(it, "含禁用套话") ||
			strings.Contains(it, "含禁用套话：总的来说") ||
			strings.Contains(it, "核心问题") ||
			strings.Contains(it, "赛道") ||
			strings.Contains(it, "抓手") ||
			strings.Contains(it, "闭环") ||
			strings.Contains(it, "先讲结论") ||
			strings.Contains(it, "至关重要") {
			hits++
		}
	}
	if hits < 3 {
		t.Fatalf("expected >=3 skill-merged banned phrase hits, got %d. issues=%v", hits, issues)
	}
}
