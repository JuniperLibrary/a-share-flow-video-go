package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/analyzer"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

// Generate 生成指定日期的日报。
// 从 DB 聚合数据 → AI 生成总结和展望 → 组装为 DailyReport。
// AI 失败时自动降级为纯数据驱动的模板报告。
func Generate(dateStr, session string) (*DailyReport, error) {
	db, err := storage.Get()
	if err != nil {
		return nil, fmt.Errorf("get db: %w", err)
	}

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
	if news, _, err := db.LoadNewsByDate(dateStr, 10, 0); err == nil {
		for _, n := range news {
			timeStr := extractTime(n.CTime)
			if timeStr == "" {
				timeStr = n.CTime
			}
			report.NewsBriefs = append(report.NewsBriefs, NewsBrief{
				Title: n.Title,
				Level: n.Level,
				Time:  timeStr,
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

	// 6. AI 生成总结和展望（失败不阻塞）
	aiCfg := config.GetAIConfig()
	if aiCfg.APIKey != "" {
		summary, outlook, aiErr := AIGenerate(dateStr, report, sectors, aiCfg)
		if aiErr == nil {
			report.Summary = summary
			report.Outlook = outlook
		} else {
			logger.Warn("日报 AI 生成失败，使用数据驱动", zap.Error(aiErr))
		}
	}

	// 7. 数据驱动 fallback summary
	if report.Summary == "" {
		report.Summary = dataDrivenSummary(report)
		report.Outlook = dataDrivenOutlook(report, sectors)
	}

	report.CreatedAt = time.Now().In(shanghaiTZ).Format("2006-01-02 15:04:05")

	// 8. 保存到 DB
	reportJSON, _ := json.Marshal(report)
	if err := db.SaveDailyReport(dateStr, session, report.Summary, report.Outlook, string(reportJSON)); err != nil {
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

// AIGenerate 调用 LLM 生成日报的 summary 和 outlook。
func AIGenerate(dateStr string, r *DailyReport, sectors []storage.SectorAll, aiCfg config.AIConfig) (string, string, error) {
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
		newsLines = append(newsLines, fmt.Sprintf("- [%s] %s", n.Level, n.Title))
	}
	if len(newsLines) == 0 {
		newsLines = append(newsLines, "- 无今日新闻")
	}

	prompt := fmt.Sprintf(PromptDailyReport,
		dateDisplay,
		netTotalStr,
		fmt.Sprintf("%d", r.InflowCount),
		fmt.Sprintf("%d", r.OutflowCount),
		strings.Join(inflowLines, "\n"),
		strings.Join(outflowLines, "\n"),
		structureDesc,
		strings.Join(timelineLines, "\n"),
		strings.Join(newsLines, "\n"),
	)

	body := map[string]any{
		"model":       aiCfg.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": 0.7,
		"max_tokens":  2000,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", aiCfg.BaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+aiCfg.APIKey)

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("API HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", "", fmt.Errorf("empty choices")
	}

	content := result.Choices[0].Message.Content
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return "", "", fmt.Errorf("no JSON in AI response")
	}

	var aiResult struct {
		Summary string `json:"summary"`
		Outlook string `json:"outlook"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &aiResult); err != nil {
		return "", "", fmt.Errorf("parse AI JSON: %w", err)
	}

	return aiResult.Summary, aiResult.Outlook, nil
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

func extractJSON(content string) string {
	start := strings.Index(content, "{")
	if start == -1 {
		return ""
	}
	end := strings.LastIndex(content, "}")
	if end == -1 || end < start {
		return ""
	}
	return content[start : end+1]
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
