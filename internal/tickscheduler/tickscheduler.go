package tickscheduler

import (
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/tickfetcher"
)

// shanghaiTZ is the Asia/Shanghai timezone, matching tickfetcher.
var shanghaiTZ = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}()

type TickScheduler struct {
	mu       sync.Mutex
	enabled  bool
	fetcher  *tickfetcher.TickFetcher
	stopCh   chan struct{}
	running  bool
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
	if now.Weekday() >= time.Saturday {
		return
	}

	// 15:00 之后自动关闭采集
	if s.shouldStop(now) {
		s.fetcher.Stop()
		return
	}

	if s.shouldRun(now) {
		s.fetcher.Start()
	}
}

func (s *TickScheduler) shouldRun(now time.Time) bool {
	h, m := now.Hour(), now.Minute()
	currentMin := h*60 + m

	morningStart := 9*60 + 28
	fullStart := 12*60 + 58

	if currentMin >= morningStart && currentMin < morningStart+2 {
		return true
	}
	if currentMin >= fullStart && currentMin < fullStart+2 {
		return true
	}
	return false
}

func (s *TickScheduler) shouldStop(now time.Time) bool {
	h, m := now.Hour(), now.Minute()
	currentMin := h*60 + m

	// 11:31 - 12:57 停止早盘采集
	if currentMin >= 11*60+31 && currentMin < 12*60+58 {
		return true
	}
	// 15:01 之后停止全天采集
	if currentMin >= 15*60+1 {
		return true
	}
	return false
}

func (s *TickScheduler) StartManual() error {
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
