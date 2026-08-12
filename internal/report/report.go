package report

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/ai"
	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

// Generate 生成指定日期的日报。
// 从 DB 聚合数据 → AI 生成总结和展望 → 组装为 DailyReport。
// AI 失败时自动降级为纯数据驱动的模板报告。
func Generate(dateStr string) (*DailyReport, error) {
	db, err := storage.Get()
	if err != nil {
		return nil, fmt.Errorf("get db: %w", err)
	}

	const session = "full"
	report := &DailyReport{
		Date:    dateStr,
		Session: session,
	}

	// 1. 加载板块数据
	sectors, err := db.LoadSectorsAll(dateStr)
	if err != nil || len(sectors) == 0 {
		// 降级：尝试从 sectors(tick) 表加载
		latest, loadErr := db.LoadFullSectors(dateStr)
		if loadErr != nil || len(latest) == 0 {
			return nil, fmt.Errorf("no sector data for %s", dateStr)
		}
		sectors = toSectorAll(latest)
	}

	// 2. 计算大盘概况
	computeMarketOverview(report, sectors)

	// 3. 加载事件时间线
	if events, err := db.LoadTickEvents(dateStr, session); err == nil && events != nil {
		var payload struct {
			Timeline []analyzer.TimelineEvent `json:"timeline"`
			Events   []analyzer.MarketEvent   `json:"events"`
			Ticker   []analyzer.TickerItem    `json:"ticker"`
		}
		if json.Unmarshal(events, &payload) == nil {
			report.Timeline = payload.Timeline
		}
	}

	// 4. 加载新闻
	if news, _, err := db.LoadNewsByDate(dateStr, 15, 0); err == nil {
		for _, n := range news {
			timeStr := extractTime(n.CTime)
			if timeStr == "" {
				timeStr = n.CTime
			}
			brief := strings.TrimSpace(n.Brief)
			if brief == "" {
				brief = strings.TrimSpace(n.Content)
			}
			if len([]rune(brief)) > 80 {
				brief = string([]rune(brief)[:80]) + "…"
			}
			report.NewsBriefs = append(report.NewsBriefs, NewsBrief{
				Title:   n.Title,
				Level:   n.Level,
				Time:    timeStr,
				Brief:   brief,
				Sectors: parseNewsSectors(n.Sectors),
			})
		}
	}

	// 5. 加载当日文案
	if cw, err := db.LoadCopywritingBySession(dateStr, session); err == nil {
		for _, typ := range []string{"ai", "ai_tick", "template", "template_tick"} {
			if c, ok := cw[typ]; ok {
				report.Copywriting = c
				break
			}
		}
	}

	// 6. 板块轮动分析（用于 AI prompt）
	sectorRotation := AnalyzeSectorRotation(toSectorSlice(sectors), 5)
	sentiment := AnalyzeSentiment(report)

	// 7. AI 生成总结和展望（失败不阻塞）
	aiCfg := config.GetAIConfigFor("report")
	if aiCfg.APIKey != "" {
		summary, outlook, cards, thematicCards, aiErr := AIGenerateThematic(dateStr, report, sectors, sectorRotation, sentiment, aiCfg)
		if aiErr == nil {
			report.Summary = summary
			report.Outlook = outlook
			if len(cards) > 0 {
				report.Cards = cards
			}
			if len(thematicCards) > 0 {
				report.ThematicCards = thematicCards
			}
		} else {
			logger.Warn("日报 AI 生成失败，使用数据驱动", zap.Error(aiErr))
		}
	}

	// 8. 数据驱动 fallback
	if report.Summary == "" {
		report.Summary = dataDrivenSummary(report)
	}
	if report.Outlook == "" {
		report.Outlook = dataDrivenOutlook(report, sectors)
	}
	if len(report.Cards) == 0 {
		report.Cards = BuildCardsFromData(report)
	}

	report.CreatedAt = time.Now().In(shanghaiTZ).Format("2006-01-02 15:04:05")

	// 9. 保存到 DB
	reportJSON, err := json.Marshal(report)
	if err != nil {
		logger.Warn("序列化日报失败", zap.Error(err))
	} else if err := db.SaveDailyReport(dateStr, session, report.Summary, report.Outlook, string(reportJSON)); err != nil {
		logger.Warn("保存日报失败", zap.Error(err))
	}

	return report, nil
}

// computeMarketOverview 计算大盘概况指标。
func computeMarketOverview(r *DailyReport, sectors []storage.SectorAll) {
	var totalSuper, totalBig float64
	for _, s := range sectors {
		r.NetTotal += s.Net
		totalSuper += s.SuperNet
		totalBig += s.BigNet
		if s.Net > 0 {
			r.InflowCount++
		} else {
			r.OutflowCount++
		}
	}
	r.SuperNetTotal = totalSuper
	r.BigNetTotal = totalBig

	// 资金结构描述
	if r.NetTotal > 0 {
		if totalSuper > 0 && totalSuper/r.NetTotal > 0.6 {
			r.StructureDesc = "机构主导的集中流入"
		} else if totalBig > 0 && totalBig/r.NetTotal > 0.6 {
			r.StructureDesc = "大户主导的分散流入"
		} else {
			r.StructureDesc = "机构和大户共同参与"
		}
	} else if r.NetTotal < 0 {
		absTotal := -r.NetTotal
		if totalSuper < 0 && -totalSuper/absTotal > 0.6 {
			r.StructureDesc = "机构主导的集中出逃"
		} else if totalBig < 0 && -totalBig/absTotal > 0.6 {
			r.StructureDesc = "大户主导的分散出逃"
		} else {
			r.StructureDesc = "机构和大户同步减仓"
		}
	} else {
		r.StructureDesc = "资金面相对均衡"
	}

	// TOP 排行
	var inflows, outflows []SectorSummary
	for _, s := range sectors {
		item := SectorSummary{
			Name:                 s.Name,
			Category:             s.Category,
			Net:                  s.Net,
			ChangePct:            s.ChangePct,
			SuperNet:             s.SuperNet,
			BigNet:               s.BigNet,
			SuperRate:            s.SuperRate,
			BigRate:              s.BigRate,
			TurnoverRate:         s.TurnoverRate,
			LeadStockName:        s.LeadStockName,
			LeadStockChangePct:   s.LeadStockChangePct,
			TotalMarketCap:       s.TotalMarketCap,
			CirculatingMarketCap: s.CirculatingMarketCap,
		}
		if s.Net > 0 {
			inflows = append(inflows, item)
		} else {
			outflows = append(outflows, item)
		}
	}
	sort.Slice(inflows, func(i, j int) bool { return inflows[i].Net > inflows[j].Net })
	sort.Slice(outflows, func(i, j int) bool { return outflows[i].Net < outflows[j].Net })

	n := 5
	if len(inflows) < n {
		n = len(inflows)
	}
	r.TopInflows = inflows[:n]

	n = 5
	if len(outflows) < n {
		n = len(outflows)
	}
	r.TopOutflows = outflows[:n]
}

func normalizeAICards(r *DailyReport, cards []ReportCard) []ReportCard {
	if len(cards) == 0 {
		return nil
	}
	if len(cards) > 3 {
		cards = cards[:3]
	}
	total := len(cards)
	for i := range cards {
		cards[i].Index = i + 1
		cards[i].Total = total
		if cards[i].Tag == "" {
			cards[i].Tag = fmt.Sprintf("%s 复盘", shortDateTag(r.Date))
		}
	}
	return cards
}

// dataDrivenSummary 数据驱动生成日报总结（AI 降级）。
func dataDrivenSummary(r *DailyReport) string {
	dir := "净流入"
	if r.NetTotal < 0 {
		dir = "净流出"
	}
	return fmt.Sprintf("今日收盘，全市场%d个板块%s，%d个板块净流出，合计%s%.0f亿。%s。流入前3为", r.InflowCount, dir, r.OutflowCount, dir, absF(r.NetTotal), r.StructureDesc) +
		func() string {
			var names []string
			for i, s := range r.TopInflows {
				if i >= 3 {
					break
				}
				names = append(names, fmt.Sprintf("%s(+%.0f亿)", s.Name, s.Net))
			}
			if len(names) == 0 {
				return "无明显流入板块"
			}
			return strings.Join(names, "、")
		}() +
		"。操作上建议关注资金持续流入方向。"
}

// dataDrivenOutlook 数据驱动生成明日展望（AI 降级）。
func dataDrivenOutlook(r *DailyReport, sectors []storage.SectorAll) string {
	if len(r.TopInflows) == 0 {
		return "等待明日开盘观察资金方向。"
	}
	top := r.TopInflows[0]
	dir := "延续流入"
	if len(r.TopOutflows) > 0 {
		topOut := r.TopOutflows[0]
		return fmt.Sprintf("关注%s能否延续流入，以及%s是否出现情绪修复机会。整体市场%s，建议控制仓位，等待明确信号。", top.Name, topOut.Name, r.StructureDesc)
	}
	return fmt.Sprintf("关注%s能否%s，带动市场情绪进一步回暖。", top.Name, dir)
}

// toSectorAll 将 Sector（tick 格式）转为 SectorAll。
func toSectorAll(sectors []storage.Sector) []storage.SectorAll {
	var result []storage.SectorAll
	for _, s := range sectors {
		result = append(result, storage.SectorAll{
			Name:                 s.Name,
			Net:                  s.Net,
			ChangePct:            s.ChangePct,
			SuperNet:             s.SuperNet,
			SuperRate:            s.SuperRate,
			BigNet:               s.BigNet,
			BigRate:              s.BigRate,
			Volume:               s.Volume,
			Turnover:             s.Turnover,
			TurnoverRate:         s.TurnoverRate,
			LeadStockName:        s.LeadStockName,
			LeadStockChangePct:   s.LeadStockChangePct,
			TotalMarketCap:       s.TotalMarketCap,
			CirculatingMarketCap: s.CirculatingMarketCap,
		})
	}
	return result
}

func parseDateDisplay(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return fmt.Sprintf("%d月%d日", t.Month(), t.Day())
}

func extractTime(datetime string) string {
	if idx := strings.Index(datetime, " "); idx > 0 && idx+6 < len(datetime) {
		return datetime[idx+1 : idx+6]
	}
	return ""
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

var shanghaiTZ = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}()

// MarketStyleAnalysis 市场风格分析结构体
func getToneFromValue(v float64) string {
	if v > 0 {
		return "up"
	}
	if v < 0 {
		return "down"
	}
	return "neutral"
}

// MarketStyleAnalysis 市场风格分析结构体
func getMarketStyleTone(fundType string) string {
	if fundType == "机构主导型" {
		return "high"
	} else if fundType == "大户主导型" {
		return "medium"
	} else {
		return "low"
	}
}

// MarketStyleAnalysis 市场风格分析结构体
func getRotationTone(rotationSpeed string) string {
	if rotationSpeed == "快" {
		return "high"
	} else if rotationSpeed == "中" {
		return "medium"
	} else {
		return "low"
	}
}

// MarketStyleAnalysis 市场风格分析结构体
func calculateRotationSpeed(r *DailyReport) string {
	if len(r.TopInflows) == 0 || len(r.TopOutflows) == 0 {
		return "慢"
	}

	inflowSector := r.TopInflows[0]
	outflowSector := r.TopOutflows[0]

	// 计算涨跌幅差异
	inflowChange := inflowSector.ChangePct
	outflowChange := outflowSector.ChangePct

	if inflowChange > 5 && outflowChange < -5 {
		return "快"
	} else if inflowChange > 2 && outflowChange < -2 {
		return "中"
	} else {
		return "慢"
	}
}

// MarketStyleAnalysis 市场风格分析结构体
func getHighDensitySectors(r *DailyReport) []string {
	var highDensity []string

	if len(r.TopInflows) > 0 {
		for i, s := range r.TopInflows {
			if i < 3 && s.Net > 10 { // 资金超过10亿
				highDensity = append(highDensity, s.Name)
			}
		}
	}

	return highDensity
}

// MarketStyleAnalysis 市场风格分析结构体
func getLowDensitySectors(r *DailyReport) []string {
	var lowDensity []string

	if len(r.TopOutflows) > 0 {
		for i, s := range r.TopOutflows {
			if i < 3 && s.Net < -5 { // 资金流出超过5亿
				lowDensity = append(lowDensity, s.Name)
			}
		}
	}

	return lowDensity
}

// MarketStyleAnalysis 市场风格分析结构体
func getProfessionalRecommendations(r *DailyReport, fundStruct FundStructureAnalysis, industryDist IndustryDistributionAnalysis) string {
	var recommendations []string

	// 1. 资金结构建议
	if fundStruct.FundType == "机构主导型" {
		recommendations = append(recommendations, "关注机构持股板块，控制仓位")
	} else if fundStruct.FundType == "大户主导型" {
		recommendations = append(recommendations, "警惕大户操纵，控制风险")
	} else {
		recommendations = append(recommendations, "市场风格温和，关注行业轮动")
	}

	// 2. 行业龙头建议
	if len(r.TopInflows) > 0 {
		recommendations = append(recommendations, fmt.Sprintf("关注龙头%s的资金持续性", r.TopInflows[0].Name))
	}

	// 3. 市场情绪建议
	if fundStruct.RiskLevel == "高" {
		recommendations = append(recommendations, "市场风险较高，建议减仓")
	} else if fundStruct.RiskLevel == "中高" {
		recommendations = append(recommendations, "市场风险适中，建议控制仓位")
	} else {
		recommendations = append(recommendations, "市场风险较低，可适度参与")
	}

	return strings.Join(recommendations, "；")
}

// AIGenerateThematic 调用 LLM 生成专题卡片版日报。
// 使用 PromptDailyReportThematic（CoT + Few-shot）输出 ThematicCard[]。
func AIGenerateThematic(dateStr string, r *DailyReport, sectors []storage.SectorAll, rotation SectorRotationResult, sentiment SentimentIndicators, aiCfg config.AIConfig) (string, string, []ReportCard, []ThematicCard, error) {
	dateDisplay := parseDateDisplay(dateStr)
	netTotalStr := fmt.Sprintf("%+.0f亿", r.NetTotal)

	var inflowLines, outflowLines []string
	for _, s := range r.TopInflows {
		inflowLines = append(inflowLines, fmt.Sprintf("- %s: +%.0f亿(涨%.1f%%, 超大单%+.0f/大单%+.0f)", s.Name, s.Net, s.ChangePct, s.SuperNet, s.BigNet))
	}
	for _, s := range r.TopOutflows {
		outflowLines = append(outflowLines, fmt.Sprintf("- %s: %.0f亿(跌%.1f%%, 超大单%.0f/大单%.0f)", s.Name, s.Net, s.ChangePct, s.SuperNet, s.BigNet))
	}

	var structureDesc string
	if r.NetTotal != 0 {
		if r.SuperNetTotal > 0 {
			structureDesc = fmt.Sprintf("超大单合计净流入%.0f亿，大单合计净流入%.0f亿。%s。", r.SuperNetTotal, r.BigNetTotal, r.StructureDesc)
		} else {
			structureDesc = fmt.Sprintf("超大单合计净流出%.0f亿，大单合计净流出%.0f亿。%s。", -r.SuperNetTotal, -r.BigNetTotal, r.StructureDesc)
		}
	}

	var timelineLines []string
	for _, ev := range r.Timeline {
		timelineLines = append(timelineLines, fmt.Sprintf("- %s: %s(%s)", ev.Time, ev.Title, ev.Description))
	}
	if len(timelineLines) == 0 {
		timelineLines = append(timelineLines, "- 无明显异动事件")
	}

	var newsLines []string
	for _, n := range r.NewsBriefs {
		line := fmt.Sprintf("- [%s %s] %s", n.Level, n.Time, n.Title)
		if n.Brief != "" {
			line += fmt.Sprintf(" | %s", n.Brief)
		}
		if len(n.Sectors) > 0 {
			line += fmt.Sprintf(" | 关联板块: %s", strings.Join(n.Sectors, "、"))
		}
		newsLines = append(newsLines, line)
	}
	if len(newsLines) == 0 {
		newsLines = append(newsLines, "- 无今日新闻")
	}

	rotationSummary := BuildSectorRotationSummary(rotation)
	sentimentSummary := fmt.Sprintf("赚钱效应:%s, 涨停约%d家, 情绪评分%.1f, 炸板率约%.0f%%",
		sentiment.ProfitEffect, sentiment.LimitUpCount, sentiment.SentimentScore, sentiment.BreakRate)

	fundStructure := analyzeFundStructure(r)
	industryDist := analyzeIndustryDistribution(r)

	prompt := fmt.Sprintf(PromptDailyReportThematic,
		dateDisplay,
		netTotalStr,
		fmt.Sprintf("%d", r.InflowCount),
		fmt.Sprintf("%d", r.OutflowCount),
		strings.Join(inflowLines, "\n"),
		strings.Join(outflowLines, "\n"),
		structureDesc,
		strings.Join(timelineLines, "\n"),
		strings.Join(newsLines, "\n"),
		rotationSummary,
		sentimentSummary,
		getProfessionalRecommendations(r, fundStructure, industryDist),
	)

	var aiResult struct {
		Reasoning     string         `json:"reasoning"`
		Summary       string         `json:"summary"`
		Outlook       string         `json:"outlook"`
		ThematicCards []ThematicCard `json:"thematicCards"`
	}
	if err := ai.ChatCompletionJSON(context.Background(), aiCfg, prompt, 0.5, 4096, &aiResult); err != nil {
		return "", "", nil, nil, fmt.Errorf("AI 日报生成失败: %w", err)
	}

	logger.Info("日报 AI 生成成功",
		zap.String("date", dateStr),
		zap.Int("thematicCards", len(aiResult.ThematicCards)),
		zap.Float64("conviction", avgConviction(aiResult.ThematicCards)),
	)

	var allCards []ReportCard
	for _, tc := range aiResult.ThematicCards {
		allCards = append(allCards, tc.Cards...)
	}
	allCards = normalizeAICards(r, allCards)

	return aiResult.Summary, aiResult.Outlook, allCards, aiResult.ThematicCards, nil
}

// avgConviction 计算专题卡片平均确信度。
func avgConviction(cards []ThematicCard) float64 {
	if len(cards) == 0 {
		return 0
	}
	var sum float64
	for _, c := range cards {
		sum += c.Conviction
	}
	return sum / float64(len(cards))
}

// toSectorSlice 将 SectorAll 转成 Sector 切片（用于 sector_cycle 分析）。
func toSectorSlice(sectors []storage.SectorAll) []fetcher.Sector {
	var result []fetcher.Sector
	for _, s := range sectors {
		result = append(result, fetcher.Sector{
			Name: s.Name,
			Net:  s.Net,
		})
	}
	return result
}
