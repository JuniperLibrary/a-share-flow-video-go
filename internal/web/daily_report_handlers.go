package web

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/report"
	"github.com/a-share-flow-video-go/internal/storage"
)

func registerDailyReportRoutes(r *gin.Engine) {
	r.GET("/api/daily-report/dates", handleDailyReportDates)
	r.GET("/api/daily-report/:date", handleDailyReport)
	r.POST("/api/daily-report/generate", handleDailyReportGenerate)
}

// handleDailyReportDates 返回所有已有日报的日期列表。
func handleDailyReportDates(c *gin.Context) {
	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	dates, err := db.ListDailyReportDates()
	if err != nil {
		logger.InternalError(c, "查询日报列表失败", err)
		return
	}
	if dates == nil {
		dates = []string{}
	}

	c.JSON(200, gin.H{"dates": dates})
}

// reportMetrics 从日报 JSON 字符串中提取的扁平化指标。
type reportMetrics struct {
	NetTotal      float64 `json:"netTotal"`
	InflowCount   int     `json:"inflowCount"`
	OutflowCount  int     `json:"outflowCount"`
	SuperNetTotal float64 `json:"superNetTotal"`
	BigNetTotal   float64 `json:"bigNetTotal"`
	StructureDesc string  `json:"structureDesc"`
}

// extractReportMetrics 从日报 JSON 中提取扁平化指标，解析失败返回零值。
func extractReportMetrics(reportJSON string) reportMetrics {
	if reportJSON == "" {
		return reportMetrics{}
	}
	var m reportMetrics
	if err := json.Unmarshal([]byte(reportJSON), &m); err != nil {
		logger.Warn("解析日报指标失败，使用零值: " + err.Error())
	}
	return m
}

// dailyReportResponse 构建日报 API 统一响应体。
func dailyReportResponse(dateStr, session, summary, outlook, reportJSON string, generated bool) gin.H {
	m := extractReportMetrics(reportJSON)
	resp := gin.H{
		"date":          dateStr,
		"session":       session,
		"summary":       summary,
		"outlook":       outlook,
		"report":        reportJSON,
		"netTotal":      m.NetTotal,
		"inflowCount":   m.InflowCount,
		"outflowCount":  m.OutflowCount,
		"superNetTotal": m.SuperNetTotal,
		"bigNetTotal":   m.BigNetTotal,
		"structureDesc": m.StructureDesc,
	}
	if generated {
		resp["generated"] = true
	}
	return resp
}

// validDateFormat 验证日期格式是否为 YYYY-MM-DD。
func validDateFormat(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Format("2006-01-02") == s
}

func handleDailyReport(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		logger.BadRequest(c, "缺少日期参数")
		return
	}
	if !validDateFormat(dateStr) {
		logger.BadRequest(c, "日期格式错误，应为 YYYY-MM-DD")
		return
	}

	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	reportJSON, session, summary, outlook, err := db.LoadDailyReport(dateStr)
	if err != nil {
		logger.InternalError(c, "查询日报失败", err)
		return
	}

	if reportJSON == "" {
		c.JSON(404, gin.H{"error": "该日期暂无日报，请先生成"})
		return
	}

	c.JSON(200, dailyReportResponse(dateStr, session, summary, outlook, reportJSON, false))
}

func handleDailyReportGenerate(c *gin.Context) {
	var req struct {
		Date string `json:"date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.BadRequest(c, "请求体格式错误: "+err.Error())
		return
	}
	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}
	if !validDateFormat(req.Date) {
		logger.BadRequest(c, "日期格式错误，应为 YYYY-MM-DD")
		return
	}

	session := "full"
	r, err := report.Generate(req.Date)
	if err != nil {
		logger.InternalError(c, "日报生成失败", err)
		return
	}

	reportBytes, err := json.Marshal(r)
	if err != nil {
		logger.InternalError(c, "序列化日报失败", err)
		return
	}
	c.JSON(200, dailyReportResponse(req.Date, session, r.Summary, r.Outlook, string(reportBytes), true))
}
