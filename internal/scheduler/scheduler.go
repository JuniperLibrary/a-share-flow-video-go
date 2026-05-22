// Package scheduler 提供定时视频生成调度器。
// 默认每天 15:05（收盘后5分钟）自动执行：拉取数据 → 分析 → Remotion 渲染 → 保存文案。
// 跳过周末，每日仅执行一次，支持手动 RunNow 触发。
package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/copy"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/renderer"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

// Scheduler 定时调度器，负责在指定时间自动执行视频生成管道。
// Enabled: 是否启用定时任务 | RunTime: 全天执行时间(HH:MM) | MorningRunTime: 早盘执行时间(HH:MM)
// mu: 保护并发访问 | lastRun: 上次全天执行时间 | lastMorningRun: 上次早盘执行时间
// isRunning: 防止重复执行 | stopCh: 用于停止轮询循环
type Scheduler struct {
	Enabled        bool
	RunTime        string
	MorningRunTime string
	mu             sync.Mutex
	lastRun        time.Time
	lastMorningRun time.Time
	lastStatus     string
	isRunning      bool
	stopCh         chan struct{}
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		RunTime:        "15:05",
		MorningRunTime: "11:35",
		stopCh:         make(chan struct{}),
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
	go s.execute("full")
}

func (s *Scheduler) GetStatus() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	var nextRun, nextMorningRun string
	if s.Enabled {
		now := time.Now()
		h, m := parseTime(s.MorningRunTime)
		morningTarget := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
		if morningTarget.Before(now) {
			ly, lm, ld := s.lastMorningRun.Date()
			ny, nm, nd := now.Date()
			if ly == ny && lm == nm && ld == nd {
				morningTarget = morningTarget.AddDate(0, 0, 1)
			}
		}
		nextMorningRun = morningTarget.Format(time.RFC3339)

		h, m = parseTime(s.RunTime)
		fullTarget := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
		if fullTarget.Before(now) {
			ly, lm, ld := s.lastRun.Date()
			ny, nm, nd := now.Date()
			if ly == ny && lm == nm && ld == nd {
				fullTarget = fullTarget.AddDate(0, 0, 1)
			}
		}
		nextRun = fullTarget.Format(time.RFC3339)
	}

	var lastRunStr, lastMorningRunStr string
	if !s.lastRun.IsZero() {
		lastRunStr = s.lastRun.Format(time.RFC3339)
	}
	if !s.lastMorningRun.IsZero() {
		lastMorningRunStr = s.lastMorningRun.Format(time.RFC3339)
	}

	return map[string]any{
		"enabled":          s.Enabled,
		"run_time":         s.RunTime,
		"morning_run_time": s.MorningRunTime,
		"last_run":         lastRunStr,
		"last_morning_run": lastMorningRunStr,
		"last_status":      s.lastStatus,
		"next_run":         nextRun,
		"next_morning_run": nextMorningRun,
		"is_running":       s.isRunning,
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
			if session := s.shouldRun(); session != "" {
				s.execute(session)
			}
		}
	}
}

// shouldRun 检查是否满足执行条件，返回要执行的 session（"morning"/"full"）或空字符串。
func (s *Scheduler) shouldRun() string {
	s.mu.Lock()
	enabled := s.Enabled
	running := s.isRunning
	lastRun := s.lastRun
	lastMorningRun := s.lastMorningRun
	runTime := s.RunTime
	morningRunTime := s.MorningRunTime
	s.mu.Unlock()

	if !enabled || running {
		return ""
	}
	now := time.Now()
	if now.Weekday() >= time.Saturday {
		return ""
	}
	ly, lm, ld := now.Date()

	// 早盘检查
	h, m := parseTime(morningRunTime)
	morningTarget := time.Date(ly, lm, ld, h, m, 0, 0, now.Location())
	if (now.After(morningTarget) || now.Equal(morningTarget)) &&
		(lastMorningRun.IsZero() || !sameDay(lastMorningRun, now)) {
		return "morning"
	}

	// 全天检查
	h, m = parseTime(runTime)
	fullTarget := time.Date(ly, lm, ld, h, m, 0, 0, now.Location())
	if (now.After(fullTarget) || now.Equal(fullTarget)) &&
		(lastRun.IsZero() || !sameDay(lastRun, now)) {
		return "full"
	}

	return ""
}

func sameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// execute 执行完整的视频生成管道：
// 数据拉取 → 保存CSV → 事件分析 → Remotion渲染(mobile+tv) → 文案生成(模板+AI)
func (s *Scheduler) execute(session string) {
	s.mu.Lock()
	s.isRunning = true
	s.lastStatus = ""
	s.mu.Unlock()

	todayStr := time.Now().Format("2006-01-02")
	sessCfg := config.SessionConfigs[session]
	logger.Info("scheduler 启动", zap.String("session", sessCfg.TitleSuffix), zap.String("date", todayStr))

	defer func() {
		s.mu.Lock()
		if session == "morning" {
			s.lastMorningRun = time.Now()
		} else {
			s.lastRun = time.Now()
		}
		s.isRunning = false
		s.mu.Unlock()
	}()

	sectors, err := fetcher.FetchTop21HotSectors()
	if err != nil || len(sectors) == 0 {
		s.mu.Lock()
		s.lastStatus = fmt.Sprintf("error: %v", err)
		s.mu.Unlock()
		logger.Error("scheduler 获取数据失败", zap.Error(err))
		return
	}

	if err := fetcher.SaveDailyData(sectors, todayStr); err != nil {
		logger.Error("scheduler 保存数据失败", zap.Error(err))
	}

	allSectors, _ := fetcher.FetchAllRaw()

	outputDir := config.GetOutputDir()
	os.MkdirAll(filepath.Join(outputDir, todayStr), 0755)

	events, timeline, ticker := analyzer.AnalyzeAllContent(allSectors, todayStr, session)
	if len(events) == 0 {
		events = analyzer.GetFallbackEvents(config.TotalFrames)
	}

	outPath := filepath.Join(outputDir, todayStr, fmt.Sprintf("%s.mp4", sessCfg.FilenameSuffix))
	if _, err := renderer.RenderVideo(sectors, todayStr, outPath, events, timeline, ticker, "tv", session); err != nil {
		logger.Error("scheduler 渲染失败", zap.String("session", sessCfg.TitleSuffix), zap.Error(err))
		s.mu.Lock()
		s.lastStatus = fmt.Sprintf("error: render %s: %v", sessCfg.TitleSuffix, err)
		s.mu.Unlock()
		return
	}

	aiCfg := config.GetAIConfig()
	if aiCfg.APIKey != "" {
		aiText, err := copy.GenerateCopywritingAI(sectors, todayStr, session)
		if err != nil {
			logger.Warn("scheduler AI文案生成失败", zap.String("session", sessCfg.TitleSuffix), zap.Error(err))
			tplCopy := copy.GenerateCopywriting(sectors, todayStr, session)
			if db, err := storage.Get(); err == nil {
				_ = db.SaveCopywriting(storage.Copywriting{
					Date:    todayStr,
					Session: session,
					Type:    "template",
					Content: tplCopy,
				})
			}
		} else {
			if db, err := storage.Get(); err == nil {
				_ = db.SaveCopywriting(storage.Copywriting{
					Date:    todayStr,
					Session: session,
					Type:    "ai",
					Content: aiText,
				})
			}
		}
	} else {
		tplCopy := copy.GenerateCopywriting(sectors, todayStr, session)
		if db, err := storage.Get(); err == nil {
			_ = db.SaveCopywriting(storage.Copywriting{
				Date:    todayStr,
				Session: session,
				Type:    "template",
				Content: tplCopy,
			})
		}
	}

	s.mu.Lock()
	s.lastStatus = "success"
	s.mu.Unlock()
	logger.Info("scheduler 完成", zap.String("date", todayStr))
}

func parseTime(s string) (int, int) {
	var h, m int
	fmt.Sscanf(s, "%d:%d", &h, &m)
	return h, m
}
