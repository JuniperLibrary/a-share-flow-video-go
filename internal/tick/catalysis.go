package tick

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/hotnews"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

const (
	confidenceHigh   = "high"
	confidenceMedium = "medium"
	confidenceLow    = "low"
)

// CatalysisInsight 对应 React/Remotion 端每条新闻的"证据洞察"。
// Text 是 UI 主显示 + TTS 朗读的一段；Source 是原始新闻标题（UI 底栏来源）。
type CatalysisInsight struct {
	Text      string  `json:"text"`
	Source    string  `json:"source,omitempty"`
	Label     string  `json:"label,omitempty"`
	TimeMatch int     `json:"timeMatch,omitempty"`
	FlowDelta float64 `json:"flowDelta,omitempty"`
	Score     float64 `json:"score,omitempty"`
}

// CatalysisSectorAnalysis 是 React 端渲染的板块级催化分析结构，旧字段 Insights / Analysis 留位兼容。
type CatalysisSectorAnalysis struct {
	Sector       string             `json:"sector"`
	Analysis     string             `json:"analysis"`
	Insights     []string           `json:"insights"`
	NewInsights  []CatalysisInsight `json:"newInsights,omitempty"`
	Confidence   string             `json:"confidence,omitempty"`
	OverallScore float64            `json:"overallScore,omitempty"`
	Flow         float64            `json:"flow,omitempty"`
	OralBlock    string             `json:"oralBlock,omitempty"`
}

type CatalysisResult struct {
	Sectors []CatalysisSectorAnalysis `json:"sectors"`
}

func newsMinutes(ctime string) (int, bool) {
	// 形如 2026-08-07 14:25:00 或 14:25
	if ctime == "" {
		return 0, false
	}
	hm := ctime
	if len(ctime) >= 16 {
		hm = ctime[11:16]
	} else if len(ctime) < 5 {
		return 0, false
	}
	var h, m int
	if _, err := fmt.Sscanf(hm, "%d:%d", &h, &m); err != nil {
		return 0, false
	}
	return h*60 + m, true
}

func sectorIndexAtTime(times []string, targetMin int) int {
	// times 形如 09:35, 按 tick 序；返回最接近 targetMin 的下标，0 表示第一个 timePoint
	if len(times) == 0 {
		return -1
	}
	best := 0
	bestDiff := math.MaxInt
	for i, t := range times {
		m, ok := newsMinutes(t)
		if !ok {
			continue
		}
		diff := int(math.Abs(float64(m - targetMin)))
		if diff < bestDiff {
			bestDiff = diff
			best = i
		}
	}
	return best
}

func scoreTimeDiff(mins int) int {
	if mins <= 15 {
		return 3
	}
	if mins <= 30 {
		return 2
	}
	if mins <= 60 {
		return 1
	}
	return 0
}

func timeMatchLabel(t int) string {
	switch t {
	case 3:
		return "时点确认"
	case 2:
		return "时点强相关"
	case 1:
		return "时点弱相关"
	default:
		return "情绪映射"
	}
}

// BuildCatalysisScores 纯规则打分引擎：时间窗 + 资金斜率 + 热度 + 板块序位。
// 返回按板块聚合的证据集合，后续 LLM 只负责把证据写成人话。
func BuildCatalysisScores(sectorTicks []SectorTick, tickTimes []string, newsPages []hotnews.NewsPage) []hotnews.CatalysisSectorEvidence {
	flowMap := make(map[string]float64, len(sectorTicks))
	sectorIndex := make(map[string]int, len(sectorTicks))
	absRank := make([]string, 0, len(sectorTicks))
	for i, st := range sectorTicks {
		cum := 0.0
		for _, v := range st.Data {
			cum += v
		}
		flowMap[st.Name] = cum
		sectorIndex[st.Name] = i
		absRank = append(absRank, st.Name)
	}
	sort.SliceStable(absRank, func(i, j int) bool {
		return math.Abs(flowMap[absRank[i]]) > math.Abs(flowMap[absRank[j]])
	})
	absFlowRank := make(map[string]int, len(absRank))
	for i, name := range absRank {
		absFlowRank[name] = i
	}

	top21Rank := make(map[string]int, len(fetcher.Top21HotSectors))
	for i, s := range fetcher.Top21HotSectors {
		top21Rank[s] = i
	}

	// 合并新闻：同板块跨页聚合
	pageMap := make(map[string][]hotnews.NewsItem)
	var maxReading int64 = 1
	for _, p := range newsPages {
		for _, sn := range p.Sectors {
			for _, it := range sn.News {
				pageMap[sn.Sector] = append(pageMap[sn.Sector], it)
				if it.ReadingNum > maxReading {
					maxReading = it.ReadingNum
				}
			}
		}
	}

	result := make([]hotnews.CatalysisSectorEvidence, 0, len(pageMap))
	sectorOrder := make([]string, 0, len(pageMap))
	for name := range pageMap {
		sectorOrder = append(sectorOrder, name)
	}
	sort.SliceStable(sectorOrder, func(i, j int) bool {
		return absFlowRank[sectorOrder[i]] < absFlowRank[sectorOrder[j]]
	})

	for _, sector := range sectorOrder {
		items := pageMap[sector]
		stIdx, ok := sectorIndex[sector]
		if !ok {
			continue
		}
		st := sectorTicks[stIdx]

		flow := flowMap[sector]
		newsScores := make([]hotnews.CatalysisNewsScore, 0, len(items))
		for _, it := range items {
			ns := hotnews.CatalysisNewsScore{
				Title:           it.Title,
				Brief:           it.Brief,
				Level:           it.Level,
				Time:            it.Time,
				SectorRankScore: 0,
			}
			if r, ok := top21Rank[sector]; ok {
				ns.SectorRankScore = float64(len(fetcher.Top21HotSectors)-r) / float64(len(fetcher.Top21HotSectors)) * 10
			}
			switch it.Level {
			case "A":
				ns.HeatScore += 4
			case "B":
				ns.HeatScore += 2.5
			default:
				ns.HeatScore += 1
			}
			if maxReading > 1 && it.ReadingNum > 0 {
				ns.HeatScore = math.Min(10, ns.HeatScore+float64(it.ReadingNum)/float64(maxReading)*6)
			}

			if nm, ok := newsMinutes(it.Time); ok {
				pubIdx := sectorIndexAtTime(tickTimes, nm)
				if pubIdx >= 0 {
					totalLen := len(st.Data)
					endIdx := pubIdx
					// 看 30 分钟约等于 6 个 tick（5 分钟一格）
					window := 6
					lookStart := pubIdx
					lookEnd := pubIdx + window
					if lookEnd > totalLen {
						lookEnd = totalLen
					}
					pubCum := 0.0
					postCum := 0.0
					preCum := 0.0
					preStart := pubIdx - window
					if preStart < 0 {
						preStart = 0
					}
					for k := 0; k < lookStart; k++ {
						if k < len(st.Data) {
							pubCum += st.Data[k]
						}
					}
					for k := preStart; k < lookStart; k++ {
						if k >= 0 && k < len(st.Data) {
							preCum += st.Data[k]
						}
					}
					for k := lookStart; k < lookEnd; k++ {
						if k < len(st.Data) {
							postCum += st.Data[k]
						}
					}
					ns.FlowDelta = postCum
					preMinutes := float64(lookStart - preStart)
					postMinutes := float64(lookEnd - lookStart)
					if preMinutes <= 0 {
						preMinutes = 1
					}
					if postMinutes <= 0 {
						postMinutes = 1
					}
					// 斜率：亿/小时；5 分钟一格 = 5/60 小时
					slopePerMin := 5.0 / 60.0
					pre := preCum / (preMinutes * slopePerMin)
					post := postCum / (postMinutes * slopePerMin)
					ns.FlowSlope = post - pre

					// 计算新闻发布时间距离最近交易时的分钟差
					tradeMin := 9*60 + 30
					if endIdx < len(tickTimes) {
						if m, ok := newsMinutes(tickTimes[endIdx]); ok {
							tradeMin = m
						}
					}
					diff := int(math.Abs(float64(tradeMin - nm)))
					ns.TimeMatch = scoreTimeDiff(diff)
				}
			}

			// 总分 100
			timeWeight := []float64{10, 22, 32, 40}[ns.TimeMatch]
			// 斜率分：|post-pre| / 3 亿，封顶 20
			slopeScore := math.Min(20, math.Abs(ns.FlowSlope)/3.0*20)
			// flowDelta 分：绝对流量 / 5 亿，封顶 15
			flowScore := math.Min(15, math.Abs(ns.FlowDelta)/5.0*15)
			heatScore := ns.HeatScore * 1.5 // 0-10 -> 0-15
			sectorScore := ns.SectorRankScore * 1.0
			ns.TotalScore = math.Round((timeWeight+slopeScore+flowScore+heatScore+sectorScore)*10) / 10
			if ns.TotalScore > 100 {
				ns.TotalScore = 100
			}

			newsScores = append(newsScores, ns)
		}

		sort.SliceStable(newsScores, func(i, j int) bool {
			return newsScores[i].TotalScore > newsScores[j].TotalScore
		})

		overall := 0.0
		if len(newsScores) > 0 {
			topN := newsScores
			if len(topN) > 2 {
				topN = topN[:2]
			}
			sum := 0.0
			for _, ns := range topN {
				sum += ns.TotalScore
			}
			overall = sum / float64(len(topN))
		}
		// 板块流量本身也贡献 0-10 分的底色
		absFlowNorm := math.Min(10, math.Abs(flow)/8.0*10)
		overall = math.Round((overall*0.85+absFlowNorm*1.5)*10) / 10
		if overall > 100 {
			overall = 100
		}

		confidence := confidenceLow
		if overall >= 70 {
			confidence = confidenceHigh
		} else if overall >= 40 {
			confidence = confidenceMedium
		}

		fr := 99
		if r, ok := absFlowRank[sector]; ok {
			fr = r
		}
		result = append(result, hotnews.CatalysisSectorEvidence{
			Sector:       sector,
			Flow:         flow,
			FlowRank:     fr,
			Confidence:   confidence,
			OverallScore: overall,
			NewsScores:   newsScores,
			OralScript:   buildOralScriptFromEvidence(sector, flow, newsScores, confidence),
		})
	}

	return result
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

func buildOralScriptFromEvidence(sector string, flow float64, newsScores []hotnews.CatalysisNewsScore, confidence string) []string {
	// 生成与 insights 一一对应的口语段，未来 GenerateTTSText 直接拿来念，保证音画对齐。
	top := newsScores
	if len(top) > 2 {
		top = top[:2]
	}
	out := make([]string, 0, len(top))
	for i, ns := range top {
		action := "表现平淡"
		if flow >= 1.0 {
			action = "获得资金承接"
		} else if flow <= -1.0 {
			action = "承受抛压"
		}
		var phrase string
		switch confidence {
		case confidenceHigh:
			phrase = "这条新闻基本坐实了催化方向"
		case confidenceMedium:
			phrase = "这条新闻与资金行为方向一致"
		default:
			phrase = "这类新闻对市场情绪有映射"
		}
		shortTitle := ns.Title
		if runeLen(shortTitle) > 22 {
			runes := []rune(shortTitle)
			shortTitle = string(runes[:22]) + "…"
		}
		line := fmt.Sprintf("催化%d：%s，%s，%s今日%s。", i+1, shortTitle, phrase, sector, action)
		out = append(out, line)
	}
	return out
}

// buildEvidenceSummary 把打分证据喂给 LLM，要求它只做"润色成专业文案"。
func buildEvidenceSummary(sectorTicks []SectorTick, ev []hotnews.CatalysisSectorEvidence) string {
	var sb strings.Builder
	sb.WriteString("【板块资金流向】\n")
	for i, st := range sectorTicks {
		if i >= 12 {
			break
		}
		cum := 0.0
		for _, v := range st.Data {
			cum += v
		}
		sb.WriteString(fmt.Sprintf("- %s: 主力净流向%+.1f亿  超大单%+.1f亿  大单%+.1f亿  涨跌幅%+.2f%%\n",
			st.Name, cum, st.SuperNet, st.BigNet, st.ChangePct))
	}
	sb.WriteString("\n【规则打分证据（必须据此写文案，禁止自由脑补）】\n")
	for _, e := range ev {
		sb.WriteString(fmt.Sprintf("- 板块 %s：累计净流向%+.1f亿，证据总评分 %.0f，置信度 %s\n",
			e.Sector, e.Flow, e.OverallScore, e.Confidence))
		for j, ns := range e.NewsScores {
			if j >= 2 {
				break
			}
			lbl := timeMatchLabel(ns.TimeMatch)
			sb.WriteString(fmt.Sprintf("  · 新闻%d[%s｜score=%.0f｜发布后Δ流%+.1f亿｜斜率%+.1f亿/h｜热度%.1f｜序位分%.1f]: %s\n",
				j+1, lbl, ns.TotalScore, ns.FlowDelta, ns.FlowSlope, ns.HeatScore, ns.SectorRankScore, ns.Title))
		}
	}
	sb.WriteString("\n【严格约束】\n")
	sb.WriteString("- 所有 analysis/insights 只能引用上面的板块名与数值；不得凭空新增板块或金额\n")
	sb.WriteString("- evidence 中 confidence=low 的板块：不得使用\"催化\"\"坐实\"\"确定\"等强因果词；改用\"情绪映射\"\"弱相关\"等措辞\n")
	sb.WriteString("- confidence=high：允许使用\"时点确认\"\"资金方向一致\"等表述\n")
	sb.WriteString("- 每条 insights 严格 15-25 字，analysis 严格 20-40 字\n")
	return sb.String()
}

// ValidateCatalysis 对 LLM 返回的催化分析做硬性校验：白名单、条数、字数、幻觉。
// 返回 (清洗后的结果, 被打回的问题数)。问题数 >0 时，调用方决定是否走 fallback。
func ValidateCatalysis(raw *CatalysisResult, allowedSectors map[string]bool, newsPages []hotnews.NewsPage, ev []hotnews.CatalysisSectorEvidence) (*CatalysisResult, int) {
	if raw == nil {
		return nil, 1
	}
	newsTitleSet := make(map[string]bool)
	sectorNewsTitles := make(map[string]map[string]bool)
	for _, p := range newsPages {
		for _, sn := range p.Sectors {
			set, ok := sectorNewsTitles[sn.Sector]
			if !ok {
				set = make(map[string]bool)
				sectorNewsTitles[sn.Sector] = set
			}
			for _, n := range sn.News {
				newsTitleSet[n.Title] = true
				set[n.Title] = true
				if len(n.Brief) > 0 {
					newsTitleSet[n.Brief] = true
				}
			}
		}
	}
	evMap := make(map[string]*hotnews.CatalysisSectorEvidence, len(ev))
	for i := range ev {
		evMap[ev[i].Sector] = &ev[i]
	}
	blackWords := []string{"综上所述", "值得注意的是", "我们认为", "整体来看", "不可忽视", "一言以蔽之", "不难发现", "从这个角度"}

	problems := 0
	cleaned := make([]CatalysisSectorAnalysis, 0, len(raw.Sectors))
	for _, s := range raw.Sectors {
		if s.Sector == "" {
			problems++
			continue
		}
		if allowedSectors != nil && !allowedSectors[s.Sector] {
			// 幻觉新增板块：整条打回
			problems++
			continue
		}
		newsTitles, hasTitles := sectorNewsTitles[s.Sector]
		maxNews := 0
		if hasTitles {
			maxNews = len(newsTitles)
		}
		if e, ok := evMap[s.Sector]; ok {
			if len(e.NewsScores) > maxNews {
				maxNews = len(e.NewsScores)
			}
		}
		if maxNews == 0 {
			maxNews = 2
		}
		if maxNews > 3 {
			maxNews = 3
		}
		// analysis 字数：20-40 中文
		ar := runeLen(s.Analysis)
		if ar < 12 {
			problems++
			continue
		}
		if ar > 60 {
			runes := []rune(s.Analysis)
			s.Analysis = string(runes[:60])
			problems++
		}
		for _, w := range blackWords {
			if strings.Contains(s.Analysis, w) {
				problems++
				break
			}
		}
		// insights：最多 maxNews 条，每条 10-30 字，不引入原新闻里不存在的板块
		trimmed := s.Insights
		if len(trimmed) > maxNews {
			trimmed = trimmed[:maxNews]
			problems++
		}
		validInsights := make([]string, 0, len(trimmed))
		newInsights := make([]CatalysisInsight, 0, len(trimmed))
		for i, txt := range trimmed {
			txt = strings.TrimSpace(txt)
			if txt == "" {
				continue
			}
			l := runeLen(txt)
			if l < 8 || l > 40 {
				problems++
				// 不整条打回，保留截断版
				if l > 40 {
					runes := []rune(txt)
					txt = string(runes[:40])
				}
			}
			for _, w := range blackWords {
				if strings.Contains(txt, w) {
					problems++
					break
				}
			}
			validInsights = append(validInsights, txt)
			// 回填 NewInsight，同时对齐来源标题
			ni := CatalysisInsight{
				Text:  txt,
				Label: "催化",
			}
			if e, ok := evMap[s.Sector]; ok && i < len(e.NewsScores) {
				ns := e.NewsScores[i]
				ni.TimeMatch = ns.TimeMatch
				ni.FlowDelta = ns.FlowDelta
				ni.Score = ns.TotalScore
				ni.Label = timeMatchLabel(ns.TimeMatch)
				ni.Source = ns.Title
			} else if hasTitles {
				// 回退到同下标的 news 标题
				idx := 0
				for title := range newsTitles {
					if idx == i {
						ni.Source = title
						break
					}
					idx++
				}
			}
			newInsights = append(newInsights, ni)
		}
		analysisFinal := s.Analysis
		// 构造板块级别的置信度与整体分
		confidence := confidenceLow
		overall := 0.0
		flow := 0.0
		oralBlock := ""
		if e, ok := evMap[s.Sector]; ok {
			confidence = e.Confidence
			overall = e.OverallScore
			flow = e.Flow
			oralBlock = strings.Join(e.OralScript, "")
		}
		// low 置信度强措辞清洗
		if confidence == confidenceLow {
			for _, w := range []string{"催化", "坐实", "确定", "证实", "直接引发", "明确引导"} {
				if strings.Contains(analysisFinal, w) {
					analysisFinal = strings.ReplaceAll(analysisFinal, w, "情绪映射")
					problems++
				}
			}
			for i := range validInsights {
				for _, w := range []string{"催化", "坐实", "确定", "证实", "直接引发"} {
					if strings.Contains(validInsights[i], w) {
						validInsights[i] = strings.ReplaceAll(validInsights[i], w, "情绪映射")
						problems++
					}
				}
			}
		}

		cleaned = append(cleaned, CatalysisSectorAnalysis{
			Sector:       s.Sector,
			Analysis:     analysisFinal,
			Insights:     validInsights,
			NewInsights:  newInsights,
			Confidence:   confidence,
			OverallScore: overall,
			Flow:         flow,
			OralBlock:    oralBlock,
		})
	}
	return &CatalysisResult{Sectors: cleaned}, problems
}

// buildFallbackFromEvidence 当 LLM 失败或校验打回时，直接用打分证据生成结构化文案，比纯标题 fallback 强。
func buildFallbackFromEvidence(ev []hotnews.CatalysisSectorEvidence) *CatalysisResult {
	if len(ev) == 0 {
		return nil
	}
	out := make([]CatalysisSectorAnalysis, 0, len(ev))
	for _, e := range ev {
		top := e.NewsScores
		if len(top) > 2 {
			top = top[:2]
		}
		var insights []string
		var newInsights []CatalysisInsight
		var oral []string
		for i, ns := range top {
			lbl := timeMatchLabel(ns.TimeMatch)
			insTxt := ""
			switch e.Confidence {
			case confidenceHigh:
				insTxt = fmt.Sprintf("%s：%s，发布后资金%+.1f亿方向确认。", lbl, truncateTo(ns.Title, 14), ns.FlowDelta)
			case confidenceMedium:
				insTxt = fmt.Sprintf("%s：%s，与资金方向基本一致。", lbl, truncateTo(ns.Title, 16))
			default:
				insTxt = fmt.Sprintf("%s：%s，提供情绪参考。", lbl, truncateTo(ns.Title, 16))
			}
			insights = append(insights, insTxt)
			newInsights = append(newInsights, CatalysisInsight{
				Text:      insTxt,
				Source:    ns.Title,
				Label:     lbl,
				TimeMatch: ns.TimeMatch,
				FlowDelta: ns.FlowDelta,
				Score:     ns.TotalScore,
			})
			if i < len(e.OralScript) {
				oral = append(oral, e.OralScript[i])
			}
		}
		an := ""
		switch e.Confidence {
		case confidenceHigh:
			an = fmt.Sprintf("%s今日净流向%+.1f亿，核心新闻与资金行为在时点上高度吻合。", e.Sector, e.Flow)
		case confidenceMedium:
			an = fmt.Sprintf("%s净流向%+.1f亿，相关新闻与资金方向基本同向。", e.Sector, e.Flow)
		default:
			an = fmt.Sprintf("%s净流向%+.1f亿，新闻更多提供情绪映射，不作强因果。", e.Sector, e.Flow)
		}
		out = append(out, CatalysisSectorAnalysis{
			Sector:       e.Sector,
			Analysis:     an,
			Insights:     insights,
			NewInsights:  newInsights,
			Confidence:   e.Confidence,
			OverallScore: e.OverallScore,
			Flow:         e.Flow,
			OralBlock:    strings.Join(oral, ""),
		})
	}
	return &CatalysisResult{Sectors: out}
}

func truncateTo(s string, n int) string {
	if n <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// GenerateCatalysis B 方案落地：先规则打分证据，再 LLM 润色成专业克制文案，失败或校验打回则走"证据结构化 fallback"。
// tickTimes 用于做新闻时点 vs 资金曲线的 15/30/60 min 对齐。
func GenerateCatalysis(sectorTicks []SectorTick, tickTimes []string, newsPages []hotnews.NewsPage) *CatalysisResult {
	if len(newsPages) == 0 {
		return nil
	}

	ev := BuildCatalysisScores(sectorTicks, tickTimes, newsPages)
	allowed := make(map[string]bool, len(fetcher.Top21HotSectors))
	for _, s := range fetcher.Top21HotSectors {
		allowed[s] = true
	}
	// 没有证据就走最老的 fallback（极端情况：newsPages 完全没板块）
	if len(ev) == 0 {
		return buildLegacyFallbackCatalysis(sectorTicks, newsPages)
	}

	summary := buildEvidenceSummary(sectorTicks, ev)
	prompt := fmt.Sprintf(PromptCatalysis, summary)

	var result CatalysisResult
	if err := llmChatCompletionJSON(prompt, 0.6, 2200, &result); err != nil {
		logger.Warn("资金催化 LLM 生成失败，使用规则证据 fallback",
			zap.Error(err))
		return buildFallbackFromEvidence(ev)
	}

	cleaned, problems := ValidateCatalysis(&result, allowed, newsPages, ev)
	if cleaned == nil || len(cleaned.Sectors) == 0 {
		logger.Warn("资金催化 LLM 校验为空，使用规则证据 fallback",
			zap.Int("problems", problems))
		return buildFallbackFromEvidence(ev)
	}
	if problems > 0 {
		logger.Info("资金催化 LLM 输出存在硬校验问题，已清洗",
			zap.Int("problems", problems),
			zap.Int("cleanedSectors", len(cleaned.Sectors)))
	}
	return cleaned
}

// buildLegacyFallbackCatalysis 当连规则证据都凑不出来时的最简退化：纯新闻标题。
func buildLegacyFallbackCatalysis(sectorTicks []SectorTick, newsPages []hotnews.NewsPage) *CatalysisResult {
	flowMap := make(map[string]float64, len(sectorTicks))
	for _, st := range sectorTicks {
		cum := 0.0
		for _, v := range st.Data {
			cum += v
		}
		flowMap[st.Name] = cum
	}
	var sectors []CatalysisSectorAnalysis
	seen := make(map[string]bool)
	for _, page := range newsPages {
		for _, sn := range page.Sectors {
			if seen[sn.Sector] {
				continue
			}
			seen[sn.Sector] = true
			flow := flowMap[sn.Sector]
			var insights []string
			for _, item := range sn.News {
				title := item.Title
				if runeLen(title) > 30 {
					title = truncateTo(title, 30)
				}
				insights = append(insights, title)
			}
			analysis := "市场对此反应平淡"
			if flow > 1.0 {
				analysis = "主力资金净流入"
			} else if flow < -1.0 {
				analysis = "主力资金净流出"
			}
			sectors = append(sectors, CatalysisSectorAnalysis{
				Sector:     sn.Sector,
				Analysis:   analysis,
				Insights:   insights,
				Confidence: confidenceLow,
				Flow:       flow,
			})
		}
	}
	if len(sectors) == 0 {
		return nil
	}
	return &CatalysisResult{Sectors: sectors}
}
