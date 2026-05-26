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
	"strings"
	"sync"
)

// Video parameters
const (
	FPS         = 30
	TotalFrames = 2700

	// MobileWidth Mobile dimensions (9:16)
	MobileWidth  = 1080
	MobileHeight = 1920

	// TVWidth TV dimensions (16:9)
	TVWidth  = 1920
	TVHeight = 1080

	// MinSectorCount Minimum sectors to display
	MinSectorCount = 20
)

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
		XLim:           [2]int{0, 330},
		XTicks:         []int{0, 60, 120, 180, 240, 300},
		XTickLabels:    []string{"9:30", "10:30", "11:30", "13:00", "14:00", "15:00"},
		TitleSuffix:    "全天",
		FilenameSuffix: "全天",
		TotalSpan:      330,
		BgColor:        "",
	},
}

// AIConfig holds LLM API configuration.
type AIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

var (
	projectRoot string
	rootOnce    sync.Once
)

// GetProjectRoot returns the project root directory.
func GetProjectRoot() string {
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
		os.Setenv(key, val)
	}
	return scanner.Err()
}

// GetAIConfig reads AI configuration from environment variables.
func GetAIConfig() AIConfig {
	cfg := AIConfig{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		Model:   os.Getenv("AI_MODEL"),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	// Remove trailing slash
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	return cfg
}

// SaveAIConfig writes AI configuration to .env file.
func SaveAIConfig(cfg AIConfig) error {
	envPath := GetEnvPath()

	// Read existing .env to preserve other variables
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

	// Update AI config
	existing["OPENAI_API_KEY"] = cfg.APIKey
	existing["OPENAI_BASE_URL"] = cfg.BaseURL
	existing["AI_MODEL"] = cfg.Model

	// Write back
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
