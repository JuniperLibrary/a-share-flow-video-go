package tick

import (
	"fmt"
	"strings"

	"github.com/a-share-flow-video-go/internal/hotnews"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// CatalysisSectorAnalysis 单个板块的催化分析。
type CatalysisSectorAnalysis struct {
	Sector   string   `json:"sector"`
	Analysis string   `json:"analysis"`
	Insights []string `json:"insights"`
}

// CatalysisResult LLM 输出的资金催化分析。
type CatalysisResult struct {
	Sectors []CatalysisSectorAnalysis `json:"sectors"`
}

// buildCatalysisSummary 构建发送给 LLM 的数据摘要。
func buildCatalysisSummary(sectorTicks []SectorTick, newsPages []hotnews.NewsPage) string {
	var sb strings.Builder

	sb.WriteString("【板块资金流向】\n")
	for i, st := range sectorTicks {
		if i >= 10 {
			break
		}
		cum := 0.0
		for _, v := range st.Data {
			cum += v
		}
		sb.WriteString(fmt.Sprintf("- %s: 主力净流向%+.0f亿\n", st.Name, cum))
	}

	sb.WriteString("\n【关联新闻】\n")
	for _, page := range newsPages {
		for _, sn := range page.Sectors {
			sb.WriteString(fmt.Sprintf("- %s:\n", sn.Sector))
			for _, item := range sn.News {
				title := item.Title
				if len([]rune(title)) > 60 {
					title = string([]rune(title)[:60]) + "…"
				}
				sb.WriteString(fmt.Sprintf("  · [%s] %s\n", item.Level, title))
			}
		}
	}

	return sb.String()
}

// buildFallbackCatalysis 从原始新闻数据生成基础的催化分析。
// 当 LLM 生成失败时作为后备方案。
func buildFallbackCatalysis(sectorTicks []SectorTick, newsPages []hotnews.NewsPage) *CatalysisResult {
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
				if len([]rune(title)) > 30 {
					title = string([]rune(title)[:30]) + "…"
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
				Sector:   sn.Sector,
				Analysis: analysis,
				Insights: insights,
			})
		}
	}

	if len(sectors) == 0 {
		return nil
	}
	return &CatalysisResult{Sectors: sectors}
}

// GenerateCatalysis 调用 LLM 生成资金催化分析。
// 失败时使用原始新闻生成基础分析。
func GenerateCatalysis(sectorTicks []SectorTick, newsPages []hotnews.NewsPage) *CatalysisResult {
	if len(newsPages) == 0 {
		return nil
	}

	summary := buildCatalysisSummary(sectorTicks, newsPages)
	prompt := fmt.Sprintf(PromptCatalysis, summary)

	var result CatalysisResult
	if err := llmChatCompletionJSON(prompt, 0.7, 2000, &result); err != nil {
		logger.Warn("资金催化 LLM 生成失败，使用原始新闻",
			zap.Error(err))
		return buildFallbackCatalysis(sectorTicks, newsPages)
	}

	if len(result.Sectors) == 0 {
		return buildFallbackCatalysis(sectorTicks, newsPages)
	}
	return &result
}
