// Package renderer 提供 Python → Remotion 渲染桥接。
// 将 Go 端的板块数据、事件分析结果序列化为 JSON props，
// 通过 `npx remotion render` CLI 调用 React 前端渲染 MP4 视频。
package renderer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

const (
	FPS         = config.FPS
	TotalFrames = config.TotalFrames
)

type SectorData struct {
	Name  string  `json:"name"`
	Net   float64 `json:"net"`
	Color string  `json:"color"`
}

// RenderProps 传递给 Remotion 的 JSON 属性，必须与 TypeScript types.ts 保持一致。
// 任何字段变更需同步修改前端 TypeScript 类型定义。
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
}

// RenderVideo 调用 Remotion CLI 渲染视频。
// sectors: 板块数据 | dateStr: 日期 | outputPath: 输出路径
// events/timeline/ticker: 分析结果 | format: mobile/tv | session: morning/full
func RenderVideo(sectors []fetcher.Sector, dateStr, outputPath string, events []analyzer.MarketEvent, timeline []analyzer.TimelineEvent, ticker []analyzer.TickerItem, format string, session string) (string, error) {
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
		sectorData[i] = SectorData{Name: s.Name, Net: s.Net, Color: s.Color}
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

	logger.Info("remotion 渲染开始",
		zap.Int("sectors", len(sectors)),
		zap.String("output", outputPath),
		zap.String("format", format))

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
