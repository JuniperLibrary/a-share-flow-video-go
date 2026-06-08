package tick

import (
	"testing"
)

// ---- minutesToTime ----

func TestMinutesToTime_Morning(t *testing.T) {
	cases := []struct {
		min  int
		want string
	}{
		{0, "09:30"},
		{30, "10:00"},
		{60, "10:30"},
		{90, "11:00"},
		{119, "11:29"},
		{120, "13:00"},
	}
	for _, c := range cases {
		got := minutesToTime(c.min)
		if got != c.want {
			t.Errorf("minutesToTime(%d) = %q, want %q", c.min, got, c.want)
		}
	}
}

func TestMinutesToTime_Afternoon(t *testing.T) {
	cases := []struct {
		min  int
		want string
	}{
		{120, "13:00"},
		{150, "13:30"},
		{180, "14:00"},
		{210, "14:30"},
		{230, "14:50"},
		{240, "15:00"},
	}
	for _, c := range cases {
		got := minutesToTime(c.min)
		if got != c.want {
			t.Errorf("minutesToTime(%d) = %q, want %q", c.min, got, c.want)
		}
	}
}

// ---- tickSchedule (run() loop logic, pure) ----

func TestTickSchedule_FullMorningFromOpen(t *testing.T) {
	// 09:30 start, interval=10, should cover 09:30-11:30
	// Only pass morning range to isolate morning behavior
	schedule := tickSchedule(0, 10, []tradingRange{{0, 119}})
	if len(schedule) == 0 {
		t.Fatal("expected non-empty schedule")
	}

	// Morning ticks at 10-min intervals starting from 09:30
	expectedMorning := 12 // 0,10,20,...,110 = 12 ticks (120/10)
	if len(schedule) != expectedMorning {
		t.Errorf("expected %d morning ticks, got %d", expectedMorning, len(schedule))
	}

	// Last morning tick should be 11:20 (minute 110, not 11:30)
	// Because {0,119} ends at 119, and next step 120 > 119, so minute 120 (11:30) is skipped
	lastMorning := schedule[len(schedule)-1]
	if lastMorning.Time != "11:20" {
		t.Errorf("last morning tick should be 11:20, got %s", lastMorning.Time)
	}
}

func TestTickSchedule_FullAfternoonFromOpen(t *testing.T) {
	// 13:00 start (minute 120), interval=10
	schedule := tickSchedule(120, 10, []tradingRange{{120, 240}})
	if len(schedule) == 0 {
		t.Fatal("expected non-empty schedule")
	}

	// Afternoon ticks at 10-min intervals from 13:00 to 15:00
	// 120,130,...,240 = 13 ticks
	if len(schedule) != 13 {
		t.Errorf("expected 13 ticks (120→240 step 10), got %d", len(schedule))
	}

	// Last tick should be 15:00
	last := schedule[len(schedule)-1]
	if last.Time != "15:00" || last.Minute != 240 {
		t.Errorf("last afternoon tick should be 15:00 (240), got %s (%d)", last.Time, last.Minute)
	}
}

func TestTickSchedule_MidSessionStart(t *testing.T) {
	// Start at 14:00 (minute 180) with interval=5
	schedule := tickSchedule(180, 5, []tradingRange{{0, 119}, {120, 240}})
	if len(schedule) == 0 {
		t.Fatal("expected non-empty schedule")
	}

	// Should skip morning entirely (119 < 180)
	// Afternoon: start at minute 180, step 5, end at 240
	// 180,185,190,...,240 = 13 ticks
	expected := 13
	if len(schedule) != expected {
		t.Errorf("expected %d ticks from 14:00 with interval=5, got %d", expected, len(schedule))
	}

	// First tick should be 14:00
	first := schedule[0]
	if first.Time != "14:00" || first.Minute != 180 {
		t.Errorf("first tick should be 14:00 (180), got %s (%d)", first.Time, first.Minute)
	}

	// Last tick should be 15:00
	last := schedule[len(schedule)-1]
	if last.Time != "15:00" || last.Minute != 240 {
		t.Errorf("last tick should be 15:00 (240), got %s (%d)", last.Time, last.Minute)
	}
}

func TestTickSchedule_MidSessionStart_Includes1500(t *testing.T) {
	// Simulate the RACE CONDITION scenario:
	// Fetcher starts at ~14:57, interval=5
	// currentMinute = nowTradingMinute() = 237 (14:57)
	// Should collect 14:55 (catch-up), then 15:00
	schedule := tickSchedule(237, 5, []tradingRange{{0, 119}, {120, 240}})

	if len(schedule) == 0 {
		t.Fatal("expected non-empty schedule")
	}

	// First tick should be 235 = 14:55 (closest 5-min boundary at or above start)
	first := schedule[0]
	if first.Time != "14:55" || first.Minute != 235 {
		t.Errorf("first tick should be 14:55 (235), got %s (%d)", first.Time, first.Minute)
	}

	// Last tick MUST be 15:00 (240)
	last := schedule[len(schedule)-1]
	if last.Time != "15:00" || last.Minute != 240 {
		t.Errorf("last tick MUST be 15:00 (240), got %s (%d) — this is the bug pattern!", last.Time, last.Minute)
	}

	// Should have exactly 2 ticks: 14:55 and 15:00
	if len(schedule) != 2 {
		t.Errorf("expected exactly 2 ticks (14:55, 15:00), got %d: %v", len(schedule), schedule)
	}
}

func TestTickSchedule_AfternoonOnly(t *testing.T) {
	// Start during noon break (minute = -1 means outside trading hours)
	// Start at afternoon open: 13:00 (minute 120)
	schedule := tickSchedule(120, 10, []tradingRange{{0, 119}, {120, 240}})

	// Morning should be skipped (119 < 120 → continue)
	// Afternoon: 120,130,...,240 = 13 ticks
	if len(schedule) != 13 {
		t.Errorf("expected 13 afternoon ticks, got %d", len(schedule))
	}

	if schedule[0].Time != "13:00" {
		t.Errorf("first afternoon tick should be 13:00, got %s", schedule[0].Time)
	}
}

func TestTickSchedule_AfterMarketClose(t *testing.T) {
	// Start after 15:00 (minute 240+) — should produce nothing
	schedule := tickSchedule(250, 10, []tradingRange{{0, 119}, {120, 240}})
	if len(schedule) != 0 {
		t.Errorf("expected empty schedule after close, got %d ticks", len(schedule))
	}
}

func TestTickSchedule_Interval5_FullDay(t *testing.T) {
	schedule := tickSchedule(0, 5, []tradingRange{{0, 119}, {120, 240}})
	// Morning: 0,5,10,...,115 = 24 ticks (120/5)
	// Afternoon: 120,125,...,240 = 25 ticks (120/5 + 1 for 240 inclusive)
	if len(schedule) != 49 {
		t.Errorf("expected 49 ticks for full day (24 morning + 25 afternoon), got %d", len(schedule))
	}

	// Verify boundaries
	if schedule[0].Time != "09:30" {
		t.Errorf("first tick should be 09:30, got %s", schedule[0].Time)
	}
	// Morning last is minute 115 = 11:25 (119 floor to 115 with step 5)
	lastMorning := schedule[23]
	if lastMorning.Time != "11:25" {
		t.Errorf("last morning tick should be 11:25 with interval=5, got %s", lastMorning.Time)
	}
	// Afternoon last is 15:00
	last := schedule[len(schedule)-1]
	if last.Time != "15:00" {
		t.Errorf("last tick should be 15:00, got %s", last.Time)
	}
}

func TestTickSchedule_Interval1_FullDay(t *testing.T) {
	// Every minute — should cover each trading minute
	schedule := tickSchedule(0, 1, []tradingRange{{0, 119}, {120, 240}})
	// Morning: 0..119 = 120 ticks
	// Afternoon: 120..240 = 121 ticks
	if len(schedule) != 241 {
		t.Errorf("expected 241 ticks (every minute), got %d", len(schedule))
	}

	// First = 09:30, last morning = 11:29 (minute 119)
	if schedule[0].Time != "09:30" {
		t.Errorf("first should be 09:30, got %s", schedule[0].Time)
	}
	if schedule[119].Time != "11:29" {
		t.Errorf("minute 119 should be 11:29, got %s", schedule[119].Time)
	}
	// Afternoon first = 13:00
	if schedule[120].Time != "13:00" {
		t.Errorf("minute 120 should be 13:00, got %s", schedule[120].Time)
	}
	// Last = 15:00
	last := schedule[len(schedule)-1]
	if last.Time != "15:00" {
		t.Errorf("last should be 15:00, got %s", last.Time)
	}
}

// ---- 11:30 edge case (morning range extended to {0, 120}) ----

func TestTickSchedule_MorningIncludes1130(t *testing.T) {
	// 09:30 start, interval=5, range={0,120} — should include minute 120 as "11:30"
	schedule := tickSchedule(0, 5, []tradingRange{{0, 120}})
	if len(schedule) == 0 {
		t.Fatal("expected non-empty schedule")
	}

	// 0,5,10,...,120 = 25 ticks
	expected := 25
	if len(schedule) != expected {
		t.Errorf("expected %d ticks, got %d", expected, len(schedule))
	}

	// Minute 120 should map to "11:30", not "13:00", because rng.start==0
	last := schedule[len(schedule)-1]
	if last.Time != "11:30" || last.Minute != 120 {
		t.Errorf("last tick should be 11:30 (120), got %s (%d)", last.Time, last.Minute)
	}
}

func TestTickSchedule_FullDayWith1130(t *testing.T) {
	// Full day with extended morning range {0,120}
	schedule := tickSchedule(0, 5, []tradingRange{{0, 120}, {120, 240}})

	// Morning: 0,5,...,115,120 = 25 ticks (120/5 + 1 for 120 inclusive)
	// Afternoon: 120,125,...,240 = 25 ticks (120/5 + 1 for 240 inclusive)
	expected := 50
	if len(schedule) != expected {
		t.Errorf("expected %d ticks (25 morning + 25 afternoon), got %d", expected, len(schedule))
	}

	// Morning last = minute 120 at "11:30"
	lastMorning := schedule[24]
	if lastMorning.Time != "11:30" || lastMorning.Minute != 120 {
		t.Errorf("last morning tick should be 11:30 (120), got %s (%d)", lastMorning.Time, lastMorning.Minute)
	}

	// Afternoon first = minute 120 at "13:00" (rng.start==120, no special case)
	firstAfternoon := schedule[25]
	if firstAfternoon.Time != "13:00" || firstAfternoon.Minute != 120 {
		t.Errorf("first afternoon tick should be 13:00 (120), got %s (%d)", firstAfternoon.Time, firstAfternoon.Minute)
	}

	// Last tick = 15:00
	last := schedule[len(schedule)-1]
	if last.Time != "15:00" {
		t.Errorf("last tick should be 15:00, got %s", last.Time)
	}
}

func TestTickSchedule_1130AfternoonDedup(t *testing.T) {
	// Start at 13:00 (minute 120) with extended morning range {0,120}
	// Morning {0,120}: 120 <= 120 → NOT skipped. Minute 120 mapped to "11:30".
	// Afternoon {120,240}: 120 <= 240 → processed. Minute 120 mapped to "13:00".
	schedule := tickSchedule(120, 10, []tradingRange{{0, 120}, {120, 240}})

	// Morning produces 1 tick: minute 120 → "11:30"
	// Afternoon: 120,130,...,240 = 13 ticks starting with 120 → "13:00"
	// Total = 1 + 13 = 14
	expected := 14
	if len(schedule) != expected {
		t.Errorf("expected %d ticks (1 morning 11:30 + 13 afternoon), got %d", expected, len(schedule))
	}

	// First tick = 11:30 (morning range handles minute 120 first)
	first := schedule[0]
	if first.Time != "11:30" || first.Minute != 120 {
		t.Errorf("first tick should be 11:30 (120), got %s (%d)", first.Time, first.Minute)
	}

	// Second tick = 13:00 (afternoon range handles minute 120 with original mapping)
	second := schedule[1]
	if second.Time != "13:00" || second.Minute != 120 {
		t.Errorf("second tick should be 13:00 (120), got %s (%d)", second.Time, second.Minute)
	}
}

// ---- nowTradingMinute wall clock simulation ----

// TestMinutesToTime_RangeBounds 验证 minutesToTime 作为纯时间转换函数的行为。
// 注意: minute 120 在 minutesToTime 中映射为 "13:00" (as 公式行为)。
// tickSchedule() 和 run() 中的 special case 会将其改写为 "11:30" (当 rng.start==0 时)。
func TestMinutesToTime_RangeBounds(t *testing.T) {
	// Verify the morning range {0,119} ends at 11:29, NOT 11:30
	morningEnd := minutesToTime(119)
	if morningEnd != "11:29" {
		t.Errorf("minute 119 should be 11:29, got %s", morningEnd)
	}

	// Minute 120 = 13:00 (start of afternoon) — pure formula, no context
	afternoonStart := minutesToTime(120)
	if afternoonStart != "13:00" {
		t.Errorf("minute 120 should be 13:00, got %s", afternoonStart)
	}

	// Afternoon ends at 15:00 = minute 240
	afternoonEnd := minutesToTime(240)
	if afternoonEnd != "15:00" {
		t.Errorf("minute 240 should be 15:00, got %s", afternoonEnd)
	}
}
