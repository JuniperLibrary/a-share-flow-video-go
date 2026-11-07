package debate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
)

// callLLM 以前固定用 context.Background(),调用方的超时/取消全都不生效:
// debategate 设的 30 分钟上限形同虚设,web 端断开后请求还在烧钱。
func TestCallLLMHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := config.AIConfig{
		APIKey:     "test-key",
		BaseURL:    "http://127.0.0.1:1/v1",
		Model:      "test-model",
		TimeoutSec: 30,
		MaxRetries: 1,
	}

	start := time.Now()
	_, err := callLLM(ctx, cfg, "ping", 0.1, 128)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("ctx 已取消却仍然发起成功调用")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("错误里应保留 context canceled,实际: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("取消后仍等了 %v,说明 ctx 没有真正传下去", elapsed)
	}
}
