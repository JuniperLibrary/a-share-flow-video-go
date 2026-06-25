package tick

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
)

// llmChatCompletion 调用 OpenAI 兼容 API，返回文本。
func llmChatCompletion(prompt string, temperature float64, maxTokens int) (string, error) {
	aiCfg := config.GetAIConfigFor("tick")
	if aiCfg.APIKey == "" {
		return "", fmt.Errorf("LLM 需要设置 OPENAI_API_KEY")
	}

	body := map[string]any{
		"model":       aiCfg.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", aiCfg.BaseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+aiCfg.APIKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API 请求失败: HTTP %d %s", resp.StatusCode, string(b))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM API 返回空 choices")
	}
	return result.Choices[0].Message.Content, nil
}

// llmChatCompletionJSON 调用 LLM 并解析 JSON 输出到目标结构体。
func llmChatCompletionJSON(prompt string, temperature float64, maxTokens int, dest any) error {
	text, err := llmChatCompletion(prompt, temperature, maxTokens)
	if err != nil {
		return fmt.Errorf("llm call: %w", err)
	}
	cleaned := extractJSON(text)
	if err := json.Unmarshal([]byte(cleaned), dest); err != nil {
		return fmt.Errorf("llm JSON 解析失败: %w\n原始输出: %s", err, text)
	}
	return nil
}
