package rendercheck

import (
	"fmt"
	"strings"
)

// Format 表示作品形态。不同形态的分辨率与静音预算不同。
type Format string

const (
	FormatMobile  Format = "mobile"
	FormatTV      Format = "tv"
	FormatUnknown Format = "unknown"
)

// Spec 是一期成品视频必须满足的不变量。
type Spec struct {
	Format Format

	// MaxBlankSec 单段黑屏超过该时长即判红。
	MaxBlankSec float64
	// BlankLuma / BlankSat / BlankYMax 共同判定「画面无内容」：
	// 平均亮度低、几乎无色、且没有高光。品牌深色底（Y≈20, SAT≈8.6, YMAX=255）不会被误判。
	BlankLuma float64
	BlankSat  float64
	BlankYMax float64
	// SampleFPS 黑屏判定的采样帧率。
	SampleFPS float64

	// MaxAVDeltaSec 音频与视频时长差的容忍上限。
	MaxAVDeltaSec float64
	// MaxSilenceGapSec 单段静音的上限，超过视为口播空洞。
	MaxSilenceGapSec float64
	// MaxSilenceRatio 静音总时长占视频时长的上限。
	MaxSilenceRatio float64
	// MaxFPSDeviation 帧率容忍偏差。
	MaxFPSDeviation float64

	ExpectWidth  int
	ExpectHeight int
	ExpectFPS    float64

	// NarrationSegments > 0 时校验口播分段数，少于清单即视为缺段。
	NarrationSegments int
}

// DefaultSpec 返回指定形态的默认不变量。
func DefaultSpec(f Format) Spec {
	s := Spec{
		Format:           f,
		MaxBlankSec:      0.5,
		BlankLuma:        18,
		BlankSat:         2,
		BlankYMax:        100,
		SampleFPS:        2,
		MaxAVDeltaSec:    0.15,
		MaxSilenceGapSec: 1.5,
		MaxSilenceRatio:  0.25,
		MaxFPSDeviation:  0.05,
	}
	switch f {
	case FormatTV:
		s.ExpectWidth, s.ExpectHeight, s.ExpectFPS = 1920, 1080, 30
	case FormatMobile:
		s.ExpectWidth, s.ExpectHeight, s.ExpectFPS = 1080, 1920, 30
	}
	return s
}

// IsBlank 判断单帧是否属于「画面无内容」。
func (s Spec) IsBlank(f FrameStat) bool {
	return f.Luma <= s.BlankLuma && f.Sat <= s.BlankSat && f.YMax <= s.BlankYMax
}

// InferFormat 优先按文件名判断形态，文件名无法判断时退回按分辨率判断。
func InferFormat(path string, width, height int) Format {
	name := strings.ToLower(path)
	switch {
	case strings.Contains(name, "_tv"), strings.Contains(name, "-tv"), strings.Contains(name, "tv."):
		return FormatTV
	case strings.Contains(name, "mobile"):
		return FormatMobile
	}
	switch {
	case width > height:
		return FormatTV
	case height > width:
		return FormatMobile
	}
	return FormatUnknown
}

// SpecFor 依据已探测的媒体信息返回默认不变量。
func SpecFor(path string, media MediaInfo) Spec {
	return DefaultSpec(InferFormat(path, media.Width, media.Height))
}

func (s Spec) String() string {
	return fmt.Sprintf("format=%s blank<=%.1fs silenceGap<=%.1fs silenceRatio<=%.0f%% avDelta<=%.0fms",
		s.Format, s.MaxBlankSec, s.MaxSilenceGapSec, s.MaxSilenceRatio*100, s.MaxAVDeltaSec*1000)
}
