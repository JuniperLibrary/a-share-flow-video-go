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
	"github.com/a-share-flow-video-go/internal/renderer"
)

var sep70 = strings.Repeat("=", 70)
var sep65 = strings.Repeat("-", 65)

// CLI 入口：命令行视频生成器。
// 支持参数：--ai(AI文案模式) --session=morning/full YYYY-MM-DD(指定日期)
func main() {
	useAI := false
	session := ""
	days := 0
	var dates []string

	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--ai":
			useAI = true
		case strings.HasPrefix(arg, "--session="):
			session = strings.TrimPrefix(arg, "--session=")
		case strings.HasPrefix(arg, "--days="):
			days, _ = strconv.Atoi(strings.TrimPrefix(arg, "--days="))
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
	fmt.Printf("=== 板块资金流向视频生成（%s模式, %s） ===\n", mode, sessLabel)
	fmt.Printf("将处理 %d 个日期: %s\n", len(dates), dates)

	successCount := 0
	for _, dateStr := range dates {
		if processDate(dateStr, useAI, session) {
			successCount++
		}
	}

	fmt.Printf("\n%s\n", sep70)
	fmt.Printf("完成！成功处理 %d/%d 个日期\n", successCount, len(dates))
	fmt.Printf("输出目录: output/YYYY-MM-DD/\n")
	fmt.Printf("数据目录: data/YYYY-MM-DD/\n")
	fmt.Printf("%s\n", sep70)
}

// processDate 处理单个日期的完整管道
func processDate(dateStr string, useAI bool, sessionOverride string) bool {
	fmt.Printf("\n%s\n", sep70)
	fmt.Printf("处理日期: %s\n", dateStr)
	fmt.Printf("%s\n", sep70)

	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		fmt.Printf("错误: 无效日期格式 '%s'，请使用 YYYY-MM-DD 格式\n", dateStr)
		return false
	}

	dateDir := filepath.Join(config.GetDataDir(), dateStr)
	today := time.Now().Format("2006-01-02")

	// 确定要处理的 session 列表
	var sessions []string
	if sessionOverride != "" {
		sessions = []string{sessionOverride}
	} else if dateStr == today {
		// 当天：根据当前时间自动判断
		now := time.Now()
		if now.Hour() < 13 {
			sessions = []string{"morning"}
		} else {
			sessions = []string{"full"}
		}
	} else {
		// 历史日期：检查哪些数据文件存在
		if _, err := os.Stat(filepath.Join(dateDir, "sectors_morning.csv")); err == nil {
			sessions = append(sessions, "morning")
		}
		if _, err := os.Stat(filepath.Join(dateDir, "sectors.csv")); err == nil {
			sessions = append(sessions, "full")
		}
		if len(sessions) == 0 {
			// 都没有，尝试获取历史全天数据
			sessions = []string{"full"}
		}
	}

	for _, session := range sessions {
		sessCfg := config.SessionConfigs[session]
		fmt.Printf("\n--- 生成 %s 视频 ---\n", sessCfg.TitleSuffix)

		sectorsFile := filepath.Join(dateDir, "sectors.csv")
		if session == "morning" {
			sectorsFile = filepath.Join(dateDir, "sectors_morning.csv")
		}

		var sectors []fetcher.Sector
		var err error

		if _, statErr := os.Stat(sectorsFile); statErr == nil {
			fmt.Printf("从本地加载 %s %s数据...\n", dateStr, sessCfg.TitleSuffix)
			sectors, err = fetcher.LoadSessionData(dateStr, session)
			if err != nil {
				fmt.Printf("错误: 加载缓存数据失败: %v\n", err)
				continue
			}
			fmt.Printf("已加载 %d 个板块\n", len(sectors))
			printSectorsTable(sectors)
		} else if dateStr == today && session == "morning" {
			fmt.Printf("获取 %s 早盘实时热门板块数据...\n", dateStr)
			sectors, err = fetcher.FetchTop18HotSectors()
			if err != nil || len(sectors) == 0 {
				fmt.Printf("错误: 无法获取热门板块数据\n")
				continue
			}
			fetcher.SaveSessionData(sectors, dateStr, session)
			fmt.Printf("热门板块: 匹配 %d 个\n", len(sectors))
			printSectorsTable(sectors)
		} else if dateStr == today && session == "full" {
			fmt.Printf("获取 %s 全天实时热门板块数据...\n", dateStr)
			sectors, err = fetcher.FetchTop18HotSectors()
			if err != nil || len(sectors) == 0 {
				fmt.Printf("错误: 无法获取热门板块数据\n")
				continue
			}
			fetcher.SaveSessionData(sectors, dateStr, session)
			fmt.Printf("热门板块: 匹配 %d 个\n", len(sectors))
			printSectorsTable(sectors)
		} else {
			fmt.Printf("尝试获取 %s 历史热门数据...\n", dateStr)
			sectors, err = fetcher.FetchHistoricalSectors(dateStr)
			if err != nil || len(sectors) == 0 {
				fmt.Printf("错误: %s 历史数据获取失败\n", dateStr)
				fmt.Printf("提示: 请检查网络连接，或手动保存数据到 data/%s/sectors.csv\n", dateStr)
				continue
			}
			fetcher.SaveSessionData(sectors, dateStr, session)
			fmt.Printf("历史热门: 匹配 %d 个\n", len(sectors))
			printSectorsTable(sectors)
		}

		generateSession(sectors, dateStr, dateDir, useAI, session)
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

	for _, format := range []string{"mobile", "tv"} {
		formatSuffix := ""
		if format == "tv" {
			formatSuffix = "_tv"
		}
		outPath := filepath.Join(outputDir, dateStr, fmt.Sprintf("%s%s.mp4", sessCfg.FilenameSuffix, formatSuffix))

		if _, err := renderer.RenderVideo(sectors, dateStr, outPath, events, timeline, ticker, format, session); err != nil {
			fmt.Printf("错误: Remotion 渲染失败(%s %s): %v\n", sessCfg.TitleSuffix, format, err)
			continue
		}
		fmt.Printf("视频已保存: output/%s/%s%s.mp4\n", dateStr, sessCfg.FilenameSuffix, formatSuffix)
	}

	copyDir := filepath.Join(config.GetCopyDir(), dateStr)
	os.MkdirAll(copyDir, 0755)

	var text string
	if useAI {
		var err error
		text, err = copy.GenerateCopywritingAI(sectors, dateStr, session)
		if err != nil {
			fmt.Printf("警告: AI文案(%s)生成失败: %v\n", sessCfg.TitleSuffix, err)
			text = copy.GenerateCopywriting(sectors, dateStr, session)
		}
	} else {
		text = copy.GenerateCopywriting(sectors, dateStr, session)
	}
	prefix := "文案"
	if useAI {
		prefix = "文案_ai"
	}
	os.WriteFile(filepath.Join(copyDir, fmt.Sprintf("%s_%s.txt", prefix, sessCfg.TitleSuffix)), []byte(text), 0644)
	fmt.Printf("文案已保存: %s\n", sessCfg.TitleSuffix)
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
	fmt.Printf("%s\n", sep70)
	fmt.Printf("=== 近%d日板块资金流向 Bar Chart Race 视频生成 ===\n", days)
	fmt.Printf("截止日期: %s\n", endDate)
	fmt.Printf("%s\n", sep70)

	tradingDays, err := fetcher.GetTradingDays(endDate, days)
	if err != nil {
		fmt.Printf("错误: 无法获取交易日: %v\n", err)
		return
	}
	fmt.Printf("交易日: %s\n", tradingDays)

	dayData, err := fetcher.LoadMultiDaySectors(tradingDays)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Printf("成功加载 %d 日数据\n", len(dayData))
	for _, d := range tradingDays {
		if sectors, ok := dayData[d]; ok {
			fmt.Printf("  %s: %d 个板块\n", d, len(sectors))
		}
	}

	analysis := analyzer.MultiDayAnalyze(dayData, tradingDays)

	outputDir := config.GetOutputDir()
	dateLabel := tradingDays[0]
	if len(tradingDays) > 1 {
		dateLabel = tradingDays[0] + "_to_" + tradingDays[len(tradingDays)-1]
	}

	os.MkdirAll(filepath.Join(outputDir, dateLabel), 0755)

	for _, format := range []string{"mobile", "tv"} {
		formatSuffix := ""
		if format == "tv" {
			formatSuffix = "_tv"
		}
		outPath := filepath.Join(outputDir, dateLabel, fmt.Sprintf("三日资金流向%s.mp4", formatSuffix))

		if _, err := renderer.RenderMultiDayVideo(dayData, tradingDays, outPath, analysis, format); err != nil {
			fmt.Printf("错误: Remotion 渲染失败(%s): %v\n", format, err)
			continue
		}
		fmt.Printf("视频已保存: output/%s/三日资金流向%s.mp4\n", dateLabel, formatSuffix)
	}

	fmt.Printf("\n%s\n", sep70)
	fmt.Printf("完成！Bar Chart Race 视频已生成\n")
	fmt.Printf("输出目录: output/%s/\n", dateLabel)
	fmt.Printf("%s\n", sep70)
}
