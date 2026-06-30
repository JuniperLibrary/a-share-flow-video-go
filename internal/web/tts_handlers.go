package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/tts"
)

func registerTTSRoutes(r *gin.Engine) {
	r.POST("/api/tts/generate-script", handleTTSScriptGenerate)
	r.POST("/api/tts/synthesize", handleTTSSynthesize)
	r.GET("/api/tts/file/:file", handleTTSFile)
}

func handleTTSSynthesize(c *gin.Context) {
	var body struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, "请求体需为 JSON")
		return
	}

	body.Text = strings.TrimSpace(body.Text)
	if body.Text == "" {
		logger.BadRequest(c, "文本不能为空")
		return
	}
	const maxTTSRunes = 2500
	if len([]rune(body.Text)) > maxTTSRunes {
		logger.BadRequest(c, fmt.Sprintf("文本过长，最多 %d 字", maxTTSRunes))
		return
	}

	logger.Info("TTS 合成请求",
		zap.Int("chars", len([]rune(body.Text))),
	)

	audioDir := filepath.Join(config.GetRendererDir(), "public", "tts")
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		logger.InternalError(c, "创建音频目录失败", err)
		return
	}

	fileName := newTTSFileName()
	outputPath := filepath.Join(audioDir, fileName)
	if err := tts.TextToSpeech(body.Text, outputPath); err != nil {
		logger.InternalError(c, "TTS 合成失败", err)
		return
	}

	duration, _ := tts.GetAudioDuration(outputPath)

	logger.Info("TTS 合成完成",
		zap.String("file", fileName),
		zap.Float64("durationSec", duration),
		zap.Int("chars", len([]rune(body.Text))),
	)

	c.JSON(200, gin.H{
		"file":        fileName,
		"durationSec": duration,
		"chars":       len([]rune(body.Text)),
	})
}

func handleTTSScriptGenerate(c *gin.Context) {
	var body struct {
		Topic string `json:"topic"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		logger.BadRequest(c, "请求体需为 JSON")
		return
	}
	body.Topic = strings.TrimSpace(body.Topic)
	if body.Topic == "" {
		logger.BadRequest(c, "主题不能为空")
		return
	}

	result, err := tts.GenerateScript(body.Topic)
	if err != nil {
		logger.InternalError(c, "口播稿生成失败", err)
		return
	}

	c.JSON(200, gin.H{
		"script": result.Script,
	})
}

func handleTTSFile(c *gin.Context) {
	fileName := filepath.Base(c.Param("file"))
	if fileName == "." || !strings.HasSuffix(fileName, ".mp3") {
		logger.BadRequest(c, "文件名无效")
		return
	}
	p := filepath.Join(config.GetRendererDir(), "public", "tts", fileName)
	if _, err := os.Stat(p); err != nil {
		logger.NotFound(c, "音频文件不存在")
		return
	}
	c.Header("Content-Type", "audio/mpeg")
	c.File(p)
}

func newTTSFileName() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "tts_" + time.Now().Format("20060102_150405") + "_" + hex.EncodeToString(b) + ".mp3"
}
