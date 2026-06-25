package tick

import (
	"fmt"
	"sort"
	"strings"

	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// MainStructureResult LLM 输出的主线结构收尾分析。
type MainStructureResult struct {
	Conclusion    string `json:"conclusion"`
	Concentration string `json:"concentration"`
	Risk          string `json:"risk"`
	Structure     string `json:"structure"`
	Outlook       string `json:"outlook"`
	Signal        string `json:"signal,omitempty"`
	Action        string `json:"action,omitempty"`
}

type sectorStat struct {
	Name  string
	Net   float64
	Super float64
	Big   float64
	Rate  float64
}

// buildMainStructureSummary 构建发送给 LLM 的板块结构摘要。
func buildMainStructureSummary(sectorTicks []SectorTick) string {
	var stats []sectorStat
	var totalInflow, totalOutflow float64
	var inflowCount, outflowCount int

	for _, st := range sectorTicks {
		cum := 0.0
		for _, v := range st.Data {
			cum += v
		}
		s := sectorStat{Name: st.Name, Net: cum, Super: st.SuperNet, Big: st.BigNet, Rate: st.Rate}
		stats = append(stats, s)
		if cum > 0 {
			totalInflow += cum
			inflowCount++
		} else {
			totalOutflow += cum
			outflowCount++
		}
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Net > stats[j].Net
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("全市场：净流入 %d 个板块，净流出 %d 个板块\n", inflowCount, outflowCount))
	sb.WriteString(fmt.Sprintf("流入总额：%.0f 亿，流出总额：%.0f 亿\n\n", totalInflow, totalOutflow*-1))

	sb.WriteString("板块排名（按净流向）：\n")
	for i, s := range stats {
		if i >= 15 {
			break
		}
		sb.WriteString(fmt.Sprintf("%d. %s: %+.0f亿 (超大单%+.0f, 大单%+.0f, 涨幅%.1f%%)\n",
			i+1, s.Name, s.Net, s.Super, s.Big, s.Rate))
	}

	var inflows []sectorStat
	var outflows []sectorStat
	for _, s := range stats {
		if s.Net > 0 {
			inflows = append(inflows, s)
		}
		if s.Net < 0 {
			outflows = append(outflows, s)
		}
	}
	if len(inflows) > 0 && totalInflow > 0 {
		top1Share := inflows[0].Net / totalInflow * 100
		top2Share := top1Share
		if len(inflows) > 1 {
			top2Share = (inflows[0].Net + inflows[1].Net) / totalInflow * 100
		}
		sb.WriteString(fmt.Sprintf("\n集中度：Top1 %s 占流入 %.0f%%，Top2 合计占流入 %.0f%%\n",
			inflows[0].Name, top1Share, top2Share))
	}
	if len(outflows) > 0 && totalOutflow < 0 {
		sb.WriteString(fmt.Sprintf("主要压力：%s 净流出 %.0f亿，占流出侧 %.0f%%\n",
			outflows[len(outflows)-1].Name,
			outflows[len(outflows)-1].Net*-1,
			outflows[len(outflows)-1].Net/totalOutflow*100))
	}

	return sb.String()
}

// GenerateMainStructure 调用 LLM 生成主线结构收尾分析。
// 失败时返回 nil，调用方 fallback 到原始逻辑。
func GenerateMainStructure(sectorTicks []SectorTick) *MainStructureResult {
	if len(sectorTicks) == 0 {
		return nil
	}

	summary := buildMainStructureSummary(sectorTicks)
	prompt := fmt.Sprintf(PromptMainStructure, summary)

	var result MainStructureResult
	if err := llmChatCompletionJSON(prompt, 0.7, 1000, &result); err != nil {
		logger.Warn("主线结构收尾 LLM 生成失败，使用模板逻辑",
			zap.Error(err))
		return nil
	}

	if result.Conclusion == "" {
		return nil
	}
	return &result
}
