package tickscheduler

import (
	"testing"
	"time"
)

func shanghai(h, m int) time.Time {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	return time.Date(2026, 5, 25, h, m, 0, 0, loc)
}

// ---- isTradingSession ----

func TestIsTradingSession_BeforeOpen(t *testing.T) {
	if isTradingSession(9, 24) {
		t.Error("09:24 should NOT be trading session")
	}
}

func TestIsTradingSession_MorningOpen(t *testing.T) {
	if !isTradingSession(9, 25) {
		t.Error("09:25 should be trading session (pre-start)")
	}
}

func TestIsTradingSession_MorningNormal(t *testing.T) {
	if !isTradingSession(10, 30) {
		t.Error("10:30 should be trading session")
	}
}

func TestIsTradingSession_MorningEnd(t *testing.T) {
	if !isTradingSession(11, 30) {
		t.Error("11:30 should be trading session (inclusive)")
	}
}

func TestIsTradingSession_NoonBreak(t *testing.T) {
	if isTradingSession(12, 0) {
		t.Error("12:00 should NOT be trading session (noon break)")
	}
	if isTradingSession(12, 30) {
		t.Error("12:30 should NOT be trading session (noon break)")
	}
}

func TestIsTradingSession_NoonBeforeAfternoon(t *testing.T) {
	if isTradingSession(12, 54) {
		t.Error("12:54 should NOT be trading session")
	}
}

func TestIsTradingSession_AfternoonPreStart(t *testing.T) {
	if !isTradingSession(12, 55) {
		t.Error("12:55 should be trading session (pre-start)")
	}
}

func TestIsTradingSession_AfternoonOpen(t *testing.T) {
	if !isTradingSession(13, 0) {
		t.Error("13:00 should be trading session")
	}
}

func TestIsTradingSession_AfternoonEnd(t *testing.T) {
	if !isTradingSession(15, 0) {
		t.Error("15:00 should be trading session (inclusive)")
	}
}

func TestIsTradingSession_AfterClose(t *testing.T) {
	if isTradingSession(15, 1) {
		t.Error("15:01 should NOT be trading session")
	}
}

func TestIsTradingSession_WeekendMidday(t *testing.T) {
	// isTradingSession is time-agnostic about weekday, pure h/m check
	if isTradingSession(3, 0) {
		t.Error("03:00 should NOT be trading session")
	}
}

// ---- shouldRun ----

func TestShouldRun_BeforeOpen(t *testing.T) {
	s := &TickScheduler{}
	if s.shouldRun(shanghai(9, 24)) {
		t.Error("shouldRun should be false at 09:24")
	}
}

func TestShouldRun_AtOpen(t *testing.T) {
	s := &TickScheduler{}
	if !s.shouldRun(shanghai(9, 25)) {
		t.Error("shouldRun should be true at 09:25")
	}
}

func TestShouldRun_AtClose(t *testing.T) {
	s := &TickScheduler{}
	if !s.shouldRun(shanghai(15, 0)) {
		t.Error("shouldRun should be true at 15:00")
	}
}

func TestShouldRun_AfterClose(t *testing.T) {
	s := &TickScheduler{}
	if s.shouldRun(shanghai(15, 1)) {
		t.Error("shouldRun should be false at 15:01")
	}
}

// ---- shouldStop ----

func TestShouldStop_MorningNormal(t *testing.T) {
	s := &TickScheduler{}
	if s.shouldStop(shanghai(10, 30)) {
		t.Error("should NOT stop at 10:30")
	}
}

func TestShouldStop_NoonBreakStart(t *testing.T) {
	s := &TickScheduler{}
	if !s.shouldStop(shanghai(11, 31)) {
		t.Error("should stop at 11:31 (noon break start)")
	}
}

func TestShouldStop_NoonBreakMiddle(t *testing.T) {
	s := &TickScheduler{}
	if !s.shouldStop(shanghai(12, 0)) {
		t.Error("should stop at 12:00 (noon break)")
	}
	if !s.shouldStop(shanghai(12, 30)) {
		t.Error("should stop at 12:30 (noon break)")
	}
}

func TestShouldStop_NoonBreakEnd(t *testing.T) {
	s := &TickScheduler{}
	if s.shouldStop(shanghai(12, 55)) {
		t.Error("should NOT stop at 12:55 (afternoon pre-start)")
	}
}

func TestShouldStop_AfternoonNormal(t *testing.T) {
	s := &TickScheduler{}
	if s.shouldStop(shanghai(14, 30)) {
		t.Error("should NOT stop at 14:30")
	}
}

func TestShouldStop_AtClose(t *testing.T) {
	s := &TickScheduler{}
	// 15:00 is still trading — should NOT stop
	if s.shouldStop(shanghai(15, 0)) {
		t.Error("should NOT stop at 15:00 — fetcher may be collecting 15:00 tick")
	}
}

func TestShouldStop_AfterCloseBuffer(t *testing.T) {
	s := &TickScheduler{}
	// 15:01 to 15:05 — grace period for last interval
	if s.shouldStop(shanghai(15, 1)) {
		t.Error("should NOT stop at 15:01 — 5min interval may still be in progress")
	}
	if s.shouldStop(shanghai(15, 5)) {
		t.Error("should NOT stop at 15:05 — buffer for last tick + interval")
	}
}

func TestShouldStop_AfterCloseFinal(t *testing.T) {
	s := &TickScheduler{}
	// 15:06 — enough time for last tick + 5min interval + buffer
	if !s.shouldStop(shanghai(15, 6)) {
		t.Error("should stop at 15:06")
	}
	if !s.shouldStop(shanghai(16, 0)) {
		t.Error("should stop at 16:00")
	}
}

// ---- Full lifecycle simulation ----

func TestScheduleLifecycle_Weekday(t *testing.T) {
	s := &TickScheduler{}

	type check struct {
		h, m   int
		run    bool
		stop   bool
		session bool
	}

	cases := []check{
		// Pre-market
		{9, 24, false, false, false},
		// Morning pre-start
		{9, 25, true, false, true},
		// Mid-morning
		{10, 30, true, false, true},
		// Morning close (inclusive)
		{11, 30, true, false, true},
		// Noon break starts
		{11, 31, false, true, false},
		// Mid-noon
		{12, 0, false, true, false},
		{12, 30, false, true, false},
		{12, 54, false, true, false},
		// Afternoon pre-start
		{12, 55, true, false, true},
		// Afternoon open
		{13, 0, true, false, true},
		// Mid-afternoon
		{14, 30, true, false, true},
		// Market close (inclusive)
		{15, 0, true, false, true},
		// Grace period — fetcher may still be collecting last tick
		{15, 1, false, false, false},
		{15, 5, false, false, false},
		// After grace period — should stop
		{15, 6, false, true, false},
		{16, 0, false, true, false},
	}

	for _, c := range cases {
		now := shanghai(c.h, c.m)
		gotRun := s.shouldRun(now)
		gotStop := s.shouldStop(now)
		gotSession := isTradingSession(c.h, c.m)

		if gotRun != c.run {
			t.Errorf("%02d:%02d shouldRun=%v, want %v", c.h, c.m, gotRun, c.run)
		}
		if gotStop != c.stop {
			t.Errorf("%02d:%02d shouldStop=%v, want %v", c.h, c.m, gotStop, c.stop)
		}
		if gotSession != c.session {
			t.Errorf("%02d:%02d isTradingSession=%v, want %v", c.h, c.m, gotSession, c.session)
		}
	}
}
