package clsnews

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

const classifyQueueSize = 200

// AI 分类并发 worker 数。多个 worker 同时消费队列，避免单条阻塞拖慢整体。
const numClassifyWorkers = 3

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

	classifier       *AINewsClassifier
	classifierOnce   sync.Once
	classifyQueue    chan CLSNews
	classifyWorkerWg sync.WaitGroup
	workerProcessed  atomic.Int64
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
	s.classifyWorkerWg.Wait()
	logger.Info("财联社新闻轮询已停止，分类 worker 已退出", zap.Int64("processed", s.workerProcessed.Load()))
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

// saveAllRaw 将新闻全量写入数据库（无标签时为 "[]"）。
// INSERT OR IGNORE 按 id 去重，已存在的新闻不会覆盖。
func (s *NewsScheduler) saveAllRaw(news []CLSNews) int {
	db, err := storage.Get()
	if err != nil {
		logger.Warn("数据库连接失败", zap.Error(err))
		return 0
	}

	records := make([]storage.CLSNewsRecord, 0, len(news))
	for _, n := range news {
		sectorsJSON := "[]"
		if len(n.Sectors) > 0 {
			if b, e := json.Marshal(n.Sectors); e == nil {
				sectorsJSON = string(b)
			}
		}
		records = append(records, storage.CLSNewsRecord{
			ID:         n.ID,
			Title:      n.Title,
			Content:    n.Content,
			Brief:      n.Brief,
			Level:      n.Level,
			ReadingNum: n.ReadingNum,
			CTime:      n.CTime.Format("2006-01-02 15:04:05"),
			ShareURL:   n.ShareURL,
			Sectors:    sectorsJSON,
		})
	}

	saved, err := db.SaveCLSNews(records)
	if err != nil {
		logger.Warn("保存新闻失败", zap.Error(err))
		return 0
	}
	return saved
}

// enqueueNews 将新闻推入 AI 分类队列（非阻塞），返回入队数和丢弃数。
func (s *NewsScheduler) enqueueNews(news []CLSNews) (queued, dropped int) {
	for _, n := range news {
		select {
		case s.classifyQueue <- n:
			queued++
		default:
			dropped++
		}
	}
	return
}

// poll 拉取最新新闻 → 全量落库 → 入队异步分类。
// 先保存到数据库确保不丢数据，再入队让 worker 逐条打标签。
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

	logger.Info("财联社电报拉取",
		zap.Int("total", len(news)),
		zap.Int64("lastTime", s.lastTime),
	)

	// 1. 全量写入数据库（sectors="[]"），不丢数据
	saved := s.saveAllRaw(news)

	// 2. 更新最新时间戳（基于已落库的完整列表）
	maxTime := ExtractMaxCTime(news)
	if maxTime > s.lastTime {
		s.lastTime = maxTime
		logger.Debug("财联社最新时间戳", zap.Int64("lastTime", maxTime))
	}

	// 3. 入队异步分类（非阻塞，队列满丢弃不影响已落库数据）
	queued, dropped := s.enqueueNews(news)

	s.mu.Lock()
	s.lastPoll = time.Now()
	s.lastCount = saved
	s.mu.Unlock()

	if dropped > 0 {
		logger.Warn("AI 分类队列已满，部分新闻暂未打标签",
			zap.Int("total", len(news)),
			zap.Int("saved", saved),
			zap.Int("queued", queued),
			zap.Int("dropped", dropped),
			zap.Int("queueCapacity", classifyQueueSize),
		)
	} else {
		logger.Info("AI 分类入队", zap.Int("queued", queued))
	}
}

// classifyWorker 启动 numClassifyWorkers 个并发 worker 从队列消费新闻，
// 逐条调用 AI 分类并保存结果。同时开启进度日志（每 30s），确保队列背压可见。
func (s *NewsScheduler) classifyWorker() {
	// 分类器只初始化一次，所有 worker 共享同一实例（含 HTTP 连接池）
	s.classifierOnce.Do(func() {
		s.classifier = NewAINewsClassifier(config.GetAIConfig())
	})
	if !s.classifier.IsAvailable() {
		logger.Warn("AI 新闻分类未启用（未配置 API Key），worker 退出，新闻将无标签入库")
		return
	}

	for i := range numClassifyWorkers {
		s.classifyWorkerWg.Add(1)
		go s.classifyOneWorker(i)
	}

	<-s.stopCh
	logger.Info("AI 分类 worker 池退出", zap.Int64("processed", s.workerProcessed.Load()))
}

func (s *NewsScheduler) classifyOneWorker(id int) {
	defer s.classifyWorkerWg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Error("AI 分类 worker panic",
				zap.Int("workerID", id),
				zap.Any("recover", r),
			)
		}
	}()

	logger.Debug("AI 分类 worker 启动", zap.Int("workerID", id))
	for {
		select {
		case <-s.stopCh:
			return
		case news := <-s.classifyQueue:
			tags, err := s.classifier.ClassifyOne(news)
			if err != nil {
				logger.Warn("AI 新闻分类失败",
					zap.Int64("id", news.ID),
					zap.Error(err),
				)
				continue
			}
			if len(tags) == 0 {
				continue
			}
			news.Sectors = tags
			s.updateSectors(news)
			processed := s.workerProcessed.Add(1)

			if processed%10 == 0 {
				queueLen := len(s.classifyQueue)
				logger.Info("AI 分类进度",
					zap.Int64("processed", processed),
					zap.Int("queueRemaining", queueLen),
				)
			}
		}
	}
}

// updateSectors 更新单条新闻的板块标签（数据库已由 poll() 写入，仅更新 sectors 列）。
func (s *NewsScheduler) updateSectors(news CLSNews) {
	db, err := storage.Get()
	if err != nil {
		logger.Warn("数据库连接失败", zap.Error(err))
		return
	}

	sectorsJSON, _ := json.Marshal(news.Sectors)
	if err := db.UpdateCLSNewsSectors(news.ID, string(sectorsJSON)); err != nil {
		logger.Warn("更新新闻标签失败", zap.Int64("id", news.ID), zap.Error(err))
		return
	}

	s.mu.Lock()
	s.totalNews++
	s.mu.Unlock()

	if config.DataMode() == "json" {
		if db, err := storage.Get(); err == nil {
			db.ExportJSON()
		}
		DumpNews([]CLSNews{news})
	}

	logger.Info("新闻分类完成", zap.Int64("id", news.ID), zap.Strings("tags", news.Sectors))
}
