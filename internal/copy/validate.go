package copy

import (
	"fmt"
	"strings"
)

var bannedPhrases = []string{
	"资金只认", "谁打的", "把自己点燃", "典型节奏", "就一句话",
	"机构根本没动手", "机构零参与", "全是游资", "散户推的",
	"数据不会说谎", "来看全景", "这说明什么问题", "有意思的是",
	"超大单和大单都是零", "超大单为零", "净量都是零",

	"总的来说", "能有效", "往往", "至关重要", "精心打造",
	"确保", "直接讲", "先讲结论", "先讲清楚", "先讲背景",
	"先说结论", "先说背景",
	"差别不在于", "而在于", "不仅更", "不仅也",
	"一个事实", "关键差异", "最可怕的不是", "核心问题",
	"令人不安的事实", "坐不住", "系统性地", "很精准",
	"精准地", "只有才能", "诚实面对", "很清楚", "讲清楚",
	"非常清楚", "结构性的", "守得住", "守不住", "收在这里",
	"某种程度上", "在多数情况下", "研究指出", "资料显示", "数据显示",

	"闭环", "抓手", "颗粒度", "对齐", "拉齐", "赋能", "赛道",
	"弯道超车", "心智", "占领心智",
}

var hookGreetingPhrases = []string{
	"各位好", "朋友们", "老粉都知道", "收盘了", "收盘聊两句",
	"说句实在话", "我是阿川", "阿川聊盘", "大家好", "收盘复盘",
}

// stanceAnchorPhrases [答案] 段的立场锚点：重仓科技博主的观点必须落在这些表达上，
// 防止 AI 退回“纯数据播报”的中立腔。
var stanceAnchorPhrases = []string{
	"主线", "承接", "分歧", "高低切", "高低切换", "回调不改",
	"兑现", "洗盘", "重仓", "我一直", "低位", "共振", "切换",
}

// stanceBannedPhrases 与「重仓科技博主」立场直接矛盾的表达，出现即判失败。
var stanceBannedPhrases = []string{
	"科技完了", "科技已死", "科技到头了", "科技没戏了", "科技不行了",
	"科技结束了", "科技不是主线", "放弃科技", "别碰科技", "永远不碰科技",
}

var bannedCasualBroPhrases = []string{
	"兄弟们", "老铁", "家人们", "老铁们", "猛干", "干就完了",
	"冲", "冲啊", "赶紧冲", "杀进去", "满仓干",
}

var transitionPhrases = []string{
	"很多人没看懂", "你想过没有", "问题来了", "更奇怪的是", "这里面有个细节",
	"更关键的来了", "真正的问题在这", "你再往深看", "你再往深看一层", "我盯了一天",
	"所以盘面的真相是", "说白了", "核心逻辑只有一个", "一句话讲清楚", "真相是",
	"你再仔细看", "真正有意思的是", "更重要的是",
}

var closingInteractionPhrases = []string{
	"开盘见", "盘中再跟踪", "盘中再说", "点个赞",
	"评论区", "收藏一下", "关注不迷路", "关注我", "收藏好",
	"盯着", "盯紧", "记住", "记好", "别忘了",
	"我再提醒你", "我会提醒你",
}

var bodyBannedStructureWords = []string{"钩子", "悬念", "反转", "答案", "收尾"}

var closingOperationPhrases = []string{
	"盯", "盯紧", "盯着",
	"承接", "放量", "收敛", "回流", "转弱",
	"观望", "减仓", "保护", "先不动", "别急",
	"先看", "观察", "等", "守住",
}

func countChinesePunctuatedSentences(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	cnt := strings.Count(s, "。")
	if cnt == 0 {
		cnt += strings.Count(s, "；")
	}
	if cnt == 0 {
		if len(s) > 20 {
			return 1
		}
		return 0
	}
	last := s[len(s)-1:]
	if last != "。" && last != "；" && last != "！" && last != "?" && last != "？" {
		return cnt + 1
	}
	return cnt
}

// ValidateCopy 校验成稿是否踩禁词、是否与简报关键事实矛盾、以及是否具备博主语气锚点与科技立场。
func ValidateCopy(text string, brief *NarrativeBrief, inflows, outflows []sectorFlow) []string {
	var issues []string
	for _, p := range bannedPhrases {
		if strings.Contains(text, p) {
			issues = append(issues, fmt.Sprintf("含禁用套话：%s", p))
		}
	}

	for _, p := range stanceBannedPhrases {
		if strings.Contains(text, p) {
			issues = append(issues, fmt.Sprintf("含与「重仓科技博主」立场矛盾的表达：%s（科技回调只能写分歧/兑现/高低切，非科技吸金只能写轮动/避险）", p))
		}
	}

	allowed := extractAllowedSectorNames(brief, inflows)
	if strings.Contains(text, "新能源") && !allowed["新能源"] {
		issues = append(issues, "出现素材包不存在的板块名「新能源」")
	}

	if len(inflows) > 0 {
		leader := inflows[0].Name
		if !strings.Contains(text, leader) {
			issues = append(issues, fmt.Sprintf("未提及流入龙头「%s」", leader))
		}
		if absF(inflows[0].SuperNet) >= 50 {
			for _, p := range []string{"机构没动手", "机构未参与", "零参与", "全是中小单"} {
				if strings.Contains(text, p) {
					issues = append(issues, fmt.Sprintf("龙头超大单 %+.0f 亿但与「%s」矛盾", inflows[0].SuperNet, p))
				}
			}
		}
	}

	if !hasSceneTags(text) {
		issues = append(issues, "缺少或打乱 [钩子]/[悬念]/[反转]/[答案]/[收尾] 五段标签")
	} else {
		scenes := extractSceneContents(text)
		if hook := scenes["钩子"]; hook != "" {
			if !containsAny(hook, hookGreetingPhrases) {
				issues = append(issues, "[钩子] 缺少博主开场招呼词（建议加「各位好 / 收盘聊两句 / 朋友们 / 老粉都知道 / 收盘了说句实在话」等）")
			}
		}
		for _, sceneKey := range []string{"悬念", "反转", "答案"} {
			if body := scenes[sceneKey]; body != "" {
				if !containsAny(body, transitionPhrases) {
					issues = append(issues, fmt.Sprintf("[%s] 缺少人话过渡词（建议加「很多人没看懂 / 更关键的来了 / 说白了」等）", sceneKey))
				}
			}
		}
		if techPresentInFlows(inflows, outflows) {
			if answer := scenes["答案"]; answer != "" && !containsAny(answer, stanceAnchorPhrases) {
				issues = append(issues, "[答案] 缺少立场锚点，重仓科技博主必须有观点（建议落点：「主线确认 / 承接 / 分歧 / 高低切 / 回调不改 / 我一直盯的方向」），不能写成纯数据播报")
			}
		}
		if tail := scenes["收尾"]; tail != "" {
			if !containsAny(tail, closingInteractionPhrases) {
				issues = append(issues, "[收尾] 缺少收束互动锚点（建议加「8月17日开盘见 / 盯着 / 点个赞 / 我再提醒你」等）")
			}
			sentences := countChinesePunctuatedSentences(tail)
			if sentences < 3 {
				issues = append(issues, fmt.Sprintf("[收尾] 信息量不够，至少要包含 3 句：行情概括 + 风险提醒 + 操作建议，当前仅 %d 句", sentences))
			}
			opHits := 0
			for _, op := range closingOperationPhrases {
				if strings.Contains(tail, op) {
					opHits++
					if opHits >= 2 {
						break
					}
				}
			}
			if opHits < 2 {
				issues = append(issues, "[收尾] 操作建议不够具体，至少要包含 2 个可落地动作锚点词（盯承接 / 看收敛 / 等回流 / 减仓保护 / 观望 等）")
			}
		}
		for sceneKey, body := range scenes {
			if body == "" {
				continue
			}
			for _, w := range bodyBannedStructureWords {
				if strings.Contains(body, w) {
					issues = append(issues, fmt.Sprintf("[%s] 正文中出现结构锚点词「%s」，观众不能听到这类内部标签词，需删除或替换为口语同义表达", sceneKey, w))
				}
			}
			if strings.Contains(body, "[") || strings.Contains(body, "]") {
				issues = append(issues, fmt.Sprintf("[%s] 正文中残留方括号内容或标签（%q），只允许行首 [钩子]/[悬念]/[反转]/[答案]/[收尾] 做分段，正文内所有 [] 必须删除", sceneKey, body))
			}
			for _, w := range bannedCasualBroPhrases {
				if strings.Contains(body, w) {
					issues = append(issues, fmt.Sprintf("[%s] 正文中出现过于社群化/喊单口语「%s」，不符合阿川 80 万粉专业财经博主语气，请替换成更克制的表达（如「朋友们」「各位好」「收盘了」等）", sceneKey, w))
				}
			}
		}
	}

	return issues
}

// BlockingCopyIssues 只保留值得再发起一次 LLM 修复的严重问题。
// 这类问题要么会影响事实正确性，要么会直接破坏口播结构。
// 其余偏风格类建议不再强制触发 FixCopy，避免每次都走第二轮。
func BlockingCopyIssues(issues []string) []string {
	var out []string
	for _, issue := range issues {
		switch {
		case strings.Contains(issue, "含禁用套话"),
			strings.Contains(issue, "与「重仓科技博主」立场矛盾"),
			strings.Contains(issue, "素材包不存在的板块名"),
			strings.Contains(issue, "未提及流入龙头"),
			strings.Contains(issue, "龙头超大单"),
			strings.Contains(issue, "缺少或打乱 [钩子]/[悬念]/[反转]/[答案]/[收尾]"),
			strings.Contains(issue, "正文中出现结构锚点词"),
			strings.Contains(issue, "正文中残留方括号"),
			strings.Contains(issue, "过于社群化/喊单口语"):
			out = append(out, issue)
		}
	}
	return out
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// techPresentInFlows 判断当日资金榜（流入/流出 TOP3）是否出现科技主线板块。
// 仅当科技在场时才启用立场校验，避免非科技主导行情下误报。
func techPresentInFlows(inflows, outflows []sectorFlow) bool {
	for i, v := range inflows {
		if i >= 3 {
			break
		}
		if techSectorNames[v.Name] {
			return true
		}
	}
	for i, v := range outflows {
		if i >= 3 {
			break
		}
		if techSectorNames[v.Name] {
			return true
		}
	}
	return false
}

func extractSceneContents(text string) map[string]string {
	required := []string{"[钩子]", "[悬念]", "[反转]", "[答案]", "[收尾]"}
	out := make(map[string]string, len(required))
	lines := strings.Split(text, "\n")
	currentKey := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		for _, tag := range required {
			if strings.HasPrefix(line, tag) {
				currentKey = strings.Trim(tag, "[]")
				remainder := strings.TrimSpace(strings.TrimPrefix(line, tag))
				out[currentKey] = remainder
				goto nextLine
			}
		}
		if currentKey != "" {
			out[currentKey] = out[currentKey] + " " + line
		}
	nextLine:
	}
	return out
}

func extractAllowedSectorNames(brief *NarrativeBrief, inflows []sectorFlow) map[string]bool {
	out := make(map[string]bool)
	for _, s := range inflows {
		if s.Name != "" {
			out[s.Name] = true
		}
	}
	if brief == nil {
		return out
	}
	addBlock := func(block string) {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "- ") {
				rest := strings.TrimSpace(strings.TrimPrefix(line, "- "))
				if i := strings.Index(rest, ":"); i > 0 {
					name := strings.TrimSpace(rest[:i])
					if name != "" {
						out[name] = true
					}
				}
			} else if i := strings.Index(line, "："); i > 0 {
				name := strings.TrimSpace(line[:i])
				if name != "" {
					out[name] = true
				}
			}
		}
	}
	addBlock(brief.InflowLeaders)
	addBlock(brief.OutflowLeaders)
	addBlock(brief.Continuity)
	return out
}

func hasSceneTags(text string) bool {
	required := []string{"[钩子]", "[悬念]", "[反转]", "[答案]", "[收尾]"}
	cursor := -1
	for _, tag := range required {
		idx := strings.Index(text, tag)
		if idx < 0 || idx <= cursor {
			return false
		}
		cursor = idx
	}
	return true
}
