package tickrenderer

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
	"github.com/a-share-flow-video-go/internal/tickfetcher"
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
	// Voiceover fields — if empty, renders without voiceover
	TitleText          string `json:"titleText,omitempty"`
	ContentText        string `json:"contentText,omitempty"`
	TitleAudioFile     string `json:"titleAudioFile,omitempty"`
	ContentAudioFile   string `json:"contentAudioFile,omitempty"`
	TitleAudioFrames   int    `json:"titleAudioFrames,omitempty"`
	ContentAudioFrames int    `json:"contentAudioFrames,omitempty"`
	BaseAnimationFrames int   `json:"baseAnimationFrames,omitempty"`
	// News scene fields
	NewsPages       []hotnews.NewsPage `json:"newsPages,omitempty"`
	NewsAudioFiles  []string           `json:"newsAudioFiles,omitempty"`
	NewsAudioFrames []int              `json:"newsAudioFrames,omitempty"`
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

	points, err := tickfetcher.LoadTickCSV(dateStr, session)
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
			events, timeline, ticker = analyzer.AnalyzeTickContent(points, dateStr, session)
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

	baseFrames := TotalFrames
	var titleFrames, contentFrames int
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
			titleText, contentText := tts.ParseCopywriting(copywriteText)
			titlePath := filepath.Join(voiceoverDir, "title.mp3")
			contentPath := filepath.Join(voiceoverDir, "content.mp3")

			if err := tts.TextToSpeech(titleText, titlePath, tts.Xiaoxiao); err != nil {
				logger.Warn("Tick 标题 TTS 合成失败，跳过", zap.Error(err))
			}
			if err := tts.TextToSpeech(contentText, contentPath, tts.Xiaoxiao); err != nil {
				logger.Warn("Tick 正文 TTS 合成失败，跳过", zap.Error(err))
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

			titleFrames = int(math.Ceil(titleDur * FPS))
			contentFrames = int(math.Ceil(contentDur * FPS))

			const audioPadding = 10
			if titleFrames > 0 {
				titleFrames += audioPadding
			}
			if contentFrames > 0 {
				contentFrames += audioPadding
			}

			props.TitleText = titleText
			props.ContentText = contentText
			props.TitleAudioFile = "voiceover/title.mp3"
			props.ContentAudioFile = "voiceover/content.mp3"
			props.TitleAudioFrames = titleFrames
			props.ContentAudioFrames = contentFrames

			logger.Info("Tick 文案语音合成完成",
				zap.Float64("titleAudioSec", titleDur),
				zap.Float64("contentAudioSec", contentDur))
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

	totalVideoFrames := titleFrames + baseFrames + newsTotalFrames + contentFrames
	props.BaseAnimationFrames = baseFrames
	props.NewsPages = newsPages
	props.NewsAudioFiles = newsAudioFiles
	props.NewsAudioFrames = newsAudioFrames
	props.TotalFrames = totalVideoFrames

	if hasVoiceover {
		logger.Info("Tick 语音合成完成",
			zap.Int("titleFrames", titleFrames),
			zap.Int("baseFrames", baseFrames),
			zap.Int("newsTotalFrames", newsTotalFrames),
			zap.Int("contentFrames", contentFrames),
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

func buildSectorTicks(points []tickfetcher.TickPoint) []SectorTick {
	timeOrder := uniqueTimes(points)

	sectorData := make(map[string][]float64)
	sectorPrev := make(map[string]float64)
	sectorLatest := make(map[string]tickfetcher.TickPoint)

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
		})
	}

	sort.Slice(result, func(i, j int) bool {
		sumI := sumAbs(result[i].Data)
		sumJ := sumAbs(result[j].Data)
		return sumI > sumJ
	})

	return result
}

func uniqueTimes(points []tickfetcher.TickPoint) []string {
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

func snapshotToSectors(points []tickfetcher.TickPoint) []fetcher.Sector {
	latest := make(map[string]tickfetcher.TickPoint)
	for _, p := range points {
		latest[p.Name] = p
	}
	var sectors []fetcher.Sector
	for _, p := range latest {
		sectors = append(sectors, fetcher.Sector{Name: p.Name, Net: p.Net, Rate: p.Rate})
	}
	return sectors
}
