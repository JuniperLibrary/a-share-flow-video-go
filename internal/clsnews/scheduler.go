package clsnews

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

// NewsScheduler 财联社新闻轮询调度器。
// 交易时段每 30s 拉取一次，非交易时段每 5min 拉取一次。
type NewsScheduler struct {
	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	lastTime  int64           // 上次拉取到的最新时间戳
	totalNews int             // 累计拉取的新闻数
	lastPoll  time.Time       // 上次轮询时间
	lastCount int             // 上次轮询新增条数
}

func NewNewsScheduler() *NewsScheduler {
	return &NewsScheduler{}
}

func (s *NewsScheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go s.poll()

	go s.loop()
	logger.Info("财联社新闻轮询已启动")
}

func (s *NewsScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	close(s.stopCh)
	s.running = false
	logger.Info("财联社新闻轮询已停止")
}

func (s *NewsScheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Status 返回调度器状态。
func (s *NewsScheduler) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := "running"
	if !s.running {
		status = "stopped"
	}
	return map[string]any{
		"status":     status,
		"total_news": s.totalNews,
		"last_poll":  s.lastPoll.Format(time.RFC3339),
		"last_count": s.lastCount,
	}
}

func (s *NewsScheduler) loop() {
	for {
		interval := GetPollInterval()
		select {
		case <-s.stopCh:
			return
		case <-time.After(interval):
			s.poll()
		}
	}
}

func (s *NewsScheduler) poll() {
	news, err := FetchTelegraphList(s.lastTime)
	if err != nil {
		logger.Warn("财联社电报拉取失败", zap.Error(err))
		return
	}

	if len(news) == 0 {
		s.mu.Lock()
		s.lastPoll = time.Now()
		s.lastCount = 0
		s.mu.Unlock()
		return
	}

	// 匹配板块标签
	MatchSectorsToNews(news)

	// 更新最新时间戳
	maxTime := ExtractMaxCTime(news)
	if maxTime > s.lastTime {
		s.lastTime = maxTime
		logger.Debug("财联社最新时间戳", zap.Int64("lastTime", maxTime))
	}

	// 转 storage 记录并保存
	db, err := storage.Get()
	if err != nil {
		logger.Warn("数据库连接失败", zap.Error(err))
		return
	}

	records := make([]storage.CLSNewsRecord, 0, len(news))
	for _, n := range news {
		sectorsJSON, _ := json.Marshal(n.Sectors)
		records = append(records, storage.CLSNewsRecord{
			ID:         n.ID,
			Title:      n.Title,
			Content:    n.Content,
			Brief:      n.Brief,
			Level:      n.Level,
			ReadingNum: n.ReadingNum,
			CTime:      n.CTime.Format("2006-01-02 15:04:05"),
			ShareURL:   n.ShareURL,
			Sectors:    string(sectorsJSON),
		})
	}

	saved, err := db.SaveCLSNews(records)
	if err != nil {
		logger.Warn("保存新闻失败", zap.Error(err))
		return
	}

	s.mu.Lock()
	s.totalNews += saved
	s.lastPoll = time.Now()
	s.lastCount = saved
	s.mu.Unlock()

	if saved > 0 && config.DataMode() == "json" {
		if db, err := storage.Get(); err == nil {
			db.ExportJSON()
		}
		DumpNews(news[:min(saved, len(news))])
		msg := fmt.Sprintf("📰 新增 %d 条财联社电报", saved)
		logger.Info(msg, zap.Int64("lastTime", s.lastTime))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
