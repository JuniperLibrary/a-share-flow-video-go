package rendercheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// 注入式证伪：故意制造缺陷，检查器必须报红。
// 这些用例需要 ffmpeg，缺失时跳过，但 CI 上应视为必跑。

type clipOpts struct {
	dur       float64
	size      string
	audio     bool
	blackFrom float64
	blackTo   float64
	muteFrom  float64
	muteTo    float64
}

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		// CI 里静默跳过会让门禁失效：注入式用例一条没跑也会显示绿灯。
		if os.Getenv("RENDERCHECK_REQUIRE_FFMPEG") != "" {
			t.Fatalf("RENDERCHECK_REQUIRE_FFMPEG 已设置，但找不到 ffmpeg：注入式证伪无法执行")
		}
		t.Skip("ffmpeg 未安装，跳过注入式用例")
	}
}

func makeClip(t *testing.T, dir, name string, o clipOpts) string {
	t.Helper()
	out := filepath.Join(dir, name)
	args := []string{"-y", "-v", "error",
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=0x203040:s=%s:r=30", o.size)}
	if o.audio {
		args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000")
	}
	args = append(args, "-t", fmt.Sprintf("%.2f", o.dur))

	if o.blackTo > o.blackFrom {
		args = append(args, "-vf", fmt.Sprintf(
			"drawbox=x=0:y=0:w=iw:h=ih:color=black:t=fill:enable='between(t,%.2f,%.2f)'",
			o.blackFrom, o.blackTo))
	}
	if o.audio && o.muteTo > o.muteFrom {
		args = append(args, "-af", fmt.Sprintf(
			"volume=enable='between(t,%.2f,%.2f)':volume=0", o.muteFrom, o.muteTo))
	}
	args = append(args, "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p")
	if o.audio {
		args = append(args, "-c:a", "aac", "-shortest")
	}
	args = append(args, out)

	if outBytes, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("生成测试片段失败: %v\n%s", err, outBytes)
	}
	return out
}

func TestCheckFile_CleanClipPasses(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	path := makeClip(t, dir, "clean_mobile.mp4", clipOpts{dur: 4, size: "1080x1920", audio: true})

	report, err := CheckFile(path)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if !report.OK {
		t.Fatalf("干净片段应判绿，实际违规: %+v（指标 %+v）", report.Violations, report.Metrics)
	}
}

func TestCheckFile_InjectedBlackFrameFails(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	path := makeClip(t, dir, "blank_mobile.mp4", clipOpts{
		dur: 4, size: "1080x1920", audio: true, blackFrom: 1, blackTo: 2.4,
	})

	report, err := CheckFile(path)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if !hasRule(report, RuleBlankFrame) {
		t.Fatalf("1.4s 黑屏未被抓到: %+v", report.Violations)
	}
	if report.Metrics.BlankCount == 0 {
		t.Fatal("blank count = 0，黑屏检测未生效")
	}
}

func TestCheckFile_InjectedSilenceFails(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	path := makeClip(t, dir, "silence_mobile.mp4", clipOpts{
		dur: 4, size: "1080x1920", audio: true, muteFrom: 0.6, muteTo: 2.6,
	})

	report, err := CheckFile(path)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if !hasRule(report, RuleSilenceGap) {
		t.Fatalf("2.6s 静音空洞未被抓到: %+v（指标 %+v）", report.Violations, report.Metrics)
	}
}

func TestCheckFile_NoAudioFails(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	path := makeClip(t, dir, "noaudio_mobile.mp4", clipOpts{dur: 3, size: "1080x1920", audio: false})

	report, err := CheckFile(path)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if !hasRule(report, RuleAudioMissing) {
		t.Fatalf("缺音轨未被抓到: %+v", report.Violations)
	}
}

func TestCheckFile_WrongResolutionFails(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	path := makeClip(t, dir, "wrong_mobile.mp4", clipOpts{dur: 3, size: "640x480", audio: true})

	report, err := CheckFile(path)
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if !hasRule(report, RuleFormatMismatch) {
		t.Fatalf("分辨率不符未抓到: %+v", report.Violations)
	}
}
