package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/copy"
	"github.com/a-share-flow-video-go/internal/hotnews"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tick"
	"go.uber.org/zap"
)

func registerTickRoutes(r *gin.Engine, tickSched *tick.TickScheduler) {
	r.GET("/api/tick/status", func(c *gin.Context) {
		c.JSON(200, tickSched.GetStatus())
	})
	r.POST("/api/tick/start", func(c *gin.Context) {
		if err := tickSched.StartManual(); err != nil {
			logger.BadRequest(c, err.Error())
			return
		}
		c.JSON(200, gin.H{"ok": true, "message": "tick 采集已启动"})
	})
	r.POST("/api/tick/force-collect", handleTickForceCollect)
	r.POST("/api/tick/stop", func(c *gin.Context) {
		tickSched.StopManual()
		c.JSON(200, gin.H{"ok": true, "message": "tick 采集已停止"})
	})
	r.POST("/api/tick/enable", func(c *gin.Context) {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			logger.BadRequest(c, err.Error())
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
			logger.BadRequest(c, err.Error())
			return
		}
		tickSched.GetFetcher().SetIntervalMinutes(body.IntervalMinutes)
		c.JSON(200, gin.H{"ok": true, "intervalMinutes": body.IntervalMinutes})
	})
	r.GET("/api/tick-data/:date", func(c *gin.Context) {
		dateStr := c.Param("date")
		session := c.DefaultQuery("session", "full")
		points, err := tick.LoadTickCSV(dateStr, session)
		if err != nil {
			logger.NotFound(c, "no tick data")
			return
		}
		c.JSON(200, gin.H{"date": dateStr, "session": session, "points": points})
	})
	r.POST("/api/generate-tick", handleGenerateTick)
	r.GET("/api/tick/stream", func(c *gin.Context) {
		handleTickStream(c, tickSched)
	})
}

func handleTickForceCollect(c *gin.Context) {
	var body struct {
		Time string `json:"time"`
		Date string `json:"date"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, err.Error())
		return
	}
	if body.Time == "" {
		logger.BadRequest(c, "缺少 time 参数")
		return
	}

	dateStr := body.Date
	if dateStr == "" {
		dateStr = time.Now().In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02")
	}
	datetime := storage.DateToDatetimeTick(dateStr, body.Time)

	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "数据库初始化失败", err)
		return
	}
	exists, _ := db.HasTickData(datetime)
	if exists {
		c.JSON(200, gin.H{"ok": true, "message": fmt.Sprintf("%s %s 的 tick 数据已存在", dateStr, body.Time)})
		return
	}

	sectors, err := tick.FetchTickSectors()
	if err != nil {
		logger.ErrorResponse(c, 502, "东方财富 API 请求失败", err)
		return
	}

	inputDate := time.Now().In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02 15:04:05")
	records := make([]storage.Sector, 0, len(sectors))
	for _, s := range sectors {
		records = append(records, storage.Sector{
			Datetime:             datetime,
			Name:                 s.Name,
			Net:                  s.Net,
			Rate:                 s.Rate,
			ChangePct:            s.ChangePct,
			SuperNet:             s.SuperNet,
			SuperRate:            s.SuperRate,
			BigNet:               s.BigNet,
			BigRate:              s.BigRate,
			Volume:               s.Volume,
			Turnover:             s.Turnover,
			BKCode:               s.BKCode,
			TurnoverRate:         s.TurnoverRate,
			LeadStockName:        s.LeadStockName,
			LeadStockChangePct:   s.LeadStockChangePct,
			TotalMarketCap:       s.TotalMarketCap,
			CirculatingMarketCap: s.CirculatingMarketCap,
			InputDate:            inputDate,
		})
	}
	if err := db.SaveSectors(records); err != nil {
		logger.InternalError(c, "保存失败", err)
		return
	}

	c.JSON(200, gin.H{
		"ok":      true,
		"message": fmt.Sprintf("已强制采集 %s %s 的 tick 数据", dateStr, body.Time),
		"count":   len(records),
	})
}

func handleGenerateTick(c *gin.Context) {
	var body struct {
		Date     string `json:"date"`
		Session  string `json:"session"`
		CopyMode string `json:"copy_mode"`
		Format   string `json:"format"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, err.Error())
		return
	}
	if body.Date == "" {
		body.Date = time.Now().Format("2006-01-02")
	}
	if body.Session == "" {
		body.Session = "full"
	}
	if body.Format == "" {
		body.Format = "mobile"
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

	if pts, err := tick.LoadTickCSV(body.Date, body.Session); err == nil {
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

	points, pErr := tick.LoadTickCSV(body.Date, body.Session)
	var copywriteText string
	cwType := "template_tick"
	if pErr != nil || len(points) == 0 {
		sse.Send("log", "⚠️ 无 tick 数据，跳过文案生成")
	} else {
		sectors := tick.PointsToSectors(points)
		sse.Send("progress", "生成文案中...")
		if body.CopyMode == "ai" {
			sse.Send("log", "🤖 AI 文案生成中...")
			var aiErr error
			prevPrediction := loadPrevCopywriting(body.Date, body.Session)
			copywriteText, aiErr = copy.GenerateCopywritingAI(sectors, body.Date, body.Session, prevPrediction)
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

		outMobile, rErr := tick.RenderTickVideo(body.Date, outPathMobile, "mobile", body.Session, nil, nil, nil, copywriteText, newsPagesMobile)
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

		outTV, rErr := tick.RenderTickVideo(body.Date, outPathTV, "tv", body.Session, nil, nil, nil, copywriteText, newsPagesTV)
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

func handleTickStream(c *gin.Context, tickSched *tick.TickScheduler) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.WriteHeaderNow()
	c.Writer.Flush()

	ch, unsubscribe := tickSched.GetFetcher().Subscribe()
	defer unsubscribe()

	snapshot := tickSched.GetFetcher().GetSnapshot()
	data, _ := json.Marshal(snapshot)
	line := fmt.Sprintf(`{"type":"tick","text":%s}`, jsonStr(string(data)))
	c.Writer.Write([]byte(line + "\n"))
	c.Writer.Flush()

	ctx := c.Request.Context()
	keepalive := time.NewTicker(3 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			if _, err := c.Writer.Write([]byte(": keepalive\n")); err != nil {
				return
			}
			c.Writer.Flush()
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
