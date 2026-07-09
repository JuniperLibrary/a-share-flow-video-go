package clsnews

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/a-share-flow-video-go/internal/ai"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// AINewsClassifier 使用大模型对新闻进行板块标签分类。
type AINewsClassifier struct {
	cfg config.AIConfig
}

type aiClassifyResult struct {
	Tags []string `json:"tags"`
}

type aiClassifyResponse struct {
	Results []aiClassifyResult `json:"results"`
}

type ClassificationResult struct {
	Tags   []string
	Status string // "classified", "skipped", "discarded", "retry"
	Error  string
}

type classifyItem struct {
	OrigIdx int    `json:"idx"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// sectorTagDescriptions 21 个监控板块的描述，用于 AI prompt。
var sectorTagDescriptions = []string{
	"半导体: 芯片、集成电路、晶圆、光刻机、封测、EDA、第三代半导体",
	"AI应用: 生成式AI、AIGC、AI+行业(医疗/教育/金融)、多模态、AI智能体、AI终端落地",
	"CPO概念: 共封装光学、硅光、光模块(800G/1.6T)、LPO、光互联、相干光学",
	"有色金属: 铜、铝、锌、镍、锡、铅、稀土、黄金、贵金属、小金属、战略金属",
	"锂矿概念: 碳酸锂、氢氧化锂、盐湖提锂、锂辉石、锂云母、电池级锂盐",
	"商业航天: 商业火箭、商业卫星、卫星互联网、低轨卫星、星链、太空经济",
	"电池: 固态电池、锂电池、磷酸铁锂、钠离子电池、动力电池、储能电池、电池材料",
	"机器人: 人形机器人、具身智能、减速器、伺服电机、灵巧手、工业机器人",
	"创新药: 靶向药、抗体(单抗/双抗)、ADC、CAR-T、GLP-1、临床试验、FDA批准",
	"白酒: 高端白酒(茅台/五粮液)、酱酒、酿酒、白酒消费、白酒动销",
	"消费电子: 手机、折叠屏、AR/VR/MR、可穿戴、面板(OLED/MiniLED)、AI PC、AI眼镜",
	"银行: 商业银行、净息差、信贷、存款、不良率、国有大行、股份行",
	"人工智能: 大模型、LLM、算力、AI芯片、机器学习、深度学习、自然语言处理(NLP)、GPT、OpenAI",
	"云计算: 云服务、IaaS/PaaS/SaaS、公有云/私有云、数据中心(IDC)、算力租赁、云原生",
	"低空经济: eVTOL、飞行汽车、无人机、空管、低空基础设施、通用航空",
	"电网设备: 特高压、变压器、智能电网、配电网、充电桩、输变电、电力物联网",
	"通信设备: 5G/5.5G/6G、基站、光通信、光纤光缆、交换机、卫星通信",
	"传媒: 游戏、影视、短剧、出版、广告营销、新媒体、短视频、直播、IP",
	"国产芯片: GPU/NPU/AI芯片、CPU、信创、自主可控、操作系统、国产替代、华为芯片",
	"元件: MLCC、电容电阻电感、连接器、功率器件(IGBT/MOSFET)、传感器、PCB、被动元件",
	"通信服务: 电信运营商(移动/电信/联通)、5G套餐、宽带、云通信、增值电信",
}

const systemPromptForClassifier = `你是A股新闻板块分类器。根据标题+正文判断关联板块，每个新闻0~3个标签。

## 21个监控板块
%s

## 其他板块
新能源汽车、光伏、军工、证券、房地产、煤炭、医药、教育、氢能源、核电、汽车整车、零部件、旅游酒店、食品饮料、农业、钢铁、环保、物流、港口航运

## 规则
1. 每条新闻返回0~3个板块标签
2. 优先从21个监控板块中选择
3. 无关新闻返回空数组
4. 标签必须是真实A股板块名

## 输出格式（严格JSON，不要有任何其他文字）
输入3条新闻时输出：
{"results":[{"index":0,"tags":["半导体"]},{"index":1,"tags":["AI应用","人工智能"]},{"index":2,"tags":[]}]}

注意：
- 只输出JSON，不要有其他文字
- results数组长度必须等于输入新闻数量
- tags数组可以为空[]`

// userPromptForClassifier 是每次请求的 user message 模板，只含新闻数据。
const userPromptForClassifier = `## 输入新闻（JSON数组）
%s`

// NewAINewsClassifier 创建一个 AI 新闻分类器。
func NewAINewsClassifier(cfg config.AIConfig) *AINewsClassifier {
	return &AINewsClassifier{
		cfg: cfg,
	}
}

// IsAvailable 返回是否已配置 AI API Key。
func (c *AINewsClassifier) IsAvailable() bool {
	return c.cfg.APIKey != ""
}

// ClassifyOne 对单条新闻进行 AI 板块标签分类。
// 返回最多 3 个板块标签，AI 无法判断时返回 nil。
func (c *AINewsClassifier) ClassifyOne(news CLSNews) ([]string, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("AI 新闻分类未启用")
	}
	title := strings.TrimSpace(news.Title)
	content := strings.TrimSpace(news.Content)
	if utf8.RuneCountInString(title) < 5 && utf8.RuneCountInString(content) < 10 {
		return nil, nil
	}
	item := classifyItem{OrigIdx: 0, Title: title, Content: content}
	allTags, err := c.classifyBatch([]classifyItem{item})
	if err != nil {
		return nil, err
	}
	if len(allTags) == 0 {
		return nil, nil
	}
	return allTags[0], nil
}

// ClassifyBatch 对一批新闻进行 AI 标签分类，并返回可观测的分类状态。
func (c *AINewsClassifier) ClassifyBatch(news []CLSNews) []ClassificationResult {
	if !c.IsAvailable() {
		logger.Debug("AI 新闻分类未启用（未配置 API Key）")
		results := make([]ClassificationResult, len(news))
		for i := range results {
			results[i] = ClassificationResult{Status: "retry", Error: "AI 新闻分类未启用"}
		}
		return results
	}

	var items []classifyItem
	results := make([]ClassificationResult, len(news))
	for i, n := range news {
		title := strings.TrimSpace(n.Title)
		content := strings.TrimSpace(n.Content)
		if utf8.RuneCountInString(title) >= 5 || utf8.RuneCountInString(content) >= 10 {
			items = append(items, classifyItem{OrigIdx: i, Title: title, Content: content})
		} else {
			results[i] = ClassificationResult{Status: "skipped", Error: "内容过短"}
		}
	}

	if len(items) == 0 {
		logger.Debug("AI 新闻分类跳过：所有新闻正文过短", zap.Int("total", len(news)))
		return results
	}

	logger.Debug("AI 新闻分类开始",
		zap.Int("totalNews", len(news)),
		zap.Int("qualified", len(items)),
	)

	const batchSize = 10
	processed := make(map[int]bool)
	taggedCount := 0

	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		batch := items[start:end]

		tags, err := c.classifyBatch(batch)
		if err != nil {
			logger.Warn("AI 新闻分类批次失败",
				zap.Int("batchSize", len(batch)),
				zap.Error(err),
			)
			for _, item := range batch {
				results[item.OrigIdx] = ClassificationResult{Status: "retry", Error: err.Error()}
				processed[item.OrigIdx] = true
			}
			continue
		}

		for j, tagList := range tags {
			origIdx := batch[j].OrigIdx
			if len(tagList) > 0 {
				results[origIdx] = ClassificationResult{Status: "classified", Tags: tagList}
				taggedCount++
			} else {
				results[origIdx] = ClassificationResult{Status: "discarded", Error: "AI 未返回标签"}
			}
			processed[origIdx] = true
		}
	}

	for i := range news {
		if !processed[i] {
			if results[i].Status == "" {
				results[i] = ClassificationResult{Status: "retry", Error: "AI 分类未完成"}
			}
		}
	}

	logger.Info("AI 新闻分类完成",
		zap.Int("total", len(news)),
		zap.Int("tagged", taggedCount),
		zap.Int("skipped", len(news)-taggedCount),
	)

	return results
}

// classifyBatch 对一批新闻进行 AI 板块标签分类。
func (c *AINewsClassifier) classifyBatch(items []classifyItem) ([][]string, error) {
	if len(items) == 0 {
		return nil, nil
	}

	preClassified := make(map[int][]string)
	needAI := make([]classifyItem, 0, len(items))
	needAIIdxMap := make(map[int]int)

	for _, item := range items {
		tags := preFilterByKeywords(item.Title + " " + item.Content)
		if tags != nil {
			preClassified[item.OrigIdx] = tags
		} else {
			needAIIdxMap[item.OrigIdx] = len(needAI)
			needAI = append(needAI, item)
		}
	}

	if len(needAI) == 0 {
		results := make([][]string, len(items))
		for i, item := range items {
			results[i] = preClassified[item.OrigIdx]
		}
		return results, nil
	}

	newsJSON, _ := json.Marshal(needAI)
	sectorDesc := strings.Join(sectorTagDescriptions, "\n")
	systemContent := fmt.Sprintf(systemPromptForClassifier, sectorDesc)
	userContent := fmt.Sprintf(userPromptForClassifier, string(newsJSON))

	content, err := ai.ChatCompletionRaw(context.Background(), c.cfg,
		[]ai.ChatMessage{
			{Role: "system", Content: systemContent},
			{Role: "user", Content: userContent},
		}, 0.1, 4096)
	if err != nil {
		return nil, fmt.Errorf("新闻分类 AI 请求失败: %w", err)
	}

	aiResults, err := parseClassifyResult(content, needAI)
	if err != nil {
		return nil, err
	}

	results := make([][]string, len(items))
	for i, item := range items {
		if tags, ok := preClassified[item.OrigIdx]; ok {
			results[i] = tags
		} else {
			results[i] = aiResults[needAIIdxMap[item.OrigIdx]]
		}
	}
	return results, nil
}

func preFilterByKeywords(text string) []string {
	lower := strings.ToLower(text)
	matched := make(map[string]bool)

	keywordMap := map[string][]string{
		"半导体":   {"芯片", "集成电路", "晶圆", "光刻机", "封测", "eda", "半导体"},
		"AI应用":  {"aigc", "ai+", "多模态", "ai智能体", "ai终端", "ai眼镜", "ai pc"},
		"CPO概念": {"共封装", "硅光", "光模块", "800g", "1.6t", "lpo", "光互联"},
		"有色金属":  {"铜价", "铝价", "锌", "镍", "锡", "铅", "稀土", "黄金", "贵金属"},
		"锂矿概念":  {"碳酸锂", "氢氧化锂", "盐湖提锂", "锂辉石", "锂云母"},
		"商业航天":  {"商业火箭", "商业卫星", "卫星互联网", "低轨卫星", "星链", "太空经济"},
		"电池":    {"固态电池", "锂电池", "磷酸铁锂", "钠离子电池", "动力电池", "储能电池"},
		"机器人":   {"人形机器人", "具身智能", "减速器", "伺服电机", "灵巧手", "工业机器人"},
		"创新药":   {"靶向药", "单抗", "双抗", "adc", "car-t", "glp-1", "临床试验", "fda批准"},
		"白酒":    {"茅台", "五粮液", "酱酒", "酿酒", "白酒消费", "白酒动销"},
		"消费电子":  {"手机", "折叠屏", "ar/vr", "可穿戴", "oled", "miniled", "ai pc"},
		"银行":    {"商业银行", "净息差", "信贷", "存款", "不良率", "国有大行", "股份行"},
		"人工智能":  {"大模型", "llm", "算力", "ai芯片", "机器学习", "深度学习", "nlp", "gpt"},
		"云计算":   {"云服务", "iaas", "paas", "saas", "公有云", "私有云", "idc", "算力租赁"},
		"低空经济":  {"evtol", "飞行汽车", "无人机", "空管", "低空基础设施", "通用航空"},
		"电网设备":  {"特高压", "变压器", "智能电网", "配电网", "充电桩", "输变电"},
		"通信设备":  {"5g", "6g", "基站", "光通信", "光纤光缆", "交换机"},
		"传媒":    {"游戏", "影视", "短剧", "出版", "广告营销", "新媒体", "短视频", "直播", "ip"},
		"国产芯片":  {"gpu", "npu", "ai芯片", "cpu", "信创", "自主可控", "操作系统", "华为芯片"},
		"元件":    {"mlcc", "电容", "电阻", "电感", "连接器", "igbt", "mosfet", "传感器", "pcb"},
		"通信服务":  {"移动", "电信", "联通", "5g套餐", "宽带", "云通信"},
	}

	for tag, keywords := range keywordMap {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				matched[tag] = true
				break
			}
		}
	}

	if len(matched) == 0 {
		return nil
	}

	tags := make([]string, 0, len(matched))
	for tag := range matched {
		tags = append(tags, tag)
	}
	if len(tags) > 3 {
		tags = tags[:3]
	}
	return tags
}

func parseClassifyResult(content string, items []classifyItem) ([][]string, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var respData aiClassifyResponse
	if err := json.Unmarshal([]byte(content), &respData); err != nil {
		fixed := fixTruncatedJSON(content)
		if fixed != content {
			if err2 := json.Unmarshal([]byte(fixed), &respData); err2 == nil {
				logger.Debug("AI 返回 JSON 被截断，已自动修复")
				content = fixed
			} else {
				logger.Warn("AI 返回 JSON 解析失败",
					zap.String("content", content),
					zap.Error(err),
				)
				return nil, fmt.Errorf("parse AI JSON output: %w", err)
			}
		} else {
			logger.Warn("AI 返回 JSON 解析失败",
				zap.String("content", content),
				zap.Error(err),
			)
			return nil, fmt.Errorf("parse AI JSON output: %w", err)
		}
	}

	if len(respData.Results) != len(items) {
		return nil, fmt.Errorf("AI returned %d results, expected %d", len(respData.Results), len(items))
	}

	results := make([][]string, len(items))
	for i, r := range respData.Results {
		tags := make([]string, 0, len(r.Tags))
		seen := make(map[string]bool)
		for _, tag := range r.Tags {
			tag = strings.TrimSpace(tag)
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			tags = append(tags, tag)
		}
		if len(tags) > 3 {
			tags = tags[:3]
		}
		results[i] = tags
	}

	tagged := 0
	for _, t := range results {
		if len(t) > 0 {
			tagged++
		}
	}
	if tagged > 0 {
		logger.Debug("AI 分类批次结果",
			zap.Int("batchSize", len(items)),
			zap.Int("tagged", tagged),
		)
	}

	return results, nil
}

func fixTruncatedJSON(s string) string {
	if s == "" {
		return s
	}

	unclosedBraces := 0
	unclosedBrackets := 0
	inString := false
	escaped := false

	for _, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			unclosedBraces++
		case '}':
			unclosedBraces--
		case '[':
			unclosedBrackets++
		case ']':
			unclosedBrackets--
		}
	}

	fixed := s
	for i := 0; i < unclosedBrackets; i++ {
		fixed += "]"
	}
	for i := 0; i < unclosedBraces; i++ {
		fixed += "}"
	}

	return fixed
}
