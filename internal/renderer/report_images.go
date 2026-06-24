package renderer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/report"
	"go.uber.org/zap"
)

// ReportImageProps 传递给 Playwright 截图脚本的 props。
type ReportImageProps struct {
	Report        *report.DailyReport  `json:"report"`
	ThematicCards []report.ThematicCard `json:"thematicCards"`
	Date          string               `json:"date"`
}

// screenshotScriptPath 返回 Playwright 截图脚本的绝对路径。
func screenshotScriptPath() string {
	return filepath.Join(config.GetRendererDir(), "scripts", "report-screenshot.mjs")
}

// RenderReportImages 渲染日报图片。
// 对每个 ThematicCard 生成 1 张 1080×1920 竖屏 PNG：
//   - 封面（sceneIndex=-1）— 第 1 张
//   - 专题 0（sceneIndex=0）— 第 2 张
//   - 专题 1（sceneIndex=1）— 第 3 张（如果有）
//
// 保存到 output/<date>/report_<index>.png。
func RenderReportImages(r *report.DailyReport, thematicCards []report.ThematicCard, dateStr string) ([]string, error) {
	outputDir := filepath.Join(config.GetOutputDir(), dateStr)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	script := screenshotScriptPath()
	if _, err := os.Stat(script); os.IsNotExist(err) {
		return nil, fmt.Errorf("screenshot script not found: %s", script)
	}

	// 写 Props 到临时文件
	props := ReportImageProps{
		Report:        r,
		ThematicCards: thematicCards,
		Date:          dateStr,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("marshal props: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "report-props-*.json")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(propsJSON); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// 场景列表：先渲染封面，再渲染每张专题卡片
	var sceneIndices []int
	sceneIndices = append(sceneIndices, -1) // 封面
	for i := 0; i < len(thematicCards); i++ {
		sceneIndices = append(sceneIndices, i)
	}

	var paths []string
	for idx, sceneIdx := range sceneIndices {
		outputPath := filepath.Join(outputDir, fmt.Sprintf("report_%d.png", idx+1))

		cmd := exec.Command("node", script, tmpPath, outputPath, fmt.Sprintf("%d", sceneIdx))
		cmd.Dir = config.GetRendererDir()

		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.Warn("日报图片渲染失败",
				zap.Int("scene", sceneIdx),
				zap.Error(err),
				zap.String("output", truncateOutput(string(output))),
			)
			continue
		}

		paths = append(paths, outputPath)
		logger.Info("日报图片已生成",
			zap.String("path", outputPath),
			zap.Int("scene", sceneIdx),
		)
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("no report images generated")
	}
	return paths, nil
}

// RenderReportImageSingle 渲染单张日报图片（指定场景索引）。
func RenderReportImageSingle(r *report.DailyReport, thematicCards []report.ThematicCard, sceneIndex int, dateStr string) (string, error) {
	script := screenshotScriptPath()
	if _, err := os.Stat(script); os.IsNotExist(err) {
		return "", fmt.Errorf("screenshot script not found: %s", script)
	}

	props := ReportImageProps{
		Report:        r,
		ThematicCards: thematicCards,
		Date:          dateStr,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", fmt.Errorf("marshal props: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "report-props-*.json")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(propsJSON); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpPath)

	outputDir := filepath.Join(config.GetOutputDir(), dateStr)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	outputPath := filepath.Join(outputDir, fmt.Sprintf("report_scene_%d.png", sceneIndex))

	cmd := exec.Command("node", script, tmpPath, outputPath, fmt.Sprintf("%d", sceneIndex))
	cmd.Dir = config.GetRendererDir()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("render scene %d: %w, output: %s", sceneIndex, err, truncateOutput(string(output)))
	}

	return outputPath, nil
}

func truncateOutput(s string) string {
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}
