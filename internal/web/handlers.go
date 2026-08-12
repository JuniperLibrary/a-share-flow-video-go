package web

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/clsnews"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/renderer"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tick"
	"go.uber.org/zap"
)

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
	b, err := json.Marshal(s)
	if err != nil {
		logger.Warn("序列化 SSE 文本失败", zap.Error(err))
		return ""
	}
	return string(b)
}

// SetupRouter 注册所有 HTTP 路由。
func SetupRouter(tickSched *tick.TickScheduler, newsSched *clsnews.NewsScheduler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(logger.RecoveryMiddleware())
	r.Use(logger.RequestLoggerMiddleware())
	r.Use(corsMiddleware())

	registerDocsRoute(r)

	r.GET("/api/dates", handleDates)
	r.GET("/api/data/:date", handleData)
	registerExportRoutes(r)
	registerSectorsAllRoutes(r)
	registerSectorWatchlistRoutes(r)

	registerNotesRoutes(r)

	registerDebateRoutes(r)

	r.POST("/api/generate-multiday", handleGenerateMultiDay)
	registerConfigRoutes(r)
	registerTTSRoutes(r)
	registerDailyReportRoutes(r)

	r.GET("/api/files/:date", handleFiles)
	registerTickRoutes(r, tickSched)

	r.GET("/api/dashboard", handleDashboard)

	registerNewsRoutes(r, newsSched)

	r.GET("/output/:date/:file", serveVideo)

	configureDebateCallbacks()

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
		logger.BadRequest(c, err.Error())
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
	if len(tradingDays) == 0 {
		sse.Send("error", "无交易日数据，无法生成多日视频")
		return
	}
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

func serveVideo(c *gin.Context) {
	dateStr := c.Param("date")
	filename := c.Param("file")
	p := filepath.Join(config.GetOutputDir(), dateStr, filename)
	if _, err := os.Stat(p); err != nil {
		logger.NotFound(c, "not found")
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

	datePattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

	var dates []string
	for d := range seen {
		if datePattern.MatchString(d) {
			dates = append(dates, d)
		}
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

// handleDashboard 返回仪表盘所需的全量聚合数据（单次请求）。
func handleDashboard(c *gin.Context) {
	dates := getDates()
	var allDates []string
	for _, d := range dates {
		allDates = append(allDates, d)
	}

	resp := gin.H{
		"dates":          allDates,
		"marketOverview": gin.H{"totalSectors": 0, "inflowCount": 0, "outflowCount": 0, "totalNet": 0, "topSector": nil, "worstSector": nil},
		"ranking":        []gin.H{},
		"events":         []gin.H{},
		"trend":          map[string][]gin.H{},
		"trendDates":     []string{},
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
