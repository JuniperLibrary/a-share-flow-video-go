package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/copy"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/hotnews"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/renderer"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tick"
	"go.uber.org/zap"
)

// CLI 入口：命令行视频生成器。
// 支持参数：--ai(AI文案模式) --session=morning/full YYYY-MM-DD(指定日期)
// SIGINT / SIGTERM 会先取消正在跑的 Remotion 进程再退出。
func main() {
	if err := logger.InitFromEnv(); err != nil {
		panic(err)
	}
	defer logger.Sync()

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if _, err := storage.Get(); err != nil {
		logger.Fatal("SQLite 初始化失败", zap.Error(err))
	}

	db, err := storage.Get()
	if err != nil {
		logger.Fatal("获取数据库实例失败", zap.Error(err))
	}
	if err := db.MigrateAIConfigFromEnv(); err != nil {
		logger.Warn("AI 配置迁移失败", zap.Error(err))
	}

	useAI := false
	reportImage := false
	session := ""
	days := 0
	useTick := false
	collectOnly := false
	format := "mobile"
	var dates []string

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "--ai":
			useAI = true
		case arg == "--report-image":
			reportImage = true
		case strings.HasPrefix(arg, "--report-image="):
			dateStr := strings.TrimPrefix(arg, "--report-image=")
			if err := runReportImage(dateStr); err != nil {
				fmt.Fprintf(os.Stderr, "生成日报图片失败: %v\n", err)
				os.Exit(1)
			}
			return
		case strings.HasPrefix(arg, "--session="):
			session = strings.TrimPrefix(arg, "--session=")
		case strings.HasPrefix(arg, "--days="):
			days, _ = strconv.Atoi(strings.TrimPrefix(arg, "--days="))
		case arg == "--tick":
			useTick = true
		case arg == "--collect-only":
			collectOnly = true
		case strings.HasPrefix(arg, "--format="):
			format = strings.TrimPrefix(arg, "--format=")
		default:
			dates = append(dates, arg)
		}
	}

	// --report-image 处理（支持 --report-image 2026-06-15 和 --report-image=2026-06-15）
	if reportImage {
		dateStr := ""
		if len(dates) > 0 {
			dateStr = dates[0]
		} else {
			dateStr = time.Now().Format("2006-01-02")
		}
		if err := runReportImage(dateStr); err != nil {
			fmt.Fprintf(os.Stderr, "生成日报图片失败: %v\n", err)
			os.Exit(1)
		}
		return
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
		logger.Info("Tick 视频生成",
			zap.String("mode", "tick"),
			zap.Int("count", len(dates)),
			zap.String("dates", strings.Join(dates, ", ")),
		)
		successCount := 0
		for _, dateStr := range dates {
			if rootCtx.Err() != nil {
				logger.Warn("收到退出信号，停止处理后续日期")
				break
			}
			if processTickDate(rootCtx, dateStr, session, useAI, format) {
				successCount++
			}
		}
		logger.Info("Tick 视频生成完成", zap.Int("success", successCount), zap.Int("total", len(dates)))
		return
	}

	if len(dates) == 0 {
		dates = []string{time.Now().Format("2006-01-02")}
	}

	mode := "template"
	if useAI {
		mode = "ai"
	}
	sessLabel := "auto"
	if session != "" {
		sessLabel = config.SessionConfigs[session].TitleSuffix
	}
	logger.Info("板块资金流向视频生成",
		zap.String("mode", mode),
		zap.String("session", sessLabel),
		zap.Int("count", len(dates)),
		zap.String("dates", strings.Join(dates, ", ")),
	)

	successCount := 0
	for _, dateStr := range dates {
		if rootCtx.Err() != nil {
			logger.Warn("收到退出信号，停止处理后续日期")
			break
		}
		if processDate(rootCtx, dateStr, useAI, session, collectOnly) {
			successCount++
		}
	}

	logger.Info("处理完成",
		zap.Int("success", successCount),
		zap.Int("total", len(dates)),
		zap.String("output", "output/YYYY-MM-DD/"),
		zap.String("data", "data/YYYY-MM-DD/"),
	)
}

func processDate(ctx context.Context, dateStr string, useAI bool, sessionOverride string, collectOnly bool) bool {
	l := logger.With(zap.String("date", dateStr))
	l.Info("处理日期")

	if ctx != nil && ctx.Err() != nil {
		l.Info("已取消，跳过日期处理")
		return false
	}

	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		l.Error("无效日期格式", zap.String("date", dateStr), zap.String("expected", "YYYY-MM-DD"))
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
		if ctx != nil && ctx.Err() != nil {
			l.Info("已取消，跳过剩余 session")
			break
		}
		sessCfg := config.SessionConfigs[session]
		sl := l.With(zap.String("session", sessCfg.TitleSuffix))
		sl.Info("生成视频")

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
				sl.Warn("历史数据获取失败，请检查网络或手动保存数据",
					zap.String("hint", "data/"+dateStr+"/sectors.csv"),
				)
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

	// 在渲染前生成文案，用于 TTS 语音合成
	var copywriteText string
	if useAI {
		var err error
		prevPrediction := loadPrevPrediction(dateStr, session)
		copywriteText, err = copy.GenerateCopywritingAI(sectors, dateStr, session, prevPrediction)
		if err != nil {
			l.Warn("AI文案生成失败，降级模板模式", zap.Error(err))
			copywriteText = copy.GenerateCopywriting(sectors, dateStr, session)
		}
	} else {
		copywriteText = copy.GenerateCopywriting(sectors, dateStr, session)
	}

	if _, err := renderer.RenderVideo(sectors, dateStr, outPath, events, timeline, ticker, "tv", session, copywriteText); err != nil {
		l.Error("Remotion 渲染失败", zap.Error(err))
	} else {
		l.Info("视频已保存", zap.String("output", outPath))
	}

	var cwType string
	if useAI {
		cwType = "ai"
	} else {
		cwType = "template"
	}

	if db, err := storage.Get(); err == nil {
		_ = db.SaveCopywriting(storage.Copywriting{
			Date:    dateStr,
			Session: session,
			Type:    cwType,
			Content: copywriteText,
		})
	}
	l.Info("文案已保存")
}

func processTickDate(ctx context.Context, dateStr string, sessionOverride string, useAI bool, format string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	l := logger.With(zap.String("date", dateStr))
	l.Info("处理 Tick 视频")

	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		l.Error("无效日期格式", zap.String("expected", "YYYY-MM-DD"))
		return false
	}

	var sessions []string
	if sessionOverride != "" {
		sessions = []string{sessionOverride}
	} else {
		sessions = []string{"full"}
	}

	outputDir := config.GetOutputDir()
	success := false
	for _, sess := range sessions {
		if ctx.Err() != nil {
			l.Warn("收到取消信号，停止 Tick 渲染")
			return false
		}
		sessCfg := config.SessionConfigs[sess]
		sl := l.With(zap.String("session", sessCfg.TitleSuffix))
		sl.Info("生成 Tick 视频")

		// 加载 tick 数据用于文案生成
		points, err := tick.LoadTickCSV(dateStr, sess)
		if err != nil || len(points) == 0 {
			sl.Warn("无 tick 数据，跳过")
			continue
		}
		sectors := tick.PointsToSectors(points)

		if db, dbErr := storage.Get(); dbErr == nil {
			if daily, loadErr := db.LoadSectorsAll(dateStr); loadErr == nil && len(daily) > 0 {
				sectors = fetcher.MergeSectorsWithDaily(sectors, daily, 5)
				sl.Info("已合并全板块日线行情",
					zap.Int("tickSectors", len(tick.PointsToSectors(points))),
					zap.Int("mergedSectors", len(sectors)),
					zap.Int("dailyRows", len(daily)))
			}
		}
		// 在渲染前生成文案，用于 TTS 语音合成
		var copywriteText string
		if useAI {
			prevPrediction := loadPrevPrediction(dateStr, sess)
			text, aiErr := copy.GenerateCopywritingAI(sectors, dateStr, sess, prevPrediction)
			if aiErr != nil {
				sl.Warn("AI文案生成失败，降级模板模式", zap.Error(aiErr))
				copywriteText = copy.GenerateCopywriting(sectors, dateStr, sess)
			} else {
				copywriteText = text
			}
		} else {
			copywriteText = copy.GenerateCopywriting(sectors, dateStr, sess)
		}

		renderMobile := format == "all" || format == "mobile"
		renderTV := format == "all" || format == "tv"

		var newsPagesMobile, newsPagesTV []hotnews.NewsPage
		if renderMobile {
			pages, err := hotnews.LoadForVideo(dateStr, "mobile")
			if err != nil {
				sl.Warn("新闻加载失败 (mobile)", zap.Error(err))
			} else {
				newsPagesMobile = pages
			}
		}
		if renderTV {
			pages, err := hotnews.LoadForVideo(dateStr, "tv")
			if err != nil {
				sl.Warn("新闻加载失败 (tv)", zap.Error(err))
			} else {
				newsPagesTV = pages
			}
		}

		if renderMobile {
			if ctx.Err() != nil {
				return false
			}
			outPathMobile := filepath.Join(outputDir, dateStr, fmt.Sprintf("%s_tick_mobile.mp4", sessCfg.FilenameSuffix))
			outMobile, rErr := tick.RenderTickVideo(ctx, dateStr, outPathMobile, "mobile", sess, nil, nil, nil, copywriteText, newsPagesMobile)
			if rErr != nil {
				if ctx.Err() != nil {
					sl.Warn("Tick Mobile 渲染被用户取消")
					return false
				}
				sl.Error("Tick Mobile Remotion 渲染失败", zap.Error(rErr))
			} else {
				sl.Info("Tick Mobile 视频已保存", zap.String("output", outMobile))
				success = true
			}
		}

		if renderTV {
			if ctx.Err() != nil {
				return false
			}
			outPathTV := filepath.Join(outputDir, dateStr, fmt.Sprintf("%s_tick_tv.mp4", sessCfg.FilenameSuffix))
			outTV, rErr := tick.RenderTickVideo(ctx, dateStr, outPathTV, "tv", sess, nil, nil, nil, copywriteText, newsPagesTV)
			if rErr != nil {
				if ctx.Err() != nil {
					sl.Warn("Tick TV 渲染被用户取消")
					return false
				}
				sl.Error("Tick TV Remotion 渲染失败", zap.Error(rErr))
			} else {
				sl.Info("Tick TV 视频已保存", zap.String("output", outTV))
				success = true
			}
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
				Content: copywriteText,
			})
		}
		sl.Info("文案已保存", zap.String("type", "tick"))
	}

	if config.DataMode() == "json" {
		if db, err := storage.Get(); err == nil {
			db.ExportJSON()
		}
	}
	return success
}

func printSectorsTable(sectors []fetcher.Sector) {
	sep := strings.Repeat("-", 65)
	end := strings.Repeat("=", 70)
	fmt.Println()
	fmt.Printf("%-12s | %14s | %s\n", "板块", "主力资金净流入(亿)", "趋势")
	fmt.Println(sep)
	for _, info := range sectors {
		trend := "↓ 净流出"
		if info.Net > 0 {
			trend = "↑ 净流入"
		}
		fmt.Printf("%-12s | %14.2f | %s\n", info.Name, info.Net, trend)
	}
	fmt.Println(end)
	fmt.Println()
}

func generateMultiDayVideo(endDate string, days int, useAI bool) {
	l := logger.With(zap.String("date", endDate), zap.Int("days", days))
	l.Info("Bar Chart Race 视频生成")

	tradingDays, err := fetcher.GetTradingDays(endDate, days)
	if err != nil {
		l.Error("无法获取交易日", zap.Error(err))
		return
	}
	l.Info("交易日", zap.Strings("days", tradingDays))

	dayData, err := fetcher.LoadMultiDaySectors(tradingDays)
	if err != nil {
		l.Error("加载多日数据失败", zap.Error(err))
		return
	}

	l.Info("成功加载数据", zap.Int("days", len(dayData)))
	for _, d := range tradingDays {
		if sectors, ok := dayData[d]; ok {
			l.Info("日期数据", zap.String("date", d), zap.Int("sectors", len(sectors)))
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

	l.Info("Bar Chart Race 视频生成完成",
		zap.String("output", "output/"+dateLabel+"/"),
	)
}

// loadPrevPrediction 加载上一交易日的文案作为今日 AI 生成的上下文。
func loadPrevPrediction(todayStr, session string) string {
	prevDate := calcPrevDate(todayStr)
	if prevDate == "" {
		return ""
	}
	db, err := storage.Get()
	if err != nil {
		return ""
	}
	bySession, err := db.LoadCopywritingBySession(prevDate, session)
	if err == nil {
		for _, typ := range []string{"ai", "ai_tick"} {
			if c, ok := bySession[typ]; ok {
				return c
			}
		}
	}
	list, err := db.LoadCopywriting(prevDate)
	if err != nil || len(list) == 0 {
		return ""
	}
	for _, cw := range list {
		if cw.Type == "ai" || cw.Type == "ai_tick" {
			return cw.Content
		}
	}
	return list[0].Content
}

// calcPrevDate 计算上一个交易日（跳过周末）。
func calcPrevDate(todayStr string) string {
	t, err := time.Parse("2006-01-02", todayStr)
	if err != nil {
		return ""
	}
	for i := 1; i <= 3; i++ {
		d := t.AddDate(0, 0, -i)
		w := d.Weekday()
		if w == time.Saturday || w == time.Sunday {
			continue
		}
		return d.Format("2006-01-02")
	}
	return ""
}
