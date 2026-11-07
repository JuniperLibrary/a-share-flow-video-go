package debate

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// 算式验算:模型会写出"看起来对"的算式,但除法方向反了、百分号少乘 100、
// 减法顺序颠倒都很常见。这类错误比缺算式更危险——它有一个正确的形状,
// 观众和下游都不会再查。
//
// 判定方式是以每个等号为锚点,向左解析同类运算链、向右解析结果数:
//   - 只接受同一优先级的链(a+b-c、a×b÷c),混合优先级需要真正的表达式解析,
//     收益不抵复杂度,交给人眼;
//   - 括号里的式子先递归验证,再把括号整体归约成一个数,于是
//     (20.84+3.65)÷195.92=12.5% 与 1435.23÷(营收1332.84×1.13=1506.11)=0.953
//     都能验。
//
// 为什么不用正则:真实文本里运算符有全角变体(减号常用 − 而不是 -),
// 算式也会连写(a÷b=c÷d=e),正则要么漏匹配、要么把链式算式截成半段
// 把正确结论判成错。2026-11-06 的深度产出里 5 处减法全部因为 − 被漏验。
//
// 误判的代价不对称:漏判只是少兜一层底,误判会让每轮多烧一次模型调用
// (实测单轮 10-27s)。所以拿不准的一律跳过。

// arithIssue 是一条验算不通过的算式。
type arithIssue struct {
	Expr     string
	Expected float64
	Stated   float64
	Unit     string
}

func (a arithIssue) String() string {
	unit := a.Unit
	if unit == "" {
		unit = "（无单位）"
	}
	return fmt.Sprintf("%s 实际算得 %.4g,文中写 %.4g%s", a.Expr, a.Expected, a.Stated, unit)
}

// checkArithmetic 找出正文里算错的算式。
//
// 容差 3%:报告里的数字普遍保留 1-2 位小数、口径可能四舍五入,
// 卡太紧会把"840.59-70.9=769.69"这种正确算式误判。
func checkArithmetic(text string) []arithIssue {
	var issues []arithIssue
	// 括号内的算式单独验一遍:归约之后原来的式子就不存在了,
	// 不先验就会漏掉"括号里算错、但外层结果对"的情况。
	for _, g := range innermostParenGroups(text) {
		issues = append(issues, checkArithmetic(text[g[0]+1:g[1]])...)
	}
	issues = append(issues, checkPlainEquations(reduceParenGroups(text))...)
	return issues
}

func checkPlainEquations(text string) []arithIssue {
	var issues []arithIssue
	for _, eq := range equalSigns(text) {
		expr, expected, ok := parseLeftSide(text[:eq.start])
		if !ok {
			continue
		}
		stated, unit, ok := parseRightSide(text[eq.start+eq.width:])
		if !ok {
			continue
		}
		// 结果是百分数时,算式算出来的是小数,正文写的是"×100 后的数"。
		if unit == "%" {
			expected *= 100
		}
		// 四舍五入到 0 的结果没有比较意义。
		if expected == 0 {
			continue
		}
		tolerance := 0.03 * absF(expected)
		if tolerance < 0.05 {
			tolerance = 0.05
		}
		if absF(expected-stated) <= tolerance {
			continue
		}
		issues = append(issues, arithIssue{
			Expr:     expr,
			Expected: expected,
			Stated:   stated,
			Unit:     unit,
		})
	}
	return issues
}

// parseLeftSide 解析等号左边紧邻的同类运算链并求出结果。
func parseLeftSide(prefix string) (expr string, value float64, ok bool) {
	runes := []rune(prefix)
	end := skipFillerBack(runes, len(runes), 4)
	if end <= 0 {
		return "", 0, false
	}

	first, start, ok := readOperandBack(runes, end)
	if !ok {
		return "", 0, false
	}
	// 逆序收集:values[0] 是最右操作数,ops[k] 连接 values[k+1] 与 values[k]。
	values := []float64{first}
	var ops []string
	cursor := start

	for len(values) < 5 {
		afterOp := skipFillerBack(runes, cursor, 6)
		if afterOp <= 0 {
			break
		}
		op := normalizeOperator(runes[afterOp-1])
		if op == "" {
			break
		}
		beforeOp := skipFillerBack(runes, afterOp-1, 6)
		v, start2, ok := readOperandBack(runes, beforeOp)
		if !ok {
			break
		}
		ops = append(ops, op)
		values = append(values, v)
		cursor = start2
	}

	// 只有一个操作数说明等号左边不是算式。
	if len(ops) == 0 {
		return "", 0, false
	}
	// 左端还紧接运算符或左括号,说明它是更长算式的一段:
	// 要么是混合优先级(a+b×c),要么是链式算式的前半段。单算这一段必然错。
	if precededByOperatorRunes(runes, cursor) {
		return "", 0, false
	}

	// 逆序还原成书写顺序。
	ordered := make([]float64, len(values))
	for i, v := range values {
		ordered[len(values)-1-i] = v
	}
	orderedOps := make([]string, len(ops))
	for i, op := range ops {
		orderedOps[len(ops)-1-i] = op
	}

	acc, ok := evalChain(ordered, orderedOps)
	if !ok {
		return "", 0, false
	}
	return string(runes[cursor:end]), acc, true
}

// evalChain 只接受同一优先级的运算链,混合优先级直接放弃。
func evalChain(values []float64, ops []string) (float64, bool) {
	if len(values) != len(ops)+1 || len(ops) == 0 {
		return 0, false
	}
	hasAddSub, hasMulDiv := false, false
	for _, op := range ops {
		switch op {
		case "+", "-":
			hasAddSub = true
		case "×", "÷":
			hasMulDiv = true
		}
	}
	if hasAddSub && hasMulDiv {
		return 0, false
	}
	acc := values[0]
	for i, op := range ops {
		v := values[i+1]
		switch op {
		case "+":
			acc += v
		case "-":
			acc -= v
		case "×":
			acc *= v
		case "÷":
			if v == 0 {
				return 0, false
			}
			acc /= v
		default:
			return 0, false
		}
	}
	return acc, true
}

// parseRightSide 解析等号右边紧邻的结果数。
// 链式算式(a÷b=c÷d=e)里,第一个等号右边是下一段的被除数而不是结果,
// 靠"结果后面紧跟运算符"来排除。
func parseRightSide(rest string) (stated float64, unit string, ok bool) {
	runes := []rune(rest)
	i := 0
	// 等号后可能跟"约""为"这类连接字,最多 4 个。
	for skipped := 0; i < len(runes) && skipped < 4; skipped++ {
		r := runes[i]
		if r == ' ' || r == '\t' || r == '\u3000' {
			i++
			skipped--
			continue
		}
		if isFormulaRune(r) {
			break
		}
		i++
	}

	neg := false
	if i < len(runes) && isMinusRune(runes[i]) {
		neg = true
		i++
	}
	start := i
	for i < len(runes) && (isDigitRune(runes[i]) || runes[i] == '.') {
		i++
	}
	if i == start {
		return 0, "", false
	}
	v, err := strconv.ParseFloat(string(runes[start:i]), 64)
	if err != nil {
		return 0, "", false
	}
	if neg {
		v = -v
	}

	j := skipSpaceRunes(runes, i)
	if j < len(runes) {
		switch runes[j] {
		case '%', '％':
			unit = "%"
			j++
		case '倍', '亿', '天', '元':
			unit = string(runes[j])
			j++
		}
	}

	k := skipSpaceRunes(runes, j)
	if k < len(runes) && normalizeOperator(runes[k]) != "" {
		return 0, "", false
	}
	return v, unit, true
}

// readOperandBack 从 end 向左读一个操作数,返回值和它的起始下标。
// 支持后缀百分号(按 ÷100 处理)。
func readOperandBack(runes []rune, end int) (float64, int, bool) {
	i := skipSpaceRunesBack(runes, end)
	pct := false
	if i > 0 && (runes[i-1] == '%' || runes[i-1] == '％') {
		pct = true
		i--
	}
	j := i
	for j > 0 && (isDigitRune(runes[j-1]) || runes[j-1] == '.') {
		j--
	}
	if j == i {
		return 0, end, false
	}
	v, err := strconv.ParseFloat(string(runes[j:i]), 64)
	if err != nil {
		return 0, end, false
	}
	if pct {
		v /= 100
	}
	return v, j, true
}

func precededByOperatorRunes(runes []rune, start int) bool {
	for i := start - 1; i >= 0; i-- {
		switch runes[i] {
		case ' ', '\t', '\u3000':
			continue
		case '(', '（':
			return true
		}
		return normalizeOperator(runes[i]) != ""
	}
	return false
}

// skipFillerBack 向左跳过运算符与数字之间的"非算式字符"(科目名、单位、连接词),
// 最多 max 个,避免跨句粘连。
func skipFillerBack(runes []rune, end, max int) int {
	i := end
	skipped := 0
	for i > 0 {
		r := runes[i-1]
		if r == ' ' || r == '\t' || r == '\u3000' {
			i--
			continue
		}
		if isFormulaRune(r) {
			return i
		}
		if skipped >= max {
			return i
		}
		skipped++
		i--
	}
	return i
}

func skipSpaceRunes(runes []rune, i int) int {
	for i < len(runes) && (runes[i] == ' ' || runes[i] == '\t' || runes[i] == '\u3000') {
		i++
	}
	return i
}

func skipSpaceRunesBack(runes []rune, i int) int {
	for i > 0 && (runes[i-1] == ' ' || runes[i-1] == '\t' || runes[i-1] == '\u3000') {
		i--
	}
	return i
}

func isDigitRune(r rune) bool { return r >= '0' && r <= '9' }

func isMinusRune(r rune) bool {
	switch r {
	case '-', '−', '－', '–':
		return true
	}
	return false
}

// normalizeOperator 把全角/变体运算符归一。
// 破折号 — 不算减号:它在中金研报式行文里是标点,当运算符会误判。
func normalizeOperator(r rune) string {
	switch r {
	case '+', '＋':
		return "+"
	case '-', '−', '－', '–':
		return "-"
	case '×', '*', '＊':
		return "×"
	case '÷', '/', '／':
		return "÷"
	}
	return ""
}

// isFormulaRune 判断是否属于算式本身的字符(用于区分科目名与算式边界)。
func isFormulaRune(r rune) bool {
	if isDigitRune(r) || isMinusRune(r) || normalizeOperator(r) != "" {
		return true
	}
	switch r {
	case '+', '＋', '=', '＝', '(', ')', '（', '）', '%', '％', '.', '≥', '≤', '<', '>':
		return true
	}
	return false
}

type equalSign struct {
	start int
	width int
}

func equalSigns(s string) []equalSign {
	var out []equalSign
	for i := 0; i < len(s); {
		if s[i] == '=' {
			out = append(out, equalSign{i, 1})
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "＝") {
			out = append(out, equalSign{i, len("＝")})
			i += len("＝")
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return out
}

// parenGroups 返回文本里所有成对括号的位置(按左括号出现顺序)。
func parenGroups(s string) [][2]int {
	var out [][2]int
	var stack []int
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch r {
		case '(', '（':
			stack = append(stack, i)
		case ')', '）':
			if n := len(stack); n > 0 {
				out = append(out, [2]int{stack[n-1], i})
				stack = stack[:n-1]
			}
		}
		i += size
	}
	return out
}

// innermostParenGroups 只返回内部没有嵌套括号的组。
func innermostParenGroups(s string) [][2]int {
	var out [][2]int
	for _, g := range parenGroups(s) {
		inner := s[g[0]+1 : g[1]]
		if strings.ContainsAny(inner, "()（）") {
			continue
		}
		out = append(out, g)
	}
	return out
}

// reduceParenGroups 把括号内的算式替换成计算结果。
// 括号里是"算式=结果"时取算式算出来的值,而不是模型写的那个结果:
// 那个结果已经由括号内的递归检查单独判定,用它来算外层会让错误级联。
func reduceParenGroups(text string) string {
	for iter := 0; iter < 3; iter++ {
		groups := innermostParenGroups(text)
		if len(groups) == 0 {
			return text
		}
		replaced := false
		// 从后往前替换,前面的下标才不会失效。
		for i := len(groups) - 1; i >= 0; i-- {
			g := groups[i]
			inner := text[g[0]+1 : g[1]]
			v, ok := groupValue(inner)
			if !ok {
				continue
			}
			text = text[:g[0]] + formatNumber(v) + text[g[1]+1:]
			replaced = true
		}
		if !replaced {
			return text
		}
	}
	return text
}

// groupValue 求括号内容的值:纯二元式直接算,"算式=结果"取算式部分,
// 单个数字(嵌套括号归约后会出现 (24.49) 这种)原样取用。
func groupValue(inner string) (float64, bool) {
	exprPart := inner
	if eqs := equalSigns(inner); len(eqs) > 0 {
		exprPart = inner[:eqs[len(eqs)-1].start]
	}
	if _, v, ok := parseLeftSide(exprPart); ok {
		return v, true
	}
	trimmed := strings.TrimSpace(exprPart)
	if trimmed == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// formatNumber 保留足够精度让外层验算的 3% 容差仍然成立,但去掉尾随 0,
// 避免把"1506.1"写成"1506.100000"这类噪声。
func formatNumber(v float64) string {
	s := strconv.FormatFloat(v, 'f', 6, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
