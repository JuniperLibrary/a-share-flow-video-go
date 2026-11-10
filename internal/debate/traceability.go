package debate

import (
	"regexp"
	"strconv"
	"strings"
)

// 数字溯源:裁决段不该出现"没有出处"的金额。
//
// 实跑(2026-11-06,茅台 2026Q3)里 Synthesizer 写了一句
// "Bull 需期初合同负债 91.72 亿,2025 年报实际仅 80.07 亿,短 11.65 亿",
// 而 transcript、财报原文、工具结果里都没有 80.07 这个数——
// 它来自模型记忆,却直接决定了"预收兑现论差一块"这个结论。
// 这比算错更危险:算式错至少还有个算式可以查。
//
// 只对裁决段生效,理由有两个:
//   - 其他角色的职责就是引入新的科目与工具结果,溯源池天然不完整。
//     用同一套判据扩到非裁决轮,四场真实产出的 55 轮里有 20 轮会被标记
//     (36%),多数是「613.91 亿,计提 10% 跌价=61.39 亿」这类隐含推导:
//     算式被拆成两句写,机器只看到等号左边是「10%跌价」;
//   - 裁决者的职责只是称量已有证据,它没有任何理由引入新数字。
//
// 检验卡整段豁免:那里写的是将来的触发阈值(如"跳增超 10 亿"),
// 本来就是新数字。
//
// 已知边界:溯源池包含前序发言,所以"别人先编一个数、裁决者引用它"能通过。
// 这一层挡的是裁决者自己新造数字,不是全场串通意义上的编造——
// 后者要靠非裁决轮溯源,而那一层的误判率目前还不可接受。
var amountRe = regexp.MustCompile(`([-−－]?\d{1,3}(?:[,，]\d{3})+(?:\.\d+)?|[-−－]?\d+(?:\.\d+)?)\s*(?:亿|元)`)

// 任意数字(不带单位的也算出处:正文里"合同负债 20.84 亿"能佐证"20.84")。
// 支持千分位:模型偶尔把 "1332.84 亿" 写成 "1,332.84 亿",
// 不认逗号会把 1332.84 误判成 332.84,然后当成编造。
var anyNumberRe = regexp.MustCompile(`[-−－]?\d{1,3}(?:[,，]\d{3})+(?:\.\d+)?|[-−－]?\d+(?:\.\d+)?`)

// unverifiedAmounts 找出 text 里带金额单位、但在 sources 中找不到出处的数字。
// 容差 1%:正文常把 195.92 写成 196,把 173.27 写成 173。
func unverifiedAmounts(text string, sources ...string) []string {
	pool := make([]float64, 0, 256)
	for _, src := range sources {
		for _, raw := range anyNumberRe.FindAllString(src, -1) {
			if v, err := strconv.ParseFloat(parseNumberText(raw), 64); err == nil {
				pool = append(pool, v)
			}
		}
	}
	if len(pool) == 0 {
		return nil
	}

	body := verdictBody(text)
	// "840.59-70.9=769.69 亿"里的 769.69 是算出来的,不是引用的;
	// 它的正确性由算式验算器负责,不该再要求它在原文里出现过。
	derived := derivedAmounts(body)

	var out []string
	seen := make(map[string]bool)
	for _, m := range amountRe.FindAllStringSubmatch(body, -1) {
		v, err := strconv.ParseFloat(parseNumberText(m[1]), 64)
		if err != nil || v == 0 {
			continue
		}
		if hasSource(v, pool) || hasSource(v, derived) {
			continue
		}
		label := m[1] + "亿"
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	return out
}

// derivedAmounts 收集所有"显式算式的右侧结果"。
// 只认左右两侧都能解析的等式,避免把"缺口=80.07 亿"这种单边写法也算作出处。
func derivedAmounts(text string) []float64 {
	var out []float64
	for _, eq := range equalSigns(text) {
		if _, _, ok := parseLeftSide(text[:eq.start]); !ok {
			continue
		}
		if v, _, ok := parseRightSide(text[eq.start+eq.width:]); ok {
			out = append(out, v)
		}
	}
	return out
}

func hasSource(v float64, pool []float64) bool {
	tolerance := 0.01 * absF(v)
	if tolerance < 0.01 {
		tolerance = 0.01
	}
	for _, p := range pool {
		if absF(v-p) <= tolerance {
			return true
		}
	}
	return false
}

func normalizeMinus(s string) string {
	return strings.NewReplacer("−", "-", "－", "-").Replace(s)
}

// parseNumberText 归一全角减号与千分位,交给 strconv。
func parseNumberText(s string) string {
	return strings.NewReplacer("−", "-", "－", "-", ",", "", "，", "").Replace(s)
}

// verdictBody 去掉裁决段的检验卡部分:那里是将来要看的阈值,允许新数字。
// 只认"检验卡"/"检验"后面紧跟冒号的写法,避免把"需要检验"这类动词当成标题。
func verdictBody(text string) string {
	cut := -1
	for _, marker := range []string{"检验卡:", "检验卡：", "检验:", "检验："} {
		if idx := strings.LastIndex(text, marker); idx > cut {
			cut = idx
		}
	}
	if cut > 0 {
		return text[:cut]
	}
	return text
}
