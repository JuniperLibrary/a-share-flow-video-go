package tick

import (
	"context"
	"fmt"

	"github.com/a-share-flow-video-go/internal/ai"
	"github.com/a-share-flow-video-go/internal/config"
)

// llmChatCompletion 调用 OpenAI 兼容 API，返回文本。超时与重试由 internal/ai 统一处理。
func llmChatCompletion(prompt string, temperature float64, maxTokens int) (string, error) {
	aiCfg := config.GetAIConfigFor("tick")
	if aiCfg.APIKey == "" {
		return "", fmt.Errorf("LLM 需要设置 OPENAI_API_KEY")
	}
	return ai.ChatCompletion(context.Background(), aiCfg, prompt, temperature, maxTokens)
}

// llmChatCompletionJSON 调用 LLM 并解析 JSON 输出到目标结构体。
func llmChatCompletionJSON(prompt string, temperature float64, maxTokens int, dest any) error {
	aiCfg := config.GetAIConfigFor("tick")
	if aiCfg.APIKey == "" {
		return fmt.Errorf("LLM 需要设置 OPENAI_API_KEY")
	}
	return ai.ChatCompletionJSON(context.Background(), aiCfg, prompt, temperature, maxTokens, dest)
}
