package debate

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// Render 调 Remotion 渲染 DebateVideo composition。
// 总帧数 = sum(audioTurns[].Frames) + 60 帧开场/收尾动画。
func Render(taskID string, script Script, audioTurns []AudioTurn, outputPath string, format string) (string, *ProbeResult, error) {
	if taskID == "" {
		return "", nil, fmt.Errorf("taskID 不能为空")
	}
	if len(audioTurns) == 0 {
		return "", nil, fmt.Errorf("audioTurns 为空，请先调用 GenerateAudio")
	}

	totalFrames := 0
	for _, at := range audioTurns {
		totalFrames += at.Frames
	}
	const introOutroFrames = 60
	totalFrames += introOutroFrames

	w, h := config.MobileWidth, config.MobileHeight
	if format == "tv" {
		w, h = config.TVWidth, config.TVHeight
	}

	reportTitle := "财报辩论"
	if script.StockName != "" {
		if script.ReportPeriod != "" {
			reportTitle = script.StockName + " · " + script.ReportPeriod + " · 多空辩论"
		} else {
			reportTitle = script.StockName + " · 多空辩论"
		}
	}

	props := RenderProps{
		TaskID:      taskID,
		BullName:    "乐观派",
		BearName:    "谨慎派",
		StockName:   script.StockName,
		ReportPeriod: script.ReportPeriod,
		ReportTitle: reportTitle,
		Turns:       script.Turns,
		AudioTurns:  audioTurns,
		TotalFrames: totalFrames,
		Width:       w,
		Height:      h,
		Format:      format,
	}

	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", nil, fmt.Errorf("marshal props: %w", err)
	}

	rendererDir := config.GetRendererDir()
	entry := filepath.Join(rendererDir, "src", "renderer", "index.ts")

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return "", nil, fmt.Errorf("create output dir: %w", err)
	}

	args := []string{
		"remotion", "render",
		entry,
		"DebateVideo",
		outputPath,
		"--props", string(propsJSON),
		"--overwrite",
		"--fps", fmt.Sprintf("%d", config.FPS),
		"--frames", fmt.Sprintf("0-%d", totalFrames-1),
		"--bitrate", "8M",
	}

	logger.Info("debate 渲染开始",
		zap.String("taskId", taskID),
		zap.Int("turns", len(audioTurns)),
		zap.Int("totalFrames", totalFrames),
		zap.String("output", outputPath))

	cmd := exec.Command("npx", args...)
	cmd.Dir = rendererDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", nil, fmt.Errorf("DebateVideo render failed: %w", err)
	}

	logger.Info("debate 渲染完成", zap.String("taskId", taskID), zap.String("output", outputPath))

	expectedSec := float64(totalFrames) / float64(config.FPS)
	probe, probeErr := ProbeVideo(outputPath, expectedSec)
	if probeErr != nil {
		logger.Warn("debate ffprobe 失败", zap.String("taskId", taskID), zap.Error(probeErr))
	} else if probe.OK {
		logger.Info("debate 校验通过",
			zap.String("taskId", taskID),
			zap.String("audioCodec", probe.AudioCodec),
			zap.Float64("audioSec", probe.AudioSec))
	} else {
		logger.Warn("debate 校验告警", zap.String("taskId", taskID), zap.Strings("warnings", probe.Warnings))
	}

	return outputPath, probe, nil
}
