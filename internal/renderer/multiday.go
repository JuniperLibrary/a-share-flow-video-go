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
	Bars []SectorData  `json:"bars"`
}

// RenderMultiDayVideo 渲染多日 Bar Chart Race 视频。
func RenderMultiDayVideo(dayData map[string][]fetcher.Sector, dates []string,
	outputPath string, analysis analyzer.MultiDayAnalysis, format string) (string, error) {

	if _, err := time.Parse("2006-01-02", dates[0]); err != nil {
		return "", fmt.Errorf("parse date: %w", err)
	}

	compID := "BloombergVideo3Day"
	w, h := config.MobileWidth, config.MobileHeight
	if format == "tv" {
		compID = "BloombergVideo3DayTV"
		w, h = config.TVWidth, config.TVHeight
	}

	// 构建快照
	snapshots := fetcher.BuildBarSnapshots(dayData, dates)
	renderSnapshots := make([]BarSnapshot, len(snapshots))
	for i, snap := range snapshots {
		bars := make([]SectorData, len(snap.Bars))
		for j, b := range snap.Bars {
			bars[j] = SectorData{Name: b.Name, Net: b.Net, Color: b.Color}
		}
		renderSnapshots[i] = BarSnapshot{
			Date: snap.Date,
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
		"--browser-executable", "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
	}

	fmt.Printf("[remotion] rendering %d-day bar chart race → %s (%s)\n", len(dates), outputPath, format)
	fmt.Printf("[remotion] %d frames @ %dfps = %.1fs\n", config.TotalFrames, config.FPS, float64(config.TotalFrames)/float64(config.FPS))

	cmd := exec.Command("npx", args...)
	cmd.Dir = rendererDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Remotion render failed: %w", err)
	}

	fmt.Printf("[remotion] done → %s\n", outputPath)
	return outputPath, nil
}
