package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
)

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		want bool
	}{
		{"nil 200 ok", nil, 200, false},
		{"nil 400 client", nil, 400, false},
		{"nil 429 rate limit", nil, 429, true},
		{"nil 500 server", nil, 500, true},
		{"nil 503 server", nil, 503, true},
		{"context deadline", context.DeadlineExceeded, 0, true},
		{"client timeout", errors.New("Post https://x: Client.Timeout exceeded while awaiting headers"), 0, true},
		{"connection refused", errors.New("dial tcp: connection refused"), 0, true},
		{"connection reset", errors.New("read tcp: connection reset by peer"), 0, true},
		{"unexpected EOF", errors.New("unexpected EOF"), 0, true},
		{"io timeout", errors.New("net/http: i/o timeout"), 0, true},
		{"tls handshake timeout", errors.New("TLS handshake timeout"), 0, true},
		{"no such host", errors.New("dial tcp: lookup x: no such host"), 0, true},
		{"401 auth permanent", errors.New("API HTTP 401"), 401, false},
		{"400 bad request", nil, 400, false},
	}
	for _, c := range cases {
		got := isRetryable(c.err, c.code)
		if got != c.want {
			t.Errorf("%s: isRetryable(%v,%d) = %v, want %v", c.name, c.err, c.code, got, c.want)
		}
	}
}

func TestStripCodeFence(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"```json\n{\"a\":1}\n```", `{"a":1}`},
		{"```\nhello\n```", "hello"},
		{"plain text", "plain text"},
		{"  spaced  ", "spaced"},
	}
	for _, c := range cases {
		if got := stripCodeFence(c.in); got != c.want {
			t.Errorf("stripCodeFence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`x{"a":1}y`, `{"a":1}`},
		{`前缀 [1,2,3] 后缀`, `[1,2,3]`},
		{`no json here`, ""},
		{`{"nested":{"k":2}}`, `{"nested":{"k":2}}`},
	}
	for _, c := range cases {
		if got := extractJSON(c.in); got != c.want {
			t.Errorf("extractJSON(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestChatCompletionRetriesOn5xx(t *testing.T) {
	t.Setenv("AI_MAX_RETRIES", "2") // 3 attempts total
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
		})
	}))
	defer ts.Close()

	got, err := ChatCompletion(context.Background(), testCfg(ts.URL), "p", 0.7, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Errorf("content = %q, want %q", got, "ok")
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Errorf("server hits = %d, want 3 (must retry through 5xx)", hits)
	}
}

func TestChatCompletionNoRetryOn4xx(t *testing.T) {
	t.Setenv("AI_MAX_RETRIES", "3")
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	_, err := ChatCompletion(context.Background(), testCfg(ts.URL), "p", 0.7, 100)
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("server hits = %d, want 1 (4xx must not retry)", hits)
	}
}

func TestChatCompletionJSONStripsFence(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{
				"content": "结果如下:\n```json\n{\"value\":42}\n```",
			}}},
		})
	}))
	defer ts.Close()

	var dest struct {
		Value int `json:"value"`
	}
	if err := ChatCompletionJSON(context.Background(), testCfg(ts.URL), "p", 0.7, 100, &dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dest.Value != 42 {
		t.Errorf("dest.Value = %d, want 42", dest.Value)
	}
}

func TestChatCompletionTimeoutRetries(t *testing.T) {
	t.Setenv("AI_MAX_RETRIES", "2")
	t.Setenv("AI_TIMEOUT_SEC", "1") // 1s per-attempt deadline
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			// Exceed the 1s deadline so the client cancels (context deadline exceeded).
			time.Sleep(2 * time.Second)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "recovered"}}},
		})
	}))
	defer ts.Close()

	got, err := ChatCompletion(context.Background(), testCfg(ts.URL), "p", 0.7, 100)
	if err != nil {
		t.Fatalf("expected recovery after timeout retry, got error: %v", err)
	}
	if got != "recovered" {
		t.Errorf("content = %q, want %q", got, "recovered")
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Errorf("server hits = %d, want 2 (timeout must trigger a retry)", hits)
	}
}

func testCfg(baseURL string) config.AIConfig {
	return config.AIConfig{APIKey: "test-key", BaseURL: baseURL, Model: "test-model"}
}

func TestChatCompletionRawWithSystemUser(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(req.Messages) != 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
			t.Errorf("unexpected messages: %+v", req.Messages)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "classified"}}},
		})
	}))
	defer ts.Close()

	got, err := ChatCompletionRaw(context.Background(), testCfg(ts.URL),
		[]ChatMessage{{Role: "system", Content: "sys"}, {Role: "user", Content: "usr"}}, 0.1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "classified" {
		t.Errorf("content = %q, want %q", got, "classified")
	}
}
