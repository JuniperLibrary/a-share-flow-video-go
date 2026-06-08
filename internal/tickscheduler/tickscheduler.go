package tickscheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/tickfetcher"
)

var shanghaiTZ = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}()

type TickScheduler struct {
	mu      sync.Mutex
	enabled bool
	fetcher *tickfetcher.TickFetcher
	stopCh  chan struct{}
	running bool
}

func New() *TickScheduler {
	s := &TickScheduler{
		fetcher: tickfetcher.New(),
		stopCh:  make(chan struct{}),
	}
	return s
}

func (s *TickScheduler) Start() {
	s.mu.Lock()
	s.enabled = true
	shouldStart := !s.running
	s.running = true
	s.mu.Unlock()

	if shouldStart {
		go s.loop()
	}
}

func (s *TickScheduler) Stop() {
	s.mu.Lock()
	s.enabled = false
	s.fetcher.Stop()
	s.mu.Unlock()
}

func (s *TickScheduler) Shutdown() {
	s.mu.Lock()
	s.enabled = false
	s.fetcher.Stop()
	close(s.stopCh)
	s.running = false
	s.mu.Unlock()
}

func (s *TickScheduler) loop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkAndRun()
		}
	}
}

func (s *TickScheduler) checkAndRun() {
	s.mu.Lock()
	enabled := s.enabled
	s.mu.Unlock()

	if !enabled {
		return
	}

	now := time.Now().In(shanghaiTZ)
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return
	}

	if s.shouldStop(now) {
		s.fetcher.Stop()
		return
	}

	if s.shouldRun(now) && !s.fetcher.IsRunning() {
		s.fetcher.Start()
	}
}

func isTradingSession(h, m int) bool {
	min := h*60 + m
	morningPreStart := 9*60 + 25
	morningEnd := 11*60 + 30
	afternoonPreStart := 12*60 + 55
	afternoonEnd := 15 * 60

	inMorning := min >= morningPreStart && min <= morningEnd
	inAfternoon := min >= afternoonPreStart && min <= afternoonEnd
	return inMorning || inAfternoon
}

func (s *TickScheduler) shouldRun(now time.Time) bool {
	h, m := now.Hour(), now.Minute()
	return isTradingSession(h, m)
}

func (s *TickScheduler) shouldStop(now time.Time) bool {
	h, m := now.Hour(), now.Minute()
	min := h*60 + m

	if min >= 11*60+31 && min < 12*60+55 {
		return true
	}
	if min >= 15*60+6 {
		return true
	}
	return false
}

func (s *TickScheduler) StartManual() error {
	now := time.Now().In(shanghaiTZ)
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return fmt.Errorf("今天是周末，非交易日")
	}
	s.mu.Lock()
	s.enabled = true
	s.mu.Unlock()
	return s.fetcher.Start()
}

func (s *TickScheduler) StopManual() {
	s.fetcher.Stop()
}

func (s *TickScheduler) GetStatus() map[string]any {
	s.mu.Lock()
	enabled := s.enabled
	s.mu.Unlock()

	status := s.fetcher.GetStatus()
	status["enabled"] = enabled
	return status
}

func (s *TickScheduler) SetEnabled(on bool) {
	s.mu.Lock()
	s.enabled = on
	s.mu.Unlock()
}

func (s *TickScheduler) GetFetcher() *tickfetcher.TickFetcher {
	return s.fetcher
}
