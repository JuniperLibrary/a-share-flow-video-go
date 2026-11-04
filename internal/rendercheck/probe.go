package rendercheck

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// MediaInfo 是从 ffprobe 读到的客观媒体信息。
type MediaInfo struct {
	Path       string  `json:"path"`
	VideoSec   float64 `json:"videoSec"`
	AudioSec   float64 `json:"audioSec"`
	HasAudio   bool    `json:"hasAudio"`
	AudioCodec string  `json:"audioCodec"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FPS        float64 `json:"fps"`
	FrameCount int     `json:"frameCount"`
}

// AVDiff 返回音频与视频的时长差（秒）。
func (m MediaInfo) AVDiff() float64 {
	if !m.HasAudio {
		return 0
	}
	d := m.AudioSec - m.VideoSec
	if d < 0 {
		return -d
	}
	return d
}

type probeJSON struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		RFrameRate   string `json:"r_frame_rate"`
		AvgFrameRate string `json:"avg_frame_rate"`
		Duration     string `json:"duration"`
		NbFrames     string `json:"nb_frames"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// ProbeFile 调用 ffprobe 读取媒体信息。
func ProbeFile(path string) (MediaInfo, error) {
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-show_streams",
		"-show_format",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return MediaInfo{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}
	return parseProbeJSON(path, out)
}

func parseProbeJSON(path string, data []byte) (MediaInfo, error) {
	var raw probeJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return MediaInfo{}, fmt.Errorf("解析 ffprobe 输出: %w", err)
	}

	info := MediaInfo{Path: path}
	info.VideoSec = parseFloat(raw.Format.Duration)

	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			info.Width = s.Width
			info.Height = s.Height
			info.FPS = parseRate(s.RFrameRate)
			if info.FPS == 0 {
				info.FPS = parseRate(s.AvgFrameRate)
			}
			info.FrameCount = parseInt(s.NbFrames)
			if d := parseFloat(s.Duration); d > 0 {
				info.VideoSec = d
			}
		case "audio":
			info.HasAudio = true
			info.AudioCodec = s.CodecName
			info.AudioSec = parseFloat(s.Duration)
		}
	}
	if info.Width == 0 || info.VideoSec <= 0 {
		return MediaInfo{}, fmt.Errorf("%s 不是可解析的视频文件", path)
	}
	return info, nil
}

// parseRate 解析 "30/1" 形式的帧率。
func parseRate(s string) float64 {
	if s == "" || s == "N/A" {
		return 0
	}
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		return parseFloat(s)
	}
	n, d := parseFloat(num), parseFloat(den)
	if d == 0 {
		return 0
	}
	return n / d
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

func parseInt(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return v
}
