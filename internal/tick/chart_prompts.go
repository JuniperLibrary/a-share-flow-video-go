package tick

// 图表解说分段文案提示词模板
// generateChartNarrationSegments 使用的所有模板字符串。

const (
	// ChartNarrationTmplInflection 拐点解说模板。
	// Args: timePrefix ("09:45，"), sectorName, action, direction, delta, cum.
	// 示例: "注意看，09:45，有色金属突然加速，单笔净流入12.5亿，累计85.3亿。"
	ChartNarrationTmplInflection = "注意看，%s%s%s，单笔%s%.1f亿，累计%.1f亿。"

	// ChartNarrationActionAccelerate 突然加速
	ChartNarrationActionAccelerate = "突然加速"
	// ChartNarrationActionWeaken 明显转弱
	ChartNarrationActionWeaken = "明显转弱"

	// ChartNarrationDirectionInflow 净流入
	ChartNarrationDirectionInflow = "净流入"
	// ChartNarrationDirectionNetOutflow 净流出
	ChartNarrationDirectionNetOutflow = "净流出"

	// ChartNarrationTmplOpen 开盘段：有显著板块。
	// Args: top2Joined, direction, absTotal.
	// 示例: "开盘后资金率先涌入有色金属、AI应用，整体净流入85亿元。"
	ChartNarrationTmplOpen = "开盘后资金率先涌入%s，整体%s%.0f亿元。"
	// ChartNarrationTmplOpenFallback 开盘段：资金变动不显著。
	ChartNarrationTmplOpenFallback = "开盘后各板块资金变动不大，整体%s%.0f亿元。"

	// ChartNarrationTmplMid 盘中段：有显著板块。
	// 示例: "盘中有色金属持续领跑，累计净流入85亿元。"
	ChartNarrationTmplMid = "盘中%s持续领跑，累计%s%.0f亿元。"
	// ChartNarrationTmplMidFallback 盘中段：资金变动不显著。
	ChartNarrationTmplMidFallback = "盘中资金格局平稳，累计%s%.0f亿元。"

	// ChartNarrationTmplClose 尾盘段：有显著板块。
	// 示例: "尾盘来看，有色金属领先，全天净流入85亿元。"
	ChartNarrationTmplClose = "尾盘来看，%s领先，全天%s%.0f亿元。"
	// ChartNarrationTmplCloseFallback 尾盘段：资金变动不显著。
	ChartNarrationTmplCloseFallback = "收盘板块资金整体%s%.0f亿元，分布较为分散。"

	// ChartNarrationSectorFmt 板块方向金额拼接格式。
	// Args: sectorName, direction, absNet.
	// 示例: "有色金属净流入85亿"
	ChartNarrationSectorFmt = "%s%s%.0f亿"
)
