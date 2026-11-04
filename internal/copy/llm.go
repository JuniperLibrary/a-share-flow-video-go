package copy

import (
	"context"
	"fmt"

	"github.com/a-share-flow-video-go/internal/ai"
	"github.com/a-share-flow-video-go/internal/config"
)

func chatCompletion(prompt string, temperature float64, maxTokens int) (string, error) {
	return chatCompletionWithContext(context.Background(), prompt, temperature, maxTokens)
}

func chatCompletionWithContext(ctx context.Context, prompt string, temperature float64, maxTokens int) (string, error) {
	aiCfg := config.GetAIConfigFor("copy")
	if aiCfg.APIKey == "" {
		return "", fmt.Errorf("AI模式需要设置 OPENAI_API_KEY 环境变量")
	}
	return ai.ChatCompletion(ctx, aiCfg, prompt, temperature, maxTokens)
}
