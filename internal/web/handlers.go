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
	"github.com/a-share-flow-video-go/internal/renderer"
	"github.com/a-share-flow-video-go/internal/scheduler"
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
					Name:   name,
					Net:    roundTo2(f / 1e8),
					Source: "eastmoney",
				})
			}
		}
		return page, len(result.Data.Diff) < 100, nil
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
func SetupRouter(sched *scheduler.Scheduler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.GET("/api/dates", handleDates)
	r.GET("/api/data/:date", handleData)
	r.GET("/api/export-all/:date", handleExportAll)
	r.GET("/api/export-all/status/:task_id", handleExportStatus)
	r.GET("/api/export-all/file/:task_id", handleExportFile)
	r.GET("/api/export-hot15/:date", handleExportHot15)
	r.POST("/api/fetch", handleFetch)
	r.POST("/api/generate", handleGenerate)
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
		c.JSON(200, sched.GetStatus())
	})
	r.POST("/api/scheduler/run-now", func(c *gin.Context) {
		sched.RunNow()
		c.JSON(200, gin.H{"ok": true, "message": "已触发立即执行"})
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
		sectors, _ := fetcher.LoadCachedData(d)
		items = append(items, map[string]any{
			"date":       d,
			"videos":     videos,
			"sector_count": len(sectors),
			"文案_count": len(cpy["template"]),
			"ai_count":   len(cpy["ai"]),
		})
	}
	c.JSON(200, gin.H{"dates": items})
}

func handleData(c *gin.Context) {
	dateStr := c.Param("date")
	sectors, _ := fetcher.LoadCachedData(dateStr)
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

func handleExportHot15(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	sectors, err := fetcher.FetchTop15HotSectors()
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
		label := strings.TrimSuffix(v, "_tv")
		if v == "全天" {
			videoMap["全天"] = "全天.mp4"
		} else if v == "全天_tv" {
			videoMap["全天_tv"] = "全天_tv.mp4"
		} else {
			videoMap[label] = v + ".mp4"
		}
	}

	c.JSON(200, gin.H{
		"videos": videoMap,
		"文案":   cpy,
	})
}

func handleFetch(c *gin.Context) {
	var body struct {
		Date  string `json:"date"`
		Force bool   `json:"force"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		body.Date = time.Now().Format("2006-01-02")
	}
	if body.Date == "" {
		body.Date = time.Now().Format("2006-01-02")
	}

	sse := NewSSEWriter(c)

	runPipeline := func() {
		today := time.Now().Format("2006-01-02")
		cachePath := filepath.Join(config.GetDataDir(), body.Date, "sectors.csv")

		if !body.Force {
			if _, err := os.Stat(cachePath); err == nil {
				sectors, err := fetcher.LoadCachedData(body.Date)
				if err == nil && len(sectors) > 0 {
					sse.Send("log", fmt.Sprintf("缓存命中，加载 %s 本地数据", body.Date))
					data, _ := json.Marshal(map[string]any{
						"sectors": sectors,
						"source":  "cache",
					})
					sse.Send("data", string(data))
					sse.Send("__done__", "")
					return
				}
			}
		}

		if body.Force {
			sse.Send("log", "已跳过缓存，强制从东方财富远程拉取...")
		} else {
			sse.Send("log", "检查东方财富本地缓存中...")
		}

		var sectors []fetcher.Sector
		var err error

		if body.Date == today {
			sse.Send("log", "正在获取热门板块数据（东方财富）...")
			sectors, err = fetcher.FetchTop15HotSectors()
			if err != nil {
				sse.Send("error", err.Error())
				sse.Send("__done__", "")
				return
			}
			sse.Send("log", fmt.Sprintf("热门板块获取完成：%d 个", len(sectors)))
		} else {
			sse.Send("log", fmt.Sprintf("正在获取 %s 历史数据...", body.Date))
			sectors, err = fetcher.FetchHistoricalSectors(body.Date)
			if err != nil || len(sectors) == 0 {
				sse.Send("error", fmt.Sprintf("%s 无有效交易数据，该日期可能非交易日", body.Date))
				sse.Send("__done__", "")
				return
			}
			sse.Send("log", fmt.Sprintf("历史数据获取完成：%d 个", len(sectors)))
		}

		sse.Send("log", "保存数据到本地缓存...")
		fetcher.SaveDailyData(sectors, body.Date)
		sse.Send("log", "数据已保存")

		data, _ := json.Marshal(map[string]any{
			"sectors": sectors,
			"source":  "eastmoney",
		})
		sse.Send("data", string(data))
		sse.Send("__done__", "")
	}

	done := make(chan struct{})
	go func() {
		runPipeline()
		close(done)
	}()

	select {
	case <-done:
	case <-c.Request.Context().Done():
	}
}

func handleGenerate(c *gin.Context) {
	var body struct {
		Date   string `json:"date"`
		CopyMode string `json:"copy_mode"`
		Format string `json:"format"`
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

	sse := NewSSEWriter(c)

	cachePath := filepath.Join(config.GetDataDir(), body.Date, "sectors.csv")
	if _, err := os.Stat(cachePath); err != nil {
		sse.Send("log", fmt.Sprintf("❌ %s 无数据，请先拉取", body.Date))
		sse.Send("error", fmt.Sprintf("%s 无数据，请先拉取", body.Date))
		return
	}

	sectors, err := fetcher.LoadCachedData(body.Date)
	if err != nil || len(sectors) == 0 {
		sse.Send("error", "加载板块数据失败")
		return
	}
	sse.Send("log", fmt.Sprintf("✅ 已加载 %d 个板块", len(sectors)))
	sse.Send("progress", fmt.Sprintf("已加载 %d 个板块", len(sectors)))

	var events, timeline, ticker any
	if len(sectors) > 0 {
		ev, tl, tk := analyzer.AnalyzeAllContent(sectors, body.Date)
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

	fmtLabel := "App (9:16)"
	if body.Format == "tv" {
		fmtLabel = "TV (16:9)"
	}
	sse.Send("log", fmt.Sprintf("🎬 开始渲染: %s", fmtLabel))
	sse.Send("log", fmt.Sprintf("▶️ 开始生成 %s 全天视频 (%s)...", body.Date, fmtLabel))
	sse.Send("progress", "生成视频...")

	outputDir := config.GetOutputDir()
	formatSuffix := ""
	if body.Format == "tv" {
		formatSuffix = "_tv"
	}
	outPath := filepath.Join(outputDir, body.Date, fmt.Sprintf("全天%s.mp4", formatSuffix))

	evSlice := toMarketEvents(events)
	tlSlice := toTimelineEvents(timeline)
	tkSlice := toTickerItems(ticker)

	if _, err := renderer.RenderVideo(sectors, body.Date, outPath, evSlice, tlSlice, tkSlice, body.Format); err != nil {
		sse.Send("error", fmt.Sprintf("渲染失败: %v", err))
		return
	}
	sse.Send("log", "✅ 全天视频生成完成")

	fn := copy.GenerateCopywriting
	copyDir := filepath.Join(config.GetCopyDir(), body.Date)
	os.MkdirAll(copyDir, 0755)

	sessions := []string{"full", "morning", "afternoon"}
	if body.CopyMode == "ai" {
		sse.Send("log", "✍️ 生成文案（AI模式）...")
		for _, sess := range sessions {
			sessCfg := config.SessionConfigs[sess]
			aiText, err := copy.GenerateCopywritingAI(sectors, body.Date, sess)
			if err != nil {
				sse.Send("log", fmt.Sprintf("⚠️ AI文案(%s)生成失败: %v", sessCfg.TitleSuffix, err))
			} else {
				os.WriteFile(filepath.Join(copyDir, fmt.Sprintf("文案_ai_%s.txt", sessCfg.TitleSuffix)), []byte(aiText), 0644)
				sse.Send("log", fmt.Sprintf("✅ %s文案已保存", sessCfg.TitleSuffix))
			}
		}
	} else {
		sse.Send("log", "✍️ 生成文案（模板模式）...")
		for _, sess := range sessions {
			sessCfg := config.SessionConfigs[sess]
			text := fn(sectors, body.Date, sess)
			os.WriteFile(filepath.Join(copyDir, fmt.Sprintf("文案_%s.txt", sessCfg.TitleSuffix)), []byte(text), 0644)
		}
		sse.Send("log", "✅ 文案已保存")
	}

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

	sectors, err := fetcher.LoadCachedData(body.Date)
	if err != nil || len(sectors) == 0 {
		c.JSON(400, gin.H{"error": fmt.Sprintf("%s 无板块数据", body.Date)})
		return
	}

	aiText, err := copy.GenerateCopywritingAI(sectors, body.Date, body.Session)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	sessCfg, ok := config.SessionConfigs[body.Session]
	label := body.Session
	if ok {
		label = sessCfg.TitleSuffix
	}
	os.MkdirAll(filepath.Join(config.GetCopyDir(), body.Date), 0755)
	os.WriteFile(filepath.Join(config.GetCopyDir(), body.Date, fmt.Sprintf("copy_ai_%s.txt", label)), []byte(aiText), 0644)

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
		return nil
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
	d := filepath.Join(config.GetCopyDir(), dateStr)
	result := map[string]map[string]string{
		"template": {},
		"ai":       {},
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		return result
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "文案_ai_") && strings.HasSuffix(name, ".txt") {
			label := strings.TrimSuffix(strings.TrimPrefix(name, "文案_ai_"), ".txt")
			if b, err := os.ReadFile(filepath.Join(d, name)); err == nil {
				result["ai"][label] = string(b)
			}
		} else if strings.HasPrefix(name, "文案_") && !strings.HasPrefix(name, "文案_ai_") && strings.HasSuffix(name, ".txt") {
			label := strings.TrimSuffix(strings.TrimPrefix(name, "文案_"), ".txt")
			if b, err := os.ReadFile(filepath.Join(d, name)); err == nil {
				result["template"][label] = string(b)
			}
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
