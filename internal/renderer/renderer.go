package renderer

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/tts"
	"go.uber.org/zap"
)

const (
	FPS         = config.FPS
	TotalFrames = config.TotalFrames
)

type SectorData struct {
	Name  string  `json:"name"`
	Net   float64 `json:"net"`
	Rate  float64 `json:"rate"`
	Color string  `json:"color"`
}

// RenderProps 传递给 Remotion 的 JSON 属性，必须与 TypeScript types.ts 保持一致。
type RenderProps struct {
	DateStr        string                   `json:"dateStr"`
	DisplayDate    string                   `json:"displayDate"`
	TotalFrames    int                      `json:"totalFrames"`
	Sectors        []SectorData             `json:"sectors"`
	TimelineEvents []analyzer.TimelineEvent `json:"timelineEvents,omitempty"`
	TickerItems    []analyzer.TickerItem    `json:"tickerItems,omitempty"`
	Events         []analyzer.MarketEvent   `json:"events,omitempty"`
	Format         string                   `json:"format"`
	Width          int                      `json:"width"`
	Height         int                      `json:"height"`
	Session        string                   `json:"session"`
	XLim           [2]int                   `json:"xLim"`
	// Voiceover fields — if empty, renders without voiceover
	TitleText         string `json:"titleText,omitempty"`
	ContentText       string `json:"contentText,omitempty"`
	TitleAudioFile    string `json:"titleAudioFile,omitempty"`
	ContentAudioFile  string `json:"contentAudioFile,omitempty"`
	TitleAudioFrames  int    `json:"titleAudioFrames,omitempty"`
	ContentAudioFrames int   `json:"contentAudioFrames,omitempty"`
	BaseAnimationFrames int  `json:"baseAnimationFrames,omitempty"`
}

// RenderVideo 调用 Remotion CLI 渲染视频。
// sectors: 板块数据 | dateStr: 日期 | outputPath: 输出路径
// events/timeline/ticker: 分析结果 | format: mobile/tv | session: morning/full
// copywriteText: AI 生成的抖音文案（可选），不为空时会生成 TTS 语音并嵌入视频开头/结尾
func RenderVideo(sectors []fetcher.Sector, dateStr, outputPath string, events []analyzer.MarketEvent, timeline []analyzer.TimelineEvent, ticker []analyzer.TickerItem, format string, session string, copywriteText string) (string, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", fmt.Errorf("parse date: %w", err)
	}
	displayDate := t.Format("01-02")

	compID := "BloombergVideo"
	w, h := config.MobileWidth, config.MobileHeight
	if format == "tv" {
		compID = "BloombergVideoTV"
		w, h = config.TVWidth, config.TVHeight
	}

	sessCfg, ok := config.SessionConfigs[session]
	if !ok {
		sessCfg = config.SessionConfigs["full"]
	}

	sectorData := make([]SectorData, len(sectors))
	for i, s := range sectors {
		sectorData[i] = SectorData{Name: s.Name, Net: s.Net, Rate: s.Rate, Color: s.Color}
	}

	if len(timeline) == 0 {
		_, timeline, ticker = analyzer.DataDrivenGenerate(sectors, session)
	}
	if len(events) == 0 {
		events = analyzer.GetFallbackEvents(TotalFrames)
	}

	props := RenderProps{
		DateStr:        dateStr,
		DisplayDate:    displayDate,
		TotalFrames:    TotalFrames,
		Sectors:        sectorData,
		TimelineEvents: timeline,
		TickerItems:    ticker,
		Events:         events,
		Format:         format,
		Width:          w,
		Height:         h,
		Session:        session,
		XLim:           sessCfg.XLim,
	}

	if copywriteText != "" {
		rendererDir := config.GetRendererDir()
		voiceoverDir := filepath.Join(rendererDir, "public", "voiceover")
		if err := os.MkdirAll(voiceoverDir, 0755); err != nil {
			return "", fmt.Errorf("create voiceover dir: %w", err)
		}

		titleText, contentText := tts.ParseCopywriting(copywriteText)
		titlePath := filepath.Join(voiceoverDir, "title.mp3")
		contentPath := filepath.Join(voiceoverDir, "content.mp3")
		baseFrames := TotalFrames

		if err := tts.TextToSpeech(titleText, titlePath, tts.Xiaoxiao); err != nil {
			logger.Warn("标题 TTS 合成失败，跳过", zap.Error(err))
		}
		if err := tts.TextToSpeech(contentText, contentPath, tts.Xiaoxiao); err != nil {
			logger.Warn("正文 TTS 合成失败，跳过", zap.Error(err))
		}

		titleDur := 0.0
		if _, err := os.Stat(titlePath); err == nil {
			if d, err := tts.GetAudioDuration(titlePath); err == nil {
				titleDur = d
			}
		}
		contentDur := 0.0
		if _, err := os.Stat(contentPath); err == nil {
			if d, err := tts.GetAudioDuration(contentPath); err == nil {
				contentDur = d
			}
		}

		titleFrames := int(math.Ceil(titleDur * FPS))
		contentFrames := int(math.Ceil(contentDur * FPS))

		// 保留给音频播放的头部/尾部静音帧
		const audioPadding = 10 // frames
		if titleFrames > 0 {
			titleFrames += audioPadding
		}
		if contentFrames > 0 {
			contentFrames += audioPadding
		}

		totalVideoFrames := titleFrames + baseFrames + contentFrames

		props.TitleText = titleText
		props.ContentText = contentText
		props.TitleAudioFile = "voiceover/title.mp3"
		props.ContentAudioFile = "voiceover/content.mp3"
		props.TitleAudioFrames = titleFrames
		props.ContentAudioFrames = contentFrames
		props.BaseAnimationFrames = baseFrames
		props.TotalFrames = totalVideoFrames

		logger.Info("语音合成完成",
			zap.Float64("titleAudioSec", titleDur),
			zap.Float64("contentAudioSec", contentDur),
			zap.Int("totalFrames", totalVideoFrames))
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", fmt.Errorf("marshal props: %w", err)
	}

	rendererDir := config.GetRendererDir()
	entry := filepath.Join(rendererDir, "src", "renderer", "index.ts")

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	args := []string{
		"remotion", "render",
		entry,
		compID,
		outputPath,
		"--props", string(propsJSON),
		"--overwrite",
		"--fps", fmt.Sprintf("%d", FPS),
		"--frames", fmt.Sprintf("0-%d", props.TotalFrames-1),
		"--bitrate", "8M",
	}

	logger.Info("remotion 渲染开始",
		zap.Int("sectors", len(sectors)),
		zap.String("output", outputPath),
		zap.String("format", format),
		zap.Int("totalFrames", props.TotalFrames))

	cmd := exec.Command("npx", args...)
	cmd.Dir = rendererDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Remotion render failed: %w", err)
	}

	logger.Info("remotion 渲染完成", zap.String("output", outputPath))
	return outputPath, nil
}
