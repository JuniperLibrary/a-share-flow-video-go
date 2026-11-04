package fetcher

import (
	"testing"
	"time"
)

func TestIsTradingDayCN_Weekday(t *testing.T) {
	if !IsTradingDayCN(mustDate(t, "2026-08-14")) { // 星期五
		t.Error("2026-08-14 应为交易日")
	}
	if IsTradingDayCN(mustDate(t, "2026-08-15")) { // 星期六
		t.Error("2026-08-15 周六不应为交易日")
	}
	if IsTradingDayCN(mustDate(t, "2026-08-16")) { // 星期日
		t.Error("2026-08-16 周日不应为交易日")
	}
}

func TestIsTradingDayCN_Holidays(t *testing.T) {
	// 2026 春节休市区间 2/15-2/23,其中 2/16 周一应休市
	if IsTradingDayCN(mustDate(t, "2026-02-16")) {
		t.Error("2026-02-16 春节休市不应为交易日")
	}
	if !IsTradingDayCN(mustDate(t, "2026-02-24")) {
		t.Error("2026-02-24 节后首日应为交易日")
	}
	// 2025 国庆中秋 10/1-10/8,其中 10/8 周三应休市
	if IsTradingDayCN(mustDate(t, "2025-10-08")) {
		t.Error("2025-10-08 国庆休市不应为交易日")
	}
}

func TestNextTradingDay_CrossWeekend(t *testing.T) {
	// 2026-08-14 (周五) -> 下个交易日是 2026-08-17 (周一)
	got, err := NextTradingDay("2026-08-14")
	want := "2026-08-17"
	if err != nil {
		t.Fatalf("NextTradingDay: %v", err)
	}
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestNextTradingDay_CrossSpringFestival(t *testing.T) {
	// 2026-02-13 (周五) -> 跳过春节(2/15-2/23) -> 2026-02-24 (周二)
	got, err := NextTradingDay("2026-02-13")
	want := "2026-02-24"
	if err != nil {
		t.Fatalf("NextTradingDay: %v", err)
	}
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestFormatCNDate(t *testing.T) {
	if got := FormatCNDate("2026-08-14"); got != "8月14日 星期五" {
		t.Errorf("got %s", got)
	}
	if got := FormatCNDate("2026-08-17"); got != "8月17日 星期一" {
		t.Errorf("got %s", got)
	}
	if got := FormatCNDate("bad-date"); got != "bad-date" {
		t.Errorf("invalid should stay raw, got %s", got)
	}
}

func TestNextTradingDayCN(t *testing.T) {
	if got := NextTradingDayCN("2026-08-14"); got != "8月17日 星期一" {
		t.Errorf("got %s", got)
	}
}

func TestTradingDayContext(t *testing.T) {
	got := TradingDayContext("2026-08-14")
	want := "当前交易日:8月14日 星期五;下一交易日:8月17日 星期一。"
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return tm
}