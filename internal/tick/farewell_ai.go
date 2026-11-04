package tick

import (
	"fmt"
	"strings"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// PromptTickFarewell 是告别仪式祝福语的 LLM prompt 模板。
// %s = 当日资金流摘要
// %s = 日期上下文(当前交易日 + 下一交易日)
const PromptTickFarewell = `你是一名A股深夜复盘主播的收尾金句写手，负责每日告别仪式的祝福语。

根据当日资金流摘要，写一句贴合当日行情的散户口味祝福语（blessing）和一句简短告别语（signoff）。

## 时间表达规则（必须严格遵守）
- 禁止使用「今天 / 明天 / 明日 / 今日 / 昨日」等相对时间词，必须使用具体日期（格式：8月14日 星期五）。
- 「下一交易日」相关语境必须用具体的下一个交易日日期。
- 日期上下文：%s

## 要求
- blessing：15-30字，口语化、接地气、正能量，可结合当日主线板块或资金面隐喻，但不要出现具体板块名、个股、点位或收益承诺。
- signoff：4-10字，必须带感叹号，用具体的下一交易日日期，如"8月17日（周一）开盘见！""8月24日见！"。禁止出现"明天"。
- blessing 与 signoff 一脉相承，可连读成完整一句话。
- 必须是原创文案，不要复述摘要中的原文。

## 当日资金流摘要
%s

## 输出
严格输出 JSON，不要任何多余文字：
{"blessing":"...","signoff":"..."}`

// sortableFarewellAIResult 是 LLM 返回的 JSON 结构。
type sortableFarewellAIResult struct {
	Blessing string `json:"blessing"`
	Signoff  string `json:"signoff"`
}

// BuildFarewellTexts 生成告别仪式祝福语。
// 优先调用 LLM 基于当日资金流生成原创祝福语；失败或未配置 key 时
// 回退到静态祝福语池 buildFarewellTexts，保证视频生成不中断。
func BuildFarewellTexts(dateStr, displayDate string, sectorTicks []SectorTick) (blessing, signoff string) {
	fallbackB, fallbackS := buildFarewellTexts(dateStr, displayDate)

	aiCfg := config.GetAIConfigFor("tick")
	if aiCfg.APIKey == "" {
		return fallbackB, fallbackS
	}

	summary := buildMainStructureSummary(sectorTicks)
	if strings.TrimSpace(summary) == "" {
		summary = "（当日暂无板块资金数据）"
	}
	prompt := fmt.Sprintf(PromptTickFarewell, summary, fetcher.TradingDayContext(dateStr))

	var result sortableFarewellAIResult
	if err := llmChatCompletionJSON(prompt, 0.9, 200, &result); err != nil {
		logger.Warn("告别仪式祝福语 LLM 生成失败，回退静态池",
			zap.Error(err))
		return fallbackB, fallbackS
	}

	b := strings.TrimSpace(result.Blessing)
	s := strings.TrimSpace(result.Signoff)
	if b == "" || s == "" || len([]rune(b)) > 40 || len([]rune(s)) > 12 ||
		containsRelativeTimeWord(b) || containsRelativeTimeWord(s) {
		logger.Warn("告别仪式祝福语 LLM 输出不合法，回退静态池")
		return fallbackB, fallbackS
	}
	return b, s
}

// containsRelativeTimeWord 检测文案中是否出现「今天/明天/今日/明日/昨日」等相对时间词。
func containsRelativeTimeWord(s string) bool {
	for _, w := range relativeTimeWords {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
