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

type abResult struct {
	Label       string         `json:"label"`
	TurnsCount  int            `json:"turnsCount"`
	Speakers    map[string]int `json:"speakers"`
	HasCitations bool          `json:"hasCitations"`
	HasPhase    bool           `json:"hasPhase"`
	TotalChars  int            `json:"totalChars"`
	LatencyMS   int64          `json:"latencyMs"`
	Error       string         `json:"error,omitempty"`
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	legacy, err := runLegacy(ctx, cfg, reportText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "legacy 失败: %v\n", err)
	}
	council, err := runCouncil(ctx, cfg, reportText, stockCode, stockName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "council 失败: %v\n", err)
	}

	out := struct {
		GeneratedAt string    `json:"generatedAt"`
		ReportChars int       `json:"reportChars"`
		Legacy      abResult  `json:"legacy"`
		Council     abResult  `json:"council"`
	}{
		GeneratedAt: time.Now().Format(time.RFC3339),
		ReportChars: len(reportText),
		Legacy:      legacy,
		Council:     council,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 输出失败: %v\n", err)
		os.Exit(1)
	}
}

func runLegacy(ctx context.Context, cfg config.AIConfig, reportText string) (abResult, error) {
	start := time.Now()
	script, err := debate.GenerateScript(reportText, cfg)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return abResult{Label: "legacy-2agent", LatencyMS: latency, Error: err.Error()}, err
	}
	return summarize("legacy-2agent", script, latency), nil
}

func runCouncil(ctx context.Context, cfg config.AIConfig, reportText, code, name string) (abResult, error) {
	start := time.Now()
	orch := debate.NewOrchestrator(cfg)
	state := debate.DebateState{
		ReportText: reportText,
		StockCode:  code,
		StockName:  name,
	}
	script, err := orch.Run(ctx, state)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return abResult{Label: "council-6agent-7phase", LatencyMS: latency, Error: err.Error()}, err
	}
	return summarize("council-6agent-7phase", script, latency), nil
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
		totalChars += len(t.Text)
	}
	return abResult{
		Label:        label,
		TurnsCount:   len(script.Turns),
		Speakers:     speakers,
		HasCitations: hasCitations,
		HasPhase:     hasPhase,
		TotalChars:   totalChars,
		LatencyMS:    latencyMS,
	}
}
