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
	Name      string  `json:"name"`
	Net       float64 `json:"net"`
	Rate      float64 `json:"rate"`
	ChangePct float64 `json:"changePct,omitempty"`
	SuperNet  float64 `json:"superNet,omitempty"`
	SuperRate float64 `json:"superRate,omitempty"`
	BigNet    float64 `json:"bigNet,omitempty"`
	BigRate   float64 `json:"bigRate,omitempty"`
	Volume    float64 `json:"volume,omitempty"`
	Turnover  float64 `json:"turnover,omitempty"`
	Color     string  `json:"color"`
}

// RenderProps 传递给 Remotion 的 JSON 属性，必须与 TypeScript types.ts 保持一致。
type RenderProps struct {
	DateStr             string                   `json:"dateStr"`
	DisplayDate         string                   `json:"displayDate"`
	TotalFrames         int                      `json:"totalFrames"`
	Sectors             []SectorData             `json:"sectors"`
	TimelineEvents      []analyzer.TimelineEvent `json:"timelineEvents,omitempty"`
	TickerItems         []analyzer.TickerItem    `json:"tickerItems,omitempty"`
	Events              []analyzer.MarketEvent   `json:"events,omitempty"`
	Format              string                   `json:"format"`
	Width               int                      `json:"width"`
	Height              int                      `json:"height"`
	Session             string                   `json:"session"`
	XLim                [2]int                   `json:"xLim"`
	Scene1Text          string                   `json:"scene1Text,omitempty"`
	Scene2Text          string                   `json:"scene2Text,omitempty"`
	Scene3Text          string                   `json:"scene3Text,omitempty"`
	Scene4Text          string                   `json:"scene4Text,omitempty"`
	Scene5Text          string                   `json:"scene5Text,omitempty"`
	Scene1Audio         string                   `json:"scene1Audio,omitempty"`
	Scene2Audio         string                   `json:"scene2Audio,omitempty"`
	Scene3Audio         string                   `json:"scene3Audio,omitempty"`
	Scene4Audio         string                   `json:"scene4Audio,omitempty"`
	Scene5Audio         string                   `json:"scene5Audio,omitempty"`
	Scene1Frames        int                      `json:"scene1Frames,omitempty"`
	Scene2Frames        int                      `json:"scene2Frames,omitempty"`
	Scene3Frames        int                      `json:"scene3Frames,omitempty"`
	Scene4Frames        int                      `json:"scene4Frames,omitempty"`
	Scene5Frames        int                      `json:"scene5Frames,omitempty"`
	BaseAnimationFrames int                      `json:"baseAnimationFrames,omitempty"`
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
		sectorData[i] = SectorData{
			Name:      s.Name,
			Net:       s.Net,
			Rate:      s.Rate,
			ChangePct: s.ChangePct,
			SuperNet:  s.SuperNet,
			SuperRate: s.SuperRate,
			BigNet:    s.BigNet,
			BigRate:   s.BigRate,
			Volume:    s.Volume,
			Turnover:  s.Turnover,
			Color:     s.Color,
		}
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
		voiceoverDir, voiceoverPrefix, err := newVoiceoverWorkspace(rendererDir, "video")
		if err != nil {
			return "", fmt.Errorf("create voiceover workspace: %w", err)
		}
		defer os.RemoveAll(voiceoverDir)

		scenes := tts.ParseCopywriting(copywriteText)
		sceneNames := []string{"hook1", "suspense", "twist", "answer", "hook2"}
		baseFrames := TotalFrames

		for i := 0; i < 5; i++ {
			if i >= len(scenes) || scenes[i] == "" {
				continue
			}
			audioPath := filepath.Join(voiceoverDir, fmt.Sprintf("%s.mp3", sceneNames[i]))
			if err := tts.TextToSpeechCommentator(scenes[i], audioPath); err != nil {
				logger.Warn("场景 TTS 合成失败", zap.Int("scene", i+1), zap.Error(err))
				continue
			}
			dur := 0.0
			if _, err := os.Stat(audioPath); err == nil {
				if d, err := tts.GetAudioDuration(audioPath); err == nil {
					dur = d
				}
			}
			frames := int(math.Ceil(dur * FPS))
			const audioPadding = 10
			if frames > 0 {
				frames += audioPadding
			}
			switch i {
			case 0:
				props.Scene1Text = scenes[i]
				props.Scene1Audio = filepath.ToSlash(filepath.Join(voiceoverPrefix, fmt.Sprintf("%s.mp3", sceneNames[i])))
				props.Scene1Frames = frames
			case 1:
				props.Scene2Text = scenes[i]
				props.Scene2Audio = filepath.ToSlash(filepath.Join(voiceoverPrefix, fmt.Sprintf("%s.mp3", sceneNames[i])))
				props.Scene2Frames = frames
			case 2:
				props.Scene3Text = scenes[i]
				props.Scene3Audio = filepath.ToSlash(filepath.Join(voiceoverPrefix, fmt.Sprintf("%s.mp3", sceneNames[i])))
				props.Scene3Frames = frames
			case 3:
				props.Scene4Text = scenes[i]
				props.Scene4Audio = filepath.ToSlash(filepath.Join(voiceoverPrefix, fmt.Sprintf("%s.mp3", sceneNames[i])))
				props.Scene4Frames = frames
			case 4:
				props.Scene5Text = scenes[i]
				props.Scene5Audio = filepath.ToSlash(filepath.Join(voiceoverPrefix, fmt.Sprintf("%s.mp3", sceneNames[i])))
				props.Scene5Frames = frames
			}
		}

		sceneTotalFrames := props.Scene1Frames + props.Scene2Frames + props.Scene3Frames + props.Scene4Frames + props.Scene5Frames
		props.BaseAnimationFrames = baseFrames
		props.TotalFrames = sceneTotalFrames + baseFrames

		logger.Info("语音合成完成",
			zap.Int("totalFrames", props.TotalFrames))
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

func newVoiceoverWorkspace(rendererDir, prefix string) (string, string, error) {
	jobID := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	publicPrefix := filepath.Join("voiceover", jobID)
	workspaceDir := filepath.Join(rendererDir, "public", publicPrefix)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return "", "", err
	}
	return workspaceDir, publicPrefix, nil
}
