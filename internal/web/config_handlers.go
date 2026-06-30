package web

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/copy"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
)

func registerConfigRoutes(r *gin.Engine) {
	r.GET("/api/config", handleGetConfig)
	r.POST("/api/config", handleSaveConfig)
	r.POST("/api/optimize-copy", handleOptimizeCopy)
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
		"models":      aiCfg.Models,
		"sessions":    sessions,
	})
}

func handleSaveConfig(c *gin.Context) {
	var body struct {
		APIKey  string            `json:"api_key"`
		APIBase string            `json:"api_base"`
		Model   string            `json:"model"`
		Models  map[string]string `json:"models"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, err.Error())
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
	if body.Models != nil {
		if aiCfg.Models == nil {
			aiCfg.Models = make(map[string]string)
		}
		for k, v := range body.Models {
			aiCfg.Models[k] = v
		}
	}
	if err := config.SaveAIConfig(aiCfg); err != nil {
		logger.InternalError(c, "保存AI配置失败", err)
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
		logger.BadRequest(c, "缺少参数 date 或 session")
		return
	}
	if body.Date == "" || body.Session == "" {
		logger.BadRequest(c, "缺少参数 date 或 session")
		return
	}

	sectors, err := fetcher.LoadSessionData(body.Date, body.Session)
	if err != nil || len(sectors) == 0 {
		logger.BadRequest(c, fmt.Sprintf("%s 无%s板块数据", body.Date, config.SessionConfigs[body.Session].TitleSuffix))
		return
	}

	prevPrediction := loadPrevCopywriting(body.Date, body.Session)
	brief, _ := copy.BuildNarrativeBrief(sectors, body.Date, body.Session, prevPrediction)
	aiText, err := copy.GenerateCopywritingAI(sectors, body.Date, body.Session, prevPrediction)
	if err != nil {
		logger.InternalError(c, "操作失败", err)
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

	resp := gin.H{"text": aiText}
	if brief != nil {
		resp["brief"] = brief.Format()
	}
	c.JSON(200, resp)
}

func loadPrevCopywriting(todayStr, session string) string {
	prevDate := calcPrevDate(todayStr)
	if prevDate == "" {
		return ""
	}
	db, err := storage.Get()
	if err != nil {
		return ""
	}
	bySession, err := db.LoadCopywritingBySession(prevDate, session)
	if err == nil {
		for _, typ := range []string{"ai", "ai_tick"} {
			if c, ok := bySession[typ]; ok {
				return c
			}
		}
	}
	list, err := db.LoadCopywriting(prevDate)
	if err != nil || len(list) == 0 {
		return ""
	}
	for _, cw := range list {
		if cw.Type == "ai" || cw.Type == "ai_tick" {
			return cw.Content
		}
	}
	return list[0].Content
}

func calcPrevDate(todayStr string) string {
	t, err := time.Parse("2006-01-02", todayStr)
	if err != nil {
		return ""
	}
	for i := 1; i <= 3; i++ {
		d := t.AddDate(0, 0, -i)
		w := d.Weekday()
		if w == time.Saturday || w == time.Sunday {
			continue
		}
		return d.Format("2006-01-02")
	}
	return ""
}
