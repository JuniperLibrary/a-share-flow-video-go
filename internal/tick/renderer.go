package tick

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/hotnews"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tts"
	"go.uber.org/zap"
)

const (
	FPS         = config.FPS
	TotalFrames = config.TotalFrames
)

type SectorTick struct {
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Data      []float64 `json:"data"`
	Times     []string  `json:"times"`
	Rate      float64   `json:"rate"`
	ChangePct float64   `json:"changePct"`
	SuperNet  float64   `json:"superNet"`
	SuperRate float64   `json:"superRate"`
	BigNet    float64   `json:"bigNet"`
	BigRate   float64   `json:"bigRate"`
	MainRate  float64   `json:"mainRate"`
	Volume    float64   `json:"volume"`
	Turnover  float64   `json:"turnover"`
}

type TickRenderProps struct {
	DateStr        string                   `json:"dateStr"`
	DisplayDate    string                   `json:"displayDate"`
	TotalFrames    int                      `json:"totalFrames"`
	SectorTicks    []SectorTick             `json:"sectorTicks"`
	TimelineEvents []analyzer.TimelineEvent `json:"timelineEvents,omitempty"`
	TickerItems    []analyzer.TickerItem    `json:"tickerItems,omitempty"`
	Events         []analyzer.MarketEvent   `json:"events,omitempty"`
	Format         string                   `json:"format"`
	Width          int                      `json:"width"`
	Height         int                      `json:"height"`
	Session        string                   `json:"session"`
	XLim           [2]int                   `json:"xLim"`
	Scene1Text     string                   `json:"scene1Text,omitempty"`
	Scene2Text     string                   `json:"scene2Text,omitempty"`
	Scene3Text     string                   `json:"scene3Text,omitempty"`
	Scene4Text     string                   `json:"scene4Text,omitempty"`
	Scene5Text     string                   `json:"scene5Text,omitempty"`
	Scene1Audio    string                   `json:"scene1Audio,omitempty"`
	Scene2Audio    string                   `json:"scene2Audio,omitempty"`
	Scene3Audio    string                   `json:"scene3Audio,omitempty"`
	Scene4Audio    string                   `json:"scene4Audio,omitempty"`
	Scene5Audio    string                   `json:"scene5Audio,omitempty"`
	Scene1Frames   int                      `json:"scene1Frames,omitempty"`
	Scene2Frames   int                      `json:"scene2Frames,omitempty"`
	Scene3Frames   int                      `json:"scene3Frames,omitempty"`
	Scene4Frames   int                      `json:"scene4Frames,omitempty"`
	Scene5Frames   int                      `json:"scene5Frames,omitempty"`
	NewsPages       []hotnews.NewsPage       `json:"newsPages,omitempty"`
	NewsAudioFiles  []string                 `json:"newsAudioFiles,omitempty"`
	NewsAudioFrames []int                    `json:"newsAudioFrames,omitempty"`
	BaseAnimationFrames int                  `json:"baseAnimationFrames,omitempty"`
}

func RenderTickVideo(dateStr, outputPath, format, session string, events []analyzer.MarketEvent, timeline []analyzer.TimelineEvent, ticker []analyzer.TickerItem, copywriteText string, newsPages []hotnews.NewsPage) (string, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", fmt.Errorf("parse date: %w", err)
	}
	displayDate := t.Format("01-02")

	compID := "BloombergVideoTick"
	w, h := config.MobileWidth, config.MobileHeight
	if format == "tv" {
		compID = "BloombergVideoTickTV"
		w, h = config.TVWidth, config.TVHeight
	}

	sessCfg, ok := config.SessionConfigs[session]
	if !ok {
		sessCfg = config.SessionConfigs["full"]
	}

	points, err := LoadTickCSV(dateStr, session)
	if err != nil || len(points) == 0 {
		return "", fmt.Errorf("no tick data for %s session=%s", dateStr, session)
	}
	logger.Info("tick 数据加载",
		zap.Int("records", len(points)),
		zap.String("date", dateStr),
		zap.String("session", session))

	sectorTicks := buildSectorTicks(points)
	if len(sectorTicks) > 0 {
		topNames := make([]string, 0, 3)
		for i, st := range sectorTicks {
			if i >= 3 {
				break
			}
			topNames = append(topNames, fmt.Sprintf("%s(%dpts)", st.Name, len(st.Data)))
		}
		logger.Info("tick 时序构建",
			zap.Int("sectors", len(sectorTicks)),
			zap.Int("timePoints", len(sectorTicks[0].Times)),
			zap.Strings("top3", topNames),
			zap.Int("totalDataPoints", len(sectorTicks)*len(sectorTicks[0].Times)))
	} else {
		logger.Warn("tick 时序构建为空", zap.String("date", dateStr))
	}

	if len(timeline) == 0 {
		db, dbErr := storage.Get()
		if dbErr == nil {
			cached, _ := db.LoadTickEvents(dateStr, session)
			if cached != nil {
				var payload struct {
					Timeline []analyzer.TimelineEvent `json:"timeline"`
					Events   []analyzer.MarketEvent   `json:"events"`
					Ticker   []analyzer.TickerItem    `json:"ticker"`
				}
				if err := json.Unmarshal(cached, &payload); err == nil && len(payload.Events) > 0 {
					events, timeline, ticker = payload.Events, payload.Timeline, payload.Ticker
					logger.Info("tick 事件从缓存加载", zap.String("date", dateStr), zap.String("session", session))
				}
			}
		}
		if len(events) == 0 {
			events, timeline, ticker = AnalyzeTickContent(points, dateStr, session)
		}
	}
	if len(events) == 0 {
		events = analyzer.GetFallbackEvents(TotalFrames)
		logger.Warn("tick 事件 fallback：使用默认事件",
			zap.String("date", dateStr))
	}

	logger.Info("tick 事件准备就绪",
		zap.Int("events", len(events)),
		zap.Int("timeline", len(timeline)),
		zap.Int("ticker", len(ticker)),
		zap.String("date", dateStr),
		zap.String("session", session))

	props := TickRenderProps{
		DateStr:        dateStr,
		DisplayDate:    displayDate,
		TotalFrames:    TotalFrames,
		SectorTicks:    sectorTicks,
		TimelineEvents: timeline,
		TickerItems:    ticker,
		Events:         events,
		Format:         format,
		Width:          w,
		Height:         h,
		Session:        session,
		XLim:           sessCfg.XLim,
	}

	baseFrames := config.GetBaseFrames(format)
	var newsTotalFrames int
	var newsAudioFiles []string
	var newsAudioFrames []int

	hasVoiceover := copywriteText != "" || len(newsPages) > 0

	if hasVoiceover {
		voiceoverDir := filepath.Join(config.GetRendererDir(), "public", "voiceover")
		if err := os.MkdirAll(voiceoverDir, 0755); err != nil {
			return "", fmt.Errorf("create voiceover dir: %w", err)
		}

		if copywriteText != "" {
			scenes := tts.ParseCopywriting(copywriteText)
			sceneNames := []string{"hook1", "suspense", "twist", "answer", "hook2"}
			sceneTexts := []string{props.Scene1Text, props.Scene2Text, props.Scene3Text, props.Scene4Text, props.Scene5Text}

			for i := 0; i < 5; i++ {
				if scenes[i] == "" {
					continue
				}
				audioPath := filepath.Join(voiceoverDir, fmt.Sprintf("%s.mp3", sceneNames[i]))
				if err := tts.TextToSpeech(scenes[i], audioPath, tts.Xiaoxiao); err != nil {
					logger.Warn("Tick 场景 TTS 合成失败", zap.Int("scene", i+1), zap.Error(err))
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
				sceneTexts[i] = scenes[i]
				switch i {
				case 0:
					props.Scene1Text = scenes[i]
					props.Scene1Audio = fmt.Sprintf("voiceover/%s.mp3", sceneNames[i])
					props.Scene1Frames = frames
				case 1:
					props.Scene2Text = scenes[i]
					props.Scene2Audio = fmt.Sprintf("voiceover/%s.mp3", sceneNames[i])
					props.Scene2Frames = frames
				case 2:
					props.Scene3Text = scenes[i]
					props.Scene3Audio = fmt.Sprintf("voiceover/%s.mp3", sceneNames[i])
					props.Scene3Frames = frames
				case 3:
					props.Scene4Text = scenes[i]
					props.Scene4Audio = fmt.Sprintf("voiceover/%s.mp3", sceneNames[i])
					props.Scene4Frames = frames
				case 4:
					props.Scene5Text = scenes[i]
					props.Scene5Audio = fmt.Sprintf("voiceover/%s.mp3", sceneNames[i])
					props.Scene5Frames = frames
				}
			}

			logger.Info("Tick 文案语音合成完成",
				zap.Int("scenes", 5))
		}

		if len(newsPages) > 0 {
			ttsTexts := hotnews.GenerateTTSText(newsPages)
			for i, text := range ttsTexts {
				newsPath := filepath.Join(voiceoverDir, fmt.Sprintf("news_%d.mp3", i))
				if err := tts.TextToSpeech(text, newsPath, tts.Xiaoxiao); err != nil {
					logger.Warn("Tick 新闻 TTS 合成失败，跳过", zap.Int("page", i), zap.Error(err))
					continue
				}
				dur := 0.0
				if d, err := tts.GetAudioDuration(newsPath); err == nil {
					dur = d
				}
				frames := int(math.Ceil(dur * FPS))
				if frames > 0 {
					frames += 10
				}
				newsAudioFiles = append(newsAudioFiles, fmt.Sprintf("voiceover/news_%d.mp3", i))
				newsAudioFrames = append(newsAudioFrames, frames)
				newsTotalFrames += frames
			}
			logger.Info("Tick 新闻语音合成完成",
				zap.Int("pages", len(newsAudioFiles)),
				zap.Int("newsTotalFrames", newsTotalFrames))
		}
	}

	sceneTotalFrames := props.Scene1Frames + props.Scene2Frames + props.Scene3Frames + props.Scene4Frames + props.Scene5Frames
	totalVideoFrames := sceneTotalFrames + baseFrames + newsTotalFrames

	const mobileTotalCap = 1800
	if format != "tv" && totalVideoFrames > mobileTotalCap {
		over := totalVideoFrames - mobileTotalCap

		if newsTotalFrames > over {
			newsTotalFrames -= over
		} else {
			newsTotalFrames = 0
		}

		if len(newsAudioFrames) > 0 {
			var trimmed []int
			var trimmedFiles []string
			var trimmedPages []hotnews.NewsPage
			remaining := newsTotalFrames
			for i, f := range newsAudioFrames {
				if f <= remaining {
					trimmed = append(trimmed, f)
					trimmedFiles = append(trimmedFiles, newsAudioFiles[i])
					if i < len(newsPages) {
						trimmedPages = append(trimmedPages, newsPages[i])
					}
					remaining -= f
				}
			}
			newsAudioFrames = trimmed
			newsAudioFiles = trimmedFiles
			newsPages = trimmedPages
		}

		totalVideoFrames = sceneTotalFrames + baseFrames + newsTotalFrames
		logger.Warn("移动端总时长超 cap,已截断新闻段",
			zap.Int("totalFrames", totalVideoFrames),
			zap.Int("cap", mobileTotalCap))
	}

	if format != "tv" {
		const sceneCap = 150
		if props.Scene1Frames > sceneCap {
			props.Scene1Frames = sceneCap
		}
		if props.Scene5Frames > sceneCap {
			props.Scene5Frames = sceneCap
		}
	}

	props.BaseAnimationFrames = baseFrames
	props.NewsPages = newsPages
	props.NewsAudioFiles = newsAudioFiles
	props.NewsAudioFrames = newsAudioFrames
	props.TotalFrames = totalVideoFrames

	if hasVoiceover {
		logger.Info("Tick 语音合成完成",
			zap.Int("scene1Frames", props.Scene1Frames),
			zap.Int("scene2Frames", props.Scene2Frames),
			zap.Int("scene3Frames", props.Scene3Frames),
			zap.Int("scene4Frames", props.Scene4Frames),
			zap.Int("scene5Frames", props.Scene5Frames),
			zap.Int("baseFrames", baseFrames),
			zap.Int("newsTotalFrames", newsTotalFrames),
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

	logger.Info("tick 渲染参数",
		zap.Int("propsSize", len(propsJSON)),
		zap.Int("sectors", len(sectorTicks)),
		zap.Int("totalFrames", props.TotalFrames),
		zap.Int("fps", FPS),
		zap.Int("width", w),
		zap.Int("height", h),
		zap.String("compID", compID),
		zap.String("output", outputPath))

	cmd := exec.Command("npx", args...)
	cmd.Dir = rendererDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Remotion tick render failed: %w", err)
	}

	var fileInfo string
	if fi, err := os.Stat(outputPath); err == nil {
		fileInfo = fmt.Sprintf("%.1fMB", float64(fi.Size())/1024/1024)
	}

	logger.Info("tick 渲染完成",
		zap.String("output", outputPath),
		zap.String("fileSize", fileInfo))
	return outputPath, nil
}

func buildSectorTicks(points []TickPoint) []SectorTick {
	timeOrder := uniqueTimes(points)

	sectorData := make(map[string][]float64)
	sectorPrev := make(map[string]float64)
	sectorLatest := make(map[string]TickPoint)

	for _, p := range points {
		prev := sectorPrev[p.Name]
		delta := p.Net - prev
		sectorData[p.Name] = append(sectorData[p.Name], delta)
		sectorPrev[p.Name] = p.Net
		sectorLatest[p.Name] = p
	}

	var result []SectorTick
	for name, data := range sectorData {
		if len(data) < len(timeOrder) {
			padded := make([]float64, len(timeOrder))
			copy(padded, data)
			data = padded
		}
		latest := sectorLatest[name]
		result = append(result, SectorTick{
			Name:      name,
			Data:      data,
			Times:     timeOrder,
			Rate:      latest.Rate,
			ChangePct: latest.ChangePct,
			SuperNet:  latest.SuperNet,
			SuperRate: latest.SuperRate,
			BigNet:    latest.BigNet,
			BigRate:   latest.BigRate,
			MainRate:  latest.MainRate,
			Volume:    latest.Volume,
			Turnover:  latest.Turnover,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		sumI := sumAbs(result[i].Data)
		sumJ := sumAbs(result[j].Data)
		return sumI > sumJ
	})

	return result
}

func uniqueTimes(points []TickPoint) []string {
	seen := make(map[string]bool)
	var times []string
	for _, p := range points {
		if !seen[p.Time] {
			seen[p.Time] = true
			times = append(times, p.Time)
		}
	}
	sort.Strings(times)
	return times
}

func timeMinutes(t string) int {
	var h, m int
	fmt.Sscanf(t, "%d:%d", &h, &m)
	base := 9*60 + 30
	val := h*60 + m - base
	if val < 0 {
		val = 0
	}
	if h >= 13 {
		val -= 90
	}
	return val
}

func sumAbs(data []float64) float64 {
	var s float64
	for _, v := range data {
		if v < 0 {
			s -= v
		} else {
			s += v
		}
	}
	return s
}

func snapshotToSectors(points []TickPoint) []fetcher.Sector {
	latest := make(map[string]TickPoint)
	for _, p := range points {
		latest[p.Name] = p
	}
	var sectors []fetcher.Sector
	for _, p := range latest {
		sectors = append(sectors, fetcher.Sector{Name: p.Name, Net: p.Net, Rate: p.Rate, Volume: p.Volume, Turnover: p.Turnover})
	}
	return sectors
}
