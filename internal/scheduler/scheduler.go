// Package scheduler 提供定时视频生成调度器。
// 默认每天 15:05（收盘后5分钟）自动执行：拉取数据 → 分析 → Remotion 渲染 → 保存文案。
// 跳过周末，每日仅执行一次，支持手动 RunNow 触发。
package scheduler

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/copy"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/renderer"
)

// Scheduler 定时调度器，负责在指定时间自动执行视频生成管道。
// Enabled: 是否启用定时任务 | RunTime: 执行时间(HH:MM)
// mu: 保护并发访问 | lastRun: 上次执行时间 | isRunning: 防止重复执行
// stopCh: 用于停止轮询循环
type Scheduler struct {
	Enabled    bool
	RunTime    string
	mu         sync.Mutex
	lastRun    time.Time
	lastStatus string
	isRunning  bool
	stopCh     chan struct{}
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		RunTime: "15:05",
		stopCh:  make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopCh == nil {
		s.stopCh = make(chan struct{})
	}
	go s.loop()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.stopCh)
	s.stopCh = nil
}

func (s *Scheduler) RunNow() {
	go s.execute()
}

func (s *Scheduler) GetStatus() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	var nextRun string
	if s.Enabled {
		h, m := parseTime(s.RunTime)
		now := time.Now()
		target := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
		if target.Before(now) {
			ly, lm, ld := s.lastRun.Date()
			ny, nm, nd := now.Date()
			if ly == ny && lm == nm && ld == nd {
				target = target.AddDate(0, 0, 1)
			}
		}
		nextRun = target.Format(time.RFC3339)
	}

	var lastRunStr string
	if !s.lastRun.IsZero() {
		lastRunStr = s.lastRun.Format(time.RFC3339)
	}

	return map[string]any{
		"enabled":     s.Enabled,
		"run_time":    s.RunTime,
		"last_run":    lastRunStr,
		"last_status": s.lastStatus,
		"next_run":    nextRun,
		"is_running":  s.isRunning,
	}
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if s.shouldRun() {
				s.execute()
			}
		}
	}
}

// shouldRun 检查是否满足执行条件（6个检查点）：
// 1. 定时任务已启用  2. 当前未在运行  3. 非周末  4. 今日尚未执行  5. 当前时间 >= 设定时间
func (s *Scheduler) shouldRun() bool {
	s.mu.Lock()
	enabled := s.Enabled
	running := s.isRunning
	lastRun := s.lastRun
	runTime := s.RunTime
	s.mu.Unlock()

	if !enabled || running {
		return false
	}
	now := time.Now()
	if now.Weekday() >= time.Saturday {
		return false
	}
	if !lastRun.IsZero() {
		ly, lm, ld := lastRun.Date()
		ny, nm, nd := now.Date()
		if ly == ny && lm == nm && ld == nd {
			return false
		}
	}

	h, m := parseTime(runTime)
	target := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	return now.After(target) || now.Equal(target)
}

// execute 执行完整的视频生成管道：
// 数据拉取 → 保存CSV → 事件分析 → Remotion渲染(mobile+tv) → 文案生成(模板+AI)
func (s *Scheduler) execute() {
	s.mu.Lock()
	s.isRunning = true
	s.lastStatus = ""
	s.mu.Unlock()

	todayStr := time.Now().Format("2006-01-02")
	log.Printf("Scheduler: starting pipeline for %s", todayStr)

	defer func() {
		s.mu.Lock()
		s.lastRun = time.Now()
		s.isRunning = false
		s.mu.Unlock()
	}()

	sectors, err := fetcher.FetchTop15HotSectors()
	if err != nil || len(sectors) == 0 {
		s.mu.Lock()
		s.lastStatus = fmt.Sprintf("error: %v", err)
		s.mu.Unlock()
		log.Printf("Scheduler: failed to fetch sectors: %v", err)
		return
	}

	if err := fetcher.SaveDailyData(sectors, todayStr); err != nil {
		log.Printf("Scheduler: failed to save data: %v", err)
	}

	allSectors, _ := fetcher.FetchAllRaw()
	events, timeline, ticker := analyzer.AnalyzeAllContent(allSectors, todayStr)
	if len(events) == 0 {
		events = analyzer.GetFallbackEvents(config.TotalFrames)
	}

	outputDir := config.GetOutputDir()
	os.MkdirAll(filepath.Join(outputDir, todayStr), 0755)

	for _, format := range []string{"mobile", "tv"} {
		suffix := ""
		if format == "tv" {
			suffix = "_tv"
		}
		outPath := filepath.Join(outputDir, todayStr, fmt.Sprintf("全天%s.mp4", suffix))
		if _, err := renderer.RenderVideo(sectors, todayStr, outPath, events, timeline, ticker, format); err != nil {
			log.Printf("Scheduler: render failed (%s): %v", format, err)
			s.mu.Lock()
			s.lastStatus = fmt.Sprintf("error: render %s: %v", format, err)
			s.mu.Unlock()
			return
		}
	}

	copyDir := filepath.Join(config.GetCopyDir(), todayStr)
	os.MkdirAll(copyDir, 0755)

	for _, sess := range []string{"full", "morning", "afternoon"} {
		sessCfg := config.SessionConfigs[sess]
		tplCopy := copy.GenerateCopywriting(sectors, todayStr, sess)
		if err := os.WriteFile(filepath.Join(copyDir, fmt.Sprintf("文案_%s.txt", sessCfg.TitleSuffix)), []byte(tplCopy), 0644); err != nil {
			log.Printf("Scheduler: failed to save template copy (%s): %v", sessCfg.TitleSuffix, err)
		}
	}

	aiCfg := config.GetAIConfig()
	if aiCfg.APIKey != "" {
		for _, sess := range []string{"full", "morning", "afternoon"} {
			sessCfg := config.SessionConfigs[sess]
			aiText, err := copy.GenerateCopywritingAI(sectors, todayStr, sess)
			if err != nil {
				log.Printf("Scheduler: AI copy failed (%s): %v", sessCfg.TitleSuffix, err)
			} else {
				os.WriteFile(filepath.Join(copyDir, fmt.Sprintf("文案_ai_%s.txt", sessCfg.TitleSuffix)), []byte(aiText), 0644)
			}
		}
	}

	s.mu.Lock()
	s.lastStatus = "success"
	s.mu.Unlock()
	log.Printf("Scheduler: pipeline complete for %s", todayStr)
}

func parseTime(s string) (int, int) {
	var h, m int
	fmt.Sscanf(s, "%d:%d", &h, &m)
	return h, m
}
