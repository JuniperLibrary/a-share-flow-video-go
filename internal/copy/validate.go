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
}

// ValidateCopy 校验成稿是否踩禁词、是否与简报关键事实矛盾。
func ValidateCopy(text string, brief *NarrativeBrief, inflows []sectorFlow) []string {
	var issues []string
	for _, p := range bannedPhrases {
		if strings.Contains(text, p) {
			issues = append(issues, fmt.Sprintf("含禁用套话：%s", p))
		}
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
		issues = append(issues, "缺少 [钩子]/[悬念]/[反转]/[收尾] 五段标签")
	}

	return issues
}

func hasSceneTags(text string) bool {
	required := []string{"[钩子]", "[悬念]", "[反转]", "[收尾]"}
	for _, tag := range required {
		if !strings.Contains(text, tag) {
			return false
		}
	}
	return strings.Count(text, "[钩子]") >= 1
}
