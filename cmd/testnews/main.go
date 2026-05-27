package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/hotnews"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tts"
	"go.uber.org/zap"
)

func main() {
	if err := logger.Init("info", "console", "stdout"); err != nil {
		panic(err)
	}
	defer logger.Sync()

	if _, err := storage.Get(); err != nil {
		logger.Fatal("SQLite 初始化失败", zap.Error(err))
	}

	dateStr := time.Now().Format("2006-01-02")
	if len(os.Args) > 1 {
		dateStr = os.Args[1]
	}

	logger.Info("=== 热点新闻管道测试 ===", zap.String("date", dateStr))

	// 1. Load news for video (TV format = 3 sectors per page)
	logger.Info("--- 加载新闻 ---")
	pagesTV, err := hotnews.LoadForVideo(dateStr, "tv")
	if err != nil {
		logger.Fatal("新闻加载失败 (tv)", zap.Error(err))
	}
	logger.Info("TV 格式", zap.Int("pages", len(pagesTV)))
	for i, page := range pagesTV {
		sectorNames := make([]string, len(page.Sectors))
		totalNews := 0
		for si, sn := range page.Sectors {
			sectorNames[si] = sn.Sector
			totalNews += len(sn.News)
		}
		logger.Info(fmt.Sprintf("  第%d页: %v (共%d条新闻)", i+1, sectorNames, totalNews))
	}

	// 2. Generate TTS text
	logger.Info("--- 生成 TTS 文本 ---")
	ttsTexts := hotnews.GenerateTTSText(pagesTV)
	for i, text := range ttsTexts {
		logger.Info(fmt.Sprintf("  第%d页 TTS文本 (%d字): %s", i+1, len([]rune(text)), text))
	}

	// 3. Test TTS generation on first page only (save time)
	if len(ttsTexts) > 0 {
		voiceoverDir := filepath.Join(config.GetRendererDir(), "public", "voiceover")
		os.MkdirAll(voiceoverDir, 0755)
		testPath := filepath.Join(voiceoverDir, "test_news_0.mp3")

		logger.Info("--- 测试 TTS 合成 (仅第1页) ---", zap.String("output", testPath))
		if err := tts.TextToSpeech(ttsTexts[0], testPath, tts.Xiaoxiao); err != nil {
			logger.Warn("TTS 合成失败 (非致命)", zap.Error(err))
		} else if fi, err := os.Stat(testPath); err == nil {
			logger.Info("TTS 合成成功",
				zap.Int64("size_bytes", fi.Size()),
				zap.Float64("size_kb", float64(fi.Size())/1024))
			// Clean up test file
			os.Remove(testPath)
		}

		// Also check duration
		dur, err := tts.GetAudioDuration(testPath)
		if err != nil {
			logger.Warn("无法获取音频时长", zap.Error(err))
		} else {
			logger.Info("音频时长", zap.Float64("seconds", dur))
		}
	}

	// 4. Now test mobile format (2 sectors per page)
	pagesMobile, err := hotnews.LoadForVideo(dateStr, "mobile")
	if err != nil {
		logger.Fatal("新闻加载失败 (mobile)", zap.Error(err))
	}
	logger.Info("Mobile 格式", zap.Int("pages", len(pagesMobile)))

	ttsTextsMobile := hotnews.GenerateTTSText(pagesMobile)
	for i, text := range ttsTextsMobile {
		logger.Info(fmt.Sprintf("  第%d页 TTS (%d字): %s...", i+1, len([]rune(text)), text[:min(60, len([]rune(text)))]))
	}

	logger.Info("=== 管道测试完成 ===",
		zap.Int("tvPages", len(pagesTV)),
		zap.Int("mobilePages", len(pagesMobile)))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
