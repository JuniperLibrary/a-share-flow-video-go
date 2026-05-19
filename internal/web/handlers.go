package web

import (
	"encoding/csv"
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
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/copy"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/renderer"
	"github.com/a-share-flow-video-go/internal/scheduler"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tickfetcher"
	"github.com/a-share-flow-video-go/internal/tickrenderer"
	"github.com/a-share-flow-video-go/internal/tickscheduler"
)

// ExportTask 异步全量下载任务
type ExportTask struct {
	ID       string
	Date     string
	Status   string
	Progress string
	Page     int
	Data     []fetcher.Sector
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

	fetchPage := func(fs string, pn int) ([]fetcher.Sector, bool, error) {
		url := fmt.Sprintf("https://emdatah5.eastmoney.com/dc/ZJLX/getZDYLBData?fields=f12,f14,f62&pn=%d&pz=500&fid=f62&po=1&fs=%s&ut=b2884a393a59ad64002292a3e90d46a5", pn, fs)
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

		var page []fetcher.Sector
		for _, item := range result.Data.Diff {
			name, _ := item["f14"].(string)
			netVal := item["f62"]
			if name == "" || netVal == nil {
				continue
			}
			if f, ok := toFloat64(netVal); ok && f != 0 {
				page = append(page, fetcher.Sector{
					Name: name,
					Net:  roundTo2(f / 1e8),
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

	f, _ := os.Create(filepath.Join(dateDir, fmt.Sprintf("板块全量_%s.csv", task.Date)))
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"板块名称", "主力资金净流入(亿)", "时间", "趋势"})
	for _, s := range task.Data {
		trend := "↓ 净流出"
		if s.Net > 0 {
			trend = "↑ 净流入"
		}
		w.Write([]string{s.Name, fmt.Sprintf("%.2f", s.Net), task.Date, trend})
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
	c.Writer.WriteHeaderNow()
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
func SetupRouter(sched *scheduler.Scheduler, tickSched *tickscheduler.TickScheduler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(logger.RecoveryMiddleware())
	r.Use(logger.RequestLoggerMiddleware())

	r.GET("/api/dates", handleDates)
	r.GET("/api/data/:date", handleData)
	r.GET("/api/export-all/:date", handleExportAll)
	r.GET("/api/export-all/status/:task_id", handleExportStatus)
	r.GET("/api/export-all/file/:task_id", handleExportFile)
	r.GET("/api/export-hot-sectors/:date", handleExportHotSectors)
	r.POST("/api/generate", handleGenerate)
	r.POST("/api/generate-multiday", handleGenerateMultiDay)
	r.GET("/api/config", handleGetConfig)
	r.POST("/api/config", handleSaveConfig)
	r.POST("/api/optimize-copy", handleOptimizeCopy)
	r.GET("/api/files/:date", handleFiles)
	r.GET("/api/scheduler", func(c *gin.Context) {
		c.JSON(200, sched.GetStatus())
	})
	r.POST("/api/scheduler", func(c *gin.Context) {
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if v, ok := body["enabled"]; ok {
			sched.Enabled = v == true
		}
		if v, ok := body["run_time"]; ok {
			if s, ok := v.(string); ok {
				sched.RunTime = s
				if sched.Enabled {
					sched.Start()
				}
			}
		}
		if v, ok := body["morning_run_time"]; ok {
			if s, ok := v.(string); ok {
				sched.MorningRunTime = s
				if sched.Enabled {
					sched.Start()
				}
			}
		}
		c.JSON(200, sched.GetStatus())
	})
	r.POST("/api/scheduler/run-now", func(c *gin.Context) {
		sched.RunNow()
		c.JSON(200, gin.H{"ok": true, "message": "已触发立即执行"})
	})

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
		handleTickStream(c, tickSched.GetFetcher())
	})

	r.GET("/output/:date/:file", serveVideo)

	distDir := filepath.Join(config.GetProjectRoot(), "web", "frontend", "dist")
	if _, err := os.Stat(distDir); err == nil {
		r.Static("/assets", filepath.Join(distDir, "assets"))
		r.StaticFile("/favicon.ico", filepath.Join(distDir, "favicon.ico"))
		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/output/") {
				c.JSON(404, gin.H{"error": "not found"})
				return
			}
			c.File(filepath.Join(distDir, "index.html"))
		})
	}

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
			"date":           d,
			"videos":         videos,
			"sector_count":   len(sectorsFull),
			"morning_count":  len(sectorsMorning),
			"文案_count":     len(cpy["template"]),
			"ai_count":       len(cpy["ai"]),
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
		"文案":    getCopy(dateStr),
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
		"文案":   cpy,
	})
}

func handleGenerate(c *gin.Context) {
	var body struct {
		Date     string `json:"date"`
		CopyMode string `json:"copy_mode"`
		Format   string `json:"format"`
		Session  string `json:"session"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Date == "" {
		body.Date = time.Now().Format("2006-01-02")
	}
	if body.Format == "" {
		body.Format = "mobile"
	}
	if body.Session == "" {
		body.Session = "full"
	}

	sse := NewSSEWriter(c)

	var hasData bool
	if body.Session == "morning" {
		if db, err := storage.Get(); err == nil {
			if pts, _ := db.LoadTickSectors(body.Date); len(pts) > 0 {
				hasData = true
			}
		}
	} else {
		cachePath := filepath.Join(config.GetDataDir(), body.Date, "sectors.csv")
		if _, err := os.Stat(cachePath); err == nil {
			hasData = true
		}
	}
	if !hasData {
		sse.Send("log", fmt.Sprintf("❌ %s 无%s数据，请先拉取", body.Date, config.SessionConfigs[body.Session].TitleSuffix))
		sse.Send("error", fmt.Sprintf("%s 无数据，请先拉取", body.Date))
		return
	}

	sectors, err := fetcher.LoadSessionData(body.Date, body.Session)
	if err != nil || len(sectors) == 0 {
		sse.Send("error", "加载板块数据失败")
		return
	}
	sse.Send("log", fmt.Sprintf("✅ 已加载 %d 个板块", len(sectors)))
	sse.Send("progress", fmt.Sprintf("已加载 %d 个板块", len(sectors)))

	var events, timeline, ticker any
	if len(sectors) > 0 {
		ev, tl, tk := analyzer.AnalyzeAllContent(sectors, body.Date, body.Session)
		events, timeline, ticker = ev, tl, tk
		if len(ev) == 0 {
			events = analyzer.GetFallbackEvents(config.TotalFrames)
			sse.Send("log", "⚠️ 大模型分析失败，使用默认事件")
		} else {
			sse.Send("log", fmt.Sprintf("✅ 大模型生成 %d 个市场事件", len(ev)))
		}
	} else {
		events = analyzer.GetFallbackEvents(config.TotalFrames)
		sse.Send("log", "⚠️ 无数据，使用默认事件")
	}

	sessCfg := config.SessionConfigs[body.Session]
	fmtLabel := sessCfg.TitleSuffix + " "
	if body.Format == "mobile" {
		fmtLabel += "App (9:16)"
	} else {
		fmtLabel += "TV (16:9)"
	}
	sse.Send("log", fmt.Sprintf("🎬 开始渲染: %s", fmtLabel))
	sse.Send("log", fmt.Sprintf("▶️ 开始生成 %s %s视频 (%s)...", body.Date, sessCfg.TitleSuffix, fmtLabel))
	sse.Send("progress", "生成视频...")

	outputDir := config.GetOutputDir()
	formatSuffix := ""
	if body.Format == "tv" {
		formatSuffix = "_tv"
	}
	outPath := filepath.Join(outputDir, body.Date, fmt.Sprintf("%s%s.mp4", sessCfg.FilenameSuffix, formatSuffix))

	evSlice := toMarketEvents(events)
	tlSlice := toTimelineEvents(timeline)
	tkSlice := toTickerItems(ticker)

	if _, err := renderer.RenderVideo(sectors, body.Date, outPath, evSlice, tlSlice, tkSlice, body.Format, body.Session); err != nil {
		sse.Send("error", fmt.Sprintf("渲染失败: %v", err))
		return
	}
	sse.Send("log", fmt.Sprintf("✅ %s视频生成完成", sessCfg.TitleSuffix))

	fn := copy.GenerateCopywriting
	sessions := []string{"full", "morning"}
	if body.CopyMode == "ai" {
		sse.Send("log", "✍️ 生成文案（AI模式）...")
		for _, sess := range sessions {
			sessCfg := config.SessionConfigs[sess]
			aiText, err := copy.GenerateCopywritingAI(sectors, body.Date, sess)
			if err != nil {
				sse.Send("log", fmt.Sprintf("⚠️ AI文案(%s)生成失败: %v", sessCfg.TitleSuffix, err))
			} else {
				if db, err := storage.Get(); err == nil {
					_ = db.SaveCopywriting(storage.Copywriting{
						Date:    body.Date,
						Session: sess,
						Type:    "ai",
						Content: aiText,
					})
				}
				sse.Send("log", fmt.Sprintf("✅ %s文案已保存", sessCfg.TitleSuffix))
			}
		}
	} else {
		sse.Send("log", "✍️ 生成文案（模板模式）...")
		for _, sess := range sessions {
			text := fn(sectors, body.Date, sess)
			if db, err := storage.Get(); err == nil {
				_ = db.SaveCopywriting(storage.Copywriting{
					Date:    body.Date,
					Session: sess,
					Type:    "template",
					Content: text,
				})
			}
		}
		sse.Send("log", "✅ 文案已保存")
	}

	sse.Send("progress", "完成")
	sse.Send("done", "生成完毕")
}

func handleGenerateMultiDay(c *gin.Context) {
	var body struct {
		Date     string `json:"date"`
		Days     int    `json:"days"`
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
	if body.Days < 2 {
		body.Days = 3
	}
	if body.Format == "" {
		body.Format = "mobile"
	}

	sse := NewSSEWriter(c)

	tradingDays, err := fetcher.GetTradingDays(body.Date, body.Days)
	if err != nil {
		sse.Send("error", fmt.Sprintf("无法获取交易日: %v", err))
		return
	}
	sse.Send("log", fmt.Sprintf("📅 交易日: %s", tradingDays))

	dayData, err := fetcher.LoadMultiDaySectors(tradingDays)
	if err != nil {
		sse.Send("error", fmt.Sprintf("加载数据失败: %v", err))
		return
	}
	sse.Send("log", fmt.Sprintf("✅ 成功加载 %d 日数据", len(dayData)))

	analysis := analyzer.MultiDayAnalyze(dayData, tradingDays)
	sse.Send("log", fmt.Sprintf("✅ 趋势分析完成: %d条洞察 + %d条排名变化", len(analysis.TrendInsights), len(analysis.RankingChanges)))

	outputDir := config.GetOutputDir()
	dateLabel := tradingDays[0]
	if len(tradingDays) > 1 {
		dateLabel = tradingDays[0] + "_to_" + tradingDays[len(tradingDays)-1]
	}
	os.MkdirAll(filepath.Join(outputDir, dateLabel), 0755)

	formatSuffix := ""
	if body.Format == "tv" {
		formatSuffix = "_tv"
	}
	outPath := filepath.Join(outputDir, dateLabel, fmt.Sprintf("三日资金流向%s.mp4", formatSuffix))

	sse.Send("log", fmt.Sprintf("🎬 开始渲染 Bar Chart Race 视频 (%s)...", body.Format))
	sse.Send("progress", "渲染视频中...")

	if _, err := renderer.RenderMultiDayVideo(dayData, tradingDays, outPath, analysis, body.Format); err != nil {
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

func toMarketEvents(v any) []analyzer.MarketEvent {
	if v == nil {
		return nil
	}
	if s, ok := v.([]analyzer.MarketEvent); ok {
		return s
	}
	return nil
}

func toTimelineEvents(v any) []analyzer.TimelineEvent {
	if v == nil {
		return nil
	}
	if s, ok := v.([]analyzer.TimelineEvent); ok {
		return s
	}
	return nil
}

func toTickerItems(v any) []analyzer.TickerItem {
	if v == nil {
		return nil
	}
	if s, ok := v.([]analyzer.TickerItem); ok {
		return s
	}
	return nil
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
		Format   string `json:"format"`
		Session  string `json:"session"`
		CopyMode string `json:"copy_mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Date == "" {
		body.Date = time.Now().Format("2006-01-02")
	}
	if body.Format == "" {
		body.Format = "mobile"
	}
	if body.Session == "" {
		body.Session = "full"
	}

	sessCfg := config.SessionConfigs[body.Session]
	formatSuffix := ""
	if body.Format == "tv" {
		formatSuffix = "_tv"
	}
	outPath := filepath.Join(config.GetOutputDir(), body.Date, fmt.Sprintf("%s_tick%s.mp4", sessCfg.FilenameSuffix, formatSuffix))

	out, err := tickrenderer.RenderTickVideo(body.Date, outPath, body.Format, body.Session, nil, nil, nil)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	points, err := tickfetcher.LoadTickCSV(body.Date, body.Session)
	if err == nil && len(points) > 0 {
		sectors := tickPointsToSectors(points)

		var text string
		cwType := "template_tick"
		if body.CopyMode == "ai" {
			text, err = copy.GenerateCopywritingAI(sectors, body.Date, body.Session)
			if err != nil {
				text = copy.GenerateCopywriting(sectors, body.Date, body.Session)
			} else {
				cwType = "ai_tick"
			}
		} else {
			text = copy.GenerateCopywriting(sectors, body.Date, body.Session)
		}

		if db, err := storage.Get(); err == nil {
			_ = db.SaveCopywriting(storage.Copywriting{
				Date:    body.Date,
				Session: body.Session,
				Type:    cwType,
				Content: text,
			})
		}
	}

	c.JSON(200, gin.H{"ok": true, "output": out})
}

func tickPointsToSectors(points []tickfetcher.TickPoint) []fetcher.Sector {
	latest := make(map[string]float64)
	for _, p := range points {
		latest[p.Name] = p.Net
	}
	var sectors []fetcher.Sector
	for name, net := range latest {
		sectors = append(sectors, fetcher.Sector{Name: name, Net: net})
	}
	return sectors
}

func handleTickStream(c *gin.Context, tf *tickfetcher.TickFetcher) {
	sse := NewSSEWriter(c)

	// 先推送当前快照（历史数据）
	snapshot := tf.GetSnapshot()
	b, _ := json.Marshal(snapshot)
	sse.Send("tick", string(b))

	// 订阅新 tick
	ch, unsubscribe := tf.Subscribe()
	defer unsubscribe()

	// 心跳：每 15 秒发一次，防止连接超时
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			sse.Send("heartbeat", "")
		case snap, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(snap)
			sse.Send("tick", string(b))
		}
	}
}
