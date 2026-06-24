package report

import (
	"fmt"
	"math"
	"sort"

	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// AnalyzeSectorRotation 分析板块轮动状态。
// 输入：各板块历史 N 日资金流向数据（从 storage 加载）。
// 输出：轮动阶段、领涨/跟涨/滞后板块、轮动周期天数、加速信号。
func AnalyzeSectorRotation(history []fetcher.Sector, days int) SectorRotationResult {
	result := SectorRotationResult{
		AvgRotationDays: 5, // 默认
	}

	if len(history) < 3 {
		result.Phase = "数据不足"
		return result
	}

	// 1. 计算各板块近期累计净流入
	type sectorNet struct {
		name    string
		total   float64
		dayCount int
	}

	sectorMap := make(map[string]*sectorNet)
	for _, s := range history {
		if s.Net == 0 {
			continue
		}
		if _, ok := sectorMap[s.Name]; !ok {
			sectorMap[s.Name] = &sectorNet{name: s.Name}
		}
		entry := sectorMap[s.Name]
		entry.total += s.Net
		entry.dayCount++
	}

	if len(sectorMap) == 0 {
		result.Phase = "数据不足"
		return result
	}

	// 2. 按总净流入排序，识别领涨/跟涨/滞后
	type rankedSector struct {
		name  string
		total float64
	}
	var ranked []rankedSector
	for _, v := range sectorMap {
		ranked = append(ranked, rankedSector{name: v.name, total: v.total})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].total > ranked[j].total
	})

	// 前 3 为领涨，中间为跟涨，后 3 为滞后
	if len(ranked) > 0 {
		nLeader := 3
		if len(ranked) < nLeader {
			nLeader = len(ranked)
		}
		for i := 0; i < nLeader; i++ {
			result.LeaderSector = ranked[0].name
			if i > 0 {
				result.FollowSectors = append(result.FollowSectors, ranked[i].name)
			}
		}

		// 后 3 个为滞后
		for i := len(ranked) - 1; i >= 0 && len(result.LaggerSectors) < 3; i-- {
			result.LaggerSectors = append(result.LaggerSectors, ranked[i].name)
		}
	}

	// 3. 判断轮动阶段
	totalSectors := len(ranked)
	top3Ratio := 0.0
	if len(ranked) >= 3 {
		var top3Total, allTotal float64
		for i, r := range ranked {
			allTotal += math.Abs(r.total)
			if i < 3 {
				top3Total += math.Abs(r.total)
			}
		}
		if allTotal > 0 {
			top3Ratio = top3Total / allTotal
		}
	}

	switch {
	case top3Ratio > 0.6 && len(ranked) > 0:
		result.Phase = "加速"
		result.RotationAccelerating = true
	case top3Ratio > 0.4:
		result.Phase = "高潮"
	default:
		result.Phase = "轮动"
	}

	logger.Info("板块轮动分析",
		zap.String("phase", result.Phase),
		zap.String("leader", result.LeaderSector),
		zap.Float64("top3Ratio", top3Ratio),
		zap.Int("totalSectors", totalSectors),
		zap.Int("followCount", len(result.FollowSectors)),
	)

	return result
}

// CalculateRotationSpeed 计算板块轮动速度（覆盖/增强 report.go 中同名函数）。
// 返回: "快" / "中" / "慢"
func CalculateRotationSpeed(r *DailyReport) string {
	if len(r.TopInflows) == 0 || len(r.TopOutflows) == 0 {
		return "慢"
	}
	inflowChange := r.TopInflows[0].ChangePct
	outflowChange := r.TopOutflows[0].ChangePct

	diff := math.Abs(inflowChange - outflowChange)
	switch {
	case diff > 8:
		return "快"
	case diff > 3:
		return "中"
	default:
		return "慢"
	}
}

// BuildSectorRotationSummary 构建板块轮动文字摘要。
func BuildSectorRotationSummary(rotation SectorRotationResult) string {
	return fmt.Sprintf("当前市场处于「%s」阶段，领涨板块为%s，跟涨板块%v，轮动周期约%d个交易日",
		rotation.Phase,
		rotation.LeaderSector,
		rotation.FollowSectors,
		rotation.AvgRotationDays,
	)
}
