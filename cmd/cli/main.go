package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/copy"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/renderer"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tickfetcher"
	"github.com/a-share-flow-video-go/internal/tickrenderer"
	"go.uber.org/zap"
)

var sep70 = strings.Repeat("=", 70)
var sep65 = strings.Repeat("-", 65)

// CLI 入口：命令行视频生成器。
// 支持参数：--ai(AI文案模式) --session=morning/full YYYY-MM-DD(指定日期)
func main() {
	if err := logger.Init("info", "console", "stdout"); err != nil {
		panic(err)
	}
	defer logger.Sync()

	if _, err := storage.Get(); err != nil {
		logger.Fatal("SQLite 初始化失败", zap.Error(err))
	}

	useAI := false
	session := ""
	days := 0
	useTick := false
	collectOnly := false
	var dates []string

	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--ai":
			useAI = true
		case strings.HasPrefix(arg, "--session="):
			session = strings.TrimPrefix(arg, "--session=")
		case strings.HasPrefix(arg, "--days="):
			days, _ = strconv.Atoi(strings.TrimPrefix(arg, "--days="))
		case arg == "--tick":
			useTick = true
		case arg == "--collect-only":
			collectOnly = true
		default:
			dates = append(dates, arg)
		}
	}

	if days > 1 {
		if len(dates) == 0 {
			dates = []string{time.Now().Format("2006-01-02")}
		}
		generateMultiDayVideo(dates[0], days, useAI)
		return
	}

	if useTick {
		if len(dates) == 0 {
			dates = []string{time.Now().Format("2006-01-02")}
		}
		logger.Info("=== Tick 资金流动视频生成 ===")
		logger.Info("将处理日期", zap.Int("count", len(dates)), zap.String("dates", strings.Join(dates, ", ")))
		successCount := 0
		for _, dateStr := range dates {
			if processTickDate(dateStr, session, useAI) {
				successCount++
			}
		}
		logger.Info(sep70)
		logger.Info("Tick 视频生成完成", zap.Int("success", successCount), zap.Int("total", len(dates)))
		logger.Info(sep70)
		return
	}

	if len(dates) == 0 {
		dates = []string{time.Now().Format("2006-01-02")}
	}

	mode := "模板"
	if useAI {
		mode = "AI"
	}
	sessLabel := "自动检测"
	if session != "" {
		sessLabel = config.SessionConfigs[session].TitleSuffix
	}
	logger.Info("=== 板块资金流向视频生成 ===", zap.String("session", mode+"模式, "+sessLabel))
	logger.Info("将处理日期", zap.Int("count", len(dates)), zap.String("dates", strings.Join(dates, ", ")))

	successCount := 0
	for _, dateStr := range dates {
		if processDate(dateStr, useAI, session, collectOnly) {
			successCount++
		}
	}

	logger.Info(sep70)
	logger.Info("完成", zap.Int("success", successCount), zap.Int("total", len(dates)))
	logger.Info("输出目录: output/YYYY-MM-DD/")
	logger.Info("数据目录: data/YYYY-MM-DD/")
	logger.Info(sep70)
}

func processDate(dateStr string, useAI bool, sessionOverride string, collectOnly bool) bool {
	l := logger.With(zap.String("date", dateStr))
	l.Info(sep70)
	l.Info("处理日期: " + dateStr)
	l.Info(sep70)

	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		l.Error("无效日期格式，请使用 YYYY-MM-DD 格式", zap.String("date", dateStr))
		return false
	}

	dateDir := filepath.Join(config.GetDataDir(), dateStr)
	today := time.Now().Format("2006-01-02")

	var sessions []string
	if sessionOverride != "" {
		sessions = []string{sessionOverride}
	} else if dateStr == today {
		now := time.Now()
		if now.Hour() < 13 {
			sessions = []string{"morning"}
		} else {
			sessions = []string{"full"}
		}
	} else {
		if db, err := storage.Get(); err == nil {
			if pts, _ := db.LoadTickSectors(dateStr); len(pts) > 0 {
				sessions = append(sessions, "morning")
			}
		}
		if _, err := os.Stat(filepath.Join(dateDir, "sectors.csv")); err == nil {
			sessions = append(sessions, "full")
		}
		if len(sessions) == 0 {
			sessions = []string{"full"}
		}
	}

	for _, session := range sessions {
		sessCfg := config.SessionConfigs[session]
		sl := l.With(zap.String("session", sessCfg.TitleSuffix))
		sl.Info("--- 生成 " + sessCfg.TitleSuffix + " 视频 ---")

		var sectors []fetcher.Sector
		var err error

		sectors, err = fetcher.LoadSessionData(dateStr, session)
		if err == nil && len(sectors) > 0 {
			sl.Info("从本地加载数据", zap.Int("sectors", len(sectors)))
			printSectorsTable(sectors)
		} else if dateStr == today {
			sl.Info("获取实时热门板块数据")
			sectors, err = fetcher.FetchTop21HotSectors()
			if err != nil || len(sectors) == 0 {
				sl.Error("无法获取热门板块数据", zap.Error(err))
				continue
			}
			fetcher.SaveDailyData(sectors, dateStr)
			sl.Info("热门板块匹配", zap.Int("count", len(sectors)))
			printSectorsTable(sectors)
		} else {
			sl.Info("获取历史热门数据")
			sectors, err = fetcher.FetchHistoricalSectors(dateStr)
			if err != nil || len(sectors) == 0 {
				sl.Error("历史数据获取失败", zap.String("date", dateStr), zap.Error(err))
				sl.Warn("请检查网络连接，或手动保存数据到 data/" + dateStr + "/sectors.csv")
				continue
			}
			fetcher.SaveDailyData(sectors, dateStr)
			sl.Info("历史热门匹配", zap.Int("count", len(sectors)))
			printSectorsTable(sectors)
		}

		if !collectOnly {
			generateSession(sectors, dateStr, dateDir, useAI, session)
		}
	}

	if config.DataMode() == "json" {
		if db, err := storage.Get(); err == nil {
			db.ExportJSON()
		}
	}
	return true
}

func generateSession(sectors []fetcher.Sector, dateStr, dateDir string, useAI bool, session string) {
	os.MkdirAll(dateDir, 0755)

	sessCfg := config.SessionConfigs[session]
	allSectors, _ := fetcher.FetchAllRaw()
	events, timeline, ticker := analyzer.AnalyzeAllContent(allSectors, dateStr, session)
	if len(events) == 0 {
		events = analyzer.GetFallbackEvents(config.TotalFrames)
	}

	outputDir := config.GetOutputDir()
	os.MkdirAll(filepath.Join(outputDir, dateStr), 0755)
	l := logger.With(zap.String("date", dateStr), zap.String("session", sessCfg.TitleSuffix))

	outPath := filepath.Join(outputDir, dateStr, fmt.Sprintf("%s.mp4", sessCfg.FilenameSuffix))
	if _, err := renderer.RenderVideo(sectors, dateStr, outPath, events, timeline, ticker, "tv", session); err != nil {
		l.Error("Remotion 渲染失败", zap.String("session", sessCfg.TitleSuffix), zap.Error(err))
	} else {
		l.Info("视频已保存", zap.String("output", outPath))
	}

	var text string
	if useAI {
		var err error
		text, err = copy.GenerateCopywritingAI(sectors, dateStr, session)
		if err != nil {
			l.Warn("AI文案生成失败，降级模板模式", zap.String("session", sessCfg.TitleSuffix), zap.Error(err))
			text = copy.GenerateCopywriting(sectors, dateStr, session)
		}
	} else {
		text = copy.GenerateCopywriting(sectors, dateStr, session)
	}
	cwType := "template"
	if useAI {
		cwType = "ai"
	}

	if db, err := storage.Get(); err == nil {
		_ = db.SaveCopywriting(storage.Copywriting{
			Date:    dateStr,
			Session: session,
			Type:    cwType,
			Content: text,
		})
	}
	l.Info("文案已保存", zap.String("session", sessCfg.TitleSuffix))
}

func processTickDate(dateStr string, sessionOverride string, useAI bool) bool {
	l := logger.With(zap.String("date", dateStr))
	l.Info(sep70)
	l.Info("处理 Tick 视频: " + dateStr)
	l.Info(sep70)

	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		l.Error("无效日期格式，请使用 YYYY-MM-DD 格式", zap.String("date", dateStr))
		return false
	}

	today := time.Now().Format("2006-01-02")
	var sessions []string
	if sessionOverride != "" {
		sessions = []string{sessionOverride}
	} else if dateStr == today {
		now := time.Now()
		if now.Hour() < 13 {
			sessions = []string{"morning"}
		} else {
			sessions = []string{"full"}
		}
	} else {
		sessions = []string{"full", "morning"}
	}

	outputDir := config.GetOutputDir()
	success := false
	for _, sess := range sessions {
		sessCfg := config.SessionConfigs[sess]
		sl := l.With(zap.String("session", sessCfg.TitleSuffix))
		sl.Info("--- 生成 Tick " + sessCfg.TitleSuffix + " 视频 ---")

		outPath := filepath.Join(outputDir, dateStr, fmt.Sprintf("%s_tick.mp4", sessCfg.FilenameSuffix))
		out, err := tickrenderer.RenderTickVideo(dateStr, outPath, "tv", sess, nil, nil, nil)
		if err != nil {
			sl.Error("Tick Remotion 渲染失败", zap.String("session", sessCfg.TitleSuffix), zap.Error(err))
		} else {
			sl.Info("Tick 视频已保存", zap.String("output", out))
			success = true
		}

		points, err := tickfetcher.LoadTickCSV(dateStr, sess)
		if err != nil || len(points) == 0 {
			sl.Warn("无 tick 数据，跳过文案生成")
			continue
		}
		sectors := snapshotToSectors(points)

		var text string
		if useAI {
			text, err = copy.GenerateCopywritingAI(sectors, dateStr, sess)
			if err != nil {
				sl.Warn("AI文案生成失败，降级模板模式", zap.String("session", sessCfg.TitleSuffix), zap.Error(err))
				text = copy.GenerateCopywriting(sectors, dateStr, sess)
			}
		} else {
			text = copy.GenerateCopywriting(sectors, dateStr, sess)
		}
		cwType := "template"
		if useAI {
			cwType = "ai"
		}

		if db, err := storage.Get(); err == nil {
			_ = db.SaveCopywriting(storage.Copywriting{
				Date:    dateStr,
				Session: sess,
				Type:    cwType + "_tick",
				Content: text,
			})
		}
		sl.Info("文案已保存", zap.String("session", sessCfg.TitleSuffix+" (tick)"))
	}

	if config.DataMode() == "json" {
		if db, err := storage.Get(); err == nil {
			db.ExportJSON()
		}
	}
	return success
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

func printSectorsTable(sectors []fetcher.Sector) {
	fmt.Println()
	fmt.Printf("%-12s | %14s | %s\n", "板块", "主力资金净流入(亿)", "趋势")
	fmt.Println(sep65)
	for _, info := range sectors {
		trend := "↓ 净流出"
		if info.Net > 0 {
			trend = "↑ 净流入"
		}
		fmt.Printf("%-12s | %14.2f | %s\n", info.Name, info.Net, trend)
	}
	fmt.Println(sep70)
	fmt.Println()
}

func generateMultiDayVideo(endDate string, days int, useAI bool) {
	l := logger.Get()
	l.Info(sep70)
	l.Info("=== 近N日板块资金流向 Bar Chart Race 视频生成 ===", zap.Int("days", days), zap.String("date", endDate))
	l.Info(sep70)

	tradingDays, err := fetcher.GetTradingDays(endDate, days)
	if err != nil {
		l.Error("无法获取交易日", zap.Error(err))
		return
	}
	l.Info("交易日", zap.String("days", strings.Join(tradingDays, ", ")))

	dayData, err := fetcher.LoadMultiDaySectors(tradingDays)
	if err != nil {
		l.Error("加载多日数据失败", zap.Error(err))
		return
	}

	l.Info("成功加载数据", zap.Int("days", len(dayData)))
	for _, d := range tradingDays {
		if sectors, ok := dayData[d]; ok {
			l.Info("  "+d, zap.Int("sectors", len(sectors)))
		}
	}

	analysis := analyzer.MultiDayAnalyze(dayData, tradingDays, "ai")

	// 加载逐 tick 数据用于渲染（含时间轴）
	tickData, err := fetcher.LoadMultiDayTicks(tradingDays)
	if err != nil {
		l.Error("加载 tick 数据失败", zap.Error(err))
		return
	}
	totalTicks := 0
	for _, snaps := range tickData {
		totalTicks += len(snaps)
	}
	l.Info("逐 tick 数据", zap.Int("dates", len(tickData)), zap.Int("totalSnapshots", totalTicks))

	outputDir := config.GetOutputDir()
	dateLabel := tradingDays[0]
	if len(tradingDays) > 1 {
		dateLabel = tradingDays[0] + "_to_" + tradingDays[len(tradingDays)-1]
	}

	os.MkdirAll(filepath.Join(outputDir, dateLabel), 0755)

	outPath := filepath.Join(outputDir, dateLabel, "三日资金流向.mp4")
	if _, err := renderer.RenderMultiDayVideo(tickData, tradingDays, outPath, analysis, "tv"); err != nil {
		l.Error("Remotion 渲染失败", zap.Error(err))
	} else {
		l.Info("视频已保存", zap.String("output", outPath))
	}

	l.Info(sep70)
	l.Info("完成！Bar Chart Race 视频已生成")
	l.Info("输出目录: output/" + dateLabel + "/")
	l.Info(sep70)
}
