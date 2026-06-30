package tts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

func TTSConcurrency() int {
	v := strings.TrimSpace(os.Getenv("TTS_CONCURRENCY"))
	if v == "" {
		return 3
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 3
	}
	if n < 1 {
		return 1
	}
	if n > 16 {
		return 16
	}
	return n
}

func TextToSpeechCommentatorCached(text string) (publicPath string, absPath string, err error) {
	cleaned := CleanForTTS(text)
	if cleaned == "" {
		return "", "", fmt.Errorf("清洗后文本为空，跳过 TTS")
	}

	preset := ResolvePreset(DefaultPresetID)
	voice := preset.Voice
	rate := normalizeEdgeValue(preset.Rate, preset.Rate)
	pitch := normalizeEdgeValue(preset.Pitch, preset.Pitch)
	volume := normalizeEdgeValue(preset.Volume, preset.Volume)

	keyParts := []string{
		"preset=" + DefaultPresetID,
		"voice=" + voice,
		"rate=" + rate,
		"pitch=" + pitch,
		"volume=" + volume,
		"text=" + cleaned,
	}
	sum := sha256.Sum256([]byte(strings.Join(keyParts, "\n")))
	base := hex.EncodeToString(sum[:])

	rendererDir := config.GetRendererDir()
	rel := filepath.ToSlash(filepath.Join("voiceover_cache", base+".mp3"))
	abs := filepath.Join(rendererDir, "public", filepath.FromSlash(rel))

	if _, err := os.Stat(abs); err == nil {
		return rel, abs, nil
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return "", "", fmt.Errorf("创建缓存目录失败: %w", err)
	}

	lock := abs + ".lock"
	acquired := false
	for attempt := 0; attempt < 40; attempt++ {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			_, _ = f.WriteString(time.Now().Format(time.RFC3339Nano))
			_ = f.Close()
			acquired = true
			break
		}

		if fi, statErr := os.Stat(lock); statErr == nil {
			if time.Since(fi.ModTime()) > 10*time.Minute {
				_ = os.Remove(lock)
			}
		}
		if _, okErr := os.Stat(abs); okErr == nil {
			return rel, abs, nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !acquired {
		if _, err := os.Stat(abs); err == nil {
			return rel, abs, nil
		}
		return "", "", fmt.Errorf("获取 TTS 缓存锁失败: %s", lock)
	}
	defer func() { _ = os.Remove(lock) }()

	if _, err := os.Stat(abs); err == nil {
		return rel, abs, nil
	}

	tmp := fmt.Sprintf("%s.tmp.%d.%d", abs, time.Now().UnixNano(), rand.Intn(1_000_000))
	_ = os.Remove(tmp)

	logger.Debug("TTS 缓存 miss，开始合成",
		zap.Int("chars", len([]rune(cleaned))),
		zap.String("output", abs),
	)

	if err := TextToSpeechCommentator(cleaned, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", "", err
	}

	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		if _, statErr := os.Stat(abs); statErr == nil {
			return rel, abs, nil
		}
		return "", "", fmt.Errorf("写入缓存音频失败: %w", err)
	}

	return rel, abs, nil
}
