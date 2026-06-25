package report

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ReportMetric 卡片指标格。
type ReportMetric struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Note  string `json:"note,omitempty"`
	Tone  string `json:"tone,omitempty"` // up / down / neutral
}

// ReportHighlight 卡片高亮条目（板块/个股）。
type ReportHighlight struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Tone   string `json:"tone,omitempty"`
}

// ReportBullet 卡片要点。
type ReportBullet struct {
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// ReportCard 知识卡片。
type ReportCard struct {
	Index      int               `json:"index"`
	Total      int               `json:"total"`
	Tag        string            `json:"tag"`
	Title      string            `json:"title"`
	Subtitle   string            `json:"subtitle"`
	Metrics    []ReportMetric    `json:"metrics,omitempty"`
	Highlights []ReportHighlight `json:"highlights,omitempty"`
	Bullets    []ReportBullet    `json:"bullets,omitempty"`
	Footer     string            `json:"footer,omitempty"`
}

func BuildCardsFromData(r *DailyReport) []ReportCard {
	total := 2
	if len(r.NewsBriefs) >= 2 || len(r.TopOutflows) >= 2 {
		total = 3
	}

	cards := []ReportCard{
		buildOverviewCard(r, 1, total),
	}

	if len(r.NewsBriefs) > 0 {
		cards = append(cards, buildNewsLinkageCard(r, 2, total))
	} else {
		cards = append(cards, buildOutflowCard(r, 2, total))
	}

	if total >= 3 {
		cards = append(cards, buildStructureCard(r, 3, total))
	}

	return cards
}

func buildOverviewCard(r *DailyReport, index, total int) ReportCard {
	// 1. 获取专业资金结构分析
	fundStructure := analyzeFundStructure(r)
	
	// 2. 获取行业资金分布分析
	industryDist := analyzeIndustryDistribution(r)
	
	// 3. 判断市场风格
	marketStyle := determineMarketStyle(r)

	// 生成专业标题
	title := generateOverviewTitle(r, fundStructure)
	
	subtitle := fmt.Sprintf("%s | %s | %s",
		formatMarketSummary(r),
		fundStructure.InstitutionalRatio,
		marketStyle.Characteristics)
	
	// 生成专业指标
	metrics := buildProfessionalMetrics(r, fundStructure)
	
	// 生成专业高亮信息
	highlights := buildProfessionalHighlights(r, industryDist)
	
	return ReportCard{
		Index:      index,
		Total:      total,
		Tag:        fmt.Sprintf("%s.%s", shortDateTag(r.Date), getLeadingSector(r)),
		Title:      title,
		Subtitle:   subtitle,
		Metrics:    metrics,
		Highlights: highlights,
		Footer:     "基于当日板块资金与新闻数据的专业市场分析",
	}
}

// determineMarketStyle 判断市场风格
func determineMarketStyle(r *DailyReport) MarketStyleAnalysis {
	analysis := MarketStyleAnalysis{}
	
	if r.NetTotal > 0 {
		if len(r.TopInflows) > 0 && r.TopInflows[0].Net > r.NetTotal*0.5 {
			analysis.Style = "机构主导型"
			analysis.Tone = "积极"
			analysis.Characteristics = "资金高度集中，市场风格由机构主导"
		} else {
			analysis.Style = "大众化型"
			analysis.Tone = "温和"
			analysis.Characteristics = "资金分布较为分散，市场风格较为平衡"
		}
	} else {
		analysis.Style = "防御型"
		analysis.Tone = "谨慎"
		analysis.Characteristics = "资金整体流出，市场情绪偏谨慎"
	}
	
	return analysis
}

// formatMarketSummary 格式化市场摘要
func formatMarketSummary(r *DailyReport) string {
	return fmt.Sprintf("%d涨%d跌，%s资金", r.InflowCount, r.OutflowCount, formatSignedNet(r.NetTotal))
}

// getLeadingSector 获取领涨板块
func getLeadingSector(r *DailyReport) string {
	if len(r.TopInflows) > 0 {
		return r.TopInflows[0].Name
	} else if len(r.TopOutflows) > 0 {
		return r.TopOutflows[0].Name
	}
	return "板块资金"
}

// generateOverviewTitle 生成概览卡片标题
func generateOverviewTitle(r *DailyReport, fundStruct FundStructureAnalysis) string {
	if fundStruct.FundType == "机构主导型" {
		if len(r.TopInflows) > 0 {
			return fmt.Sprintf("%s获机构重仓，资金集中度提升", r.TopInflows[0].Name)
		} else {
			return "机构主导的资金流入，市场风格积极"
		}
	} else if fundStruct.FundType == "大户主导型" {
		if len(r.TopOutflows) > 0 {
			return fmt.Sprintf("%s大户资金流出，市场承压", r.TopOutflows[0].Name)
		} else {
			return "大户主导的市场风格，注意风险"
		}
	} else {
		if r.NetTotal > 0 {
			return "市场风格温和，资金流入较为均衡"
		} else {
			return "市场风格谨慎，资金整体流出"
		}
	}
}

// buildProfessionalMetrics 构建专业指标
func buildProfessionalMetrics(r *DailyReport, fundStruct FundStructureAnalysis) []ReportMetric {
	metrics := make([]ReportMetric, 0, 4)
	
	// 1. 市场资金密度指标
	if r.NetTotal != 0 && len(r.TopInflows) > 0 {
		metrics = append(metrics, ReportMetric{
			Label: "市场资金密度",
			Value: formatRatio(absF(r.TopInflows[0].Net) / absF(r.NetTotal)),
			Note: "龙头板块占全市场资金比例",
			Tone: getToneFromValue(r.TopInflows[0].Net),
		})
	}
	
	// 2. 行业资金集中度
	if fundStruct.ConcentrationRatio != "" {
		metrics = append(metrics, ReportMetric{
			Label: "行业资金集中度",
			Value: fundStruct.ConcentrationRatio,
			Note: "资金最集中行业占全市场比例",
			Tone: "high",
		})
	}
	
	// 3. 市场风格判断
	metrics = append(metrics, ReportMetric{
		Label: "市场风格",
		Value: fundStruct.FundType,
		Note: "机构/散户主导程度",
		Tone: getMarketStyleTone(fundStruct.FundType),
	})
	
	// 4. 板块轮动速度
	rotationSpeed := calculateRotationSpeed(r)
	metrics = append(metrics, ReportMetric{
		Label: "板块轮动速度",
		Value: rotationSpeed,
		Note: "行业板块涨跌幅差异",
		Tone: getRotationTone(rotationSpeed),
	})
	
	return metrics
}

// buildProfessionalHighlights 构建专业高亮信息
func buildProfessionalHighlights(r *DailyReport, industryDist IndustryDistributionAnalysis) []ReportHighlight {
	highlights := make([]ReportHighlight, 0, 6)
	
	// 1. 资金主导板块
	if len(r.TopInflows) > 0 {
		inflowSector := r.TopInflows[0]
		highlights = append(highlights, ReportHighlight{
			Title:  inflowSector.Name,
			Detail: fmt.Sprintf("主力%s · %s", formatSignedNet(inflowSector.Net), formatSignedPct(inflowSector.ChangePct)),
			Tone:   getToneFromValue(inflowSector.Net),
		})
	}
	
	// 2. 资金流失龙头
	if len(r.TopOutflows) > 0 {
		outflowSector := r.TopOutflows[0]
		highlights = append(highlights, ReportHighlight{
			Title:  outflowSector.Name,
			Detail: fmt.Sprintf("主力%s · %s", formatAbsNet(outflowSector.Net), formatSignedPct(outflowSector.ChangePct)),
			Tone:   "down",
		})
	}
	
	// 3. 行业资金洼地
	for _, sector := range industryDist.LowDensitySectors {
		if summary, exists := findSectorSummary(r, sector); exists {
			highlights = append(highlights, ReportHighlight{
				Title:  sector,
				Detail: fmt.Sprintf("资金洼地 · %s", formatSignedNet(summary.Net)),
				Tone:   getToneFromValue(summary.Net),
			})
		}
	}
	
	return highlights
}

func buildNewsLinkageCard(r *DailyReport, index, total int) ReportCard {
	// 1. 新闻与资金的深度匹配分析
	newsFundMatch := analyzeNewsFundMatch(r)
	
	bullets := make([]ReportBullet, 0, 4)
	metrics := make([]ReportMetric, 0, 3)
	seenMetric := map[string]bool{}
	for i, n := range r.NewsBriefs {
		if i >= 4 {
			break
		}
		detail := n.Brief
		if detail == "" {
			detail = fmt.Sprintf("[%s] %s", n.Level, n.Time)
		}
		if len(n.Sectors) > 0 {
			sectorNotes := make([]string, 0, len(n.Sectors))
			for _, name := range n.Sectors {
				if s, ok := findSectorSummary(r, name); ok {
					sectorNotes = append(sectorNotes, fmt.Sprintf("%s %s", name, formatSignedNet(s.Net)))
					if !seenMetric[name] && len(metrics) < 3 {
						seenMetric[name] = true
						metrics = append(metrics, ReportMetric{
							Label: truncateLabel(name, 6),
							Value: formatSignedNet(s.Net),
							Note:  formatSignedPct(s.ChangePct),
							Tone:  toneFromValue(s.Net),
						})
					}
				}
			}
			if len(sectorNotes) > 0 {
				detail = strings.Join(sectorNotes, " · ")
			}
		}
		bullets = append(bullets, ReportBullet{
			Title:  truncateLabel(n.Title, 28),
			Detail: detail,
		})
	}

	// 生成专业新闻标题
	title := generateNewsTitle(r, newsFundMatch)
	
	return ReportCard{
		Index:    index,
		Total:    total,
		Tag:      fmt.Sprintf("%s 资讯联动", shortDateTag(r.Date)),
		Title:    title,
		Subtitle: fmt.Sprintf("当日收录 %d 条财联社快讯，以下为与板块资金相关的深度分析", len(r.NewsBriefs)),
		Metrics:  metrics,
		Bullets:  bullets,
		Footer:   "资讯与板块资金均来自当日原始数据",
	}
}

// generateNewsTitle 生成资讯联动卡片标题
func generateNewsTitle(r *DailyReport, newsFundMatch NewsFundMatchAnalysis) string {
	if len(r.NewsBriefs) > 0 && len(r.NewsBriefs[0].Sectors) > 0 {
		if newsFundMatch.MatchScore != "0/"+fmt.Sprintf("%d", len(r.NewsBriefs)) {
			return fmt.Sprintf("%s资讯密集，资金联动性强", newsFundMatch.MatchedInflowSectors[0])
		} else {
			return fmt.Sprintf("%s资讯密集，资金反应冷清", newsFundMatch.MatchedInflowSectors[0])
		}
	}
	return "新闻与资金联动，关注主线催化"
}

func buildOutflowCard(r *DailyReport, index, total int) ReportCard {
	// 1. 流出风险分析
	outflowRiskAnalysis := analyzeOutflowRisk(r)
	
	// 2. 龙头资金流失原因分析
	leaderFundLossAnalysis := analyzeLeaderFundLoss(r)
	
	// 3. 市场结构性变化识别
	marketStructureChange := identifyMarketStructureChange(r)
	
	metrics := make([]ReportMetric, 0, 3)
	for i, s := range r.TopOutflows {
		if i >= 3 {
			break
		}
		metrics = append(metrics, ReportMetric{
			Label: truncateLabel(s.Name, 6),
			Value: formatSignedNet(s.Net),
			Note:  formatSignedPct(s.ChangePct),
			Tone:  "down",
		})
	}

	// 生成专业标题
	title := generateOutflowTitle(r, outflowRiskAnalysis)
	
	return ReportCard{
		Index:    index,
		Total:    total,
		Tag:      fmt.Sprintf("%s 风险提示", shortDateTag(r.Date)),
		Title:    title,
		Subtitle: fmt.Sprintf("资金结构：%s | %s", r.StructureDesc, marketStructureChange.Insight),
		Metrics:  metrics,
		Bullets:  buildOutflowBullets(r, leaderFundLossAnalysis),
		Footer:   "流出数据基于当日板块统计，提示市场结构性变化风险",
	}
}

// analyzeOutflowRisk 分析流出风险
func analyzeOutflowRisk(r *DailyReport) OutflowRiskAnalysis {
	analysis := OutflowRiskAnalysis{}
	
	if len(r.TopOutflows) == 0 {
		return analysis
	}
	
	// 1. 风险程度判断
	riskLevel := "低"
	if len(r.TopOutflows) > 0 {
		if r.TopOutflows[0].Net < -100 { // 资金流出超过100亿
			riskLevel = "高"
		} else if r.TopOutflows[0].Net < -50 { // 资金流出超过50亿
			riskLevel = "中高"
		}
	}
	analysis.RiskLevel = riskLevel
	
	// 2. 风险原因分析
	if len(r.TopOutflows) > 0 {
		sector := r.TopOutflows[0]
		if sector.LeadStockName != "" {
			analysis.RiskReason = fmt.Sprintf("%s领涨股%s，行业资金大幅流出", sector.Name, formatSignedPct(sector.LeadStockChangePct))
		} else {
			analysis.RiskReason = fmt.Sprintf("%s主力资金大幅流出，行业承压", sector.Name)
		}
	}
	
	// 3. 风险建议
	if riskLevel == "高" {
		analysis.RiskAdvice = "市场风险较高，建议减仓，关注资金回流信号"
	} else if riskLevel == "中高" {
		analysis.RiskAdvice = "市场风险适中，建议控制仓位，等待市场企稳"
	} else {
		analysis.RiskAdvice = "市场风险较低，建议关注行业轮动机会"
	}
	
	return analysis
}

// analyzeLeaderFundLoss 分析龙头资金流失
func analyzeLeaderFundLoss(r *DailyReport) LeaderFundLossAnalysis {
	analysis := LeaderFundLossAnalysis{}
	
	if len(r.TopOutflows) == 0 {
		return analysis
	}
	
	// 1. 资金流失原因
	sector := r.TopOutflows[0]
	analysis.LeaderSector = sector.Name
	analysis.FundLossAmount = formatAbsNet(sector.Net)
	
	if sector.LeadStockName != "" {
		analysis.FundLossReason = fmt.Sprintf("领涨股%s下跌%s，带动行业资金流出", sector.LeadStockName, formatSignedPct(sector.LeadStockChangePct))
	} else {
		analysis.FundLossReason = "行业基本面转弱，主力资金规避风险"
	}
	
	// 2. 资金流失持续性
	if sector.Net < -50 { // 资金流出超过50亿
		analysis.Continuity = "可能持续"
		analysis.Duration = "短期"
	} else {
		analysis.Continuity = "可能结束"
		analysis.Duration = "短期"
	}
	
	return analysis
}

// identifyMarketStructureChange 识别市场结构性变化
func identifyMarketStructureChange(r *DailyReport) MarketStructureChangeAnalysis {
	analysis := MarketStructureChangeAnalysis{}
	
	// 1. 资金结构变化
	if r.StructureDesc != "" {
		analysis.StructureChange = r.StructureDesc
		analysis.Insight = "资金结构发生显著变化，建议关注市场风格转换"
	}
	
	// 2. 行业轮动变化
	if len(r.TopInflows) > 0 && len(r.TopOutflows) > 0 {
		inflowSector := r.TopInflows[0]
		outflowSector := r.TopOutflows[0]
		
		analysis.RotationChange = fmt.Sprintf("%s资金流入 %s资金流出", inflowSector.Name, outflowSector.Name)
		analysis.Insight = "行业轮动加速，市场结构性变化明显"
	}
	
	// 3. 市场风格变化
	if r.NetTotal < 0 && len(r.TopOutflows) > 0 {
		analysis.StyleChange = "防御型向谨慎型转换"
		analysis.Insight = "市场风格从防御型向谨慎型转换，建议关注低波动板块"
	}
	
	return analysis
}

// generateOutflowTitle 生成流出观察卡片标题
func generateOutflowTitle(r *DailyReport, outflowRiskAnalysis OutflowRiskAnalysis) string {
	if outflowRiskAnalysis.RiskLevel == "高" {
		return "市场风险较高，建议减仓"
	} else if outflowRiskAnalysis.RiskLevel == "中高" {
		return "市场风险适中，建议控制仓位"
	} else {
		return "市场风险较低，建议关注行业轮动"
	}
}

// buildOutflowBullets 构建流出观察卡片要点
func buildOutflowBullets(r *DailyReport, leaderFundLossAnalysis LeaderFundLossAnalysis) []ReportBullet {
	bullets := []ReportBullet{
		{
			Title:  "资金流失原因",
			Detail: leaderFundLossAnalysis.FundLossReason,
		},
	}
	
	if leaderFundLossAnalysis.Continuity != "" {
		bullets = append(bullets, ReportBullet{
			Title:  "资金流失持续性",
			Detail: fmt.Sprintf("资金流失可能%s，持续时间%s", leaderFundLossAnalysis.Continuity, leaderFundLossAnalysis.Duration),
		})
	}
	
	return bullets
}

func buildStructureCard(r *DailyReport, index, total int) ReportCard {
	// 1. 资金结构分析
	fundStruct := analyzeFundStructure(r)
	
	// 2. 市场结构变化识别
	marketStructChange := identifyMarketStructureChange(r)
	
	bullets := []ReportBullet{
		{
			Title:  "资金结构",
			Detail: fmt.Sprintf("超大单%s，大单%s，%s", formatSignedNet(r.SuperNetTotal), formatSignedNet(r.BigNetTotal), r.StructureDesc),
		},
	}

	if len(r.TopInflows) > 0 && len(r.TopOutflows) > 0 {
		bullets = append(bullets, ReportBullet{
			Title: "多空对比",
			Detail: fmt.Sprintf(
				"流入龙头 %s(%s) vs 流出龙头 %s(%s)",
				r.TopInflows[0].Name, formatSignedNet(r.TopInflows[0].Net),
				r.TopOutflows[0].Name, formatSignedNet(r.TopOutflows[0].Net),
			),
		})
	}

	// 生成专业后续观察
	trendAnalysis := generateTrendAnalysis(r, fundStruct, marketStructChange)
	bullets = append(bullets, ReportBullet{
		Title:  "后续观察",
		Detail: trendAnalysis,
	})

	return ReportCard{
		Index:    index,
		Total:    total,
		Tag:      fmt.Sprintf("%s 结构复盘", shortDateTag(r.Date)),
		Title:    "资金结构拆解，后续如何观察",
		Subtitle: fmt.Sprintf("全市场合计%s，需关注主线是否延续", formatSignedNet(r.NetTotal)),
		Bullets:  bullets,
		Footer:   "观察结论仅基于当日板块资金分布，不构成投资建议",
	}
}

// generateTrendAnalysis 生成趋势分析
func generateTrendAnalysis(r *DailyReport, fundStruct FundStructureAnalysis, marketStructChange MarketStructureChangeAnalysis) string {
	// 1. 资金结构趋势分析
	var trendAnalysis string
	
	if fundStruct.FundType == "机构主导型" {
		if len(r.TopInflows) > 0 {
			trendAnalysis = fmt.Sprintf("关注%s能否延续机构资金流入带动市场情绪回暖", r.TopInflows[0].Name)
		} else {
			trendAnalysis = "机构主导的市场风格可能持续，但注意波动性"
		}
	} else if fundStruct.FundType == "大户主导型" {
		if len(r.TopOutflows) > 0 {
			trendAnalysis = fmt.Sprintf("警惕%s大户资金流出，可能导致市场波动", r.TopOutflows[0].Name)
		} else {
			trendAnalysis = "大户主导的市场风格可能带来波动性，建议谨慎"
		}
	} else {
		if r.NetTotal > 0 {
			trendAnalysis = "市场风格温和，资金流入较为均衡，建议关注行业轮动"
		} else {
			trendAnalysis = "市场风格谨慎，资金整体流出，建议关注低波动板块"
		}
	}
	
	// 2. 市场结构变化趋势
	if marketStructChange.Insight != "" {
		trendAnalysis += " " + marketStructChange.Insight
	}
	
	return trendAnalysis
}

func findSectorSummary(r *DailyReport, name string) (SectorSummary, bool) {
	for _, s := range r.TopInflows {
		if s.Name == name {
			return s, true
		}
	}
	for _, s := range r.TopOutflows {
		if s.Name == name {
			return s, true
		}
	}
	return SectorSummary{}, false
}

func parseNewsSectors(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var sectors []string
	if err := json.Unmarshal([]byte(raw), &sectors); err == nil {
		return sectors
	}
	return nil
}

func shortDateTag(dateStr string) string {
	parts := strings.Split(dateStr, "-")
	if len(parts) == 3 {
		return fmt.Sprintf("%s.%s", parts[1], parts[2])
	}
	return dateStr
}

func truncateLabel(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

func formatSignedNet(v float64) string {
	if v >= 0 {
		return fmt.Sprintf("+%.0f亿", v)
	}
	return fmt.Sprintf("%.0f亿", v)
}

func formatAbsNet(v float64) string {
	if v < 0 {
		v = -v
	}
	return fmt.Sprintf("%.0f亿", v)
}

func formatSignedPct(v float64) string {
	if !isFinite(v) {
		return "—"
	}
	if v >= 0 {
		return fmt.Sprintf("+%.1f%%", v)
	}
	return fmt.Sprintf("%.1f%%", v)
}

func formatRatio(v float64) string {
	if !isFinite(v) {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", v*100)
}

func toneFromValue(v float64) string {
	if v > 0 {
		return "up"
	}
	if v < 0 {
		return "down"
	}
	return "neutral"
}

func isFinite(v float64) bool {
	return !(v != v || v > 1e12 || v < -1e12)
}

// FundStructureAnalysis 资金结构专业分析
func analyzeFundStructure(r *DailyReport) FundStructureAnalysis {
	analysis := FundStructureAnalysis{}
	
	// 1. 资金比例计算
	totalNet := r.NetTotal
	if totalNet == 0 {
		return analysis
	}
	
	// 超大单比例
	superRatio := r.SuperNetTotal / totalNet
	bigRatio := r.BigNetTotal / totalNet
	
	analysis.InstitutionalRatio = formatRatio(superRatio)
	analysis.BigPlayerRatio = formatRatio(bigRatio)
	
	// 2. 资金分布特征
	if superRatio > 0.6 {
		analysis.FundType = "机构主导型"
		analysis.RiskLevel = "高"
		analysis.Characteristics = "资金高度集中，市场波动性较大"
	} else if bigRatio > 0.6 {
		analysis.FundType = "大户主导型"
		analysis.RiskLevel = "中高"
		analysis.Characteristics = "资金集中度较高，市场易受大户影响"
	} else {
		analysis.FundType = "大众化型"
		analysis.RiskLevel = "较低"
		analysis.Characteristics = "资金分布较为分散，市场相对稳定"
	}
	
	// 3. 资金流向持续性判断
	if len(r.TopInflows) > 0 && r.TopInflows[0].Net > totalNet * 0.3 {
		analysis.Continuity = "强势持续"
		analysis.Leadership = r.TopInflows[0].Name
	}
	
	return analysis
}

// IndustryDistributionAnalysis 行业资金分布专业分析
func analyzeIndustryDistribution(r *DailyReport) IndustryDistributionAnalysis {
	analysis := IndustryDistributionAnalysis{}
	
	// 1. 行业资金集中度计算
	if len(r.TopInflows) > 0 {
		topSectorNet := r.TopInflows[0].Net
		analysis.ConcentrationRatio = formatRatio(topSectorNet / r.NetTotal)
		analysis.LeadingSector = r.TopInflows[0].Name
		analysis.LeadingSectorNet = formatSignedNet(topSectorNet)
	}
	
	// 2. 行业资金分布特征
	if len(r.TopInflows) > 0 && len(r.TopOutflows) > 0 {
		inflowSector := r.TopInflows[0]
		outflowSector := r.TopOutflows[0]
		
		analysis.RotationDirection = "行业轮动"
		analysis.RotationDetail = fmt.Sprintf("%s资金流入 %s资金流出", 
			inflowSector.Name, outflowSector.Name)
	}
	
	// 3. 行业资金密度分析
	analysis.HighDensitySectors = getHighDensitySectors(r)
	analysis.LowDensitySectors = getLowDensitySectors(r)
	
	return analysis
}

// NewsFundMatchAnalysis 新闻与资金的深度匹配分析
func analyzeNewsFundMatch(r *DailyReport) NewsFundMatchAnalysis {
	analysis := NewsFundMatchAnalysis{}
	
	for _, news := range r.NewsBriefs {
		if len(news.Sectors) > 0 {
			// 1. 新闻提及板块与资金流向的匹配
			for _, sectorName := range news.Sectors {
				if sectorSummary, exists := findSectorSummary(r, sectorName); exists {
					if sectorSummary.Net > 0 {
						analysis.MatchedInflowSectors = append(analysis.MatchedInflowSectors, sectorName)
					} else {
						analysis.MatchedOutflowSectors = append(analysis.MatchedOutflowSectors, sectorName)
					}
				}
			}
		}
	}
	
	// 2. 新闻热点与资金关注度的匹配度
	if len(analysis.MatchedInflowSectors) > 0 {
		analysis.MatchScore = fmt.Sprintf("%d/%d", len(analysis.MatchedInflowSectors), len(r.NewsBriefs))
		analysis.Insight = "新闻热点与资金流向高度一致"
	} else {
		analysis.MatchScore = "0/" + fmt.Sprintf("%d", len(r.NewsBriefs))
		analysis.Insight = "新闻与资金流向存在背离，建议关注原因"
	}
	
	return analysis
}
