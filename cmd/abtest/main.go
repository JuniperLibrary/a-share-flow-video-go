package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/debate"
)

// abtest 对比两档辩论(quick / deep)的产出口径。
//
// 合并"经典 vs 多智能体"双轨之后,这里保留为同一套链路的档位对照:
// 轮次、角色覆盖、字数、折算成片时长、跑批耗时是否都落在预期区间。
type abResult struct {
	Label        string         `json:"label"`
	TurnsCount   int            `json:"turnsCount"`
	Speakers     map[string]int `json:"speakers"`
	HasCitations bool           `json:"hasCitations"`
	HasPhase     bool           `json:"hasPhase"`
	HasVerdict   bool           `json:"hasVerdict"`
	TotalChars   int            `json:"totalChars"`
	// 按 4.5 字/秒估算的成片时长(秒),用于对照视频长度预期。
	EstimatedVideoSec int    `json:"estimatedVideoSec"`
	LatencyMS         int64  `json:"latencyMs"`
	Error             string `json:"error,omitempty"`
}

func main() {
	var reportPath string
	flag.StringVar(&reportPath, "report", "", "财报文本文件路径(必需)")
	var stockCode string
	flag.StringVar(&stockCode, "code", "600519", "股票代码")
	var stockName string
	flag.StringVar(&stockName, "name", "贵州茅台", "股票名称")
	flag.Parse()

	if reportPath == "" {
		fmt.Fprintln(os.Stderr, "用法: abtest -report <path> [-code 600519] [-name 贵州茅台]")
		os.Exit(2)
	}

	raw, err := os.ReadFile(reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取报告失败: %v\n", err)
		os.Exit(1)
	}
	reportText := strings.TrimSpace(string(raw))
	if reportText == "" {
		fmt.Fprintln(os.Stderr, "报告文件为空")
		os.Exit(1)
	}

	if err := config.LoadEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "加载 .env 失败: %v\n", err)
		os.Exit(1)
	}
	cfg := config.GetAIConfig()

	// deep 档实测 8-9 分钟,留足余量;quick 档约 2 分钟。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	quick, err := run(ctx, cfg, debate.ProfileQuick, reportText, stockCode, stockName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "quick 失败: %v\n", err)
	}
	deep, err := run(ctx, cfg, debate.ProfileDeep, reportText, stockCode, stockName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "deep 失败: %v\n", err)
	}

	out := struct {
		GeneratedAt string   `json:"generatedAt"`
		ReportChars int      `json:"reportChars"`
		Quick       abResult `json:"quick"`
		Deep        abResult `json:"deep"`
	}{
		GeneratedAt: time.Now().Format(time.RFC3339),
		ReportChars: len(reportText),
		Quick:       quick,
		Deep:        deep,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 输出失败: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.AIConfig, profile debate.Profile, reportText, code, name string) (abResult, error) {
	label := string(profile)
	start := time.Now()
	orch := debate.NewOrchestratorWithOptions(cfg, debate.WithProfile(profile))
	state := debate.DebateState{
		ReportText: reportText,
		StockCode:  code,
		StockName:  name,
	}
	script, err := orch.Run(ctx, state)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return abResult{Label: label, LatencyMS: latency, Error: err.Error()}, err
	}
	return summarize(label, script, latency), nil
}

func summarize(label string, script debate.Script, latencyMS int64) abResult {
	speakers := make(map[string]int)
	hasCitations := false
	hasPhase := false
	totalChars := 0
	for _, t := range script.Turns {
		speakers[string(t.Speaker)]++
		if len(t.Citations) > 0 {
			hasCitations = true
		}
		if t.Phase != "" {
			hasPhase = true
		}
		totalChars += len([]rune(t.Text))
	}
	return abResult{
		Label:             label,
		TurnsCount:        len(script.Turns),
		Speakers:          speakers,
		HasCitations:      hasCitations,
		HasPhase:          hasPhase,
		HasVerdict:        strings.TrimSpace(script.Verdict) != "",
		TotalChars:        totalChars,
		EstimatedVideoSec: int(float64(totalChars) / 4.5),
		LatencyMS:         latencyMS,
	}
}
