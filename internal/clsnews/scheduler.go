package clsnews

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

const classifyQueueSize = 200

// NewsScheduler 财联社新闻调度器，支持自动轮询和手动回放两种模式。
// 自动轮询间隔由 GetPollInterval 决定（交易时段 30s、非交易时段 5min），手动回放由前端按钮触发。
// AI 标签分类通过队列+worker 异步处理：poll() 仅负责拉取和入队，
// worker goroutine 逐条调用 AI 并保存结果。
type NewsScheduler struct {
	mu          sync.Mutex
	running     bool
	stopCh      chan struct{}
	lastTime    int64     // 上次拉取到的最新时间戳
	totalNews   int       // 累计保存的新闻数（worker 更新）
	lastPoll    time.Time // 上次轮询时间
	lastCount   int       // 上次轮询入队条数

	classifier    *AINewsClassifier
	classifierOnce sync.Once
	classifyQueue chan CLSNews // 待分类新闻队列（异步）
	workerWg      sync.WaitGroup
}

func NewNewsScheduler() *NewsScheduler {
	return &NewsScheduler{}
}

// Start 启动后台自动轮询和 AI 分类 worker。
// 自动轮询间隔由 GetPollInterval 决定（交易时段 30s、非交易时段 5min）。
// AI 分类 worker 从队列中逐条消费，单条调用大模型。
func (s *NewsScheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.classifyQueue = make(chan CLSNews, classifyQueueSize)
	s.mu.Unlock()

	go s.classifyWorker()
	go s.loop()
	go s.pollOnce()
	logger.Info("财联社新闻自动轮询已启动（交易时段30s/非交易时段5min），AI 分类队列", zap.Int("queueSize", classifyQueueSize))
}

// Stop 停止后台自动轮询和 AI 分类 worker。
// 等待当前正在分类的新闻完成（队列中未处理的丢弃）。
func (s *NewsScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	s.workerWg.Wait()
	logger.Info("财联社新闻轮询已停止，分类 worker 已退出")
}

// IsRunning 返回自动轮询是否运行中。
func (s *NewsScheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// PollOnce 手动触发一次拉取→匹配→保存，返回本次新增的匹配新闻条数。
func (s *NewsScheduler) PollOnce() int {
	s.poll()
	s.mu.Lock()
	count := s.lastCount
	s.mu.Unlock()
	return count
}

// Status 返回调度器状态。
func (s *NewsScheduler) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := "stopped"
	if s.running {
		status = "running"
	}
	lastPollStr := ""
	if !s.lastPoll.IsZero() {
		lastPollStr = s.lastPoll.Format(time.RFC3339)
	}
	return map[string]any{
		"status":     status,
		"total_news": s.totalNews,
		"last_poll":  lastPollStr,
		"last_count": s.lastCount,
	}
}

func (s *NewsScheduler) loop() {
	for {
		select {
		case <-s.stopCh:
			return
		case <-time.After(GetPollInterval()):
			s.poll()
		}
	}
}

func (s *NewsScheduler) pollOnce() {
	s.poll()
}

// poll 拉取最新新闻并推入 AI 分类队列，不阻塞。
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

	// 更新最新时间戳（基于原始列表，避免重复拉取）
	maxTime := ExtractMaxCTime(news)
	if maxTime > s.lastTime {
		s.lastTime = maxTime
		logger.Debug("财联社最新时间戳", zap.Int64("lastTime", maxTime))
	}

	// 推入 AI 分类队列（非阻塞，队列满时丢弃）
	queued := 0
	for _, n := range news {
		select {
		case s.classifyQueue <- n:
			queued++
		default:
			logger.Warn("AI 分类队列已满，丢弃新闻", zap.Int64("id", n.ID))
		}
	}

	s.mu.Lock()
	s.lastPoll = time.Now()
	s.lastCount = queued
	s.mu.Unlock()

	logger.Debug("财联社新闻入队", zap.Int("total", len(news)), zap.Int("queued", queued))
}

// classifyWorker 后台 AI 分类 worker：逐条消费队列，调用 ClassifyOne 并保存结果。
func (s *NewsScheduler) classifyWorker() {
	s.workerWg.Add(1)
	defer s.workerWg.Done()

	s.classifierOnce.Do(func() {
		s.classifier = NewAINewsClassifier(config.GetAIConfig())
	})
	if !s.classifier.IsAvailable() {
		logger.Debug("AI 新闻分类未启用，worker 退出")
		return
	}

	for {
		select {
		case <-s.stopCh:
			logger.Debug("AI 分类 worker 退出")
			return
		case news := <-s.classifyQueue:
			tags, err := s.classifier.ClassifyOne(news)
			if err != nil {
				logger.Warn("AI 新闻分类失败", zap.Int64("id", news.ID), zap.Error(err))
				continue
			}
			if len(tags) == 0 {
				continue
			}
			news.Sectors = tags
			s.saveClassified(news)
		}
	}
}

// saveClassified 保存单条已分类新闻到数据库。
func (s *NewsScheduler) saveClassified(news CLSNews) {
	db, err := storage.Get()
	if err != nil {
		logger.Warn("数据库连接失败", zap.Error(err))
		return
	}

	sectorsJSON, _ := json.Marshal(news.Sectors)
	record := storage.CLSNewsRecord{
		ID:         news.ID,
		Title:      news.Title,
		Content:    news.Content,
		Brief:      news.Brief,
		Level:      news.Level,
		ReadingNum: news.ReadingNum,
		CTime:      news.CTime.Format("2006-01-02 15:04:05"),
		ShareURL:   news.ShareURL,
		Sectors:    string(sectorsJSON),
	}

	saved, err := db.SaveCLSNews([]storage.CLSNewsRecord{record})
	if err != nil {
		logger.Warn("保存新闻失败", zap.Int64("id", news.ID), zap.Error(err))
		return
	}

	s.mu.Lock()
	s.totalNews += saved
	s.mu.Unlock()

	if saved > 0 && config.DataMode() == "json" {
		if db, err := storage.Get(); err == nil {
			db.ExportJSON()
		}
		DumpNews([]CLSNews{news})
		logger.Info("新闻分类保存", zap.Int64("id", news.ID), zap.Strings("tags", news.Sectors))
	}
}
