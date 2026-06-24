package report

import (
	"math"

	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// AnalyzeSentiment 从板块数据计算情绪指标。
// 基于板块涨跌幅分布、流入流出比等，估算市场情绪状态。
func AnalyzeSentiment(r *DailyReport) SentimentIndicators {
	indicators := SentimentIndicators{}

	// 1. 基于流入/流出板块比例估算赚钱效应
	totalSectors := r.InflowCount + r.OutflowCount
	if totalSectors == 0 {
		indicators.ProfitEffect = "中性"
		indicators.SentimentScore = 0
		return indicators
	}

	inflowRatio := float64(r.InflowCount) / float64(totalSectors)
	// 估算涨停数（无实时数据时的近似值）
	// 当流入比例 > 0.6 时，市场偏强
	// 当流入比例 < 0.3 时，市场偏弱
	switch {
	case inflowRatio > 0.6:
		indicators.ProfitEffect = "强"
		indicators.SentimentScore = float64(r.InflowCount - r.OutflowCount)
		indicators.LimitUpCount = int(inflowRatio * 30)   // 估算
		indicators.LimitDownCount = int((1 - inflowRatio) * 10)
	case inflowRatio < 0.3:
		indicators.ProfitEffect = "弱"
		indicators.SentimentScore = float64(r.InflowCount - r.OutflowCount)
		indicators.LimitUpCount = int(inflowRatio * 10)
		indicators.LimitDownCount = int((1 - inflowRatio) * 30)
	default:
		indicators.ProfitEffect = "中"
		indicators.SentimentScore = 0
		indicators.LimitUpCount = int(inflowRatio * 20)
		indicators.LimitDownCount = int((1 - inflowRatio) * 15)
	}

	// 2. 基于板块涨跌幅估算炸板率
	avgChange := calculateAvgChangePct(r)
	indicators.BreakRate = estimateBreakRate(avgChange)

	// 3. 基于资金集中度估算连板高度（最高板）
	highBoard := estimateHighBoard(r)
	indicators.HighBoardCount = highBoard

	// 4. 综合评分
	indicators.SentimentScore = calculateSentimentScore(r, inflowRatio)

	// 限制范围
	if indicators.SentimentScore > 10 {
		indicators.SentimentScore = 10
	} else if indicators.SentimentScore < -10 {
		indicators.SentimentScore = -10
	}

	logger.Info("情绪指标分析",
		zap.String("profitEffect", indicators.ProfitEffect),
		zap.Float64("score", indicators.SentimentScore),
		zap.Float64("breakRate", indicators.BreakRate),
	)

	return indicators
}

// calculateAvgChangePct 计算板块平均涨跌幅（取 TOP 板块加权）。
func calculateAvgChangePct(r *DailyReport) float64 {
	var totalChange, weight float64
	for i := 0; i < len(r.TopInflows) && i < 5; i++ {
		w := 1.0 / float64(i+1)
		totalChange += r.TopInflows[i].ChangePct * w
		weight += w
	}
	for i := 0; i < len(r.TopOutflows) && i < 5; i++ {
		w := 1.0 / float64(i+1)
		totalChange += r.TopOutflows[i].ChangePct * w
		weight += w
	}
	if weight == 0 {
		return 0
	}
	return totalChange / weight
}

// estimateBreakRate 基于板块平均涨跌幅估算炸板率。
func estimateBreakRate(avgChange float64) float64 {
	avgChange = math.Abs(avgChange)
	switch {
	case avgChange > 5:
		return math.Max(10, 30-avgChange*2)
	case avgChange > 2:
		return 20
	default:
		return 25
	}
}

// estimateHighBoard 基于资金集中度估算最高连板高度。
func estimateHighBoard(r *DailyReport) int {
	if r.NetTotal == 0 {
		return 0
	}
	// 如果 TOP1 流入占比很高，说明资金高度集中，可能有高标
	if len(r.TopInflows) > 0 {
		top1Ratio := math.Abs(r.TopInflows[0].Net) / math.Abs(r.NetTotal)
		switch {
		case top1Ratio > 0.4:
			return 6 // 6 板以上
		case top1Ratio > 0.25:
			return 4
		default:
			return 2
		}
	}
	return 1
}

// calculateSentimentScore 计算综合情绪评分。
func calculateSentimentScore(r *DailyReport, inflowRatio float64) float64 {
	score := 0.0

	// 资金面得分
	if r.NetTotal > 0 {
		score += math.Min(r.NetTotal/50, 3) // 每 50 亿 +1 分，最多 +3
	} else {
		score -= math.Min(-r.NetTotal/50, 3)
	}

	// 宽度得分
	score += (inflowRatio - 0.5) * 10 // -5 ~ +5

	// 结构得分
	if len(r.TopInflows) > 0 && len(r.TopOutflows) > 0 {
		netRatio := math.Abs(r.TopInflows[0].Net) / (math.Abs(r.TopInflows[0].Net) + math.Abs(r.TopOutflows[0].Net))
		score += (netRatio - 0.5) * 4 // -2 ~ +2
	}

	return score
}
