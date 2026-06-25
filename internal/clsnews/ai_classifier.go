package clsnews

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// AINewsClassifier 使用大模型对新闻进行板块标签分类。
type AINewsClassifier struct {
	cfg    config.AIConfig
	client *http.Client
}

type aiClassifyResult struct {
	Tags      []string `json:"tags"`
	Reasoning string   `json:"reasoning"`
}

type aiClassifyResponse struct {
	Results []aiClassifyResult `json:"results"`
}

type classifyItem struct {
	origIdx int
	title   string
	content string
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

const aiNewsClassifierPrompt = `你是一个A股财经新闻板块标签分类专家。你的任务是根据新闻标题和正文，判断其关联的A股板块。

## 可选板块标签（21个监控板块）
%s

## 规则
1. 每条新闻分配 0~3 个板块标签。优先从上面的21个监控板块中选择。
2. 如果21个监控板块中没有合适的标签，可以输出其他合理的A股板块标签（如"新能源汽车"、"光伏"、"军工"、"证券"、"房地产"、"煤炭"、"医药"、"教育"等）。
3. 如果新闻与A股市场完全无关（如纯国际政治新闻、军事冲突、自然灾害、娱乐八卦等），tags 返回空数组 []。
4. 标签必须是真实存在的A股板块概念名称，不要虚构。
5. 只根据新闻内容判断，不要过度泛化。

## 输入新闻（JSON数组）
%s

## 输出格式（JSON，严格遵循）
{"results":[{"index":0,"tags":["半导体",...],"reasoning":"判断理由（10字以内）"},...]}

## 注意事项
- 输出必须是一个合法的 JSON 对象，不要包含任何其他文字。
- 只返回 JSON，不要用 markdown 代码块包裹。
- results 数组与输入新闻一一对应，顺序一致。`

// NewAINewsClassifier 创建一个 AI 新闻分类器。
func NewAINewsClassifier(cfg config.AIConfig) *AINewsClassifier {
	return &AINewsClassifier{
		cfg: cfg,
		client: &http.Client{
			Timeout: 180 * time.Second,
		},
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
	item := classifyItem{origIdx: 0, title: title, content: content}
	allTags, err := c.classifyBatch([]classifyItem{item})
	if err != nil {
		return nil, err
	}
	if len(allTags) == 0 {
		return nil, nil
	}
	return allTags[0], nil
}

// ClassifyBatch 对一批新闻进行 AI 标签分类。
// 返回与输入等长的切片，每个元素是该新闻的板块标签（最多3个）。
// nil 表示该新闻因太短或 AI 无法判断而未分类。
func (c *AINewsClassifier) ClassifyBatch(news []CLSNews) [][]string {
	if !c.IsAvailable() {
		logger.Debug("AI 新闻分类未启用（未配置 API Key）")
		return make([][]string, len(news))
	}

	var items []classifyItem
	for i, n := range news {
		title := strings.TrimSpace(n.Title)
		content := strings.TrimSpace(n.Content)
		if utf8.RuneCountInString(title) >= 5 || utf8.RuneCountInString(content) >= 10 {
			items = append(items, classifyItem{origIdx: i, title: title, content: content})
		}
	}

	if len(items) == 0 {
		logger.Debug("AI 新闻分类跳过：所有新闻正文过短", zap.Int("total", len(news)))
		return make([][]string, len(news))
	}

	logger.Debug("AI 新闻分类开始",
		zap.Int("totalNews", len(news)),
		zap.Int("qualified", len(items)),
	)

	const batchSize = 10
	results := make([][]string, len(news))
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
				processed[item.origIdx] = false
			}
			continue
		}

		for j, tagList := range tags {
			origIdx := batch[j].origIdx
			results[origIdx] = tagList
			processed[origIdx] = true
			if len(tagList) > 0 {
				taggedCount++
			}
		}
	}

	for i := range news {
		if !processed[i] {
			results[i] = nil
		}
	}

	logger.Info("AI 新闻分类完成",
		zap.Int("total", len(news)),
		zap.Int("tagged", taggedCount),
		zap.Int("skipped", len(news)-taggedCount),
	)

	return results
}

func (c *AINewsClassifier) classifyBatch(items []classifyItem) ([][]string, error) {
	newsJSON, _ := json.Marshal(items)

	sectorDesc := strings.Join(sectorTagDescriptions, "\n")
	prompt := fmt.Sprintf(aiNewsClassifierPrompt, sectorDesc, string(newsJSON))

	body := map[string]any{
		"model":       c.cfg.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": 0.1,
		"max_tokens":  4096,
	}
	bodyBytes, _ := json.Marshal(body)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt+1) * 500 * time.Millisecond
			logger.Debug("AI 新闻分类重试",
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoff),
				zap.Error(lastErr),
			)
			time.Sleep(backoff)
		}

		req, err := http.NewRequest("POST", c.cfg.BaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
		if err != nil {
			lastErr = fmt.Errorf("create request: %w", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("API request: %w", err)
			continue
		}

		b, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read response: %w", readErr)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("API HTTP %d: %s", resp.StatusCode, string(b))
			if resp.StatusCode >= 500 {
				continue
			}
			return nil, lastErr
		}

		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(b, &result); err != nil {
			lastErr = fmt.Errorf("parse API response: %w", err)
			continue
		}
		if len(result.Choices) == 0 {
			lastErr = fmt.Errorf("empty choices from API")
			continue
		}

		// success — return parsed results directly
		content := result.Choices[0].Message.Content
		return parseClassifyResult(content, items)
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("classifyBatch: unexpected exit")
}

func parseClassifyResult(content string, items []classifyItem) ([][]string, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var respData aiClassifyResponse
	if err := json.Unmarshal([]byte(content), &respData); err != nil {
		return nil, fmt.Errorf("parse AI JSON output: %w", err)
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
