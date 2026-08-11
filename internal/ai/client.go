// Package ai provides a shared, resilient OpenAI-compatible chat-completions
// client used by all AI callers in the project (analyzer, copy, tick, report,
// debate, ...). It centralizes timeout, retry-with-backoff, and transient-error
// classification so that a single transient DashScope/OpenAI blip does not force
// an immediate fallback to template output.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

// Default tunables (overridable via environment variables).
const (
	defaultTimeoutSec     = 60
	defaultMaxRetries     = 3
	defaultRetryBaseDelay = 500 * time.Millisecond
	defaultClientTimeout  = 260 * time.Second
	envTimeoutSec         = "AI_TIMEOUT_SEC"
	envMaxRetries         = "AI_MAX_RETRIES"
	envRetryBaseDelayMs   = "AI_RETRY_BASE_DELAY_MS"
)

// sharedClient is a long-lived http.Client that reuses TLS connections with a
// bounded idle pool. It avoids the classic "can't assign requested address"
// port-exhaustion bug seen when every LLM call opens a fresh TCP socket that
// lingers in TIME_WAIT. All AI callers MUST route through this client.
var sharedClient = newSharedHTTPClient()

func newSharedHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		MaxConnsPerHost:       32,
		IdleConnTimeout:       45 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
		// macOS/Linux: SO_REUSEADDR 尽量降低 TIME_WAIT 后的 bind 失败概率
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2: true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   defaultClientTimeout,
	}
}

// timeoutPerAttempt returns the per-attempt deadline duration.
func timeoutPerAttempt() time.Duration {
	if v := os.Getenv(envTimeoutSec); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultTimeoutSec * time.Second
}

// maxRetries returns the configured retry count (total attempts = 1 + maxRetries).
func maxRetries() int {
	if v := os.Getenv(envMaxRetries); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return defaultMaxRetries
}

// retryBaseDelay returns the base backoff delay for exponential backoff.
func retryBaseDelay() time.Duration {
	if v := os.Getenv(envRetryBaseDelayMs); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return defaultRetryBaseDelay
}

// chatRequest is the OpenAI-compatible request body.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

// ChatMessage is a single chat message (role + content).
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// isRetryable reports whether a failed attempt should be retried.
// Transient: network/timeout errors, connection resets, HTTP 429 (rate limit),
// and 5xx (server errors). Permanent: 4xx client errors (auth, bad request) are NOT retried.
// NOTE: Darwin/macOS 会把源端口耗尽（EADDRNOTAVAIL）包装成 "can't assign requested address"。
// 这种错误理论上是瞬态的（等 30s MSL 内核回收），我们仍标为 retryable，让 backoff 起作用。
func isRetryable(err error, statusCode int) bool {
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "context deadline exceeded") ||
			strings.Contains(msg, "Client.Timeout") ||
			strings.Contains(msg, "connection reset") ||
			strings.Contains(msg, "connection refused") ||
			strings.Contains(msg, "EOF") ||
			strings.Contains(msg, "i/o timeout") ||
			strings.Contains(msg, "TLS handshake timeout") ||
			strings.Contains(msg, "no such host") ||
			strings.Contains(msg, "can't assign requested address") ||
			strings.Contains(msg, "address already in use") {
			return true
		}
		return false
	}
	switch {
	case statusCode == http.StatusTooManyRequests:
		return true
	case statusCode >= 500 && statusCode <= 599:
		return true
	default:
		return false
	}
}

// ChatCompletion calls the OpenAI-compatible chat/completions endpoint with a
// single user message, with per-attempt context deadlines and exponential
// backoff retries on transient failures. It returns the trimmed content.
func ChatCompletion(ctx context.Context, aiCfg config.AIConfig, prompt string, temperature float64, maxTokens int) (string, error) {
	return ChatCompletionRaw(ctx, aiCfg, []ChatMessage{{Role: "user", Content: prompt}}, temperature, maxTokens)
}

// ChatCompletionRaw sends an explicit message list (e.g. system + user) to the
// chat/completions endpoint. It applies per-attempt context deadlines and
// exponential backoff retries on transient failures (timeout / connection reset
// / HTTP 429 / 5xx). Permanent 4xx errors are not retried.
func ChatCompletionRaw(ctx context.Context, aiCfg config.AIConfig, messages []ChatMessage, temperature float64, maxTokens int) (string, error) {
	if aiCfg.APIKey == "" {
		return "", fmt.Errorf("AI 调用需要设置 OPENAI_API_KEY")
	}
	if aiCfg.BaseURL == "" {
		return "", fmt.Errorf("AI 调用需要设置 OPENAI_BASE_URL")
	}

	baseURL := strings.TrimRight(aiCfg.BaseURL, "/")
	reqBody := chatRequest{
		Model:       aiCfg.Model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	attempts := 1 + maxRetries()
	var lastErr error

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := retryBaseDelay() * time.Duration(1<<uint(attempt-1))
			logger.Warn("AI 请求重试",
				zap.Int("attempt", attempt+1),
				zap.Int("max_attempts", attempts),
				zap.Duration("delay", delay),
				zap.Error(lastErr))
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		// Per-attempt deadline so a hung connection does not block forever and
		// so retries get a fresh deadline each time.
		attemptCtx, cancel := context.WithTimeout(ctx, timeoutPerAttempt())
		content, status, err := doAttempt(attemptCtx, baseURL, aiCfg.APIKey, reqBody)
		cancel()

		if err == nil {
			if status != http.StatusOK {
				lastErr = fmt.Errorf("AI 请求失败: HTTP %d", status)
				if isRetryable(nil, status) {
					continue
				}
				return "", lastErr
			}
			return content, nil
		}

		lastErr = err
		if !isRetryable(err, status) {
			return "", err
		}
	}

	return "", fmt.Errorf("AI 请求在 %d 次尝试后仍失败: %w", attempts, lastErr)
}

// doAttempt performs a single HTTP attempt. It rebuilds the request body each
// time (the previous reader is drained after a send), avoiding the empty-body
// bug seen in callers that reused a bytes.Reader across retries.
func doAttempt(ctx context.Context, baseURL, apiKey string, reqBody chatRequest) (content string, status int, err error) {
	bodyBytes, mErr := json.Marshal(reqBody)
	if mErr != nil {
		return "", 0, fmt.Errorf("marshal request: %w", mErr)
	}

	req, rErr := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if rErr != nil {
		return "", 0, rErr
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, dErr := sharedClient.Do(req)
	if dErr != nil {
		// macOS 源端口耗尽（EADDRNOTAVAIL）时，主动关闭 idle conns + 短 backoff，
		// 把 TIME_WAIT 腾出来，给下一次重试让路。
		msg := dErr.Error()
		if strings.Contains(msg, "can't assign requested address") ||
			strings.Contains(msg, "address already in use") {
			if t, ok := sharedClient.Transport.(*http.Transport); ok {
				t.CloseIdleConnections()
			}
		}
		return "", 0, dErr
	}
	defer resp.Body.Close()

	b, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", resp.StatusCode, fmt.Errorf("读取响应失败: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, nil
	}

	var parsed chatResponse
	if uErr := json.Unmarshal(b, &parsed); uErr != nil {
		return "", resp.StatusCode, fmt.Errorf("解析响应失败: %w", uErr)
	}
	if len(parsed.Choices) == 0 {
		return "", resp.StatusCode, fmt.Errorf("AI 返回空 choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), resp.StatusCode, nil
}

// ChatCompletionJSON calls ChatCompletion and unmarshals the returned content
// into dest. It tolerates markdown code fences (```json ... ```) and surrounding
// prose by stripping fences and extracting the outermost JSON object/array.
func ChatCompletionJSON(ctx context.Context, aiCfg config.AIConfig, prompt string, temperature float64, maxTokens int, dest any) error {
	raw, err := ChatCompletion(ctx, aiCfg, prompt, temperature, maxTokens)
	if err != nil {
		return err
	}
	cleaned := extractJSON(stripCodeFence(raw))
	if cleaned == "" {
		return fmt.Errorf("AI 返回无法解析为 JSON: %s", truncate(raw, 200))
	}
	if uErr := json.Unmarshal([]byte(cleaned), dest); uErr != nil {
		return fmt.Errorf("解析 AI JSON 失败: %w (原始: %s)", uErr, truncate(raw, 200))
	}
	return nil
}

// stripCodeFence removes a leading/trailing markdown code fence if present.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx > 0 {
			s = s[idx+1:]
		}
		s = strings.TrimSpace(s)
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
	}
	return strings.TrimSpace(s)
}

// extractJSON extracts the outermost JSON object or array from arbitrary text.
func extractJSON(content string) string {
	if start := strings.Index(content, "{"); start != -1 {
		if end := strings.LastIndex(content, "}"); end > start {
			return content[start : end+1]
		}
	}
	if start := strings.Index(content, "["); start != -1 {
		if end := strings.LastIndex(content, "]"); end > start {
			return content[start : end+1]
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
