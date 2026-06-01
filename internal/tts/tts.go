// Package tts 提供文本转语音功能，调用 edge-tts 命令行工具
// 将 AI 生成的视频文案合成为中文 MP3 语音，用于 Remotion 视频配音。
package tts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Voice 可选中文语音角色
type Voice string

const (
	Xiaoxiao Voice = "zh-CN-XiaoxiaoNeural" // 女声，亲切自然（默认）
	Yunyang  Voice = "zh-CN-YunyangNeural"  // 男声，沉稳
	Xiaoyi   Voice = "zh-CN-XiaoyiNeural"   // 女声，知性
	Yunjian  Voice = "zh-CN-YunjianNeural"  // 男声，年轻
)

// TextToSpeech 将文本合成为 MP3 文件。
// 使用 edge-tts 命令行工具，不依赖额外的 API 密钥。
// 网络不稳定时自动重试，最多重试 3 次，指数退避。
func TextToSpeech(text, outputPath string, voice Voice) error {
	if voice == "" {
		voice = Xiaoxiao
	}

	text = CleanForTTS(text)
	if text == "" {
		return fmt.Errorf("清洗后文本为空，跳过 TTS")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避：1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		cmd := exec.Command("edge-tts",
			"--voice", string(voice),
			"--text", text,
			"--write-media", outputPath,
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			lastErr = fmt.Errorf("edge-tts 合成失败: %w\n输出: %s", err, string(output))
			continue
		}

		if _, err := os.Stat(outputPath); err != nil {
			lastErr = fmt.Errorf("TTS 输出文件未找到: %w", err)
			continue
		}
		return nil
	}
	return fmt.Errorf("edge-tts 合成失败（重试 %d 次后放弃）: %w", maxRetries, lastErr)
}

var (
	reMarkdownBold   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reMarkdownItalic = regexp.MustCompile(`\*(.+?)\*`)
	reHashtagLine    = regexp.MustCompile(`(?m)^#\S.*$`)
	reNumberedList   = regexp.MustCompile(`(?m)^\d+[\.\)、]\s*`)
	reBulletPoint    = regexp.MustCompile(`(?m)^[-·•]\s*`)
	reEmoji          = regexp.MustCompile(`[\x{1F300}-\x{1F9FF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}\x{FE00}-\x{FE0F}\x{1F000}-\x{1FAFF}]`)
)

// CleanForTTS 清洗文本使其适合语音合成。
// 去除 markdown 格式、emoji、hashtag 行、列表符号等。
func CleanForTTS(text string) string {
	text = reMarkdownBold.ReplaceAllString(text, "$1")
	text = reMarkdownItalic.ReplaceAllString(text, "$1")
	text = reHashtagLine.ReplaceAllString(text, "")
	text = reNumberedList.ReplaceAllString(text, "")
	text = reBulletPoint.ReplaceAllString(text, "")
	text = reEmoji.ReplaceAllString(text, "")

	var cleaned strings.Builder
	for _, r := range text {
		if r == '*' || r == '_' || r == '`' || r == '~' {
			continue
		}
		if unicode.Is(unicode.So, r) {
			continue
		}
		cleaned.WriteRune(r)
	}

	lines := strings.Split(cleaned.String(), "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return strings.Join(result, "\n")
}

// GetAudioDuration 使用 ffprobe 获取音频文件时长（秒）。
func GetAudioDuration(audioPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		audioPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("ffprobe 获取时长失败: %w", err)
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, fmt.Errorf("解析音频时长失败: %w", err)
	}
	return duration, nil
}

// ParseCopywriting 解析 AI 生成的文案，提取标题和正文。
// 标题：第一段非空连续文本中的第一行（去掉 # 标签行和空行后的第一个有意义行）
// 正文：从"前3秒钩子"或第二段开始到第一个 # 标签之前的所有内容（去掉 hashtag 行）
func ParseCopywriting(text string) (title, body string) {
	lines := strings.Split(text, "\n")

	// 过滤空行和纯 hashtag 行，提取有意义行
	var meaningful []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// 跳过纯 hashtag 行
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		meaningful = append(meaningful, trimmed)
	}

	if len(meaningful) == 0 {
		return "", text
	}

	// 第一行当标题
	title = meaningful[0]

	// 从第二行开始到第一个 # 标签（原始行）之前作为正文
	bodyLines := []string{}
	inHashtag := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			inHashtag = true
			continue
		}
		if !inHashtag && trimmed != "" && trimmed != title {
			bodyLines = append(bodyLines, trimmed)
		}
	}
	body = strings.Join(bodyLines, "\n")
	if body == "" {
		body = strings.Join(meaningful[1:], "\n")
	}

	return title, body
}

// EnsureEdgeTTS 检查 edge-tts 是否可用。
func EnsureEdgeTTS() error {
	if _, err := exec.LookPath("edge-tts"); err != nil {
		return fmt.Errorf("未找到 edge-tts 命令，请运行: pip install edge-tts")
	}
	return nil
}
