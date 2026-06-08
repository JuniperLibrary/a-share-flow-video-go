package debate

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

type ProbeResult struct {
	HasAudio    bool     `json:"hasAudio"`
	AudioCodec  string   `json:"audioCodec"`
	AudioSec    float64  `json:"audioDurationSec"`
	ExpectedSec float64  `json:"expectedDurationSec"`
	DiffSec     float64  `json:"diffSec"`
	Warnings    []string `json:"warnings"`
	OK          bool     `json:"ok"`
}

func ProbeVideo(videoPath string, expectedSec float64) (*ProbeResult, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_streams",
		"-of", "json",
		videoPath,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}
	var raw struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Duration  string `json:"duration"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("ffprobe json parse: %w", err)
	}

	r := &ProbeResult{ExpectedSec: expectedSec}
	for _, s := range raw.Streams {
		if s.CodecType != "audio" {
			continue
		}
		r.HasAudio = true
		r.AudioCodec = s.CodecName
		r.AudioSec, _ = strconv.ParseFloat(s.Duration, 64)
	}
	if !r.HasAudio {
		r.Warnings = append(r.Warnings, "视频中未检测到音频流")
	}
	if r.HasAudio && expectedSec > 0 {
		r.DiffSec = r.AudioSec - expectedSec
		if r.DiffSec < -2 || r.DiffSec > 2 {
			r.Warnings = append(r.Warnings,
				fmt.Sprintf("音频时长 %.2fs 与预期 %.2fs 相差 %.2fs", r.AudioSec, expectedSec, r.DiffSec))
		}
	}
	r.OK = r.HasAudio && len(r.Warnings) == 0
	return r, nil
}
