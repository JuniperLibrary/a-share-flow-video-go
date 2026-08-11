package tick

// 图表解说分段文案提示词模板
// generateChartNarrationSegments 使用的所有模板字符串。

const (
	// ChartNarrationTmplInflection 拐点解说模板。
	// Args: timePrefix ("09:45，"), sectorName, action, deltaDirection, delta, cumDirection, cum.
	// 示例: "注意看，09:45，有色金属突然加速，单段净流入12.5亿，累计净流入85.3亿。"
	ChartNarrationTmplInflection = "注意看，%s%s%s，单段%s%.1f亿，累计%s%.1f亿。"

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
	// 示例: "开盘先看有色金属、AI应用，资金率先点火，盘面合计净流入85亿元。"
	ChartNarrationTmplOpen = "开盘先看%s，资金率先点火，盘面合计%s%.0f亿元。"
	// ChartNarrationTmplOpenFallback 开盘段：资金变动不显著。
	ChartNarrationTmplOpenFallback = "开盘资金先试探，主要方向还没完全拉开差距，盘面合计%s%.0f亿元。"

	// ChartNarrationTmplMid 盘中段：有显著板块。
	// 示例: "盘中主线逐步清晰，有色金属、AI应用处在资金前排，合计净流入85亿元。"
	ChartNarrationTmplMid = "盘中主线逐步清晰，%s处在资金前排，合计%s%.0f亿元。"
	// ChartNarrationTmplMidFallback 盘中段：资金变动不显著。
	ChartNarrationTmplMidFallback = "盘中轮动明显加快，资金仍在试探切换，合计%s%.0f亿元。"

	// ChartNarrationTmplClose 尾盘段：有显著板块。
	// 示例: "临近收盘，有色金属、AI应用仍在资金前排，全天合计净流入85亿元。"
	ChartNarrationTmplClose = "临近收盘，%s仍在资金前排，全天合计%s%.0f亿元。"
	// ChartNarrationTmplCloseFallback 尾盘段：资金变动不显著。
	ChartNarrationTmplCloseFallback = "收盘看资金分布仍偏分散，全天合计%s%.0f亿元。"

	// ChartNarrationSectorFmt 板块方向金额拼接格式。
	// Args: sectorName, direction, absNet.
	// 示例: "有色金属净流入85亿"
	ChartNarrationSectorFmt = "%s%s%.0f亿"
)
