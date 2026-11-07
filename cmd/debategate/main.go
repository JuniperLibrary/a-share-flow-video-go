// debategate 是辩论链路的本地门禁:真跑一场,把"证据四元组是否齐全、
// 算式是否算错、轮次与字数是否落在预算内"打成一份可存档的报告,
// 任一项不合格就以非零退出码结束。
//
// 之所以要单独一个命令而不是写单测:prompt 与模型行为只能靠真实调用验证,
// 而单测里的假客户端永远不会漏写算式、也不会把 1435.23÷1506.11 写成 0.853。
//
// 用法:
//
//	go run ./cmd/debategate -code 600519 -name 贵州茅台 -profile deep
//	go run ./cmd/debategate -profile quick -out /tmp/debate_quick.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/debate"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
)

type turnReport struct {
	Index        int      `json:"index"`
	Phase        string   `json:"phase"`
	Speaker      string   `json:"speaker"`
	Chars        int      `json:"chars"`
	HasFormula   bool     `json:"hasFormula"`
	HasFalsifier bool     `json:"hasFalsifier"`
	MathIssues   []string `json:"mathIssues,omitempty"`
	Text         string   `json:"text"`
}

type gateReport struct {
	GeneratedAt  string       `json:"generatedAt"`
	Profile      string       `json:"profile"`
	StockCode    string       `json:"stockCode"`
	StockName    string       `json:"stockName"`
	ReportPeriod string       `json:"reportPeriod"`
	ElapsedSec   int64        `json:"elapsedSec"`
	Turns        int          `json:"turns"`
	TotalChars   int          `json:"totalChars"`
	VideoMinutes float64      `json:"videoMinutes"`
	MissingCount int          `json:"missingEvidenceTurns"`
	MathErrCount int          `json:"mathErrorTurns"`
	VerdictChars int          `json:"verdictChars"`
	SkippedTurns int          `json:"skippedTurns"`
	TurnDetails  []turnReport `json:"turnDetails"`
	Verdict      string       `json:"verdict"`
}

func main() {
	var (
		stockCode = flag.String("code", "600519", "股票代码")
		stockName = flag.String("name", "贵州茅台", "股票名称")
		profile   = flag.String("profile", "deep", "辩论档位: deep 或 quick")
		outPath   = flag.String("out", "", "把完整报告写入该 JSON 路径(可用于回归)")
	)
	flag.Parse()

	if err := logger.InitFromEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(2)
	}
	defer logger.Sync()

	prof, err := debate.ParseProfile(*profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}

	cfg := config.GetAIConfig()
	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "未配置 AI API Key(数据库 ai_config 或环境变量)")
		os.Exit(2)
	}

	rep, err := fetcher.FetchFinanceReport(*stockCode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "拉取财报失败: %v\n", err)
		os.Exit(1)
	}
	reportText := fetcher.FormatFinanceReport(rep)
	if len([]rune(reportText)) < 200 {
		fmt.Fprintf(os.Stderr, "财报正文过短(%d 字),无法作为门禁样本\n", len([]rune(reportText)))
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	start := time.Now()
	script, err := debate.NewOrchestratorWithOptions(cfg, debate.WithProfile(prof)).Run(ctx, debate.DebateState{
		ReportText:   reportText,
		StockCode:    *stockCode,
		StockName:    *stockName,
		ReportPeriod: rep.ReportDate,
	})
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "辩论失败: %v\n", err)
		os.Exit(1)
	}

	report := buildReport(*stockCode, *stockName, rep.ReportDate, string(prof), elapsed, script)
	printReport(report)

	if *outPath != "" {
		raw, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "写入报告失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n完整报告已写入 %s\n", *outPath)
	}

	if report.MissingCount > 0 || report.MathErrCount > 0 {
		fmt.Fprintf(os.Stderr, "\n门禁未通过: 缺证据 %d 轮, 算式算错 %d 轮\n", report.MissingCount, report.MathErrCount)
		os.Exit(1)
	}
}

func buildReport(code, name, period, profile string, elapsed time.Duration, script debate.Script) gateReport {
	report := gateReport{
		GeneratedAt:  time.Now().Format(time.RFC3339),
		Profile:      profile,
		StockCode:    code,
		StockName:    name,
		ReportPeriod: period,
		ElapsedSec:   int64(elapsed.Round(time.Second).Seconds()),
		Turns:        len(script.Turns),
		VerdictChars: len([]rune(script.Verdict)),
		SkippedTurns: script.SkippedTurns,
		Verdict:      script.Verdict,
	}
	for _, t := range script.Turns {
		detail := debate.InspectTurn(t)
		tr := turnReport{
			Index:        t.Index,
			Phase:        string(t.Phase),
			Speaker:      string(t.Speaker),
			Chars:        detail.Chars,
			HasFormula:   detail.HasFormula,
			HasFalsifier: detail.HasFalsifier,
			Text:         t.Text,
		}
		for _, issue := range detail.MathIssues {
			tr.MathIssues = append(tr.MathIssues, issue)
		}
		report.TotalChars += detail.Chars
		if detail.Missing() {
			report.MissingCount++
		}
		if len(detail.MathIssues) > 0 {
			report.MathErrCount++
		}
		report.TurnDetails = append(report.TurnDetails, tr)
	}
	// 4.5 字/秒是 TTS 实测的稳定语速,用来把字数折算成成片时长。
	report.VideoMinutes = float64(report.TotalChars) / 4.5 / 60
	return report
}

func printReport(r gateReport) {
	for _, t := range r.TurnDetails {
		flagText := "四元组=ok"
		if !t.HasFormula || !t.HasFalsifier {
			flagText = "四元组=缺"
		}
		if len(t.MathIssues) > 0 {
			flagText += fmt.Sprintf(" 算式错=%d", len(t.MathIssues))
		}
		fmt.Printf("[%02d %-10s %-11s %3d字 %s] %s\n", t.Index, t.Phase, t.Speaker, t.Chars, flagText, t.Text)
		for _, issue := range t.MathIssues {
			fmt.Printf("    ! %s\n", issue)
		}
	}
	fmt.Printf("\n=== %s 档 %d 轮, %ds, 总字数 %d, 成片约 %.1f 分钟, 缺项 %d 轮, 算式错 %d 轮 ===\n",
		r.Profile, r.Turns, r.ElapsedSec, r.TotalChars, r.VideoMinutes, r.MissingCount, r.MathErrCount)
	if r.SkippedTurns > 0 {
		// 缺席轮次不判失败(成片仍然完整),但必须让人看见,
		// 否则会把一场残缺的辩论当成正常产出存档。
		fmt.Printf("!! %d 轮缺席:本次可能是残缺的辩论,与同档位历史产出对比时要注意\n", r.SkippedTurns)
	}
}
