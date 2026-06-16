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
	Title string `json:"title"`
	Level string `json:"level"` // "A"/"B"/"C"
	Time  string `json:"time"`  // "HH:mm"
}

// DailyReport 日报完整结构。
type DailyReport struct {
	Date       string         `json:"date"`       // "2026-06-09"
	CreatedAt  string         `json:"createdAt"`  // "2026-06-09 15:08:00"
	Session    string         `json:"session"`    // "full" / "morning"

	// 大盘概况
	NetTotal   float64 `json:"netTotal"`   // 整体净流向（亿）
	InflowCount int    `json:"inflowCount"` // 净流入板块数
	OutflowCount int   `json:"outflowCount"` // 净流出板块数

	// TOP 排行
	TopInflows  []SectorSummary `json:"topInflows"`  // top 5 流入
	TopOutflows []SectorSummary `json:"topOutflows"` // top 5 流出

	// 资金结构
	SuperNetTotal  float64 `json:"superNetTotal"`  // 超大单合计
	BigNetTotal    float64 `json:"bigNetTotal"`    // 大单合计
	StructureDesc  string  `json:"structureDesc"`  // "机构主导的集中流入" 等

	// AI 生成
	Summary   string `json:"summary"`   // 当日总结
	Outlook   string `json:"outlook"`   // 明日展望

	// 事件时间线（来自 tick 分析）
	Timeline []analyzer.TimelineEvent `json:"timeline,omitempty"`

	// 新闻摘要
	NewsBriefs []NewsBrief `json:"newsBriefs,omitempty"`

	// 引用文案
	Copywriting string `json:"copywriting,omitempty"` // 当日 AI 口播文案
}
