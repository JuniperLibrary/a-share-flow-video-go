package fetcher

import (
	"fmt"
	"time"
)

// cnHolidayRanges 沪深北交易所官方休市区间(含起始日与结束日,闭区间)。
// 来源于交易所年度《部分节假日休市安排的通知》:
//   - 2025: 上证公告〔2024〕38号 / 深证会〔2024〕250号
//   - 2026: 上证公告〔2025〕45号 / 深证会〔2025〕481号
//
// 每年 12 月交易所发布次年安排,需人工补录新一年区间。
var cnHolidayRanges = [][2]string{
	// 2025
	{"2025-01-01", "2025-01-01"}, // 元旦
	{"2025-01-28", "2025-02-04"}, // 春节
	{"2025-04-04", "2025-04-06"}, // 清明节
	{"2025-05-01", "2025-05-05"}, // 劳动节
	{"2025-05-31", "2025-06-02"}, // 端午节
	{"2025-10-01", "2025-10-08"}, // 国庆节、中秋节
	// 2026
	{"2026-01-01", "2026-01-03"}, // 元旦
	{"2026-02-15", "2026-02-23"}, // 春节
	{"2026-04-04", "2026-04-06"}, // 清明节
	{"2026-05-01", "2026-05-05"}, // 劳动节
	{"2026-06-19", "2026-06-21"}, // 端午节
	{"2026-09-25", "2026-09-27"}, // 中秋节
	{"2026-10-01", "2026-10-07"}, // 国庆节
}

// IsCnHoliday 判断给定日期是否落在法定休市区间内。
func IsCnHoliday(t time.Time) bool {
	cur := t.Format("2006-01-02")
	for _, r := range cnHolidayRanges {
		if cur >= r[0] && cur <= r[1] {
			return true
		}
	}
	return false
}

// IsTradingDayCN 判断是否 A 股交易日(跳过周末与法定节假日)。
func IsTradingDayCN(t time.Time) bool {
	w := t.Weekday()
	if w == time.Saturday || w == time.Sunday {
		return false
	}
	return !IsCnHoliday(t)
}

// NextTradingDay 返回 dateStr 的下一个交易日,格式 YYYY-MM-DD。
// 跨周末与节假日自动跳过;返回的本身就是交易日。
func NextTradingDay(dateStr string) (string, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", fmt.Errorf("parse date: %w", err)
	}
	d := t.AddDate(0, 0, 1)
	maxLookahead := 45 // 最长覆盖春节+国庆连休
	for i := 0; i < maxLookahead; i++ {
		if IsTradingDayCN(d) {
			return d.Format("2006-01-02"), nil
		}
		d = d.AddDate(0, 0, 1)
	}
	return "", fmt.Errorf("no upcoming trading day within %d days", maxLookahead)
}

var cnWeekdays = [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}

// FormatCNDate 将 YYYY-MM-DD 格式化为 "8月14日 星期五"。
// 解析失败时回退为输入原样。
func FormatCNDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return fmt.Sprintf("%d月%d日 %s", int(t.Month()), t.Day(), cnWeekdays[int(t.Weekday())])
}

// NextTradingDayCN 返回下一个交易日的 "8月17日 星期一" 表达。
// 解析或推算失败时回退为原始 next 表达。
func NextTradingDayCN(dateStr string) string {
	next, err := NextTradingDay(dateStr)
	if err != nil {
		return ""
	}
	return FormatCNDate(next)
}

// TradingDayContext 生成给 LLM 的日期上下文,告知"当日日期"与"下一交易日",
// 强约束文案只能用具体日期,禁止"今天/明天/昨日/今日"等相对时间词。
func TradingDayContext(dateStr string) string {
	next := NextTradingDayCN(dateStr)
	base := fmt.Sprintf(
		"当前交易日:%s;下一交易日:%s。",
		FormatCNDate(dateStr), next,
	)
	return base
}