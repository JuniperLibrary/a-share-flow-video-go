package debate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// 算式验算:模型会写出"看起来对"的算式,但除法方向反了、百分号少乘 100、
// 减法顺序颠倒都很常见。这类错误比缺算式更危险——它有一个正确的形状,
// 观众和下游都不会再查。
//
// 判定方式是以每个等号为锚点,向左取紧邻的"数 op 数",向右取紧邻的结果数。
// 不用"全文扫一遍正则在任意位置匹配",是因为真实文本里算式会连写
// (1435.23÷(1332.84×1.13)=1435.23÷1506.11=0.953),整段匹配会把链式
// 算式截成半段,把正确结论判成错。
//
// 只验算二元式:多步复合式需要优先级解析,收益不抵复杂度,交给人眼。
// 括号里的单个二元式先就地归约成结果,再验外层,这样 (20.84+3.65)÷195.92=12.5%
// 也能验。
var (
	// 左值:必须以等号前的可忽略字符结尾(单位、"亿"这类口径字)。
	lhsRe = regexp.MustCompile(
		`(-?\d+(?:\.\d+)?)\s*([%％])?` + filler + `{0,6}([÷/×*+\-])` + filler + `{0,6}(-?\d+(?:\.\d+)?)\s*([%％])?` + filler + `{0,4}$`,
	)
	// 右值:等号后第一个数,可带单位。
	rhsRe = regexp.MustCompile(
		`^\s*` + filler + `{0,4}(-?\d+(?:\.\d+)?)\s*([%％]|倍|亿|天)?`,
	)
)

// filler 是数字与运算符之间允许出现的"非算式字符"(科目名、单位、中文连接词)。
// 括号不在此列:括号内的式子要先归约,否则 operator 会跨括号匹配错位。
const filler = `[^\d÷/×*+\-=＝()（）\n]`

// 括号内只有一个运算符时才能安全归约;嵌套靠多轮替换解决。
var parenGroupRe = regexp.MustCompile(
	`[（(]\s*(-?\d+(?:\.\d+)?)\s*([÷/×*+\-])\s*(-?\d+(?:\.\d+)?)\s*[)）]`,
)

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

// checkArithmetic 找出正文里算错的二元算式。
//
// 容差 3%:报告里的数字普遍保留 1-2 位小数、口径可能四舍五入,
// 卡太紧会把"840.59-70.9=769.69"这种正确算式误判。
func checkArithmetic(text string) []arithIssue {
	reduced := reduceParenGroups(text)
	var issues []arithIssue
	for _, eq := range equalSigns(reduced) {
		expr, expected, ok := parseLeftSide(reduced[:eq.start])
		if !ok {
			continue
		}
		stated, unit, ok := parseRightSide(reduced[eq.start+eq.width:])
		if !ok {
			continue
		}
		if unit == "%" || unit == "％" {
			expected *= 100
			unit = "%"
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

type equalSign struct {
	start int
	width int
}

func equalSigns(s string) []equalSign {
	var out []equalSign
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "＝"):
			out = append(out, equalSign{i, len("＝")})
			i += len("＝")
		case s[i] == '=':
			out = append(out, equalSign{i, 1})
			i++
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
	}
	return out
}

// parseLeftSide 解析等号左边紧邻的"数 op 数",并算出结果。
func parseLeftSide(prefix string) (expr string, value float64, ok bool) {
	tail := tailRunes(prefix, 40)
	locs := lhsRe.FindStringSubmatchIndex(tail)
	if locs == nil {
		return "", 0, false
	}
	// a+b-c=d 这种多步式里,正则只能取到最后两段(b-c),单算它必然错。
	// 若左值紧跟在运算符或左括号后,说明它是一段更长的算式,跳过。
	if precededByOperator(tail, locs[0]) {
		return "", 0, false
	}
	m := lhsRe.FindStringSubmatch(tail)
	if m == nil {
		return "", 0, false
	}

	left, errL := strconv.ParseFloat(m[1], 64)
	right, errR := strconv.ParseFloat(m[4], 64)
	if errL != nil || errR != nil {
		return "", 0, false
	}
	if m[2] != "" {
		left /= 100
	}
	if m[5] != "" {
		right /= 100
	}
	value, ok = applyBinary(left, m[3], right)
	if !ok {
		return "", 0, false
	}
	return strings.TrimSpace(m[0]), value, true
}

// parseRightSide 解析等号右边紧邻的结果数。
// 链式算式(a÷b=c÷d=e)里,第一个等号右边是下一段的被除数而不是结果,
// 靠"结果后面紧跟运算符"来排除。
func parseRightSide(rest string) (stated float64, unit string, ok bool) {
	head := headRunes(rest, 40)
	locs := rhsRe.FindStringSubmatchIndex(head)
	if locs == nil {
		return 0, "", false
	}
	m := rhsRe.FindStringSubmatch(head)
	if m == nil {
		return 0, "", false
	}
	if followedByOperator(head, locs[1]) {
		return 0, "", false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, "", false
	}
	return v, m[2], true
}

func precededByOperator(s string, start int) bool {
	runes := []rune(s[:start])
	for i := len(runes) - 1; i >= 0; i-- {
		switch runes[i] {
		case ' ', '\t':
			continue
		case '+', '-', '×', '*', '÷', '/', '(', '（':
			return true
		default:
			return false
		}
	}
	return false
}

func followedByOperator(s string, end int) bool {
	for _, r := range s[end:] {
		switch r {
		case ' ', '\t':
			continue
		case '+', '-', '×', '*', '÷', '/':
			return true
		default:
			return false
		}
	}
	return false
}

func tailRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[len(runes)-n:])
}

func headRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// reduceParenGroups 把括号内的单个二元式替换成计算结果。
// 只处理纯数字:括号里带百分号时语义含糊(0.3%+0.2% 还是 0.003+0.002),直接跳过。
func reduceParenGroups(text string) string {
	for i := 0; i < 3; i++ {
		replaced := false
		text = parenGroupRe.ReplaceAllStringFunc(text, func(group string) string {
			m := parenGroupRe.FindStringSubmatch(group)
			if m == nil {
				return group
			}
			left, errL := strconv.ParseFloat(m[1], 64)
			right, errR := strconv.ParseFloat(m[3], 64)
			if errL != nil || errR != nil {
				return group
			}
			v, ok := applyBinary(left, m[2], right)
			if !ok {
				return group
			}
			replaced = true
			return formatNumber(v)
		})
		if !replaced {
			break
		}
	}
	return text
}

func applyBinary(left float64, op string, right float64) (float64, bool) {
	switch op {
	case "÷", "/":
		if right == 0 {
			return 0, false
		}
		return left / right, true
	case "×", "*":
		return left * right, true
	case "+":
		return left + right, true
	case "-":
		return left - right, true
	}
	return 0, false
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
