package debate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/tts"
	"go.uber.org/zap"
)

const (
	debateVoiceBull   = tts.Yunyang  // 乐观派：男声，沉稳自信
	debateVoiceBear   = tts.Xiaoxiao // 谨慎派：女声，知性冷静
	debateSpeechRate  = "+20%"       // 高速攻防节奏，语速加快 20%
	debatePadFrames   = 8            // 快速切换无需过多 padding
)

// GenerateAudio 为每轮发言生成 TTS 音频文件，存到 Remotion public/ 目录。
// 失败时整批回滚已生成的 mp3（避免 Remotion 引用缺失文件）。
func GenerateAudio(taskID string, script Script) ([]AudioTurn, error) {
	if taskID == "" {
		return nil, fmt.Errorf("taskID 不能为空")
	}
	if len(script.Turns) == 0 {
		return nil, fmt.Errorf("script.Turns 为空")
	}

	rendererDir := config.GetRendererDir()
	audioDir := filepath.Join(rendererDir, "public", "debate", taskID)
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		return nil, fmt.Errorf("创建音频目录失败: %w", err)
	}

	results := make([]AudioTurn, 0, len(script.Turns))
	generated := make([]string, 0, len(script.Turns))

	rollback := func() {
		for _, p := range generated {
			_ = os.Remove(p)
		}
		_ = os.Remove(audioDir)
	}

	logger.Info("辩论 TTS 合成开始",
		zap.String("taskId", taskID),
		zap.Int("turns", len(script.Turns)),
	)

	for i, turn := range script.Turns {
		voice := debateVoiceBear
		if turn.Speaker == Bull {
			voice = debateVoiceBull
		}

		relPath := fmt.Sprintf("debate/%s/turn_%d.mp3", taskID, i)
		absPath := filepath.Join(rendererDir, "public", relPath)

		logger.Debug("辩论 TTS 合成轮次",
			zap.String("taskId", taskID),
			zap.Int("index", i),
			zap.String("speaker", string(turn.Speaker)),
			zap.Int("textLen", len(turn.Text)),
			zap.String("voice", string(voice)),
		)

		if err := tts.TextToSpeechRate(turn.Text, absPath, voice, debateSpeechRate); err != nil {
			logger.Error("辩论 TTS 合成失败", zap.String("taskId", taskID), zap.Int("turn", i), zap.Error(err))
			rollback()
			return nil, fmt.Errorf("第 %d 轮 TTS 失败: %w", i, err)
		}
		generated = append(generated, absPath)

		duration, err := tts.GetAudioDuration(absPath)
		if err != nil {
			logger.Error("辩论 TTS 获取时长失败", zap.String("taskId", taskID), zap.Int("turn", i), zap.Error(err))
			rollback()
			return nil, fmt.Errorf("第 %d 轮获取时长失败: %w", i, err)
		}

		results = append(results, AudioTurn{
			Index:       i,
			Speaker:     turn.Speaker,
			Text:        turn.Text,
			AudioFile:   relPath,
			DurationSec: duration,
			Frames:      framesForAudio(duration),
		})

		logger.Debug("辩论 TTS 轮次完成",
			zap.String("taskId", taskID),
			zap.Int("index", i),
			zap.Float64("durationSec", duration),
			zap.Int("frames", framesForAudio(duration)),
		)
	}

	logger.Info("辩论 TTS 合成完成",
		zap.String("taskId", taskID),
		zap.Int("turns", len(results)),
		zap.Float64("totalDurationSec", totalDuration(results)),
	)

	return results, nil
}

// framesForAudio 音频时长 → 帧数（@FPS），加 debatePadFrames 帧 padding。
func framesForAudio(sec float64) int {
	return int(sec*float64(config.FPS)) + debatePadFrames
}

func totalDuration(turns []AudioTurn) float64 {
	var total float64
	for _, t := range turns {
		total += t.DurationSec
	}
	return total
}
