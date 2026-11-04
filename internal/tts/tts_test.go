package tts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
)

func fakeTTSTools(t *testing.T, tools map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range tools {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestSynthesizeCanceledBeforeStarting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	output := filepath.Join(t.TempDir(), "not-created", "audio.mp3")
	_, err := SynthesizeContext(ctx, "测试口播", output, Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望取消错误，得到 %v", err)
	}
	if _, err := os.Stat(filepath.Dir(output)); !os.IsNotExist(err) {
		t.Fatalf("取消后仍创建目录: %v", err)
	}
	if _, _, err := TextToSpeechCommentatorCachedContext(ctx, "测试口播"); !errors.Is(err, context.Canceled) {
		t.Fatalf("缓存入口未传递取消: %v", err)
	}
}

func TestRunCommandTimeout(t *testing.T) {
	fakeTTSTools(t, map[string]string{"blocked": "exec /bin/sleep 30"})
	start := time.Now()
	_, err := runCommand(context.Background(), 100*time.Millisecond, "blocked")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("期望超时错误，得到 %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("子进程超时后未及时退出")
	}
}

func TestSynthesizeDeadlineStopsRetry(t *testing.T) {
	fakeTTSTools(t, map[string]string{"edge-tts": "exec /bin/sleep 30"})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := SynthesizeContext(ctx, "测试口播", filepath.Join(t.TempDir(), "audio.mp3"), Options{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("期望总时限错误，得到 %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("父任务超时后仍继续重试")
	}
}

func TestWaitContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- waitContext(ctx, time.Hour) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("期望取消错误，得到 %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("重试或缓存等待未响应取消")
	}
}

func TestAudioDurationValidation(t *testing.T) {
	for _, value := range []string{"0", "-1", "NaN", "+Inf", "N/A", "12.5"} {
		t.Run(value, func(t *testing.T) {
			fakeTTSTools(t, map[string]string{"ffprobe": "printf '%s\\n' '" + value + "'"})
			d, err := GetAudioDurationContext(context.Background(), "test.mp3")
			if value == "12.5" {
				if err != nil || d != 12.5 {
					t.Fatalf("有效时长解析失败: %v %v", d, err)
				}
			} else if err == nil {
				t.Fatalf("接受了无效时长 %q", value)
			}
		})
	}
}

func TestSynthesizeKeepsValidOriginalWhenTrimFails(t *testing.T) {
	fakeTTSTools(t, map[string]string{
		"edge-tts": `while [ "$#" -gt 0 ]; do
  if [ "$1" = "--write-media" ]; then
    shift
    printf 'audio' > "$1"
    exit 0
  fi
  shift
done
exit 1`,
		"ffmpeg":  "exit 1",
		"ffprobe": "printf '12.5\\n'",
	})
	output := filepath.Join(t.TempDir(), "audio.mp3")
	result, err := SynthesizeContext(context.Background(), "测试口播", output, Options{})
	if err != nil || result.DurationSec != 12.5 {
		t.Fatalf("可用原音频不应因去静音失败而丢弃: %+v %v", result, err)
	}
	content, err := os.ReadFile(output)
	if err != nil || string(content) != "audio" {
		t.Fatalf("原音频被破坏: %q %v", content, err)
	}
}

func TestCacheLockWaitCanceled(t *testing.T) {
	oldRoot := config.GetProjectRoot()
	config.SetProjectRoot(filepath.Join(t.TempDir(), "backend"))
	t.Cleanup(func() { config.SetProjectRoot(oldRoot) })
	fakeTTSTools(t, map[string]string{
		"edge-tts": `while [ "$#" -gt 0 ]; do
  if [ "$1" = "--write-media" ]; then
    shift
    printf 'audio' > "$1"
    exit 0
  fi
  shift
done
exit 1`,
		"ffmpeg":  "exit 1",
		"ffprobe": "printf '12.5\\n'",
	})
	_, abs, err := TextToSpeechCommentatorCachedContext(context.Background(), "缓存等待测试")
	if err != nil {
		t.Fatal(err)
	}
	// 将已知缓存路径换成另一个请求持有的锁，模拟缓存竞争。
	if err := os.Rename(abs, abs+".lock"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, _, err = TextToSpeechCommentatorCachedContext(ctx, "缓存等待测试")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("缓存等待未响应取消: %v", err)
	}
	if _, err := os.Stat(abs + ".lock"); err != nil {
		t.Fatalf("取消错误地删除了其他请求的锁: %v", err)
	}
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		t.Fatalf("取消后仍发布了音频缓存: %v", err)
	}
}
