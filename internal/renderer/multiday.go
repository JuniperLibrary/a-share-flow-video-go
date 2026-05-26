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
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

// MultiDayRenderProps 传递给 Remotion 的多日 Bar Chart Race 属性。
type MultiDayRenderProps struct {
	Dates       []string                   `json:"dates"`
	FullDates   []string                   `json:"fullDates"`
	Snapshots   []BarSnapshot              `json:"snapshots"`
	Analysis    analyzer.MultiDayAnalysis  `json:"analysis"`
	Format      string                     `json:"format"`
	Width       int                        `json:"width"`
	Height      int                        `json:"height"`
	TotalFrames int                        `json:"totalFrames"`
}

// BarSnapshot 渲染用快照（与 fetcher.BarSnapshot 一致但用 SectorData）。
type BarSnapshot struct {
	Date string        `json:"date"`
	Time string        `json:"time"` // "09:30" — tick 时间点
	Bars []SectorData  `json:"bars"`
}

// RenderMultiDayVideo 渲染多日 Bar Chart Race 视频（逐 tick 数据）。
func RenderMultiDayVideo(
	dayTicks map[string][]storage.TimeSnapshot,
	dates []string,
	outputPath string,
	analysis analyzer.MultiDayAnalysis,
	format string,
) (string, error) {

	if _, err := time.Parse("2006-01-02", dates[0]); err != nil {
		return "", fmt.Errorf("parse date: %w", err)
	}

	compID := "BloombergVideo3Day"
	w, h := config.MobileWidth, config.MobileHeight
	if format == "tv" {
		compID = "BloombergVideo3DayTV"
		w, h = config.TVWidth, config.TVHeight
	}

	// 构建逐 tick 快照
	snapshots := fetcher.BuildBarSnapshotsFromTicks(dayTicks, dates)
	logger.Info("快照数据准备",
		zap.Int("snapshots", len(snapshots)),
		zap.Int("dates", len(dates)))

	renderSnapshots := make([]BarSnapshot, len(snapshots))
	for i, snap := range snapshots {
		bars := make([]SectorData, len(snap.Bars))
		for j, b := range snap.Bars {
			bars[j] = SectorData{Name: b.Name, Net: b.Net, Rate: b.Rate, Color: b.Color}
		}
		renderSnapshots[i] = BarSnapshot{
			Date: snap.Date,
			Time: snap.Time,
			Bars: bars,
		}
	}

	displayDates := make([]string, len(dates))
	for i, d := range dates {
		t, _ := time.Parse("2006-01-02", d)
		displayDates[i] = t.Format("01-02")
	}

	props := MultiDayRenderProps{
		Dates:       displayDates,
		FullDates:   dates,
		Snapshots:   renderSnapshots,
		Analysis:    analysis,
		Format:      format,
		Width:       w,
		Height:      h,
		TotalFrames: config.TotalFrames,
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", fmt.Errorf("marshal props: %w", err)
	}

	logger.Info("Remotion 渲染参数",
		zap.Int("propsSize", len(propsJSON)),
		zap.Int("snapshots", len(renderSnapshots)),
		zap.Int("totalFrames", config.TotalFrames),
		zap.Int("fps", config.FPS),
		zap.Int("width", w),
		zap.Int("height", h),
		zap.String("compID", compID),
		zap.String("output", outputPath))

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
		"--fps", fmt.Sprintf("%d", config.FPS),
		"--frames", fmt.Sprintf("0-%d", config.TotalFrames-1),
		"--bitrate", "8M",
	}

	logger.Info("remotion 多日渲染开始",
		zap.Int("days", len(dates)),
		zap.String("output", outputPath),
		zap.String("format", format))

	cmd := exec.Command("npx", args...)
	cmd.Dir = rendererDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Remotion render failed: %w", err)
	}

	var fileInfo string
	if fi, err := os.Stat(outputPath); err == nil {
		fileInfo = fmt.Sprintf("%.1fMB", float64(fi.Size())/1024/1024)
	}

	logger.Info("remotion 多日渲染完成",
		zap.String("output", outputPath),
		zap.String("fileSize", fileInfo))
	return outputPath, nil
}
