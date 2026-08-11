package main

import (
	"fmt"

	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/renderer"
	"github.com/a-share-flow-video-go/internal/report"
	"github.com/a-share-flow-video-go/internal/storage"
)

// runReportImage 生成日报图片。
// 在 main.go 中通过 --report-image 2026-06-15 调用。
func runReportImage(dateStr string) error {
	r, err := report.Generate(dateStr)
	if err != nil {
		return fmt.Errorf("generate report: %w", err)
	}

	// 加载辅助数据（失败不阻塞）
	if _, err := storage.Get(); err == nil {
		if _, err := loadDragonTiger(dateStr); err == nil {
			logger.Info("龙虎榜数据加载成功")
		}
		if _, err := loadNorthbound(dateStr); err == nil {
			logger.Info("北向资金数据加载成功")
		}
	}

	// 优先使用 AI 生成的 ThematicCards，否则从 ReportCard 转换
	var thematicCards []report.ThematicCard
	if len(r.ThematicCards) > 0 {
		thematicCards = r.ThematicCards
	} else if len(r.Cards) > 0 {
		thematicCards = buildThematicCardsFromReport(r)
	} else {
		thematicCards = buildDataDrivenThematicCards(r)
	}

	paths, err := renderer.RenderReportImages(r, thematicCards, dateStr)
	if err != nil {
		return fmt.Errorf("render report images: %w", err)
	}

	fmt.Printf("日报图片已生成（%d 张）:\n", len(paths))
	for _, p := range paths {
		fmt.Printf("  %s\n", p)
	}
	return nil
}

func buildThematicCardsFromReport(r *report.DailyReport) []report.ThematicCard {
	if len(r.Cards) == 0 {
		return nil
	}
	theme := "主线确立"
	if r.NetTotal < -50 {
		theme = "风险预警"
	} else if r.NetTotal < 0 {
		theme = "轮动节点"
	}
	conviction := 0.5
	switch {
	case r.NetTotal > 100:
		conviction = 0.8
	case r.NetTotal < -100:
		conviction = 0.7
	}
	return []report.ThematicCard{
		{
			Theme:       theme,
			Conviction:  conviction,
			TimeHorizon: "日内",
			Summary:     r.Summary,
			Cards:       r.Cards,
		},
	}
}

func buildDataDrivenThematicCards(r *report.DailyReport) []report.ThematicCard {
	theme := "轮动节点"
	if r.NetTotal > 50 {
		theme = "主线确立"
	} else if r.NetTotal < -50 {
		theme = "风险预警"
	}
	cards := report.BuildCardsFromData(r)
	return []report.ThematicCard{
		{
			Theme:       theme,
			Conviction:  0.5,
			TimeHorizon: "日内",
			Summary:     r.Summary,
			Reasoning:   "基于板块资金数据的自动化分析",
			Cards:       cards,
		},
	}
}

func loadDragonTiger(dateStr string) ([]report.DragonTigerSummary, error) {
	return nil, nil
}

func loadNorthbound(dateStr string) ([]report.NorthboundSummary, error) {
	return nil, nil
}
