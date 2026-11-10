package debate

import (
	"math"
	"strconv"
)

// 命题复算检查。
//
// 改版前主持人写"现金支撑不低于 90%""成色如何"这类命题,没有定义算法,
// 于是 Bull 会挑自己顺手的比率回答——四场真实产出里有两场前两轮完全没对上题。
// 现在要求命题自带算式(见 prompts/moderator.go),这里负责把它变成机器判据:
// 命题里得有"数÷数",第一位答者得用同一组数复算。
//
// 只查"有没有用同一组数字",不判对错——对错交给 checkArithmetic。

// PropositionRatio 是命题里"分子÷分母"的那组操作数。
type PropositionRatio struct {
	Numerator   float64
	Denominator float64
	Result      float64
	HasResult   bool
}

// ExtractPropositionRatio 找出命题文字里第一处"数字÷数字"。
// 科目名里的斜杠(CFO/归母净利)左边不是数字,会被跳过,
// 所以 "CFO/归母净利 = 840.59÷644.67 = 1.30" 取到的是 840.59 与 644.67。
func ExtractPropositionRatio(text string) (PropositionRatio, bool) {
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if normalizeOperator(runes[i]) != "÷" {
			continue
		}
		num, _, ok := readOperandBack(runes, i)
		if !ok {
			continue
		}
		den, ok := readOperandForward(runes, i+1)
		if !ok || den == 0 {
			continue
		}
		return PropositionRatio{
			Numerator:   num,
			Denominator: den,
			Result:      num / den,
			HasResult:   true,
		}, true
	}
	return PropositionRatio{}, false
}

// AnsweredRatio 判断 answer 是否复算了命题里的同一条算式。
// applicable=false 表示命题里没有算式,这一条不判(旧产出不该因此变红)。
func AnsweredRatio(proposition, answer string) (answered bool, ratio PropositionRatio, applicable bool) {
	ratio, ok := ExtractPropositionRatio(proposition)
	if !ok {
		return false, PropositionRatio{}, false
	}
	nums := numbersIn(answer)
	hasNum := func(target float64) bool {
		for _, v := range nums {
			if closeEnough(v, target) {
				return true
			}
		}
		return false
	}
	if hasNum(ratio.Numerator) && hasNum(ratio.Denominator) {
		return true, ratio, true
	}
	// 只写比值也算复算:命题已经把算式列出来了,答者给出同一个商即可核对。
	if ratio.HasResult && hasNum(ratio.Result) {
		return true, ratio, true
	}
	return false, ratio, true
}

// PropositionStatus 是一场辩论里"命题"这一关的体检结果。
type PropositionStatus struct {
	Proposition string
	HasFormula  bool
	Answer      string
	Answered    bool
}

// CheckProposition 取主持人的开场命题与紧随其后的第一条回应,
// 判断命题是否自带算式、回应是否复算了同一条算式。
func CheckProposition(turns []Turn) PropositionStatus {
	var st PropositionStatus
	idx := -1
	for i, t := range turns {
		if t.Speaker == Moderator && (t.Phase == PhaseOpen || idx < 0) {
			st.Proposition = t.Text
			idx = i
			if t.Phase == PhaseOpen {
				break
			}
		}
	}
	if idx < 0 {
		return st
	}
	_, st.HasFormula = ExtractPropositionRatio(st.Proposition)
	for _, t := range turns[idx+1:] {
		if t.Speaker == Moderator {
			continue
		}
		st.Answer = t.Text
		st.Answered, _, _ = AnsweredRatio(st.Proposition, st.Answer)
		break
	}
	return st
}

// readOperandForward 从 start 向右读一个操作数。
func readOperandForward(runes []rune, start int) (float64, bool) {
	i := skipSpaceRunes(runes, start)
	j := i
	for j < len(runes) && (isDigitRune(runes[j]) || runes[j] == '.') {
		j++
	}
	if j == i {
		return 0, false
	}
	v, err := strconv.ParseFloat(string(runes[i:j]), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// numbersIn 抽出文本里所有数字,顺带处理千分位(1,332.84 -> 1332.84)。
func numbersIn(text string) []float64 {
	runes := []rune(text)
	var out []float64
	for i := 0; i < len(runes); i++ {
		if !isDigitRune(runes[i]) {
			continue
		}
		j := i
		var buf []rune
		for j < len(runes) {
			r := runes[j]
			if isDigitRune(r) || r == '.' {
				buf = append(buf, r)
				j++
				continue
			}
			// 千分位:数字,后面还跟着三位数字时吞掉逗号。
			if (r == ',' || r == '，') && j+3 < len(runes) &&
				isDigitRune(runes[j+1]) && isDigitRune(runes[j+2]) && isDigitRune(runes[j+3]) {
				j++
				continue
			}
			break
		}
		if v, err := strconv.ParseFloat(string(buf), 64); err == nil {
			out = append(out, v)
		}
		i = j - 1
	}
	return out
}

// closeEnough 允许 0.5% 的写法差异(195.92 写成 195.9 不算换了数)。
func closeEnough(a, b float64) bool {
	tol := math.Abs(b) * 0.005
	if tol < 0.01 {
		tol = 0.01
	}
	return math.Abs(a-b) <= tol
}
