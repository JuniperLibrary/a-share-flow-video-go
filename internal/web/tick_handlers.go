package web

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

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
	r.GET("/api/generate-tick/status/:task_id", handleGenerateTickStatus)
	r.POST("/api/generate-tick/cancel/:task_id", handleGenerateTickCancel)
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
	var body tickGenerateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, err.Error())
		return
	}
	normalizeTickGenerateRequest(&body)
	if err := validateTickGenerateRequest(body); err != nil {
		logger.BadRequest(c, err.Error())
		return
	}

	logger.Info("tick 视频生成请求",
		zap.String("date", body.Date),
		zap.String("session", body.Session),
		zap.String("copyMode", body.CopyMode),
		zap.String("format", body.Format))

	task, existing := startOrReuseTickGenerateTask(body)
	status := 202
	if existing {
		status = 200
	}
	c.JSON(status, task.response(existing))
}

func handleGenerateTickStatus(c *gin.Context) {
	taskID := c.Param("task_id")
	task, ok := getTickGenerateTask(taskID)
	if !ok {
		logger.NotFound(c, "任务不存在")
		return
	}
	c.JSON(200, task.response(false))
}

func handleGenerateTickCancel(c *gin.Context) {
	taskID := c.Param("task_id")
	task, ok := getTickGenerateTask(taskID)
	if !ok {
		logger.NotFound(c, "任务不存在")
		return
	}
	if err := task.Cancel(); err != nil {
		logger.BadRequest(c, err.Error())
		return
	}
	c.JSON(200, gin.H{
		"status":  "cancelled",
		"task_id": taskID,
	})
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
