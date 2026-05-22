package tickrenderer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tickfetcher"
	"go.uber.org/zap"
)

const (
	FPS         = config.FPS
	TotalFrames = config.TotalFrames
)

type SectorTick struct {
	Name  string    `json:"name"`
	Color string    `json:"color"`
	Data  []float64 `json:"data"`
	Times []string  `json:"times"`
	Rate  float64   `json:"rate"`
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
}

func RenderTickVideo(dateStr, outputPath, format, session string, events []analyzer.MarketEvent, timeline []analyzer.TimelineEvent, ticker []analyzer.TickerItem) (string, error) {
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
	logger.Info("tick 时序构建",
		zap.Int("sectors", len(sectorTicks)),
		zap.Int("timePoints", len(sectorTicks[0].Times)),
		zap.String("date", dateStr))

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
		"--frames", fmt.Sprintf("0-%d", TotalFrames-1),
	}

	logger.Info("tick 渲染开始",
		zap.Int("sectors", len(sectorTicks)),
		zap.String("output", outputPath),
		zap.String("format", format),
		zap.String("session", session))

	cmd := exec.Command("npx", args...)
	cmd.Dir = rendererDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Remotion tick render failed: %w", err)
	}

	logger.Info("tick 渲染完成", zap.String("output", outputPath))
	return outputPath, nil
}

func buildSectorTicks(points []tickfetcher.TickPoint) []SectorTick {
	timeOrder := uniqueTimes(points)

	sectorData := make(map[string][]float64)
	sectorPrev := make(map[string]float64)
	sectorLatestRate := make(map[string]float64)

	for _, p := range points {
		prev := sectorPrev[p.Name]
		delta := p.Net - prev
		sectorData[p.Name] = append(sectorData[p.Name], delta)
		sectorPrev[p.Name] = p.Net
		sectorLatestRate[p.Name] = p.Rate
	}

	var result []SectorTick
	for name, data := range sectorData {
		if len(data) < len(timeOrder) {
			padded := make([]float64, len(timeOrder))
			copy(padded, data)
			data = padded
		}
		result = append(result, SectorTick{
			Name:  name,
			Data:  data,
			Times: timeOrder,
			Rate:  sectorLatestRate[name],
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
