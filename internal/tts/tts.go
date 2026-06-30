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

	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

const (
	DefaultPresetID = "finance_anchor"
)

type Preset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Voice       string `json:"voice"`
	Rate        string `json:"rate"`
	Pitch       string `json:"pitch"`
	Volume      string `json:"volume"`
}

type Options struct {
	Preset string `json:"preset"`
	Voice  string `json:"voice"`
	Rate   string `json:"rate"`
	Pitch  string `json:"pitch"`
	Volume string `json:"volume"`
}

type Result struct {
	OutputPath  string  `json:"outputPath"`
	DurationSec float64 `json:"durationSec"`
	Chars       int     `json:"chars"`
	Preset      Preset  `json:"preset"`
}

var presets = []Preset{
	{
		ID:          "sports_commentary",
		Name:        "足球解说",
		Description: "高能、推进感强，适合市场暴涨暴跌、强转折口播。",
		Voice:       "zh-CN-YunjianNeural",
		Rate:        "+58%",
		Pitch:       "+12Hz",
		Volume:      "+18%",
	},
	{
		ID:          "game_commentary",
		Name:        "游戏解说",
		Description: "更快、更兴奋，适合节奏密集、悬念强的短视频。",
		Voice:       "zh-CN-YunxiNeural",
		Rate:        "+68%",
		Pitch:       "+16Hz",
		Volume:      "+20%",
	},
	{
		ID:          "finance_anchor",
		Name:        "财经解说",
		Description: "稳中带冲击，适合常规复盘、投资建议和新闻播报。",
		Voice:       "zh-CN-YunjianNeural",
		Rate:        "+42%",
		Pitch:       "+8Hz",
		Volume:      "+12%",
	},
}

func Presets() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return out
}

func ResolvePreset(id string) Preset {
	if id == "" {
		id = DefaultPresetID
	}
	for _, p := range presets {
		if p.ID == id {
			return p
		}
	}
	return presets[0]
}

func TextToSpeech(text, outputPath string) error {
	_, err := Synthesize(text, outputPath, Options{Preset: DefaultPresetID})
	return err
}

func Synthesize(text, outputPath string, opts Options) (Result, error) {
	text = CleanForTTS(text)
	if text == "" {
		return Result{}, fmt.Errorf("清洗后文本为空，跳过 TTS")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return Result{}, fmt.Errorf("创建输出目录失败: %w", err)
	}

	preset := ResolvePreset(opts.Preset)
	voice := firstNonEmpty(opts.Voice, preset.Voice)
	rate := normalizeEdgeValue(firstNonEmpty(opts.Rate, preset.Rate), preset.Rate)
	pitch := normalizeEdgeValue(firstNonEmpty(opts.Pitch, preset.Pitch), preset.Pitch)
	volume := normalizeEdgeValue(firstNonEmpty(opts.Volume, preset.Volume), preset.Volume)

	logger.Debug("TTS 合成开始",
		zap.String("voice", voice),
		zap.String("rate", rate),
		zap.String("pitch", pitch),
		zap.String("volume", volume),
		zap.Int("chars", len([]rune(text))),
		zap.String("output", outputPath),
	)

	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			logger.Warn("TTS 合成重试",
				zap.Int("attempt", attempt),
				zap.Error(lastErr),
			)
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		args := []string{
			"--voice", voice,
			"--text", text,
			"--write-media", outputPath,
			"--rate", rate,
			"--pitch", pitch,
			"--volume", volume,
		}

		cmd := exec.Command("edge-tts", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			lastErr = fmt.Errorf("edge-tts 合成失败: %w\n输出: %s", err, string(output))
			continue
		}

		if _, err := os.Stat(outputPath); err != nil {
			lastErr = fmt.Errorf("TTS 输出文件未找到: %w", err)
			continue
		}
		if err := trimAudioSilence(outputPath); err != nil {
			logger.Debug("TTS 去静音失败，跳过",
				zap.String("output", outputPath),
				zap.Error(err),
			)
		}
		duration, _ := GetAudioDuration(outputPath)
		preset.Voice = voice
		preset.Rate = rate
		preset.Pitch = pitch
		preset.Volume = volume

		logger.Debug("TTS 合成成功",
			zap.String("output", outputPath),
			zap.Float64("durationSec", duration),
		)

		return Result{
			OutputPath:  outputPath,
			DurationSec: duration,
			Chars:       len([]rune(text)),
			Preset:      preset,
		}, nil
	}

	logger.Error("TTS 合成最终失败",
		zap.Int("maxRetries", maxRetries),
		zap.Error(lastErr),
	)
	return Result{}, fmt.Errorf("edge-tts 合成失败（重试 %d 次后放弃）: %w", maxRetries, lastErr)
}

// TextToSpeechCommentator 使用默认解说预设，供视频生成链路复用。
func TextToSpeechCommentator(text, outputPath string) error {
	return TextToSpeech(text, outputPath)
}

func trimAudioSilence(audioPath string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return err
	}

	origDur, _ := GetAudioDuration(audioPath)

	ext := filepath.Ext(audioPath)
	if ext == "" {
		ext = ".mp3"
	}
	tmpPath := strings.TrimSuffix(audioPath, ext) + ".trim" + ext
	origPath := strings.TrimSuffix(audioPath, ext) + ".orig" + ext

	_ = os.Remove(tmpPath)
	_ = os.Remove(origPath)

	args := []string{
		"-y",
		"-i", audioPath,
		"-af", "silenceremove=start_periods=1:start_duration=0.25:start_threshold=-55dB:stop_periods=1:stop_duration=0.25:stop_threshold=-55dB",
		"-c:a", "libmp3lame",
		"-q:a", "4",
		tmpPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg 去静音失败: %w 输出:%s", err, string(out))
	}

	fi, err := os.Stat(tmpPath)
	if err != nil || fi.Size() == 0 {
		return fmt.Errorf("ffmpeg 去静音输出无效: %s", tmpPath)
	}

	newDur, _ := GetAudioDuration(tmpPath)
	if origDur > 0 && (newDur < 0.5 || newDur < origDur*0.6) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ffmpeg 去静音结果异常: %.3fs -> %.3fs", origDur, newDur)
	}

	if err := os.Rename(audioPath, origPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, audioPath); err != nil {
		_ = os.Rename(origPath, audioPath)
		return err
	}
	_ = os.Remove(origPath)
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func normalizeEdgeValue(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	if strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") {
		return v
	}
	return "+" + v
}

var (
	reMarkdownBold   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reMarkdownItalic = regexp.MustCompile(`\*(.+?)\*`)
	reHashtagLine    = regexp.MustCompile(`(?m)^#\S.*$`)
	reNumberedList   = regexp.MustCompile(`(?m)^\d+[\.\)、]\s*`)
	reBulletPoint    = regexp.MustCompile(`(?m)^[-·•]\s*`)
	reEmoji          = regexp.MustCompile(`[\x{1F300}-\x{1F9FF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}\x{FE00}-\x{FE0F}\x{1F000}-\x{1FAFF}]`)
)

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

func ParseCopywriting(text string) (scenes []string) {
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "[") {
			if idx := strings.Index(trimmed, "]"); idx > 0 {
				content := strings.TrimSpace(trimmed[idx+1:])
				if content != "" {
					scenes = append(scenes, content)
				}
			}
		}
	}

	if len(scenes) == 0 {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				scenes = append(scenes, trimmed)
			}
		}
	}

	if len(scenes) < 5 {
		for len(scenes) < 5 {
			scenes = append(scenes, "")
		}
	}

	return scenes[:5]
}

func EnsureEdgeTTS() error {
	if _, err := exec.LookPath("edge-tts"); err != nil {
		return fmt.Errorf("未找到 edge-tts 命令，请运行: pip install edge-tts")
	}
	return nil
}
