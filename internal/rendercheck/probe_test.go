package rendercheck

import (
	"os"
	"path/filepath"
	"testing"
)

const probeSample = `{
  "streams": [
    {
      "codec_type": "video",
      "codec_name": "h264",
      "width": 1080,
      "height": 1920,
      "r_frame_rate": "30/1",
      "avg_frame_rate": "30/1",
      "duration": "130.033333",
      "nb_frames": "3901"
    },
    {
      "codec_type": "audio",
      "codec_name": "aac",
      "duration": "130.090667"
    }
  ],
  "format": { "duration": "130.090667" }
}`

func TestParseProbeJSON_ReadsVideoAndAudio(t *testing.T) {
	info, err := parseProbeJSON("x_mobile.mp4", []byte(probeSample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if info.Width != 1080 || info.Height != 1920 {
		t.Fatalf("size = %dx%d", info.Width, info.Height)
	}
	if info.FPS != 30 {
		t.Fatalf("fps = %.2f, want 30", info.FPS)
	}
	if !info.HasAudio || info.AudioCodec != "aac" {
		t.Fatalf("audio = %v/%s", info.HasAudio, info.AudioCodec)
	}
	if diff := info.AVDiff(); diff < 0.05 || diff > 0.06 {
		t.Fatalf("av diff = %.3f, want ~0.057", diff)
	}
	if info.FrameCount != 3901 {
		t.Fatalf("frames = %d", info.FrameCount)
	}
}

func TestParseProbeJSON_RejectsNonVideo(t *testing.T) {
	if _, err := parseProbeJSON("x.mp4", []byte(`{"streams":[]}`)); err == nil {
		t.Fatal("expected error for video without streams")
	}
}

func TestParseRate_HandlesFractions(t *testing.T) {
	cases := map[string]float64{
		"30/1":       30,
		"30000/1001": 29.97003,
		"N/A":        0,
		"":           0,
	}
	for in, want := range cases {
		got := parseRate(in)
		if diff := got - want; diff > 0.001 || diff < -0.001 {
			t.Fatalf("parseRate(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestInferFormat_FromFilenameAndShape(t *testing.T) {
	cases := []struct {
		path          string
		width, height int
		want          Format
	}{
		{"output/2026-08-14/全天_tick_tv.mp4", 1920, 1080, FormatTV},
		{"output/2026-08-20/全天_tick_mobile.mp4", 1080, 1920, FormatMobile},
		{"unknown.mp4", 1920, 1080, FormatTV},
		{"unknown.mp4", 1080, 1920, FormatMobile},
		{"unknown.mp4", 1000, 1000, FormatUnknown},
	}
	for _, c := range cases {
		if got := InferFormat(c.path, c.width, c.height); got != c.want {
			t.Fatalf("InferFormat(%q,%d,%d) = %s, want %s", c.path, c.width, c.height, got, c.want)
		}
	}
}

func TestCollectVideos_SkipsBackupsAndNonVideo(t *testing.T) {
	dir := t.TempDir()
	day := filepath.Join(dir, "2026-08-14")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"全天_tick_mobile.mp4",
		"全天_tick_tv.mp4",
		"全天_tick_mobile.mp4.bak_before_rerun",
		"notes.txt",
	} {
		if err := os.WriteFile(filepath.Join(day, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := CollectVideos([]string{dir})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("files = %v, want 2", got)
	}
	if filepath.Base(got[0]) != "全天_tick_mobile.mp4" || filepath.Base(got[1]) != "全天_tick_tv.mp4" {
		t.Fatalf("unexpected order: %v", got)
	}
}

func TestCollectVideos_MissingPathErrors(t *testing.T) {
	if _, err := CollectVideos([]string{"/definitely/not/here"}); err == nil {
		t.Fatal("expected error for missing path")
	}
}
