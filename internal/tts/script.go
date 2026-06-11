package tts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

const promptScript = `你是一名财经解说主播。根据用户给出的主题，生成一段解说风格的口播稿。

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

## 用户主题

%s

请根据这个主题生成一段完整的口播稿。`

type ScriptResult struct {
	Script string `json:"script"`
}

// GenerateScript 根据主题调用大模型生成解说风格口播稿。
func GenerateScript(topic string) (ScriptResult, error) {
	aiCfg := config.GetAIConfig()
	if aiCfg.APIKey == "" {
		return ScriptResult{}, fmt.Errorf("AI 模式需要设置 OPENAI_API_KEY 环境变量")
	}

	prompt := fmt.Sprintf(promptScript, topic)

	body := map[string]any{
		"model":       aiCfg.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": 0.85,
		"max_tokens":  1500,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", aiCfg.BaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return ScriptResult{}, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+aiCfg.APIKey)

	logger.Info("TTS 口播稿生成请求",
		zap.String("topic", topic),
		zap.String("model", aiCfg.Model),
	)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ScriptResult{}, fmt.Errorf("API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return ScriptResult{}, fmt.Errorf("API 请求失败: HTTP %d %s", resp.StatusCode, string(b))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &result); err != nil {
		return ScriptResult{}, fmt.Errorf("解析响应失败: %w", err)
	}
	if len(result.Choices) == 0 {
		return ScriptResult{}, fmt.Errorf("API 返回空结果")
	}

	script := result.Choices[0].Message.Content

	logger.Info("TTS 口播稿生成完成",
		zap.String("topic", topic),
		zap.Int("chars", len([]rune(script))),
	)

	return ScriptResult{Script: script}, nil
}
