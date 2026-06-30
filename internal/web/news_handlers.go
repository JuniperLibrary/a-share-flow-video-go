package web

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/a-share-flow-video-go/internal/clsnews"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
)

func registerNewsRoutes(r *gin.Engine, newsSched *clsnews.NewsScheduler) {
	if newsSched == nil {
		return
	}

	r.GET("/api/news", handleNewsList)
	r.GET("/api/news/search", handleNewsSearch)
	r.GET("/api/news/date", handleNewsByDate)
	r.GET("/api/news/status", func(c *gin.Context) {
		c.JSON(200, newsSched.Status())
	})
	r.POST("/api/news/retry/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			logger.BadRequest(c, "无效新闻 ID")
			return
		}
		if err := newsSched.RetryNews(id); err != nil {
			logger.BadRequest(c, err.Error())
			return
		}
		c.JSON(200, gin.H{"ok": true, "message": "新闻已重新入队"})
	})
	r.POST("/api/news/retry-batch", func(c *gin.Context) {
		var body struct {
			IDs []int64 `json:"ids"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			logger.BadRequest(c, err.Error())
			return
		}
		if len(body.IDs) == 0 {
			logger.BadRequest(c, "缺少新闻 ID")
			return
		}
		queued, failed := newsSched.RetryNewsBatch(body.IDs)
		c.JSON(200, gin.H{
			"ok":           true,
			"queued_count": queued,
			"failed":       failed,
		})
	})
	r.POST("/api/news/start", func(c *gin.Context) {
		newsSched.Start()
		c.JSON(200, gin.H{"ok": true, "message": "新闻轮询已启动"})
	})
	r.POST("/api/news/stop", func(c *gin.Context) {
		newsSched.Stop()
		c.JSON(200, gin.H{"ok": true, "message": "新闻轮询已停止"})
	})
}

func handleNewsList(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")
	status := c.Query("classify_status")
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	if limit < 1 || limit > 200 {
		limit = 50
	}

	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	records, err := db.LoadLatestNewsByStatus(limit, offset, status)
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	total, err := db.GetCLSNewsCountByStatus(status)
	if err != nil {
		total = 0
	}

	if records == nil {
		records = []storage.CLSNewsRecord{}
	}

	c.JSON(200, gin.H{
		"records":         records,
		"total":           total,
		"limit":           limit,
		"offset":          offset,
		"classify_status": status,
	})
}

func handleNewsSearch(c *gin.Context) {
	keyword := c.Query("q")
	status := c.Query("classify_status")
	if keyword == "" {
		logger.BadRequest(c, "缺少搜索关键词 q")
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
		logger.InternalError(c, "操作失败", err)
		return
	}

	records, total, err := db.SearchCLSNewsByStatus(keyword, limit, offset, status)
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	if records == nil {
		records = []storage.CLSNewsRecord{}
	}

	c.JSON(200, gin.H{
		"records":         records,
		"total":           total,
		"q":               keyword,
		"limit":           limit,
		"offset":          offset,
		"classify_status": status,
	})
}

func handleNewsByDate(c *gin.Context) {
	dateStr := c.Query("date")
	status := c.Query("classify_status")
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
		logger.InternalError(c, "操作失败", err)
		return
	}

	records, total, err := db.LoadNewsByDateAndStatus(dateStr, limit, offset, status)
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	if records == nil {
		records = []storage.CLSNewsRecord{}
	}

	c.JSON(200, gin.H{
		"records":         records,
		"total":           total,
		"date":            dateStr,
		"limit":           limit,
		"offset":          offset,
		"classify_status": status,
	})
}
