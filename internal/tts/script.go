package tts

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/ai"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// MarketContext 是 TTS 口播稿生成所需的市场数据上下文。
type MarketContext struct {
	Date           string
	RelevantSector *fetcher.Sector // 与主题最相关的板块
	TopInflows     []fetcher.Sector
	TopOutflows    []fetcher.Sector
	NetTotal       float64
	Sectors        []fetcher.Sector
}

const promptScript = `你是一名财经解说主播。根据用户给出的主题和实时市场数据，生成一段解说风格的口播稿。

## 风格要求

模仿足球/游戏解说的语气——有节奏感、有冲击力、有悬念，但不是夸张。

- 短句为主，一句一行，读起来有停顿感
- 用设问句制造悬念："为什么？""关键来了""注意看"
- 用类比让复杂概念直观（如"就像关键团战""像被三路围攻"）
- 每个段落只讲一个核心观点
- 适当使用"你"拉近距离
- 不要用 emoji、markdown、编号列表

## 结构（不要求严格分段，但逻辑要有层次）

1. 开场钩子：一句话抓住注意力——最反常/最冲击的现象是什么
2. 展开分析：2-3层递进，每层一个角度，用"第一股力量""第二股力量"或类似递进结构
3. 转折/深度：深入一层，揭示表面现象下的真实逻辑
4. 投资启示：给出具体、可操作的看法（不要模糊建议）
5. 结尾金句：一句有力总结，让人记住

## 参考风格（不是模板，仅展示语气）

黄金，最近被打懵了。
前面还在一路狂飙，市场一片看多。
结果转头就是一脚急刹，价格连续回撤，追高的人瞬间开始怀疑人生。

关键来了。
这轮黄金暴跌，不是避险逻辑崩了。
而是利率、美元、获利盘三路围攻，短线资金扛不住了。

## 实时市场数据（必须基于此数据生成内容，不可编造）

%s

## 用户主题

%s

请根据主题和市场数据生成一段完整的口播稿。注意：所有数据和判断必须基于上述市场数据，不可凭空编造。`

type ScriptResult struct {
	Script    string         `json:"script"`
	MarketCtx *MarketContext `json:"marketContext,omitempty"`
}

func GenerateScript(topic string) (ScriptResult, error) {
	aiCfg := config.GetAIConfigFor("tts")
	if aiCfg.APIKey == "" {
		return ScriptResult{}, fmt.Errorf("AI 模式需要设置 OPENAI_API_KEY 环境变量")
	}

	marketCtx, err := buildMarketContext(topic)
	if err != nil {
		logger.Warn("获取市场数据失败，使用纯 AI 生成", zap.Error(err))
	}

	var prompt string
	if marketCtx != nil {
		dataSummary := formatMarketDataForTTS(marketCtx)
		prompt = fmt.Sprintf(promptScript, dataSummary, topic)
	} else {
		prompt = fmt.Sprintf(promptScript, "（暂无实时数据，请基于主题生成通用分析）", topic)
	}

	logger.Info("TTS 口播稿生成请求",
		zap.String("topic", topic),
		zap.String("model", aiCfg.Model),
		zap.Bool("hasMarketData", marketCtx != nil),
	)

	script, err := ai.ChatCompletion(context.Background(), aiCfg, prompt, 0.85, 1500)
	if err != nil {
		return ScriptResult{}, fmt.Errorf("API 请求失败: %w", err)
	}

	if marketCtx != nil {
		if issues := validateScript(script, marketCtx); len(issues) > 0 {
			logger.Warn("口播稿验证发现问题", zap.Strings("issues", issues))
			fixPrompt := fmt.Sprintf(`你生成的口播稿存在以下问题，请修正：

问题列表：
%s

原稿：
%s

请基于市场数据修正上述问题，保持原有风格。`, strings.Join(issues, "\n"), script)

			fixScript, fixErr := ai.ChatCompletion(context.Background(), aiCfg, fixPrompt, 0.7, 1500)
			if fixErr == nil && fixScript != "" {
				script = fixScript
				logger.Info("口播稿已修正", zap.Int("issues", len(issues)))
			}
		}
	}

	logger.Info("TTS 口播稿生成完成",
		zap.String("topic", topic),
		zap.Int("chars", len([]rune(script))),
	)

	return ScriptResult{Script: script, MarketCtx: marketCtx}, nil
}

func buildMarketContext(topic string) (*MarketContext, error) {
	sectors, err := fetcher.FetchTop21HotSectors()
	if err != nil {
		return nil, fmt.Errorf("获取板块数据失败: %w", err)
	}

	if len(sectors) == 0 {
		return nil, fmt.Errorf("未获取到板块数据")
	}

	ctx := &MarketContext{
		Date:     time.Now().Format("2006-01-02"),
		Sectors:  sectors,
		NetTotal: 0,
	}

	for _, s := range sectors {
		ctx.NetTotal += s.Net
	}

	sort.Slice(sectors, func(i, j int) bool {
		return sectors[i].Net > sectors[j].Net
	})
	ctx.TopInflows = sectors[:min(5, len(sectors))]

	sort.Slice(sectors, func(i, j int) bool {
		return sectors[i].Net < sectors[j].Net
	})
	ctx.TopOutflows = sectors[:min(3, len(sectors))]

	ctx.RelevantSector = findRelevantSector(topic, sectors)

	return ctx, nil
}

func findRelevantSector(topic string, sectors []fetcher.Sector) *fetcher.Sector {
	topicLower := strings.ToLower(topic)
	keywords := map[string][]string{
		"半导体": {"半导体", "芯片", "国产芯片"},
		"AI":  {"ai", "人工智能", "ai应用", "云计算"},
		"电池":  {"电池", "锂矿", "新能源"},
		"白酒":  {"白酒", "消费"},
		"银行":  {"银行", "金融"},
		"医药":  {"创新药", "医药", "医疗"},
		"军工":  {"商业航天", "军工", "国防"},
		"新能源": {"电池", "锂矿", "光伏", "风电"},
	}

	for category, kws := range keywords {
		for _, kw := range kws {
			if strings.Contains(topicLower, kw) {
				for i := range sectors {
					if strings.Contains(sectors[i].Name, category) || strings.Contains(category, sectors[i].Name) {
						return &sectors[i]
					}
				}
			}
		}
	}

	for i := range sectors {
		if strings.Contains(topicLower, sectors[i].Name) {
			return &sectors[i]
		}
	}

	if len(sectors) > 0 {
		return &sectors[0]
	}

	return nil
}

func formatMarketDataForTTS(ctx *MarketContext) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("日期：%s\n", ctx.Date))
	sb.WriteString(fmt.Sprintf("全市场主力净流向：%+.0f 亿\n\n", ctx.NetTotal))

	if ctx.RelevantSector != nil {
		s := ctx.RelevantSector
		dir := "净流入"
		if s.Net < 0 {
			dir = "净流出"
		}
		sb.WriteString(fmt.Sprintf("【与主题相关的板块：%s】\n", s.Name))
		sb.WriteString(fmt.Sprintf("  涨跌幅：%+.1f%%\n", s.ChangePct))
		sb.WriteString(fmt.Sprintf("  主力净流向：%+.0f 亿（%s）\n", s.Net, dir))
		sb.WriteString(fmt.Sprintf("  超大单净流向：%+.0f 亿\n", s.SuperNet))
		sb.WriteString(fmt.Sprintf("  大单净流向：%+.0f 亿\n\n", s.BigNet))
	}

	sb.WriteString("【今日资金流入 TOP 5】\n")
	for i, s := range ctx.TopInflows {
		sb.WriteString(fmt.Sprintf("  %d. %s：涨跌幅 %+.1f%%，净流入 %+.0f 亿\n",
			i+1, s.Name, s.ChangePct, s.Net))
	}

	sb.WriteString("\n【今日资金流出 TOP 3】\n")
	for i, s := range ctx.TopOutflows {
		sb.WriteString(fmt.Sprintf("  %d. %s：涨跌幅 %+.1f%%，净流出 %+.0f 亿\n",
			i+1, s.Name, s.ChangePct, absF(s.Net)))
	}

	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func validateScript(script string, ctx *MarketContext) []string {
	var issues []string

	if ctx.RelevantSector != nil {
		s := ctx.RelevantSector
		if s.ChangePct < -1 && strings.Contains(script, "大涨") {
			issues = append(issues, fmt.Sprintf("板块 %s 今日跌 %.1f%%，但稿中提到「大涨」", s.Name, s.ChangePct))
		}
		if s.ChangePct > 1 && strings.Contains(script, "大跌") {
			issues = append(issues, fmt.Sprintf("板块 %s 今日涨 %.1f%%，但稿中提到「大跌」", s.Name, s.ChangePct))
		}
		if s.Net < -50 && strings.Contains(script, "资金涌入") {
			issues = append(issues, fmt.Sprintf("板块 %s 今日净流出 %.0f 亿，但稿中提到「资金涌入」", s.Name, absF(s.Net)))
		}
		if s.Net > 50 && strings.Contains(script, "资金出逃") {
			issues = append(issues, fmt.Sprintf("板块 %s 今日净流入 %.0f 亿，但稿中提到「资金出逃」", s.Name, s.Net))
		}
	}

	if len(ctx.TopInflows) > 0 {
		leader := ctx.TopInflows[0]
		if !strings.Contains(script, leader.Name) {
			issues = append(issues, fmt.Sprintf("未提及今日最强板块「%s」（净流入 %.0f 亿）", leader.Name, leader.Net))
		}
	}

	for _, s := range ctx.TopOutflows {
		if s.Net < -100 && strings.Contains(script, s.Name) {
			if strings.Contains(script, "强势") || strings.Contains(script, "领涨") || strings.Contains(script, "大涨") {
				issues = append(issues, fmt.Sprintf("板块 %s 今日净流出 %.0f 亿，但稿中描述为强势/领涨", s.Name, absF(s.Net)))
			}
		}
	}

	return issues
}
