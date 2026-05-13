package main

import (
	"fmt"
	"os"
	"path/filepath"
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
// 支持参数：--ai(AI文案模式) YYYY-MM-DD(指定日期)
func main() {
	useAI := false
	var dates []string

	for _, arg := range os.Args[1:] {
		switch arg {
		case "--ai":
			useAI = true
		default:
			dates = append(dates, arg)
		}
	}

	if len(dates) == 0 {
		dates = []string{time.Now().Format("2006-01-02")}
	}

	mode := "模板"
	if useAI {
		mode = "AI"
	}
	fmt.Printf("=== 板块资金流向视频生成（%s模式） ===\n", mode)
	fmt.Printf("将处理 %d 个日期: %s\n", len(dates), dates)

	successCount := 0
	for _, dateStr := range dates {
		if processDate(dateStr, useAI) {
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
func processDate(dateStr string, useAI bool) bool {
	fmt.Printf("\n%s\n", sep70)
	fmt.Printf("处理日期: %s\n", dateStr)
	fmt.Printf("%s\n", sep70)

	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		fmt.Printf("错误: 无效日期格式 '%s'，请使用 YYYY-MM-DD 格式\n", dateStr)
		return false
	}

	dateDir := filepath.Join(config.GetDataDir(), dateStr)
	sectorsFile := filepath.Join(dateDir, "sectors.csv")
	today := time.Now().Format("2006-01-02")

	var sectors []fetcher.Sector
	var err error

	if _, statErr := os.Stat(sectorsFile); statErr == nil {
		fmt.Printf("从本地加载 %s 数据...\n", dateStr)
		sectors, err = fetcher.LoadCachedData(dateStr)
		if err != nil {
			fmt.Printf("错误: 加载缓存数据失败: %v\n", err)
			return false
		}
		fmt.Printf("已加载 %d 个板块\n", len(sectors))
		printSectorsTable(sectors)
	} else if dateStr == today {
		fmt.Printf("获取 %s 实时热门板块数据...\n", dateStr)
		sectors, err = fetcher.FetchTop15HotSectors()
		if err != nil || len(sectors) == 0 {
			fmt.Printf("错误: 无法获取热门板块数据\n")
			return false
		}
		fetcher.SaveDailyData(sectors, dateStr)
		fmt.Printf("热门15板块: 匹配 %d 个\n", len(sectors))
		printSectorsTable(sectors)
	} else {
		fmt.Printf("尝试获取 %s 历史热门数据...\n", dateStr)
		sectors, err = fetcher.FetchHistoricalSectors(dateStr)
		if err != nil || len(sectors) == 0 {
			fmt.Printf("错误: %s 历史数据获取失败\n", dateStr)
			fmt.Printf("提示: 请检查网络连接，或手动保存数据到 data/%s/sectors.csv\n", dateStr)
			return false
		}
		fetcher.SaveDailyData(sectors, dateStr)
		fmt.Printf("历史热门: 匹配 %d 个\n", len(sectors))
		printSectorsTable(sectors)
	}

	generateAll(sectors, dateStr, dateDir, useAI, "mobile")
	generateAll(sectors, dateStr, dateDir, useAI, "tv")
	return true
}

func generateAll(sectors []fetcher.Sector, dateStr, dateDir string, useAI bool, format string) {
	os.MkdirAll(dateDir, 0755)

	allSectors, _ := fetcher.FetchAllRaw()
	events, timeline, ticker := analyzer.AnalyzeAllContent(allSectors, dateStr)
	if len(events) == 0 {
		events = analyzer.GetFallbackEvents(config.TotalFrames)
	}

	outputDir := config.GetOutputDir()
	os.MkdirAll(filepath.Join(outputDir, dateStr), 0755)

	formatSuffix := ""
	if format == "tv" {
		formatSuffix = "_tv"
	}
	outPath := filepath.Join(outputDir, dateStr, fmt.Sprintf("全天%s.mp4", formatSuffix))

	if _, err := renderer.RenderVideo(sectors, dateStr, outPath, events, timeline, ticker, format); err != nil {
		fmt.Printf("错误: Remotion 渲染失败: %v\n", err)
		return
	}

	formatLabel := "移动端"
	if format == "tv" {
		formatLabel = "TV端"
	}
	fmt.Printf("视频已保存: output/%s/全天%s.mp4\n", dateStr, formatLabel)

	copyDir := filepath.Join(config.GetCopyDir(), dateStr)
	os.MkdirAll(copyDir, 0755)

	for _, sess := range []string{"full", "morning", "afternoon"} {
		sessCfg := config.SessionConfigs[sess]
		var text string
		if useAI {
			var err error
			text, err = copy.GenerateCopywritingAI(sectors, dateStr, sess)
			if err != nil {
				fmt.Printf("警告: AI文案(%s)生成失败: %v\n", sessCfg.TitleSuffix, err)
				text = copy.GenerateCopywriting(sectors, dateStr, sess)
			}
		} else {
			text = copy.GenerateCopywriting(sectors, dateStr, sess)
		}
		prefix := "文案"
		if useAI {
			prefix = "文案_ai"
		}
		os.WriteFile(filepath.Join(copyDir, fmt.Sprintf("%s_%s.txt", prefix, sessCfg.TitleSuffix)), []byte(text), 0644)
	}
	fmt.Printf("文案已保存: 早盘/午盘/全天\n")
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
