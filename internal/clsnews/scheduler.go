package clsnews

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

const classifyQueueSize = 200
const pendingRetryBatchSize = 100
const pendingRetryInterval = 2 * time.Minute
const maxClassificationRetries = 5

// AI 分类并发 worker 数。多个 worker 同时消费队列，避免单条阻塞拖慢整体。
const numClassifyWorkers = 3

// NewsScheduler 财联社新闻调度器，支持自动轮询和手动回放两种模式。
// 自动轮询间隔由 GetPollInterval 决定（交易时段 30s、非交易时段 5min），手动回放由前端按钮触发。
// AI 标签分类通过队列+worker 异步处理：poll() 仅负责拉取和入队，
// worker goroutine 逐条调用 AI 并保存结果。
type NewsScheduler struct {
	mu               sync.Mutex
	running          bool
	stopCh           chan struct{}
	lastTime         int64     // 上次拉取到的最新时间戳
	totalNews        int       // 累计保存的新闻数（worker 更新）
	lastPoll         time.Time // 上次轮询时间
	lastCount        int
	lastRetry        time.Time
	lastRetryQueued  int
	lastRetryDropped int

	classifier       *AINewsClassifier
	classifierOnce   sync.Once
	classifyQueue    chan CLSNews
	classifyWorkerWg sync.WaitGroup
	workerProcessed  atomic.Int64
	queuedNewsIDs    map[int64]struct{}
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
	s.queuedNewsIDs = make(map[int64]struct{})
	s.mu.Unlock()

	go s.classifyWorker()
	go s.retryPendingLoop()
	go s.loop()
	go s.pollOnce()
	go s.retryPendingOnce()
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
	return s.poll()
}

func (s *NewsScheduler) RetryNews(id int64) error {
	if !s.IsRunning() {
		return fmt.Errorf("新闻轮询未启动")
	}

	db, err := storage.Get()
	if err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}
	record, err := db.GetCLSNewsByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("新闻不存在")
		}
		return fmt.Errorf("加载新闻失败: %w", err)
	}
	if record.ClassifyStatus == "classified" {
		return fmt.Errorf("该新闻已完成分类")
	}

	if err := db.RequeueCLSNews(id); err != nil {
		return fmt.Errorf("重置新闻状态失败: %w", err)
	}
	queued, dropped := s.enqueueNews([]CLSNews{recordToCLSNews(record)})
	if queued == 0 {
		if dropped > 0 {
			return fmt.Errorf("分类队列繁忙，请稍后重试")
		}
		return fmt.Errorf("新闻已在分类队列中")
	}
	return nil
}

func (s *NewsScheduler) RetryNewsBatch(ids []int64) (queued int, failed map[int64]string) {
	failed = make(map[int64]string)
	for _, id := range ids {
		if err := s.RetryNews(id); err != nil {
			failed[id] = err.Error()
			continue
		}
		queued++
	}
	return queued, failed
}

// Status 返回调度器状态。
func (s *NewsScheduler) Status() map[string]any {
	s.mu.Lock()
	status := "stopped"
	if s.running {
		status = "running"
	}
	lastPollStr := ""
	if !s.lastPoll.IsZero() {
		lastPollStr = s.lastPoll.Format(time.RFC3339)
	}
	lastRetryStr := ""
	if !s.lastRetry.IsZero() {
		lastRetryStr = s.lastRetry.Format(time.RFC3339)
	}
	totalNews := s.totalNews
	lastCount := s.lastCount
	lastRetryQueued := s.lastRetryQueued
	lastRetryDropped := s.lastRetryDropped
	s.mu.Unlock()

	stats := storage.CLSNewsClassificationStats{}
	if db, err := storage.Get(); err == nil {
		if got, err := db.GetCLSNewsClassificationStats(); err == nil {
			stats = got
		}
	}

	return map[string]any{
		"status":             status,
		"total_news":         totalNews,
		"last_poll":          lastPollStr,
		"last_count":         lastCount,
		"pending_count":      stats.PendingCount,
		"retrying_count":     stats.RetryingCount,
		"failed_count":       stats.FailedCount,
		"skipped_count":      stats.SkippedCount,
		"classified_count":   stats.ClassifiedCount,
		"last_retry":         lastRetryStr,
		"last_retry_at":      stats.LastRetryAt,
		"last_retry_queued":  lastRetryQueued,
		"last_retry_dropped": lastRetryDropped,
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
// 已经入队中的新闻会被跳过，避免重复分类。
func (s *NewsScheduler) enqueueNews(news []CLSNews) (queued, dropped int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, n := range news {
		if len(n.Sectors) > 0 {
			continue
		}
		if _, exists := s.queuedNewsIDs[n.ID]; exists {
			continue
		}
		select {
		case s.classifyQueue <- n:
			s.queuedNewsIDs[n.ID] = struct{}{}
			queued++
		default:
			dropped++
		}
	}
	return
}

// poll 拉取最新新闻 → 原始入库 → 入队异步分类 → worker 回填标签。
// 即使 AI 队列已满或分类失败，原始新闻也会保留，避免数据丢失。
func (s *NewsScheduler) poll() int {
	news, err := FetchTelegraphList(s.lastTime)
	if err != nil {
		logger.Warn("财联社电报拉取失败", zap.Error(err))
		return 0
	}

	if len(news) == 0 {
		s.mu.Lock()
		s.lastPoll = time.Now()
		s.mu.Unlock()
		return 0
	}

	logger.Info("财联社电报拉取",
		zap.Int("total", len(news)),
		zap.Int64("lastTime", s.lastTime),
	)

	// 更新最新时间戳
	maxTime := ExtractMaxCTime(news)
	if maxTime > s.lastTime {
		s.lastTime = maxTime
	}

	rawSaved := s.saveAllRaw(news)

	// 入队异步分类（非阻塞，队列满丢弃）
	queued, dropped := s.enqueueNews(news)

	s.mu.Lock()
	s.lastPoll = time.Now()
	s.totalNews += rawSaved
	s.lastCount = rawSaved
	s.mu.Unlock()

	if dropped > 0 {
		logger.Warn("AI 分类队列已满，部分新闻稍后补标签",
			zap.Int("total", len(news)),
			zap.Int("savedRaw", rawSaved),
			zap.Int("queued", queued),
			zap.Int("dropped", dropped),
			zap.Int("queueCapacity", classifyQueueSize),
		)
	} else {
		logger.Info("AI 分类入队", zap.Int("savedRaw", rawSaved), zap.Int("queued", queued))
	}
	return queued
}

// classifyWorker 启动 numClassifyWorkers 个并发 worker 从队列消费新闻，
// 逐条调用 AI 分类并保存结果。同时开启进度日志（每 30s），确保队列背压可见。
func (s *NewsScheduler) classifyWorker() {
	// 分类器只初始化一次，所有 worker 共享同一实例（含 HTTP 连接池）
	s.classifierOnce.Do(func() {
		s.classifier = NewAINewsClassifier(config.GetAIConfigFor("clsnews"))
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
	const batchSize = 50

	for {
		select {
		case <-s.stopCh:
			return
		case first := <-s.classifyQueue:
			batch := []CLSNews{first}
		drain:
			for len(batch) < batchSize {
				select {
				case n := <-s.classifyQueue:
					batch = append(batch, n)
				default:
					break drain
				}
			}
			s.releaseQueued(batch)

			results := s.classifier.ClassifyBatch(batch)

			db, err := storage.Get()
			if err != nil {
				logger.Warn("数据库连接失败", zap.Error(err))
				continue
			}

			saved := 0
			for i, result := range results {
				switch result.Status {
				case "classified":
					batch[i].Sectors = result.Tags
					sectorsJSON, err := json.Marshal(result.Tags)
					if err != nil {
						logger.Warn("序列化板块标签失败", zap.Error(err))
						continue
					}
					if db.CLSNewsExists(batch[i].ID) {
						err = db.UpdateCLSNewsSectors(batch[i].ID, string(sectorsJSON))
					} else {
						_, err = db.SaveCLSNews([]storage.CLSNewsRecord{{
							ID:             batch[i].ID,
							Title:          batch[i].Title,
							Content:        batch[i].Content,
							Brief:          batch[i].Brief,
							Level:          batch[i].Level,
							ReadingNum:     batch[i].ReadingNum,
							CTime:          batch[i].CTime.Format("2006-01-02 15:04:05"),
							ShareURL:       batch[i].ShareURL,
							Sectors:        string(sectorsJSON),
							ClassifyStatus: "classified",
						}})
					}
					if err == nil {
						saved++
					}
				case "skipped":
					err = db.MarkCLSNewsSkipped(batch[i].ID, result.Error)
				case "discarded":
					err = db.MarkCLSNewsDiscarded(batch[i].ID, result.Error)
				default:
					err = db.RecordCLSNewsRetry(batch[i].ID, result.Error, maxClassificationRetries)
				}
				if err != nil {
					logger.Warn("保存新闻失败", zap.Int64("id", batch[i].ID), zap.Error(err))
				}
			}

			processed := s.workerProcessed.Add(int64(saved))
			if saved > 0 {
				queueLen := len(s.classifyQueue)
				logger.Info("AI 分类进度",
					zap.Int64("processed", processed),
					zap.Int("batchClassified", len(batch)),
					zap.Int("batchSaved", saved),
					zap.Int("queueRemaining", queueLen),
				)
			}
		}
	}
}

func (s *NewsScheduler) retryPendingLoop() {
	for {
		select {
		case <-s.stopCh:
			return
		case <-time.After(pendingRetryInterval):
			s.retryPendingOnce()
		}
	}
}

func (s *NewsScheduler) retryPendingOnce() {
	db, err := storage.Get()
	if err != nil {
		logger.Warn("数据库连接失败", zap.Error(err))
		return
	}

	records, err := db.LoadPendingCLSNews(pendingRetryBatchSize)
	if err != nil {
		logger.Warn("加载待补标签新闻失败", zap.Error(err))
		return
	}
	if len(records) == 0 {
		return
	}

	news := make([]CLSNews, 0, len(records))
	for _, record := range records {
		news = append(news, recordToCLSNews(record))
	}
	queued, dropped := s.enqueueNews(news)
	if queued == 0 && dropped == 0 {
		return
	}
	s.mu.Lock()
	s.lastRetry = time.Now()
	s.lastRetryQueued = queued
	s.lastRetryDropped = dropped
	s.mu.Unlock()

	logger.Info("待补标签新闻已重入队",
		zap.Int("pending", len(records)),
		zap.Int("queued", queued),
		zap.Int("dropped", dropped),
	)
}

func (s *NewsScheduler) releaseQueued(news []CLSNews) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range news {
		delete(s.queuedNewsIDs, item.ID)
	}
}

func recordToCLSNews(record storage.CLSNewsRecord) CLSNews {
	ctime, err := time.ParseInLocation("2006-01-02 15:04:05", record.CTime, time.Local)
	if err != nil {
		ctime = time.Time{}
	}
	createdAt, err := time.ParseInLocation("2006-01-02 15:04:05", record.CreatedAt, time.Local)
	if err != nil {
		createdAt = time.Time{}
	}
	return CLSNews{
		ID:         record.ID,
		Title:      record.Title,
		Content:    record.Content,
		Brief:      record.Brief,
		Level:      record.Level,
		ReadingNum: record.ReadingNum,
		CTime:      ctime,
		ShareURL:   record.ShareURL,
		CreatedAt:  createdAt,
	}
}
