package web

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/clsnews"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/copy"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/hotnews"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/renderer"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tickfetcher"
	"github.com/a-share-flow-video-go/internal/tickrenderer"
	"github.com/a-share-flow-video-go/internal/tickscheduler"
	"go.uber.org/zap"
)

// ExportTask 异步全量下载任务
type ExportTask struct {
	ID       string
	Date     string
	Status   string
	Progress string
	Page     int
	Data     []storage.SectorAll
	Err      string
	mu       sync.Mutex
}

var exportTasks = make(map[string]*ExportTask)
var exportMu sync.Mutex
var exportHTTPClient = &http.Client{Timeout: 15 * time.Second}

func getTask(dateStr string) (*ExportTask, bool) {
	exportMu.Lock()
	defer exportMu.Unlock()
	for _, t := range exportTasks {
		if t.Date == dateStr && (t.Status == "pending" || t.Status == "running") {
			return t, true
		}
	}
	return nil, false
}

func runExportTask(task *ExportTask) {
	task.mu.Lock()
	task.Status = "running"
	task.Progress = "开始获取板块数据..."
	task.mu.Unlock()

	dateDir := filepath.Join(config.GetDataDir(), task.Date)
	os.MkdirAll(dateDir, 0755)
	checkpointPath := filepath.Join(dateDir, ".export_progress.json")

	if cp, err := os.ReadFile(checkpointPath); err == nil {
		var cpData struct {
			Page int `json:"page"`
		}
		if json.Unmarshal(cp, &cpData) == nil {
			task.Page = cpData.Page
		}
	}

	saveCheckpoint := func() {
		data, _ := json.Marshal(map[string]int{"page": task.Page})
		os.WriteFile(checkpointPath, data, 0644)
	}

	fetchPage := func(fs string, pn int) ([]storage.SectorAll, bool, error) {
		url := fmt.Sprintf("https://emdatah5.eastmoney.com/dc/ZJLX/getZDYLBData?fields=f12,f14,f3,f62,f66,f69,f72,f75,f184&pn=%d&pz=500&fid=f62&po=1&fs=%s&ut=b2884a393a59ad64002292a3e90d46a5", pn, fs)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, false, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Referer", "https://emdatah5.eastmoney.com/dc/zjlx/index")

		resp, err := exportHTTPClient.Do(req)
		if err != nil {
			return nil, false, err
		}
		defer resp.Body.Close()

		var result struct {
			Data struct {
				Diff []map[string]any `json:"diff"`
			} `json:"data"`
		}
		b, _ := io.ReadAll(resp.Body)
		json.Unmarshal(b, &result)

		var page []storage.SectorAll
		for _, item := range result.Data.Diff {
			name, _ := item["f14"].(string)
			code, _ := item["f12"].(string)
			netVal := item["f62"]
			rateVal := item["f184"]
			if name == "" || netVal == nil {
				continue
			}
			if f, ok := toFloat64(netVal); ok && f != 0 {
				rateFloat, _ := toFloat64(rateVal)
				changePctFloat, _ := toFloat64(item["f3"])
				superNetFloat, _ := toFloat64(item["f66"])
				superRateFloat, _ := toFloat64(item["f69"])
				bigNetFloat, _ := toFloat64(item["f72"])
				bigRateFloat, _ := toFloat64(item["f75"])
				page = append(page, storage.SectorAll{
					Date:      task.Date,
					Code:      code,
					Name:      name,
					Net:       roundTo2(f / 1e8),
					Rate:      roundTo2(rateFloat),
					ChangePct: roundTo2(changePctFloat),
					SuperNet:  roundTo2(superNetFloat / 1e8),
					SuperRate: roundTo2(superRateFloat),
					BigNet:    roundTo2(bigNetFloat / 1e8),
					BigRate:   roundTo2(bigRateFloat),
				})
			}
		}
		return page, len(result.Data.Diff) < 500, nil
	}

	allFS := []string{"m:90+t:2", "m:90+t:3"}
	for _, fs := range allFS {
		for pn := task.Page + 1; ; pn++ {
			task.mu.Lock()
			task.Progress = fmt.Sprintf("第%d页...", pn)
			task.mu.Unlock()

			page, done, err := fetchPage(fs, pn)
			if err != nil {
				task.mu.Lock()
				task.Status = "error"
				task.Err = err.Error()
				task.mu.Unlock()
				return
			}
			task.mu.Lock()
			task.Data = append(task.Data, page...)
			task.Page = pn
			task.mu.Unlock()
			saveCheckpoint()
			time.Sleep(200 * time.Millisecond)

			if done {
				break
			}
		}
		task.Page = 0
	}

	if db, err := storage.Get(); err == nil {
		task.mu.Lock()
		task.Progress = "正在保存到数据库..."
		task.mu.Unlock()

		if err := db.SaveSectorsAll(task.Data); err != nil {
			task.mu.Lock()
			task.Status = "error"
			task.Err = fmt.Sprintf("保存数据库失败: %v", err)
			task.mu.Unlock()
			return
		}
	}

	os.Remove(checkpointPath)

	task.mu.Lock()
	task.Status = "done"
	task.Progress = fmt.Sprintf("完成: %d 个板块", len(task.Data))
	task.mu.Unlock()
}

// SSEWriter 实现 Server-Sent Events 流式响应。
type SSEWriter struct {
	w  gin.ResponseWriter
	mu sync.Mutex
}

func NewSSEWriter(c *gin.Context) *SSEWriter {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeaderNow()
	c.Writer.Flush()
	return &SSEWriter{w: c.Writer}
}

func (s *SSEWriter) Send(msgType, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() { recover() }()
	line := fmt.Sprintf(`{"type":"%s","text":%s}`, msgType, jsonStr(text))
	s.w.Write([]byte(line + "\n"))
	s.w.Flush()
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// SetupRouter 注册所有 HTTP 路由。
func SetupRouter(tickSched *tickscheduler.TickScheduler, newsSched *clsnews.NewsScheduler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(logger.RecoveryMiddleware())
	r.Use(logger.RequestLoggerMiddleware())
	r.Use(corsMiddleware())

	registerDocsRoute(r)

	r.GET("/api/dates", handleDates)
	r.GET("/api/data/:date", handleData)
	r.GET("/api/export-all/:date", handleExportAll)
	r.GET("/api/export-all/status/:task_id", handleExportStatus)
	r.GET("/api/export-all/file/:task_id", handleExportFile)
	r.GET("/api/export-hot-sectors/:date", handleExportHotSectors)
	r.POST("/api/sectors-all/save/:date", handleSaveAllSectors)
	r.GET("/api/sectors-all/save/status/:task_id", handleSaveAllStatus)
	r.GET("/api/sectors-all/:date", handleGetAllSectors)
	r.GET("/api/sectors-all/names", handleGetSectorsAllNames)
	r.GET("/api/sectors-all/dates", handleGetSectorsAllDates)
	r.GET("/api/sectors-all/range", handleGetSectorsAllRange)

	r.GET("/api/notes", handleListNotes)
	r.POST("/api/notes", handleCreateNote)
	r.PUT("/api/notes/:id", handleUpdateNote)
	r.DELETE("/api/notes/:id", handleDeleteNote)

	r.POST("/api/generate-multiday", handleGenerateMultiDay)
	r.GET("/api/config", handleGetConfig)
	r.POST("/api/config", handleSaveConfig)
	r.POST("/api/optimize-copy", handleOptimizeCopy)
	r.GET("/api/files/:date", handleFiles)

	r.GET("/api/tick/status", func(c *gin.Context) {
		c.JSON(200, tickSched.GetStatus())
	})
	r.POST("/api/tick/start", func(c *gin.Context) {
		if err := tickSched.StartManual(); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"ok": true, "message": "tick 采集已启动"})
	})
	r.POST("/api/tick/force-collect", func(c *gin.Context) {
		var body struct {
			Time string `json:"time"` // "15:00"
			Date string `json:"date"` // 可选，默认当天
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if body.Time == "" {
			c.JSON(400, gin.H{"error": "缺少 time 参数"})
			return
		}
		dateStr := body.Date
		if dateStr == "" {
			dateStr = time.Now().In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02")
		}
		datetime := storage.DateToDatetimeTick(dateStr, body.Time)

		db, err := storage.Get()
		if err != nil {
			c.JSON(500, gin.H{"error": "数据库初始化失败"})
			return
		}
		exists, _ := db.HasTickData(datetime)
		if exists {
			c.JSON(200, gin.H{"ok": true, "message": fmt.Sprintf("%s %s 的 tick 数据已存在", dateStr, body.Time)})
			return
		}

		sectors, err := fetcher.FetchTop21HotSectors()
		if err != nil {
			c.JSON(502, gin.H{"error": fmt.Sprintf("东方财富 API 请求失败: %v", err)})
			return
		}

		inputDate := time.Now().In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02 15:04:05")
		records := make([]storage.Sector, 0, len(sectors))
		for _, s := range sectors {
			records = append(records, storage.Sector{
				Datetime:  datetime,
				Name:      s.Name,
				Net:       s.Net,
				Rate:      s.Rate,
				ChangePct: s.ChangePct,
				SuperNet:  s.SuperNet,
				SuperRate: s.SuperRate,
				BigNet:    s.BigNet,
				BigRate:   s.BigRate,
				InputDate: inputDate,
			})
		}
		if err := db.SaveSectors(records); err != nil {
			c.JSON(500, gin.H{"error": fmt.Sprintf("保存失败: %v", err)})
			return
		}

		c.JSON(200, gin.H{
			"ok":      true,
			"message": fmt.Sprintf("已强制采集 %s %s 的 tick 数据", dateStr, body.Time),
			"count":   len(records),
		})
	})
	r.POST("/api/tick/stop", func(c *gin.Context) {
		tickSched.StopManual()
		c.JSON(200, gin.H{"ok": true, "message": "tick 采集已停止"})
	})
	r.POST("/api/tick/enable", func(c *gin.Context) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		tickSched.SetEnabled(body.Enabled)
		c.JSON(200, gin.H{"ok": true})
	})
	r.GET("/api/tick/interval", func(c *gin.Context) {
		c.JSON(200, gin.H{"intervalMinutes": tickSched.GetFetcher().GetIntervalMinutes()})
	})
	r.POST("/api/tick/interval", func(c *gin.Context) {
		var body struct {
			IntervalMinutes int `json:"intervalMinutes"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		tickSched.GetFetcher().SetIntervalMinutes(body.IntervalMinutes)
		c.JSON(200, gin.H{"ok": true, "intervalMinutes": body.IntervalMinutes})
	})
	r.GET("/api/tick-data/:date", func(c *gin.Context) {
		dateStr := c.Param("date")
		session := c.DefaultQuery("session", "full")
		points, err := tickfetcher.LoadTickCSV(dateStr, session)
		if err != nil {
			c.JSON(404, gin.H{"error": "no tick data"})
			return
		}
		c.JSON(200, gin.H{"date": dateStr, "session": session, "points": points})
	})
	r.POST("/api/generate-tick", handleGenerateTick)

	r.GET("/api/tick/stream", func(c *gin.Context) {
		handleTickStream(c, tickSched)
	})

	r.GET("/api/dashboard", handleDashboard)

	// 财联社新闻路由（自动轮询 + 手动回放）
	if newsSched != nil {
		r.GET("/api/news", handleNewsList)
		r.GET("/api/news/search", handleNewsSearch)
		r.GET("/api/news/date", handleNewsByDate)
		r.GET("/api/news/status", func(c *gin.Context) {
			c.JSON(200, newsSched.Status())
		})
		r.POST("/api/news/start", func(c *gin.Context) {
			newsSched.Start()
			c.JSON(200, gin.H{"ok": true, "message": "新闻轮询已启动"})
		})
		r.POST("/api/news/stop", func(c *gin.Context) {
			newsSched.Stop()
			c.JSON(200, gin.H{"ok": true, "message": "新闻轮询已停止"})
		})
		r.POST("/api/news/replay", func(c *gin.Context) {
			count := newsSched.PollOnce()
			c.JSON(200, gin.H{"ok": true, "count": count, "message": fmt.Sprintf("回放完成，新增 %d 条新闻", count)})
		})
	}

	r.GET("/output/:date/:file", serveVideo)

	return r
}

func handleDates(c *gin.Context) {
	dates := getDates()
	items := make([]map[string]any, 0, len(dates))
	for _, d := range dates {
		videos := getVideos(d)
		cpy := getCopy(d)
		sectorsFull, _ := fetcher.LoadSessionData(d, "full")
		sectorsMorning, _ := fetcher.LoadSessionData(d, "morning")
		items = append(items, map[string]any{
			"date":          d,
			"videos":        videos,
			"sector_count":  len(sectorsFull),
			"morning_count": len(sectorsMorning),
			"文案_count":      len(cpy["template"]),
			"ai_count":      len(cpy["ai"]),
		})
	}
	c.JSON(200, gin.H{"dates": items})
}

func handleData(c *gin.Context) {
	dateStr := c.Param("date")
	session := c.Query("session")
	if session == "" {
		session = "full"
	}
	sectors, _ := fetcher.LoadSessionData(dateStr, session)
	c.JSON(200, gin.H{
		"sectors": sectors,
		"videos":  getVideos(dateStr),
		"文案":      getCopy(dateStr),
	})
}

func handleExportAll(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	if task, exists := getTask(dateStr); exists {
		c.JSON(200, gin.H{"task_id": task.ID, "status": task.Status, "progress": task.Progress})
		return
	}

	taskID := fmt.Sprintf("exp_%s_%d", dateStr, time.Now().Unix())
	task := &ExportTask{ID: taskID, Date: dateStr, Status: "pending", Progress: "等待启动..."}
	exportMu.Lock()
	exportTasks[taskID] = task
	exportMu.Unlock()

	go runExportTask(task)
	c.JSON(200, gin.H{"task_id": taskID, "status": "pending"})
}

func handleExportStatus(c *gin.Context) {
	taskID := c.Param("task_id")
	exportMu.Lock()
	task, ok := exportTasks[taskID]
	exportMu.Unlock()

	if !ok {
		c.JSON(404, gin.H{"error": "任务不存在"})
		return
	}

	task.mu.Lock()
	resp := gin.H{
		"task_id":  task.ID,
		"status":   task.Status,
		"progress": task.Progress,
	}
	if task.Status == "error" {
		resp["error"] = task.Err
	}
	task.mu.Unlock()
	c.JSON(200, resp)
}

func handleExportFile(c *gin.Context) {
	taskID := c.Param("task_id")
	exportMu.Lock()
	task, ok := exportTasks[taskID]
	exportMu.Unlock()

	if !ok || task.Status != "done" {
		c.JSON(404, gin.H{"error": "文件未就绪"})
		return
	}

	dateDir := config.GetDataDir()
	filename := fmt.Sprintf("板块全量_%s.csv", task.Date)
	c.File(filepath.Join(dateDir, task.Date, filename))
}

func handleExportHotSectors(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	sectors, err := fetcher.FetchTop21HotSectors()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"date":    dateStr,
		"sectors": sectors,
	})
}

type SaveAllTask struct {
	mu       sync.Mutex
	Date     string    `json:"date"`
	Status   string    `json:"status"`
	Progress string    `json:"progress"`
	Count    int       `json:"count"`
	Err      string    `json:"error,omitempty"`
	Created  time.Time `json:"created"`
}

var (
	saveAllTasks      = make(map[string]*SaveAllTask)
	saveAllTasksMu    sync.Mutex
	saveAllTaskCounter int
)

func handleSaveAllSectors(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	saveAllTasksMu.Lock()
	saveAllTaskCounter++
	taskID := fmt.Sprintf("save_%s_%d", dateStr, saveAllTaskCounter)
	task := &SaveAllTask{
		Date:    dateStr,
		Status:  "pending",
		Created: time.Now(),
	}
	saveAllTasks[taskID] = task
	saveAllTasksMu.Unlock()

	logger.Info("全板块数据获取任务已创建",
		zap.String("date", dateStr),
		zap.String("task_id", taskID),
	)

	go runSaveAllTask(task)

	c.JSON(200, gin.H{
		"task_id": taskID,
		"message": "已启动后台获取任务",
	})
}

func handleSaveAllStatus(c *gin.Context) {
	taskID := c.Param("task_id")
	saveAllTasksMu.Lock()
	task, ok := saveAllTasks[taskID]
	saveAllTasksMu.Unlock()

	if !ok {
		logger.Warn("全板块数据任务状态查询失败，任务不存在", zap.String("task_id", taskID))
		c.JSON(404, gin.H{"error": "任务不存在"})
		return
	}

	task.mu.Lock()
	dateStr := task.Date
	status := task.Status
	progress := task.Progress
	count := task.Count
	errMsg := task.Err
	resp := gin.H{
		"task_id":  taskID,
		"date":     dateStr,
		"status":   status,
		"progress": progress,
		"count":    count,
	}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	task.mu.Unlock()

	logger.Info("全板块数据任务状态",
		zap.String("task_id", taskID),
		zap.String("date", dateStr),
		zap.String("status", status),
		zap.String("progress", progress),
		zap.Int("count", count),
	)

	c.JSON(200, resp)
}

func runSaveAllTask(task *SaveAllTask) {
	task.mu.Lock()
	task.Status = "running"
	task.Progress = "开始获取板块数据..."
	task.mu.Unlock()

	logger.Info("全板块数据获取任务开始执行",
		zap.String("date", task.Date),
	)

	var allSectors []storage.SectorAll
	allFS := []struct {
		fs       string
		category string
	}{{"m:90+t:2", "industry"}, {"m:90+t:3", "concept"}}
	for _, item := range allFS {
		for pn := 1; ; pn++ {
			task.mu.Lock()
			task.Progress = fmt.Sprintf("获取第%d页(%s)...", pn, item.category)
			task.mu.Unlock()

			url := fmt.Sprintf("https://emdatah5.eastmoney.com/dc/ZJLX/getZDYLBData?fields=f12,f14,f3,f62,f66,f69,f72,f75,f184&pn=%d&pz=500&fid=f62&po=1&fs=%s&ut=b2884a39ad64002292a3e90d46a5", pn, item.fs)
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				task.mu.Lock()
				task.Status = "error"
				task.Err = err.Error()
				task.mu.Unlock()
				logger.Error("全板块数据API请求创建失败",
					zap.String("date", task.Date),
					zap.String("category", item.category),
					zap.Int("page", pn),
					zap.Error(err),
				)
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			req.Header.Set("Referer", "https://emdatah5.eastmoney.com/dc/zjlx/index")

			resp, err := exportHTTPClient.Do(req)
			if err != nil {
				task.mu.Lock()
				task.Status = "error"
				task.Err = fmt.Sprintf("API请求失败: %v", err)
				task.mu.Unlock()
				logger.Error("全板块数据API请求失败",
					zap.String("date", task.Date),
					zap.String("category", item.category),
					zap.Int("page", pn),
					zap.Error(err),
				)
				return
			}

			var result struct {
				Data struct {
					Diff []map[string]any `json:"diff"`
				} `json:"data"`
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			json.Unmarshal(b, &result)

			pageItemCount := len(result.Data.Diff)
			logger.Info("全板块数据页获取成功",
				zap.String("date", task.Date),
				zap.String("category", item.category),
				zap.Int("page", pn),
				zap.Int("items", pageItemCount),
			)

			for _, d := range result.Data.Diff {
				name, _ := d["f14"].(string)
				code, _ := d["f12"].(string)
				netVal := d["f62"]
				rateVal := d["f184"]
				if name == "" || netVal == nil {
					continue
				}
				if f, ok := toFloat64(netVal); ok && f != 0 {
					rateFloat, _ := toFloat64(rateVal)
					changePctFloat, _ := toFloat64(d["f3"])
					superNetFloat, _ := toFloat64(d["f66"])
					superRateFloat, _ := toFloat64(d["f69"])
					bigNetFloat, _ := toFloat64(d["f72"])
					bigRateFloat, _ := toFloat64(d["f75"])
					allSectors = append(allSectors, storage.SectorAll{
						Date:      task.Date,
						Code:      code,
						Name:      name,
						Net:       roundTo2(f / 1e8),
						Rate:      roundTo2(rateFloat),
						ChangePct: roundTo2(changePctFloat),
						SuperNet:  roundTo2(superNetFloat / 1e8),
						SuperRate: roundTo2(superRateFloat),
						BigNet:    roundTo2(bigNetFloat / 1e8),
						BigRate:   roundTo2(bigRateFloat),
						Category:  item.category,
					})
				}
			}

			if pageItemCount < 500 {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		logger.Info("全板块数据分类获取完成",
			zap.String("date", task.Date),
			zap.String("category", item.category),
		)
	}

	task.mu.Lock()
	task.Progress = "正在保存到数据库..."
	task.Count = len(allSectors)
	task.mu.Unlock()

	db, err := storage.Get()
	if err != nil {
		task.mu.Lock()
		task.Status = "error"
		task.Err = fmt.Sprintf("数据库连接失败: %v", err)
		task.mu.Unlock()
		logger.Error("全板块数据保存失败-数据库连接失败",
			zap.String("date", task.Date),
			zap.Error(err),
		)
		return
	}

	if err := db.SaveSectorsAll(allSectors); err != nil {
		task.mu.Lock()
		task.Status = "error"
		task.Err = fmt.Sprintf("保存数据库失败: %v", err)
		task.mu.Unlock()
		logger.Error("全板块数据保存失败-写入数据库失败",
			zap.String("date", task.Date),
			zap.Int("count", len(allSectors)),
			zap.Error(err),
		)
		return
	}

	task.mu.Lock()
	task.Status = "done"
	task.Progress = fmt.Sprintf("完成: 已保存 %d 个板块数据", len(allSectors))
	task.Count = len(allSectors)
	task.mu.Unlock()

	logger.Info("全板块数据获取任务完成",
		zap.String("date", task.Date),
		zap.Int("total_count", len(allSectors)),
	)
}

func handleGetAllSectors(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	sectors, err := db.LoadSectorsAll(dateStr)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"date":    dateStr,
		"sectors": sectors,
	})
}

func handleGetSectorsAllRange(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" || endDate == "" {
		c.JSON(400, gin.H{"error": "缺少参数: start_date, end_date"})
		return
	}

	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	sectors, err := db.LoadSectorsAllRange(startDate, endDate)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"sectors": sectors,
	})
}

func handleGetSectorsAllNames(c *gin.Context) {
	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	names, err := db.ListSectorsAllNames()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"names": names,
	})
}

func handleGetSectorsAllDates(c *gin.Context) {
	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	dates, err := db.ListSectorsAllDates()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"dates": dates,
	})
}

func handleFiles(c *gin.Context) {
	dateStr := c.Param("date")
	videos := getVideos(dateStr)
	cpy := getCopy(dateStr)

	videoMap := make(map[string]string)
	for _, v := range videos {
		videoMap[v] = v + ".mp4"
	}

	c.JSON(200, gin.H{
		"videos": videoMap,
		"文案":     cpy,
	})
}

func handleGenerateMultiDay(c *gin.Context) {
	var body struct {
		Date     string `json:"date"`
		Days     int    `json:"days"`
		CopyMode string `json:"copy_mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Date == "" {
		body.Date = time.Now().Format("2006-01-02")
	}
	if body.Days < 2 {
		body.Days = 3
	}

	logger.Info("多日 Bar Chart Race 视频生成请求",
		zap.String("date", body.Date),
		zap.Int("days", body.Days),
		zap.String("copyMode", body.CopyMode))

	sse := NewSSEWriter(c)
	sse.Send("log", fmt.Sprintf("📅 截止日期: %s, 对比%d个交易日, 文案模式: %s",
		body.Date, body.Days, map[string]string{"ai": "AI生成", "template": "模板"}[body.CopyMode]))

	tradingDays, err := fetcher.GetTradingDays(body.Date, body.Days)
	if err != nil {
		sse.Send("error", fmt.Sprintf("无法获取交易日: %v", err))
		return
	}
	sse.Send("log", fmt.Sprintf("📅 交易日: %v", tradingDays))

	dayData, err := fetcher.LoadMultiDaySectors(tradingDays)
	if err != nil {
		sse.Send("error", fmt.Sprintf("加载数据失败: %v", err))
		return
	}
	sse.Send("log", fmt.Sprintf("✅ 成功加载 %d 日数据", len(dayData)))
	// 按日报板块数
	totalSectors := 0
	allNames := make(map[string]bool)
	for _, d := range tradingDays {
		if sectors, ok := dayData[d]; ok {
			sse.Send("log", fmt.Sprintf("  📆 %s → %d个板块", d, len(sectors)))
			totalSectors += len(sectors)
			for _, s := range sectors {
				allNames[s.Name] = true
			}
		} else {
			sse.Send("log", fmt.Sprintf("  ⚠️ %s → 无数据", d))
		}
	}
	sse.Send("log", fmt.Sprintf("📊 跨日去重后涉及 %d 个不同板块", len(allNames)))

	sse.Send("log", fmt.Sprintf("📝 文案模式: %s", map[string]string{"ai": "AI 生成", "template": "模板"}[body.CopyMode]))
	analysis := analyzer.MultiDayAnalyze(dayData, tradingDays, body.CopyMode)
	sse.Send("log", fmt.Sprintf("✅ 趋势分析完成: %d条洞察 + %d条排名变化", len(analysis.TrendInsights), len(analysis.RankingChanges)))

	// 加载逐 tick 数据用于渲染（含时间轴）
	tickData, err := fetcher.LoadMultiDayTicks(tradingDays)
	if err != nil {
		sse.Send("error", fmt.Sprintf("加载 tick 数据失败: %v", err))
		return
	}
	totalTickSnapshots := 0
	for _, d := range tradingDays {
		if snaps, ok := tickData[d]; ok {
			totalTickSnapshots += len(snaps)
		}
	}
	sse.Send("log", fmt.Sprintf("⏱️ 逐 tick 数据: %d 日, %d 个时间点快照", len(tickData), totalTickSnapshots))

	outputDir := config.GetOutputDir()
	dateLabel := tradingDays[0]
	if len(tradingDays) > 1 {
		dateLabel = tradingDays[0] + "_to_" + tradingDays[len(tradingDays)-1]
	}
	os.MkdirAll(filepath.Join(outputDir, dateLabel), 0755)

	outPath := filepath.Join(outputDir, dateLabel, "三日资金流向.mp4")

	sse.Send("log", "🎬 开始渲染 Bar Chart Race 视频 (16:9)...")
	sse.Send("progress", "渲染视频中...")

	if _, err := renderer.RenderMultiDayVideo(tickData, tradingDays, outPath, analysis, "tv"); err != nil {
		sse.Send("error", fmt.Sprintf("渲染失败: %v", err))
		return
	}
	sse.Send("log", "✅ Bar Chart Race 视频生成完成")
	sse.Send("progress", "完成")
	sse.Send("done", "生成完毕")
}

func handleGetConfig(c *gin.Context) {
	aiCfg := config.GetAIConfig()
	sessions := make(map[string]string)
	for k, v := range config.SessionConfigs {
		sessions[k] = v.TitleSuffix
	}
	c.JSON(200, gin.H{
		"has_api_key": aiCfg.APIKey != "",
		"api_base":    aiCfg.BaseURL,
		"model":       aiCfg.Model,
		"sessions":    sessions,
	})
}

func handleSaveConfig(c *gin.Context) {
	var body struct {
		APIKey  string `json:"api_key"`
		APIBase string `json:"api_base"`
		Model   string `json:"model"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	aiCfg := config.GetAIConfig()
	if body.APIKey != "" {
		aiCfg.APIKey = body.APIKey
	}
	if body.APIBase != "" {
		aiCfg.BaseURL = body.APIBase
	}
	if body.Model != "" {
		aiCfg.Model = body.Model
	}
	if err := config.SaveAIConfig(aiCfg); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func handleOptimizeCopy(c *gin.Context) {
	var body struct {
		Date    string `json:"date"`
		Session string `json:"session"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "缺少参数 date 或 session"})
		return
	}
	if body.Date == "" || body.Session == "" {
		c.JSON(400, gin.H{"error": "缺少参数 date 或 session"})
		return
	}

	sectors, err := fetcher.LoadSessionData(body.Date, body.Session)
	if err != nil || len(sectors) == 0 {
		c.JSON(400, gin.H{"error": fmt.Sprintf("%s 无%s板块数据", body.Date, config.SessionConfigs[body.Session].TitleSuffix)})
		return
	}

	aiText, err := copy.GenerateCopywritingAI(sectors, body.Date, body.Session)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	if db, err := storage.Get(); err == nil {
		_ = db.SaveCopywriting(storage.Copywriting{
			Date:    body.Date,
			Session: body.Session,
			Type:    "ai",
			Content: aiText,
		})
	}

	c.JSON(200, gin.H{"text": aiText})
}

func serveVideo(c *gin.Context) {
	dateStr := c.Param("date")
	filename := c.Param("file")
	p := filepath.Join(config.GetOutputDir(), dateStr, filename)
	if _, err := os.Stat(p); err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	c.File(p)
}

func getDates() []string {
	seen := make(map[string]bool)
	dataDir := config.GetDataDir()
	outputDir := config.GetOutputDir()
	copyDir := config.GetCopyDir()

	if entries, err := os.ReadDir(dataDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				p := filepath.Join(dataDir, e.Name())
				if _, err := os.Stat(filepath.Join(p, "sectors.csv")); err == nil {
					seen[e.Name()] = true
				}
			}
		}
	}
	if entries, err := os.ReadDir(outputDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				seen[e.Name()] = true
			}
		}
	}
	if entries, err := os.ReadDir(copyDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				seen[e.Name()] = true
			}
		}
	}

	// Also check SQLite for tick data
	if db, err := storage.Get(); err == nil {
		if dates, err := db.ListTickDates(); err == nil {
			for _, d := range dates {
				seen[d] = true
			}
		}
	}

	var dates []string
	for d := range seen {
		dates = append(dates, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	return dates
}

func getVideos(dateStr string) []string {
	d := filepath.Join(config.GetOutputDir(), dateStr)
	entries, err := os.ReadDir(d)
	if err != nil {
		return []string{}
	}
	var videos []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".mp4") {
			videos = append(videos, strings.TrimSuffix(e.Name(), ".mp4"))
		}
	}
	return videos
}

func getCopy(dateStr string) map[string]map[string]string {
	result := map[string]map[string]string{
		"template": {},
		"ai":       {},
	}
	db, err := storage.Get()
	if err != nil {
		return result
	}
	cwList, err := db.LoadCopywriting(dateStr)
	if err != nil {
		return result
	}
	for _, cw := range cwList {
		label := cw.Session
		if strings.HasSuffix(cw.Type, "_tick") {
			label = cw.Session + "_tick"
		}
		if cw.Type == "ai" || cw.Type == "ai_tick" {
			result["ai"][label] = cw.Content
		} else {
			result["template"][label] = cw.Content
		}
	}
	return result
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	case nil:
		return 0, false
	default:
		return 0, false
	}
}

func roundTo2(x float64) float64 {
	return math.Round(x*100) / 100
}

func handleGenerateTick(c *gin.Context) {
	var body struct {
		Date     string `json:"date"`
		Session  string `json:"session"`
		CopyMode string `json:"copy_mode"`
		Format   string `json:"format"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Date == "" {
		body.Date = time.Now().Format("2006-01-02")
	}
	if body.Session == "" {
		body.Session = "full"
	}
	if body.Format == "" {
		body.Format = "all"
	}

	logger.Info("tick 视频生成请求",
		zap.String("date", body.Date),
		zap.String("session", body.Session),
		zap.String("copyMode", body.CopyMode),
		zap.String("format", body.Format))

	sse := NewSSEWriter(c)
	sse.Send("log", fmt.Sprintf("📅 日期: %s, 时段: %s, 文案: %s",
		body.Date, config.SessionConfigs[body.Session].TitleSuffix,
		map[string]string{"ai": "AI生成", "template": "模板"}[body.CopyMode]))

	sessCfg := config.SessionConfigs[body.Session]

	// 预览 tick 数据量
	if pts, err := tickfetcher.LoadTickCSV(body.Date, body.Session); err == nil {
		timeSet := make(map[string]bool)
		sectorSet := make(map[string]bool)
		for _, p := range pts {
			timeSet[p.Time] = true
			sectorSet[p.Name] = true
		}
		sse.Send("log", fmt.Sprintf("📊 数据预览: %d条记录, %d个时间点, %d个板块", len(pts), len(timeSet), len(sectorSet)))
	} else {
		sse.Send("log", fmt.Sprintf("⚠️ 数据预览失败: %v", err))
	}

	// 在渲染前生成文案，用于 TTS 语音合成
	points, pErr := tickfetcher.LoadTickCSV(body.Date, body.Session)
	var copywriteText string
	cwType := "template_tick"
	if pErr != nil || len(points) == 0 {
		sse.Send("log", "⚠️ 无 tick 数据，跳过文案生成")
	} else {
		sectors := tickPointsToSectors(points)
		sse.Send("progress", "生成文案中...")
		if body.CopyMode == "ai" {
			sse.Send("log", "🤖 AI 文案生成中...")
			var aiErr error
			copywriteText, aiErr = copy.GenerateCopywritingAI(sectors, body.Date, body.Session)
			if aiErr != nil {
				sse.Send("log", fmt.Sprintf("⚠️ AI 生成失败，降级模板: %v", aiErr))
				copywriteText = copy.GenerateCopywriting(sectors, body.Date, body.Session)
			} else {
				cwType = "ai_tick"
			}
		} else {
			sse.Send("log", "📝 模板文案生成中...")
			copywriteText = copy.GenerateCopywriting(sectors, body.Date, body.Session)
		}
	}

	renderMobile := body.Format == "all" || body.Format == "mobile"
	renderTV := body.Format == "all" || body.Format == "tv"

	var newsPagesMobile, newsPagesTV []hotnews.NewsPage
	if renderMobile {
		pages, err := hotnews.LoadForVideo(body.Date, "mobile")
		if err != nil {
			sse.Send("log", fmt.Sprintf("⚠️ 新闻加载失败 (mobile): %v", err))
		} else if len(pages) > 0 {
			newsPagesMobile = pages
			sse.Send("log", fmt.Sprintf("📰 新闻加载完成 (mobile): %d页", len(pages)))
		}
	}
	if renderTV {
		pages, err := hotnews.LoadForVideo(body.Date, "tv")
		if err != nil {
			sse.Send("log", fmt.Sprintf("⚠️ 新闻加载失败 (tv): %v", err))
		} else if len(pages) > 0 {
			newsPagesTV = pages
			sse.Send("log", fmt.Sprintf("📰 新闻加载完成 (tv): %d页", len(pages)))
		}
	}

	if renderMobile {
		outPathMobile := filepath.Join(config.GetOutputDir(), body.Date, fmt.Sprintf("%s_tick_mobile.mp4", sessCfg.FilenameSuffix))
		sse.Send("log", "🎬 开始渲染 Tick 曲线视频 (Mobile 9:16)...")
		sse.Send("progress", "渲染 Mobile 版本...")

		outMobile, rErr := tickrenderer.RenderTickVideo(body.Date, outPathMobile, "mobile", body.Session, nil, nil, nil, copywriteText, newsPagesMobile)
		if rErr != nil {
			sse.Send("log", fmt.Sprintf("⚠️ Mobile 渲染失败: %v", rErr))
		} else {
			var fileInfo string
			if fi, err := os.Stat(outMobile); err == nil {
				fileInfo = fmt.Sprintf("%.1fMB", float64(fi.Size())/1024/1024)
			}
			sse.Send("log", fmt.Sprintf("✅ Tick Mobile 渲染完成 (%s)", fileInfo))
		}
	}

	if renderTV {
		outPathTV := filepath.Join(config.GetOutputDir(), body.Date, fmt.Sprintf("%s_tick.mp4", sessCfg.FilenameSuffix))
		sse.Send("log", "🎬 开始渲染 Tick 曲线视频 (TV 16:9)...")
		sse.Send("progress", "渲染 TV 版本...")

		outTV, rErr := tickrenderer.RenderTickVideo(body.Date, outPathTV, "tv", body.Session, nil, nil, nil, copywriteText, newsPagesTV)
		if rErr != nil {
			sse.Send("error", fmt.Sprintf("TV 渲染失败: %v", rErr))
			return
		}

		var fileInfo string
		if fi, err := os.Stat(outTV); err == nil {
			fileInfo = fmt.Sprintf("%.1fMB", float64(fi.Size())/1024/1024)
		}
		sse.Send("log", fmt.Sprintf("✅ Tick TV 渲染完成 (%s)", fileInfo))
	}

	// 保存文案
	if copywriteText != "" {
		if db, err := storage.Get(); err == nil {
			_ = db.SaveCopywriting(storage.Copywriting{
				Date:    body.Date,
				Session: body.Session,
				Type:    cwType,
				Content: copywriteText,
			})
		}
		sse.Send("log", "✅ 文案已保存")
	}

	sse.Send("log", "✅ Tick 视频生成完成")
	sse.Send("progress", "完成")
	sse.Send("done", "生成完毕")
}

func tickPointsToSectors(points []tickfetcher.TickPoint) []fetcher.Sector {
	latest := make(map[string]tickfetcher.TickPoint)
	for _, p := range points {
		latest[p.Name] = p
	}
	var sectors []fetcher.Sector
	for _, p := range latest {
		sectors = append(sectors, fetcher.Sector{Name: p.Name, Net: p.Net, Rate: p.Rate})
	}
	return sectors
}

func handleTickStream(c *gin.Context, tickSched *tickscheduler.TickScheduler) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeaderNow()
	c.Writer.Flush()

	ch, unsubscribe := tickSched.GetFetcher().Subscribe()
	defer unsubscribe()

	// Send initial snapshot immediately
	snapshot := tickSched.GetFetcher().GetSnapshot()
	data, _ := json.Marshal(snapshot)
	line := fmt.Sprintf(`{"type":"tick","text":%s}`, jsonStr(string(data)))
	c.Writer.Write([]byte(line + "\n"))
	c.Writer.Flush()

	// Stream updates
	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case snap, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(snap)
			line := fmt.Sprintf(`{"type":"tick","text":%s}`, jsonStr(string(data)))
			if _, err := c.Writer.Write([]byte(line + "\n")); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

// handleDashboard 返回仪表盘所需的全量聚合数据（单次请求）。
func handleDashboard(c *gin.Context) {
	dates := getDates()
	var allDates []string
	for _, d := range dates {
		allDates = append(allDates, d)
	}

	resp := gin.H{
		"dates":               allDates,
		"marketOverview":      gin.H{"totalSectors": 0, "inflowCount": 0, "outflowCount": 0, "totalNet": 0, "topSector": nil, "worstSector": nil},
		"ranking":             []gin.H{},
		"events":              []gin.H{},
		"trend":               map[string][]gin.H{},
		"trendDates":          []string{},
	}

	if len(allDates) == 0 {
		c.JSON(200, resp)
		return
	}

	latestDate := allDates[0]
	var sectors []fetcher.Sector
	var err error

	sectors, err = fetcher.LoadSessionData(latestDate, "full")
	if err != nil || len(sectors) == 0 {
		if db, dbErr := storage.Get(); dbErr == nil {
			if records, loadErr := db.LoadSectorsAll(latestDate); loadErr == nil && len(records) > 0 {
	for _, r := range records {
				sectors = append(sectors, fetcher.Sector{Name: r.Name, Net: r.Net, Rate: r.Rate, Category: r.Category})
			}
			}
		}
	}

	netValues := make([]gin.H, 0, len(sectors))
	inflowCount := 0
	outflowCount := 0
	totalNet := 0.0
	for _, s := range sectors {
		netValues = append(netValues, gin.H{"name": s.Name, "net": s.Net, "category": s.Category})
		if s.Net >= 0 {
			inflowCount++
		} else {
			outflowCount++
		}
		totalNet += s.Net
	}
	totalNet = roundTo2(totalNet)

	sort.Slice(netValues, func(i, j int) bool {
		ni, _ := netValues[i]["net"].(float64)
		nj, _ := netValues[j]["net"].(float64)
		return ni > nj
	})

	var topSector *gin.H
	var worstSector *gin.H
	if len(netValues) > 0 {
		top := netValues[0]
		topSector = &top
		w := netValues[len(netValues)-1]
		worstSector = &w
	}

	resp["marketOverview"] = gin.H{
		"totalSectors": len(sectors),
		"inflowCount":  inflowCount,
		"outflowCount": outflowCount,
		"totalNet":     totalNet,
		"topSector":    topSector,
		"worstSector":  worstSector,
	}
	resp["ranking"] = netValues

	top5Names := make([]string, 0, 5)
	for i, item := range netValues {
		if i >= 5 {
			break
		}
		if name, ok := item["name"].(string); ok {
			top5Names = append(top5Names, name)
		}
	}

	if len(top5Names) > 0 && len(allDates) >= 2 {
		startDate := allDates[len(allDates)-1]
		endDate := allDates[0]
		trendData := make(map[string][]gin.H)
		var allTrendDates []string

		if db, dbErr := storage.Get(); dbErr == nil {
			for _, name := range top5Names {
				if records, loadErr := db.LoadSectorTrend(name, startDate, endDate); loadErr == nil {
					points := make([]gin.H, 0, len(records))
					for _, r := range records {
						points = append(points, gin.H{"date": r.Date, "net": r.Net})
					}
					trendData[name] = points
				}
			}

			dateSet := make(map[string]bool)
			for _, points := range trendData {
				for _, p := range points {
					if d, ok := p["date"].(string); ok {
						dateSet[d] = true
					}
				}
			}
			for d := range dateSet {
				allTrendDates = append(allTrendDates, d)
			}
			sort.Strings(allTrendDates)
			resp["trendDates"] = allTrendDates
		}
		resp["trend"] = trendData
	}

	c.JSON(200, resp)
}

func handleNewsList(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	if limit < 1 || limit > 200 {
		limit = 50
	}

	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	records, err := db.LoadLatestNews(limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	total, err := db.GetCLSNewsCount()
	if err != nil {
		total = 0
	}

	if records == nil {
		records = []storage.CLSNewsRecord{}
	}

	c.JSON(200, gin.H{
		"records": records,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

func handleNewsSearch(c *gin.Context) {
	keyword := c.Query("q")
	if keyword == "" {
		c.JSON(400, gin.H{"error": "缺少搜索关键词 q"})
		return
	}

	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	if limit < 1 || limit > 200 {
		limit = 50
	}

	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	records, total, err := db.SearchCLSNews(keyword, limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	if records == nil {
		records = []storage.CLSNewsRecord{}
	}

	c.JSON(200, gin.H{
		"records": records,
		"total":   total,
		"q":       keyword,
		"limit":   limit,
		"offset":  offset,
	})
}

func handleNewsByDate(c *gin.Context) {
	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	if limit < 1 || limit > 200 {
		limit = 50
	}

	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	records, total, err := db.LoadNewsByDate(dateStr, limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	if records == nil {
		records = []storage.CLSNewsRecord{}
	}

	c.JSON(200, gin.H{
		"records": records,
		"total":   total,
		"date":    dateStr,
		"limit":   limit,
		"offset":  offset,
	})
}

func handleListNotes(c *gin.Context) {
	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	notes, err := db.ListNotes()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if notes == nil {
		notes = []storage.Note{}
	}

	c.JSON(200, gin.H{"notes": notes})
}

func handleCreateNote(c *gin.Context) {
	var body struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Content == "" {
		c.JSON(400, gin.H{"error": "内容不能为空"})
		return
	}
	if body.Type != "completed" && body.Type != "planned" {
		body.Type = "planned"
	}

	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	id, err := db.SaveNote(storage.Note{Type: body.Type, Content: body.Content})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"ok": true, "id": id})
}

func handleUpdateNote(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "无效的 id"})
		return
	}

	var body struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Content == "" {
		c.JSON(400, gin.H{"error": "内容不能为空"})
		return
	}
	if body.Type != "completed" && body.Type != "planned" {
		body.Type = "planned"
	}

	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	if err := db.UpdateNote(id, body.Type, body.Content); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"ok": true})
}

func handleDeleteNote(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "无效的 id"})
		return
	}

	db, err := storage.Get()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	if err := db.DeleteNote(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"ok": true})
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := os.Getenv("CORS_ALLOWED_ORIGINS")
		if origin == "" {
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Accept")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
