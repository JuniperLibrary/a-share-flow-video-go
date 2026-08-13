// Package config 提供视频生成器的集中配置管理。
// 包括视频参数（分辨率/FPS/帧数）、交易时段配置、AI 模型配置、
// 项目路径解析以及 .env 文件的读写。
package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Video parameters
const (
	FPS = 30
	// TotalFrames Default main-animation duration (TV / 90s @30fps).
	TotalFrames = 2700
	// MobileTotalFrames 抖音规格:30s @30fps,主图动画压缩到 1/3 速度以适配完播率。
	MobileTotalFrames = 600

	// MobileWidth Mobile dimensions (9:16)
	MobileWidth  = 1080
	MobileHeight = 1920

	// TVWidth TV dimensions (16:9)
	TVWidth  = 1920
	TVHeight = 1080

	// MinSectorCount Minimum sectors to display
	MinSectorCount = 20
)

// GetBaseFrames returns the main-animation frame count for the given format.
// TV uses the default 90s; mobile uses the shortened 30s for short-video platforms.
func GetBaseFrames(format string) int {
	if format == "tv" {
		return TotalFrames
	}
	return MobileTotalFrames
}

// SessionConfig defines time-axis configuration for a trading session.
type SessionConfig struct {
	XLim           [2]int
	XTicks         []int
	XTickLabels    []string
	TitleSuffix    string
	FilenameSuffix string
	TotalSpan      int
	BgColor        string
}

// SessionConfigs maps session keys to their configurations.
var SessionConfigs = map[string]SessionConfig{
	"morning": {
		XLim:           [2]int{0, 120},
		XTicks:         []int{0, 30, 60, 90, 120},
		XTickLabels:    []string{"9:30", "10:00", "10:30", "11:00", "11:30"},
		TitleSuffix:    "早盘",
		FilenameSuffix: "早盘",
		TotalSpan:      120,
		BgColor:        "#00ff88",
	},
	"full": {
		XLim:           [2]int{0, 240},
		XTicks:         []int{0, 60, 120, 180, 240},
		XTickLabels:    []string{"9:30", "10:30", "11:30/13:00", "14:00", "15:00"},
		TitleSuffix:    "全天",
		FilenameSuffix: "全天",
		TotalSpan:      240,
		BgColor:        "",
	},
}

// AIConfig holds LLM API configuration.
type AIConfig struct {
	APIKey              string
	BaseURL             string
	Model               string
	Models              map[string]string // per-module model 覆盖，key 为模块名
	TimeoutSec          int               // 单次请求超时（秒），<=0 表示走 env/默认 60s
	MaxRetries          int               // 重试次数（总尝试=1+MaxRetries），<=0 表示走 env/默认 3
	RetryBaseDelayMs    int               // 指数退避基础延迟（毫秒），<=0 表示走 env/默认 500ms
	CopyTotalDeadlineSec int              // 文案 AI 总 deadline（秒），超过直接降级模板；<=0 走 env/默认 180s
	CopyHumanizeEnabled  bool             // 是否启用第 3 轮「人味润色」LLM（true 多 1 次 round-trip 30s+）；默认 false 省耗时
}

// aiTunablesKeys 数据库 KV key 名 与 env 名 对齐，方便 UI/CLI/后端三处一致。
const (
	AITunableKeyTimeoutSec           = "ai_timeout_sec"
	AITunableKeyMaxRetries           = "ai_max_retries"
	AITunableKeyRetryBaseDelayMs     = "ai_retry_base_delay_ms"
	AITunableKeyCopyTotalDeadlineSec = "copy_total_deadline_sec"
	AITunableKeyCopyHumanizeEnabled  = "copy_humanize_enabled"
)

// moduleModelKeys 环境变量名 → 模块名的映射。
var moduleModelKeys = map[string]string{
	"AI_MODEL_CLSNEWS":           "clsnews",
	"AI_MODEL_ANALYZER":          "analyzer",
	"AI_MODEL_ANALYZER_MULTIDAY": "analyzer_multiday",
	"AI_MODEL_TICK":              "tick",
	"AI_MODEL_COPY":              "copy",
	"AI_MODEL_REPORT":            "report",
	"AI_MODEL_TTS":               "tts",
	"AI_MODEL_DEBATE":            "debate",
}

// GetAIConfigFor 返回指定模块的 AI 配置，per-module model 覆盖默认 Model。
func GetAIConfigFor(module string) AIConfig {
	cfg := GetAIConfig()
	if cfg.Models != nil {
		if m, ok := cfg.Models[module]; ok && m != "" {
			cfg.Model = m
		}
	}
	return cfg
}

var (
	aiConfigDBReader func() (map[string]string, bool)
	aiConfigDBWriter func(key, value string) error
)

// SetAIConfigDBFuncs registers database read/write functions for AI config.
// Must be called at startup after storage is initialized.
func SetAIConfigDBFuncs(reader func() (map[string]string, bool), writer func(key, value string) error) {
	aiConfigDBReader = reader
	aiConfigDBWriter = writer
}

var (
	projectRoot string
	rootOnce    sync.Once
)

// GetProjectRoot returns the project root directory.
func GetProjectRoot() string {
	if projectRoot != "" {
		return projectRoot
	}
	rootOnce.Do(func() {
		// Try working directory first
		cwd, err := os.Getwd()
		if err == nil {
			// Check if cwd has expected subdirectories
			if _, err := os.Stat(filepath.Join(cwd, "cmd")); err == nil {
				projectRoot = cwd
				return
			}
		}
		// Fallback: directory of the executable
		exe, err := os.Executable()
		if err == nil {
			projectRoot = filepath.Dir(exe)
		} else {
			projectRoot = "."
		}
	})
	return projectRoot
}

// SetProjectRoot overrides the project root (useful for testing).
func SetProjectRoot(root string) {
	projectRoot = root
}

// GetDataDir returns the data directory path.
func GetDataDir() string {
	return filepath.Join(GetProjectRoot(), "data")
}

// GetOutputDir returns the output directory path.
func GetOutputDir() string {
	return filepath.Join(GetProjectRoot(), "output")
}

// GetCopyDir returns the copywriting directory path.
func GetCopyDir() string {
	return filepath.Join(GetProjectRoot(), "copy")
}

// GetRendererDir returns the Remotion renderer directory path.
func GetRendererDir() string {
	return filepath.Join(GetProjectRoot(), "..", "a-share-flow-video-web")
}

// GetEnvPath returns the .env file path.
func GetEnvPath() string {
	return filepath.Join(GetProjectRoot(), ".env")
}

// DataMode 返回存储模式: "sqlite" (默认, 写入文件数据库) 或 "json" (写入 JSON 文件).
func DataMode() string {
	mode := os.Getenv("DATA_MODE")
	if mode != "sqlite" && mode != "json" {
		return "sqlite"
	}
	return mode
}

// LoadEnv parses the .env file and sets environment variables.
func LoadEnv() error {
	envPath := GetEnvPath()
	f, err := os.Open(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // .env is optional
		}
		return fmt.Errorf("open .env: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// 剥掉值尾部的 inline 注释（如 "qwen-turbo  # 注释" → "qwen-turbo"）
		if idx := strings.Index(val, "#"); idx >= 0 {
			val = strings.TrimSpace(val[:idx])
		}
		os.Setenv(key, val)
	}
	return scanner.Err()
}

// parseAITunablesFromEnv 从环境变量解析 AI 超时/重试调参，未设置或非法则 fallback 到默认。
func parseAITunablesFromEnv() (timeoutSec, maxRetries, retryBaseDelayMs int, hasAny bool) {
	timeoutSec, maxRetries, retryBaseDelayMs = 0, -1, 0
	if v := os.Getenv("AI_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
			hasAny = true
		}
	}
	if v := os.Getenv("AI_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxRetries = n
			hasAny = true
		}
	}
	if v := os.Getenv("AI_RETRY_BASE_DELAY_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			retryBaseDelayMs = n
			hasAny = true
		}
	}
	return timeoutSec, maxRetries, retryBaseDelayMs, hasAny
}

// GetAIConfig reads AI configuration from database (primary) or .env (fallback).
func GetAIConfig() AIConfig {
	cfg := AIConfig{Models: make(map[string]string)}
	if aiConfigDBReader != nil {
		if all, ok := aiConfigDBReader(); ok && len(all) > 0 {
			cfg.APIKey = all["api_key"]
			cfg.BaseURL = all["base_url"]
			cfg.Model = all["model"]
			if cfg.BaseURL == "" {
				cfg.BaseURL = "https://api.openai.com/v1"
			}
			cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
			if cfg.Model == "" {
				cfg.Model = "gpt-4o-mini"
			}
			for _, module := range []string{"clsnews", "analyzer", "analyzer_multiday", "tick", "copy", "report", "tts", "debate"} {
				if m := all["model_"+module]; m != "" {
					cfg.Models[module] = m
				}
			}
			if v := all[AITunableKeyTimeoutSec]; v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cfg.TimeoutSec = n
				}
			}
			if v := all[AITunableKeyMaxRetries]; v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cfg.MaxRetries = n
				}
			}
			if v := all[AITunableKeyRetryBaseDelayMs]; v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cfg.RetryBaseDelayMs = n
				}
			}
			if v := all[AITunableKeyCopyTotalDeadlineSec]; v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cfg.CopyTotalDeadlineSec = n
				}
			}
			dbSetHumanize := false
			if v := all[AITunableKeyCopyHumanizeEnabled]; v != "" {
				dbSetHumanize = true
				switch strings.ToLower(v) {
				case "1", "true", "yes", "on":
					cfg.CopyHumanizeEnabled = true
				case "0", "false", "no", "off":
					cfg.CopyHumanizeEnabled = false
				}
			}
			if cfg.APIKey != "" {
				_ = dbSetHumanize
				return cfg
			}
		}
	}
	cfg.APIKey = os.Getenv("OPENAI_API_KEY")
	cfg.BaseURL = os.Getenv("OPENAI_BASE_URL")
	cfg.Model = os.Getenv("AI_MODEL")
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	for envKey, module := range moduleModelKeys {
		if m := os.Getenv(envKey); m != "" {
			cfg.Models[module] = m
		}
	}
	if to, mr, rb, ok := parseAITunablesFromEnv(); ok {
		if cfg.TimeoutSec == 0 {
			cfg.TimeoutSec = to
		}
		if cfg.MaxRetries == 0 {
			cfg.MaxRetries = mr
		}
		if cfg.RetryBaseDelayMs == 0 {
			cfg.RetryBaseDelayMs = rb
		}
	}
	if cfg.CopyTotalDeadlineSec == 0 {
		if v := os.Getenv("COPY_TOTAL_DEADLINE_SEC"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.CopyTotalDeadlineSec = n
			}
		}
	}
	// Humanize: 默认 false（省 1 次 round-trip 30s+），除非 DB 显式设置或 env 显式打开
	dbSetHumanize := false // env fallback 分支本来就是默认 false，这里无需再读 DB
	_ = dbSetHumanize
	switch strings.ToLower(os.Getenv("COPY_HUMANIZE_ENABLED")) {
	case "1", "true", "yes", "on":
		cfg.CopyHumanizeEnabled = true
	case "0", "false", "no", "off":
		cfg.CopyHumanizeEnabled = false
	}
	return cfg
}

// SaveAIConfig writes AI configuration to database and .env file.
func SaveAIConfig(cfg AIConfig) error {
	if aiConfigDBWriter != nil {
		aiConfigDBWriter("api_key", cfg.APIKey)
		aiConfigDBWriter("base_url", cfg.BaseURL)
		aiConfigDBWriter("model", cfg.Model)
		for module, model := range cfg.Models {
			aiConfigDBWriter("model_"+module, model)
		}
		if cfg.TimeoutSec > 0 {
			aiConfigDBWriter(AITunableKeyTimeoutSec, strconv.Itoa(cfg.TimeoutSec))
		}
		if cfg.MaxRetries > 0 {
			aiConfigDBWriter(AITunableKeyMaxRetries, strconv.Itoa(cfg.MaxRetries))
		}
		if cfg.RetryBaseDelayMs > 0 {
			aiConfigDBWriter(AITunableKeyRetryBaseDelayMs, strconv.Itoa(cfg.RetryBaseDelayMs))
		}
		if cfg.CopyTotalDeadlineSec > 0 {
			aiConfigDBWriter(AITunableKeyCopyTotalDeadlineSec, strconv.Itoa(cfg.CopyTotalDeadlineSec))
		}
		aiConfigDBWriter(AITunableKeyCopyHumanizeEnabled, strconv.FormatBool(cfg.CopyHumanizeEnabled))
	}

	envPath := GetEnvPath()
	existing := make(map[string]string)
	if f, err := os.Open(envPath); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				existing[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		f.Close()
	}

	existing["OPENAI_API_KEY"] = cfg.APIKey
	existing["OPENAI_BASE_URL"] = cfg.BaseURL
	existing["AI_MODEL"] = cfg.Model
	if cfg.TimeoutSec > 0 {
		existing["AI_TIMEOUT_SEC"] = strconv.Itoa(cfg.TimeoutSec)
	}
	if cfg.MaxRetries > 0 {
		existing["AI_MAX_RETRIES"] = strconv.Itoa(cfg.MaxRetries)
	}
	if cfg.RetryBaseDelayMs > 0 {
		existing["AI_RETRY_BASE_DELAY_MS"] = strconv.Itoa(cfg.RetryBaseDelayMs)
	}
	if cfg.CopyTotalDeadlineSec > 0 {
		existing["COPY_TOTAL_DEADLINE_SEC"] = strconv.Itoa(cfg.CopyTotalDeadlineSec)
	}
	existing["COPY_HUMANIZE_ENABLED"] = strconv.FormatBool(cfg.CopyHumanizeEnabled)

	f, err := os.Create(envPath)
	if err != nil {
		return fmt.Errorf("create .env: %w", err)
	}
	defer f.Close()

	for key, val := range existing {
		fmt.Fprintf(f, "%s=%s\n", key, val)
	}
	return nil
}

// TickConfig holds tick collector configuration.
type TickConfig struct {
	IntervalMinutes int `json:"intervalMinutes"`
}

// DefaultTickConfig returns default tick configuration.
func DefaultTickConfig() TickConfig {
	return TickConfig{IntervalMinutes: 5}
}

// GetDBPath returns the SQLite database file path.
func GetDBPath() string {
	return filepath.Join(GetProjectRoot(), "data", "a-share-flow.db")
}

// GetTickConfigPath returns the tick config file path.
func GetTickConfigPath() string {
	return filepath.Join(GetProjectRoot(), ".tick-config.json")
}

// LoadTickConfig reads tick configuration from disk.
func LoadTickConfig() TickConfig {
	cfg := DefaultTickConfig()
	f, err := os.Open(GetTickConfigPath())
	if err != nil {
		return cfg
	}
	defer f.Close()

	var data struct {
		IntervalMinutes int `json:"intervalMinutes"`
	}
	if err := json.NewDecoder(f).Decode(&data); err == nil && data.IntervalMinutes > 0 {
		cfg.IntervalMinutes = data.IntervalMinutes
	}
	return cfg
}

// SaveTickConfig writes tick configuration to disk.
func SaveTickConfig(cfg TickConfig) error {
	f, err := os.Create(GetTickConfigPath())
	if err != nil {
		return fmt.Errorf("create tick config: %w", err)
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(cfg)
}
