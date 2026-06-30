package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/debate"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
)

type debateMeta struct {
	StockCode     string
	StockName     string
	ReportSummary string
	TurnCount     int
	Format        string
}

var debateTaskMeta = make(map[string]*debateMeta)
var debateMetaMu sync.RWMutex

func registerDebateRoutes(r *gin.Engine) {
	r.POST("/api/debate/generate", handleDebateGenerate)
	r.POST("/api/debate/council/start", handleDebateCouncilStart)
	r.POST("/api/debate/audio", handleDebateAudio)
	r.POST("/api/debate/render", handleDebateRender)
	r.GET("/api/debate/render-status/:task_id", handleDebateRenderStatus)
	r.POST("/api/debate/render-cancel/:task_id", handleDebateRenderCancel)
	r.GET("/api/debate/file/:task_id", handleDebateFile)
	r.POST("/api/debate/fetch-report", handleDebateFetchReport)
	r.POST("/api/debate/search-stock", handleDebateSearchStock)
	r.GET("/api/debate/audio/:task_id/:turn_index", handleDebateAudioFile)
	r.GET("/api/debate/history", handleDebateHistoryList)
	r.GET("/api/debate/history/:task_id", handleDebateHistoryDetail)
	r.DELETE("/api/debate/history/:task_id", handleDebateHistoryDelete)
}

func configureDebateCallbacks() {
	debate.GlobalRenderManager.OnComplete = func(taskID string, probe *debate.ProbeResult) {
		debateMetaMu.RLock()
		meta, ok := debateTaskMeta[taskID]
		debateMetaMu.RUnlock()
		if !ok || meta == nil {
			return
		}
		outputPath := filepath.Join(config.GetOutputDir(), "debate", taskID+".mp4")
		history := storage.DebateHistory{
			TaskID:        taskID,
			StockCode:     meta.StockCode,
			StockName:     meta.StockName,
			ReportSummary: meta.ReportSummary,
			TurnCount:     meta.TurnCount,
			Format:        meta.Format,
			VideoPath:     outputPath,
		}
		if db, err := storage.Get(); err == nil {
			_ = db.SaveDebateHistory(history)
		}
	}
}

func handleDebateGenerate(c *gin.Context) {
	var body struct {
		Report           string         `json:"report"`
		StructuredReport map[string]any `json:"structuredReport,omitempty"`
		StockCode        string         `json:"stockCode,omitempty"`
		StockName        string         `json:"stockName,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, "请求体需为 JSON 且含 report 字段")
		return
	}
	aiCfg := config.GetAIConfigFor("debate")
	if aiCfg.APIKey == "" {
		logger.BadRequest(c, "未配置 AI API Key，请先在 AI 配置页填写")
		return
	}

	script, err := debate.GenerateScript(body.Report, aiCfg, body.StructuredReport)
	if err != nil {
		logger.InternalError(c, "LLM 编排失败", err)
		return
	}

	script.StockCode = body.StockCode
	script.StockName = body.StockName
	if body.StructuredReport != nil {
		if rd, ok := body.StructuredReport["reportDate"]; ok {
			if s, ok := rd.(string); ok {
				script.ReportPeriod = s
			}
		}
	}

	taskID := newDebateTaskID()

	summary := body.Report
	if len([]rune(summary)) > 200 {
		summary = string([]rune(summary)[:200])
	}

	debateMetaMu.Lock()
	debateTaskMeta[taskID] = &debateMeta{
		StockCode:     body.StockCode,
		StockName:     body.StockName,
		ReportSummary: summary,
		TurnCount:     len(script.Turns),
	}
	debateMetaMu.Unlock()

	c.JSON(200, gin.H{
		"taskId": taskID,
		"script": script,
	})
}

func handleDebateCouncilStart(c *gin.Context) {
	var body struct {
		Report           string         `json:"report"`
		StructuredReport map[string]any `json:"structuredReport,omitempty"`
		StockCode        string         `json:"stockCode,omitempty"`
		StockName        string         `json:"stockName,omitempty"`
		ReportPeriod     string         `json:"reportPeriod,omitempty"`
		UseMemory        bool           `json:"useMemory"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, "请求体需为 JSON 且含 report 字段")
		return
	}
	if strings.TrimSpace(body.Report) == "" {
		logger.BadRequest(c, "report 字段不能为空")
		return
	}

	aiCfg := config.GetAIConfigFor("debate")
	if aiCfg.APIKey == "" {
		logger.BadRequest(c, "未配置 AI API Key，请先在 AI 配置页填写")
		return
	}

	reportText := body.Report
	if len(reportText) > 15000 {
		reportText = reportText[:15000]
	}

	state := debate.DebateState{
		ReportText:   reportText,
		StockCode:    body.StockCode,
		StockName:    body.StockName,
		ReportPeriod: body.ReportPeriod,
	}

	if body.StructuredReport != nil {
		state.StructuredMetrics = structuredPromptForOrchestrator(body.StructuredReport)
		if rp, ok := body.StructuredReport["reportDate"].(string); ok && state.ReportPeriod == "" {
			state.ReportPeriod = rp
		}
	}

	orchestrator := debate.NewOrchestrator(aiCfg)
	if body.UseMemory {
		mem, err := debate.NewDebateMemorySQLite(config.GetDBPath())
		if err == nil {
			orchestrator = debate.NewOrchestratorWithOptions(aiCfg, debate.WithMemory(mem))
		}
	}

	script, err := orchestrator.Run(context.Background(), state)
	if err != nil {
		logger.InternalError(c, "辩论编排失败", err)
		return
	}

	taskID := newDebateTaskID()

	summary := body.Report
	if len([]rune(summary)) > 200 {
		summary = string([]rune(summary)[:200])
	}

	debateMetaMu.Lock()
	debateTaskMeta[taskID] = &debateMeta{
		StockCode:     body.StockCode,
		StockName:     body.StockName,
		ReportSummary: summary,
		TurnCount:     len(script.Turns),
	}
	debateMetaMu.Unlock()

	currentPhase := ""
	if len(script.Turns) > 0 {
		currentPhase = string(script.Turns[len(script.Turns)-1].Phase)
	}

	c.JSON(200, gin.H{
		"taskId":    taskID,
		"script":    script,
		"phase":     currentPhase,
		"turnCount": len(script.Turns),
	})
}

// structuredPromptForOrchestrator 从结构化数据生成 metrics 字符串
func structuredPromptForOrchestrator(data map[string]any) string {
	getF := func(key string) float64 {
		v, ok := data[key]
		if !ok || v == nil {
			return 0
		}
		switch n := v.(type) {
		case float64:
			return n
		case string:
			f, _ := strconv.ParseFloat(n, 64)
			return f
		}
		return 0
	}
	getS := func(key string) string {
		v, ok := data[key]
		if !ok || v == nil {
			return ""
		}
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}

	var b strings.Builder
	if name := getS("name"); name != "" {
		b.WriteString(fmt.Sprintf("股票: %s(%s)\n", name, getS("code")))
	}
	b.WriteString(fmt.Sprintf("报告期: %s\n", getS("reportDate")))
	b.WriteString(fmt.Sprintf("报告类型: %s\n", getS("reportType")))

	rev := getF("revenue") / 1e8
	revYoY := getF("revenueYoY")
	np := getF("netProfit") / 1e8
	npYoY := getF("netProfitYoY")
	dr := getF("deductedProfit") / 1e8
	op := getF("operatingProfit") / 1e8

	b.WriteString(fmt.Sprintf("\n核心指标|数值|同比\n"))
	b.WriteString(fmt.Sprintf("营业收入|%.2f亿|%+.1f%%\n", rev, revYoY))
	b.WriteString(fmt.Sprintf("归母净利润|%.2f亿|%+.1f%%\n", np, npYoY))
	b.WriteString(fmt.Sprintf("扣非净利润|%.2f亿\n", dr))
	b.WriteString(fmt.Sprintf("营业利润|%.2f亿\n", op))
	b.WriteString(fmt.Sprintf("毛利率|%.1f%%\n", getF("grossMargin")))
	b.WriteString(fmt.Sprintf("净利率|%.1f%%\n", getF("netMargin")))
	if roe := getF("roe"); roe > 0 {
		b.WriteString(fmt.Sprintf("ROE|%.1f%%\n", roe))
	}
	if eps := getF("eps"); eps > 0 {
		b.WriteString(fmt.Sprintf("EPS|%.2f元\n", eps))
	}
	b.WriteString(fmt.Sprintf("资产负债率|%.1f%%\n", getF("debtAssetRatio")))
	if cr := getF("currentRatio"); cr > 0 {
		b.WriteString(fmt.Sprintf("流动比率|%.1f\n", cr))
	}
	b.WriteString(fmt.Sprintf("合同负债|%.2f亿\n", getF("contractLiability")/1e8))
	b.WriteString(fmt.Sprintf("经营现金流|%.2f亿\n", getF("operatingCashFlow")/1e8))

	return b.String()
}

func handleDebateAudio(c *gin.Context) {
	var body struct {
		TaskID string        `json:"taskId"`
		Script debate.Script `json:"script"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, "请求体需为 JSON 且含 taskId/script 字段")
		return
	}
	if body.TaskID == "" {
		logger.BadRequest(c, "taskID 不能为空")
		return
	}

	audioTurns, err := debate.GenerateAudio(body.TaskID, body.Script)
	if err != nil {
		logger.InternalError(c, "TTS 合成失败", err)
		return
	}

	c.JSON(200, gin.H{
		"taskId":     body.TaskID,
		"audioTurns": audioTurns,
	})
}

func handleDebateRender(c *gin.Context) {
	var body struct {
		TaskID     string             `json:"taskId"`
		Script     debate.Script      `json:"script"`
		AudioTurns []debate.AudioTurn `json:"audioTurns"`
		Format     string             `json:"format"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, "请求体需为 JSON 且含 taskId/script/audioTurns 字段")
		return
	}
	if body.TaskID == "" {
		logger.BadRequest(c, "taskID 不能为空")
		return
	}
	if body.Format == "" {
		body.Format = "mobile"
	}

	debateMetaMu.Lock()
	if meta, ok := debateTaskMeta[body.TaskID]; ok {
		meta.Format = body.Format
	}
	debateMetaMu.Unlock()

	debate.GlobalRenderManager.Start(body.TaskID, body.Script, body.AudioTurns, body.Format)

	c.JSON(200, gin.H{
		"taskId": body.TaskID,
		"status": "pending",
	})
}

func handleDebateRenderStatus(c *gin.Context) {
	taskID := c.Param("task_id")
	task := debate.GlobalRenderManager.GetStatus(taskID)
	if task == nil {
		logger.NotFound(c, "渲染任务不存在")
		return
	}
	resp := gin.H{
		"taskId":   task.ID,
		"status":   task.Status,
		"progress": task.Progress,
	}
	if task.Status == "done" {
		resp["videoUrl"] = "/api/debate/file/" + task.ID
		resp["path"] = task.OutputPath
	}
	if task.Probe != nil {
		resp["probe"] = task.Probe
	}
	if task.Err != "" {
		resp["error"] = task.Err
	}
	c.JSON(200, resp)
}

func handleDebateRenderCancel(c *gin.Context) {
	taskID := c.Param("task_id")
	if err := debate.GlobalRenderManager.Cancel(taskID); err != nil {
		logger.BadRequest(c, err.Error())
		return
	}
	c.JSON(200, gin.H{"status": "cancelled"})
}

func handleDebateFile(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		logger.BadRequest(c, "task_id 不能为空")
		return
	}
	p := filepath.Join(config.GetOutputDir(), "debate", taskID+".mp4")
	if _, err := os.Stat(p); err != nil {
		logger.NotFound(c, "视频文件不存在，可能还在渲染")
		return
	}
	c.File(p)
}

func handleDebateFetchReport(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, "请求体需为 JSON 且含 code 字段")
		return
	}
	if body.Code == "" {
		logger.BadRequest(c, "股票代码不能为空")
		return
	}

	report, err := fetcher.FetchFinanceReport(body.Code)
	if err != nil {
		logger.InternalError(c, "获取财报数据失败", err)
		return
	}

	text := fetcher.FormatFinanceReport(report)

	c.JSON(200, gin.H{
		"report": report,
		"text":   text,
	})
}

func handleDebateSearchStock(c *gin.Context) {
	var body struct {
		Keyword string `json:"keyword"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Keyword == "" {
		logger.BadRequest(c, "请提供搜索关键词")
		return
	}

	stocks, err := fetcher.SearchStock(body.Keyword)
	if err != nil {
		logger.InternalError(c, "搜索失败", err)
		return
	}

	c.JSON(200, gin.H{"stocks": stocks})
}

func handleDebateAudioFile(c *gin.Context) {
	taskID := c.Param("task_id")
	turnIdx := c.Param("turn_index")
	if taskID == "" || turnIdx == "" {
		logger.BadRequest(c, "task_id 和 turn_index 不能为空")
		return
	}
	p := filepath.Join(config.GetRendererDir(), "public", "debate", taskID, fmt.Sprintf("turn_%s.mp3", turnIdx))
	if _, err := os.Stat(p); err != nil {
		logger.NotFound(c, "音频文件不存在")
		return
	}
	c.File(p)
}

func handleDebateHistoryList(c *gin.Context) {
	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "数据库连接失败", err)
		return
	}
	history, err := db.ListDebateHistory(100, 0)
	if err != nil {
		logger.InternalError(c, "查询历史失败", err)
		return
	}
	if history == nil {
		history = []storage.DebateHistory{}
	}
	c.JSON(200, gin.H{"history": history})
}

func handleDebateHistoryDetail(c *gin.Context) {
	taskID := c.Param("task_id")
	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "数据库连接失败", err)
		return
	}
	h, err := db.GetDebateHistory(taskID)
	if err != nil {
		logger.InternalError(c, "查询失败", err)
		return
	}
	if h == nil {
		logger.NotFound(c, "记录不存在")
		return
	}
	c.JSON(200, gin.H{"entry": h})
}

func handleDebateHistoryDelete(c *gin.Context) {
	taskID := c.Param("task_id")
	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "数据库连接失败", err)
		return
	}
	if err := db.DeleteDebateHistory(taskID); err != nil {
		logger.InternalError(c, "删除失败", err)
		return
	}
	c.JSON(200, gin.H{"status": "deleted"})
}

func newDebateTaskID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "d_" + time.Now().Format("20060102_150405") + "_" + hex.EncodeToString(b)
}
