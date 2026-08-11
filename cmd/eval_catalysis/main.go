package main

import (
	"fmt"
	"os"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/evaluator"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/hotnews"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/tick"
	"go.uber.org/zap"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./cmd/eval_catalysis <YYYY-MM-DD> [session=full|morning|afternoon]")
		os.Exit(1)
	}
	dateStr := os.Args[1]
	session := "full"
	if len(os.Args) >= 3 {
		session = os.Args[2]
	}
	_ = config.LoadEnv()
	if err := logger.Init("info", "console", "stdout"); err != nil {
		panic(err)
	}
	l := logger.With(zap.String("date", dateStr), zap.String("session", session))

	sectors, err := fetcher.FetchHistoricalSectors(dateStr)
	if err != nil || len(sectors) == 0 {
		l.Warn("历史板块获取失败，尝试 DB / CSV")
	}
	_ = sectors

	points, err := tick.LoadTickCSV(dateStr, session)
	if err != nil {
		l.Error("tick CSV 加载失败", zap.Error(err))
		os.Exit(2)
	}
	sectorTicks, tickTimes := tick.BuildSectorTicks(points)
	newsPages, err := hotnews.LoadForVideo(dateStr, "mobile")
	if err != nil || len(newsPages) == 0 {
		newsPagesTV, err2 := hotnews.LoadForVideo(dateStr, "tv")
		if err2 == nil {
			newsPages = newsPagesTV
		} else {
			l.Warn("新闻加载失败，只跑规则打分", zap.Error(err), zap.NamedError("tvErr", err2))
			newsPages = nil
		}
	}
	l.Info("评估骨架数据就绪",
		zap.Int("sectorTicks", len(sectorTicks)),
		zap.Int("tickTimes", len(tickTimes)),
		zap.Int("newsPages", len(newsPages)))

	result := tick.GenerateCatalysis(sectorTicks, tickTimes, newsPages)
	allowed := make(map[string]bool, len(fetcher.Top21HotSectors))
	for _, s := range fetcher.Top21HotSectors {
		allowed[s] = true
	}
	bundle := evaluator.BuildCatalysisEvalBundle(dateStr, sectorTicks, tickTimes, newsPages, result, allowed)
	outDir := config.GetOutputDir()
	path, err := evaluator.SaveCatalysisEval(outDir, dateStr, bundle)
	if err != nil {
		l.Error("评估 JSON 写盘失败", zap.Error(err))
		os.Exit(3)
	}
	fmt.Printf("[%s] catalysis eval bundle 已生成: %s\n", time.Now().Format("15:04:05"), path)
}
