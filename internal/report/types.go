package report

import "github.com/a-share-flow-video-go/internal/analyzer"

// SectorSummary 板块摘要，用于日报排行。
type SectorSummary struct {
	Name                 string  `json:"name"`
	Net                  float64 `json:"net"`                  // 主力净流入（亿）
	ChangePct            float64 `json:"changePct"`            // 涨跌幅（%）
	SuperNet             float64 `json:"superNet"`             // 超大单净流入（亿）
	BigNet               float64 `json:"bigNet"`               // 大单净流入（亿）
	SuperRate            float64 `json:"superRate"`            // 超大单占比（%）
	BigRate              float64 `json:"bigRate"`              // 大单占比（%）
	TurnoverRate         float64 `json:"turnoverRate"`         // 换手率（%）
	LeadStockName        string  `json:"leadStockName"`        // 领涨股名称
	LeadStockChangePct   float64 `json:"leadStockChangePct"`   // 领涨股涨跌幅（%）
	TotalMarketCap       float64 `json:"totalMarketCap"`       // 总市值（亿）
	CirculatingMarketCap float64 `json:"circulatingMarketCap"` // 流通市值（亿）
}

// NewsBrief 新闻摘要。
type NewsBrief struct {
	Title   string   `json:"title"`
	Level   string   `json:"level"` // "A"/"B"/"C"
	Time    string   `json:"time"`  // "HH:mm"
	Brief   string   `json:"brief,omitempty"`
	Sectors []string `json:"sectors,omitempty"`
}

// DailyReport 日报完整结构。
type DailyReport struct {
	Date      string `json:"date"`      // "2026-06-09"
	CreatedAt string `json:"createdAt"` // "2026-06-09 15:08:00"
	Session   string `json:"session"`   // "full" / "morning"

	// 大盘概况
	NetTotal     float64 `json:"netTotal"`     // 整体净流向（亿）
	InflowCount  int     `json:"inflowCount"`  // 净流入板块数
	OutflowCount int     `json:"outflowCount"` // 净流出板块数

	// TOP 排行
	TopInflows  []SectorSummary `json:"topInflows"`  // top 5 流入
	TopOutflows []SectorSummary `json:"topOutflows"` // top 5 流出

	// 资金结构
	SuperNetTotal float64 `json:"superNetTotal"` // 超大单合计
	BigNetTotal   float64 `json:"bigNetTotal"`   // 大单合计
	StructureDesc string  `json:"structureDesc"` // "机构主导的集中流入" 等

	// AI 生成
	Summary string       `json:"summary"`         // 当日总结
	Outlook string       `json:"outlook"`         // 明日展望
	Cards   []ReportCard `json:"cards,omitempty"` // 知识卡片 2-3 张

	// 专题卡片（New Thematic Flow）
	ThematicCards []ThematicCard `json:"thematicCards,omitempty"` // AI 生成的专题卡片

	// 事件时间线（来自 tick 分析）
	Timeline []analyzer.TimelineEvent `json:"timeline,omitempty"`

	// 新闻摘要
	NewsBriefs []NewsBrief `json:"newsBriefs,omitempty"`

	// 引用文案
	Copywriting string `json:"copywriting,omitempty"` // 当日 AI 口播文案
}

// MarketStyleAnalysis 市场风格分析结构体
type MarketStyleAnalysis struct {
	Style          string `json:"style"`
	Tone           string `json:"tone"`
	Characteristics string `json:"characteristics"`
}

// FundStructureAnalysis 资金结构分析结构体
type FundStructureAnalysis struct {
	InstitutionalRatio string `json:"institutionalRatio"`
	BigPlayerRatio     string `json:"bigPlayerRatio"`
	FundType           string `json:"fundType"`
	RiskLevel          string `json:"riskLevel"`
	Characteristics    string `json:"characteristics"`
	Continuity         string `json:"continuity"`
	Leadership         string `json:"leadership"`
	ConcentrationRatio string `json:"concentrationRatio"`
}

// IndustryDistributionAnalysis 行业资金分布分析结构体
type IndustryDistributionAnalysis struct {
	ConcentrationRatio  string   `json:"concentrationRatio"`
	LeadingSector       string   `json:"leadingSector"`
	LeadingSectorNet    string   `json:"leadingSectorNet"`
	RotationDirection  string   `json:"rotationDirection"`
	RotationDetail      string   `json:"rotationDetail"`
	HighDensitySectors []string `json:"highDensitySectors"`
	LowDensitySectors  []string `json:"lowDensitySectors"`
}

// MarketStructureChangeAnalysis 市场结构变化分析结构体
type MarketStructureChangeAnalysis struct {
	ChangeType      string `json:"changeType"`
	ChangeDetail    string `json:"changeDetail"`
	Insight         string `json:"insight"`
	StructureChange string `json:"structureChange"`
	RotationChange  string `json:"rotationChange"`
	StyleChange     string `json:"styleChange"`
}

// OutflowRiskAnalysis 流出风险分析结构体
type OutflowRiskAnalysis struct {
	RiskLevel   string `json:"riskLevel"`
	RiskReason  string `json:"riskReason"`
	RiskAdvice  string `json:"riskAdvice"`
}

// LeaderFundLossAnalysis 龙头资金流失分析结构体
type LeaderFundLossAnalysis struct {
	LeaderSector    string `json:"leaderSector"`
	FundLossAmount  string `json:"fundLossAmount"`
	FundLossReason  string `json:"fundLossReason"`
	Continuity      string `json:"continuity"`
	Duration        string `json:"duration"`
}

// ThematicCard 专题卡片 - 替代原有固定卡片模板，支持主题化、带确信度的输出。
type ThematicCard struct {
	Theme       string       `json:"theme"`       // "主线确立" / "风险预警" / "轮动节点" / "情绪修复" / "资金异动"
	Conviction  float64      `json:"conviction"`  // 确信度 0-1
	TimeHorizon string       `json:"timeHorizon"` // "日内" / "Swing 3-5日" / "趋势 2-4周"
	Summary     string       `json:"summary"`     // 本主题一句话总结
	Reasoning   string       `json:"reasoning"`   // AI 推理链路（CoT 产出）
	Cards       []ReportCard `json:"cards"`       // 1-3 张细分知识卡片
}

// SectorRotationResult 板块轮动分析结果。
type SectorRotationResult struct {
	Phase       string   `json:"phase"`       // "启动"/"加速"/"高潮"/"退潮"
	LeaderSector string  `json:"leaderSector"` // 当前领涨板块
	FollowSectors []string `json:"followSectors"` // 跟涨板块
	LaggerSectors []string `json:"laggerSectors"` // 滞后板块
	AvgRotationDays int    `json:"avgRotationDays"` // 平均轮动周期（交易日）
	RotationAccelerating bool `json:"rotationAccelerating"` // 轮动是否在加速
}

// SentimentIndicators 情绪指标量化结果。
type SentimentIndicators struct {
	ProfitEffect   string  `json:"profitEffect"`   // 赚钱效应: "强"/"中"/"弱"
	LimitUpCount   int     `json:"limitUpCount"`   // 涨停数
	LimitDownCount int     `json:"limitDownCount"` // 跌停数
	HighBoardCount int     `json:"highBoardCount"` // 连板高度（最高）
	BreakRate      float64 `json:"breakRate"`       // 炸板率 %
	SentimentScore float64 `json:"sentimentScore"`  // 情绪综合评分 -10 ~ 10
}

// DragonTigerSummary 龙虎榜汇总（用于卡片展示）。
type DragonTigerSummary struct {
	OrgBuyTotal    float64 `json:"orgBuyTotal"`    // 机构净买入合计（亿）
	DealerBuyTotal float64 `json:"dealerBuyTotal"` // 游资净买入合计（亿）
	TopStocks      []struct {
		Name      string  `json:"name"`
		NetBuyAmt float64 `json:"netBuyAmt"`
		Dealer    string  `json:"dealer"`
	} `json:"topStocks"`
}

// NorthboundSummary 北向资金汇总（用于卡片展示）。
type NorthboundSummary struct {
	DailyNet    float64 `json:"dailyNet"`    // 当日净买入
	WeekNet     float64 `json:"weekNet"`      // 近5日累计
	MonthNet    float64 `json:"monthNet"`     // 近20日累计
	TopBuyStocks []struct {
		Name  string  `json:"name"`
		Value float64 `json:"value"`
	} `json:"topBuyStocks"`
	Direction string `json:"direction"` // "大幅流入"/"小幅流入"/"均衡"/"小幅流出"/"大幅流出"
}

// MarginSummaryExt 融资融券汇总（用于卡片展示）。
type MarginSummaryExt struct {
	RZNetBuyDaily float64 `json:"rzNetBuyDaily"` // 当日融资净买入
	RZBalance     float64 `json:"rzBalance"`     // 融资余额
	RQBalance     float64 `json:"rqBalance"`     // 融券余额
	RZNetTrend    string  `json:"rzNetTrend"`    // "加杠杆"/"去杠杆"/"中性"
}

// NewsFundMatchAnalysis 新闻与资金匹配分析结构体
type NewsFundMatchAnalysis struct {
	MatchedInflowSectors   []string `json:"matchedInflowSectors"`
	MatchedOutflowSectors   []string `json:"matchedOutflowSectors"`
	MatchScore             string   `json:"matchScore"`
	Insight                string   `json:"insight"`
}
