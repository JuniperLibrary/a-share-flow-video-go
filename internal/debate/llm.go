package debate

import (
	"context"
	"fmt"
	"strings"

	"github.com/a-share-flow-video-go/internal/ai"
	"github.com/a-share-flow-video-go/internal/config"
)

// callLLM 是所有轮次共用的单次调用入口(编排器按轮串行调用它)。
//
// 历史上这里还有一个"经典"路径 GenerateScript:一次调用吐整篇 8-12 轮辩论。
// 它只有 bull/bear 两个角色、没有 citations 与 verdict、也不查表,
// 与多智能体路径是两套质量标准,已于 2026-11 合并——现在只有这一条链路,
// 深度差异由 Orchestrator 的 Profile(deep/quick)表达。
// callLLM 必须带上调用方的 ctx:此前固定用 context.Background(),
// 结果是 debategate/abtest 设的 30 分钟超时形同虚设,请求方断开也没人停。
func callLLM(ctx context.Context, aiCfg config.AIConfig, prompt string, temperature float64, maxTokens int) (string, error) {
	raw, err := ai.ChatCompletion(ctx, aiCfg, prompt, temperature, maxTokens)
	if err != nil {
		return "", fmt.Errorf("LLM 请求失败: %w", err)
	}
	return stripCodeFence(raw), nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx > 0 {
			s = s[idx+1:]
		}
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
	}
	return strings.TrimSpace(s)
}
