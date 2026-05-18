package tickscheduler

import (
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/tickfetcher"
)

type TickScheduler struct {
	mu      sync.Mutex
	enabled bool
	fetcher *tickfetcher.TickFetcher
	stopCh  chan struct{}
}

func New() *TickScheduler {
	return &TickScheduler{
		fetcher: tickfetcher.New(),
	}
}

func (s *TickScheduler) Start() {
	s.mu.Lock()
	s.enabled = true
	if s.stopCh == nil {
		s.stopCh = make(chan struct{})
	}
	s.mu.Unlock()
	go s.loop()
}

func (s *TickScheduler) Stop() {
	s.mu.Lock()
	s.enabled = false
	s.fetcher.Stop()
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
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

	now := time.Now()
	if now.Weekday() >= time.Saturday {
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
