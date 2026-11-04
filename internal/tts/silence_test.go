package tts

import (
	"context"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
)

// 真实的 edge-tts 输出：开头 0.14s 静音、结尾 0.62s 静音。
const sampleLog = `[silencedetect @ 0x7f8] silence_start: 0
[silencedetect @ 0x7f8] silence_end: 0.137167 | silence_duration: 0.137167
[silencedetect @ 0x7f8] silence_start: 0.8045
[silencedetect @ 0x7f8] silence_end: 0.979417 | silence_duration: 0.174917
[silencedetect @ 0x7f8] silence_start: 6.602958
[silencedetect @ 0x7f8] silence_end: 7.224 | silence_duration: 0.621042
`

func TestParseAudioSilence_ReadsHeadAndTail(t *testing.T) {
	got := parseAudioSilence(sampleLog, 7.224)

	if math.Abs(got.LeadSec-0.137167) > 1e-6 {
		t.Fatalf("lead = %v, want 0.137167", got.LeadSec)
	}
	if math.Abs(got.TailSec-0.621042) > 1e-6 {
		t.Fatalf("tail = %v, want 0.621042", got.TailSec)
	}
}

// 句中停顿不算首尾静音：它属于正常语流，不能被当成可回收的空白。
func TestParseAudioSilence_IgnoresMidSentencePauses(t *testing.T) {
	log := `[silencedetect @ 0x7f8] silence_start: 2.4
[silencedetect @ 0x7f8] silence_end: 2.9 | silence_duration: 0.5
`
	got := parseAudioSilence(log, 6)

	if got.LeadSec != 0 || got.TailSec != 0 {
		t.Fatalf("句中停顿被当成首尾静音: %+v", got)
	}
}

// 静音一直持续到文件末尾时没有 silence_end，要用总时长补齐。
func TestParseAudioSilence_OpenEndedTail(t *testing.T) {
	log := `[silencedetect @ 0x7f8] silence_start: 8.905208
`
	got := parseAudioSilence(log, 9.84)

	if math.Abs(got.TailSec-0.934792) > 1e-6 {
		t.Fatalf("tail = %v, want 0.934792", got.TailSec)
	}
}

// 整段静音：不能算出负的可听时长。
func TestParseAudioSilence_AllSilentFile(t *testing.T) {
	log := `[silencedetect @ 0x7f8] silence_start: 0
[silencedetect @ 0x7f8] silence_end: 3 | silence_duration: 3
`
	got := parseAudioSilence(log, 3)

	if got.LeadSec+got.TailSec > 3 {
		t.Fatalf("首尾静音合计 %v 超过音频时长 3s", got.LeadSec+got.TailSec)
	}
}

func TestParseAudioSilence_NoSilence(t *testing.T) {
	if got := parseAudioSilence("", 5); got.LeadSec != 0 || got.TailSec != 0 {
		t.Fatalf("无静音却测出 %+v", got)
	}
}

// 端到端：造一段“两侧静音”的音频，量出来的首尾静音必须落在合理区间。
func TestMeasureAudioSilence_RealFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg 未安装")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "padded.mp3")
	// 0.5s 静音 + 1.5s 正弦 + 0.8s 静音
	cmd := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1.5",
		"-af", "adelay=500|500,apad=pad_dur=0.8",
		"-ar", "24000", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("造样例音频失败: %v\n%s", err, out)
	}

	got, err := MeasureAudioSilence(context.Background(), path)
	if err != nil {
		t.Fatalf("测量失败: %v", err)
	}

	if got.LeadSec < 0.4 || got.LeadSec > 0.6 {
		t.Fatalf("lead = %v，预期约 0.5s", got.LeadSec)
	}
	if got.TailSec < 0.7 || got.TailSec > 0.9 {
		t.Fatalf("tail = %v，预期约 0.8s", got.TailSec)
	}
}

func TestSilenceToFrames_TruncatesTowardZero(t *testing.T) {
	lead, tail := SilenceToFrames(AudioSilence{LeadSec: 0.137, TailSec: 0.621}, 30)

	if lead != 4 || tail != 18 {
		t.Fatalf("lead/tail frames = %d/%d, want 4/18", lead, tail)
	}
}
