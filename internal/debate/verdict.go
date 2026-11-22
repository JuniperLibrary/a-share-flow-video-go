package debate

import "strings"

// 裁决段的形状检查。
//
// 2026-11-10 的 quick 产出里,"最致命反证"整段写的是"扣非占 99.78%、
// 非经常仅 1.42 亿无粉饰"——这是支持结论的证据,真正的反证(营收 +1.8%
// 而归母 -0.2%)被当作附带从句塞在句尾。裁决者把最强证据和最强反证
// 记反了,观众会以为结论毫无争议。
//
// 这里只查两件机器能查的事:
//   1. 四段标签是否齐全;
//   2. 第 3 段有没有写"这条反证成立之后会怎样"(若/一旦/意味着/否则)。
// 没有后果句的那一段,通常就是在罗列事实而不是反证。

// verdictIssue 是一处裁决段形状问题。
func CheckVerdictShape(verdict string) []string {
	var issues []string
	if strings.TrimSpace(verdict) == "" {
		return []string{"裁决段为空"}
	}
	for _, label := range []string{"结论", "最强证据", "最致命反证", "检验"} {
		if !strings.Contains(verdict, label) {
			issues = append(issues, "缺 "+label+" 段")
		}
	}
	seg, ok := segmentAfter(verdict, "最致命反证", "检验")
	if !ok {
		return issues
	}
	if !hasAny(seg, []string{"若", "如果", "一旦", "意味着", "否则"}) {
		issues = append(issues, "最致命反证没有写成立的后果(缺 若/一旦/意味着/否则),读起来只是陈述事实")
	}
	return issues
}

// segmentAfter 取 start 标签之后、end 标签之前的正文。
func segmentAfter(text, start, end string) (string, bool) {
	i := strings.Index(text, start)
	if i < 0 {
		return "", false
	}
	rest := text[i+len(start):]
	if j := strings.Index(rest, end); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(strings.Trim(rest, ":：*")), true
}

func hasAny(text string, cues []string) bool {
	for _, c := range cues {
		if strings.Contains(text, c) {
			return true
		}
	}
	return false
}
