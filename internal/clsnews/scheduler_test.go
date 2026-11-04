package clsnews

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
)

func awaitSchedulerSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("等待调度器状态超时")
	}
}

func TestSchedulerStopWaitsBeforeRestart(t *testing.T) {
	logger.Get()
	s := NewNewsScheduler()
	stopping := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		s.Stop()
	})
	s.start(func() {
		<-s.stopCh
		close(stopping)
		<-release
	})
	oldStop := s.stopCh
	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()
	awaitSchedulerSignal(t, stopping)
	if s.IsRunning() {
		t.Fatal("停止期间不应再接受分类任务")
	}
	if queued, dropped := s.enqueueNews([]CLSNews{{ID: 1}}); queued != 0 || dropped != 1 {
		t.Fatalf("停止后仍入队: queued=%d dropped=%d", queued, dropped)
	}
	restarted := make(chan struct{})
	go func() {
		s.start(func() { <-s.stopCh })
		close(restarted)
	}()
	select {
	case <-stopped:
		t.Fatal("旧任务退出前 Stop 已返回")
	case <-restarted:
		t.Fatal("旧任务退出前发生重启")
	case <-time.After(20 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	awaitSchedulerSignal(t, stopped)
	awaitSchedulerSignal(t, restarted)
	if s.stopCh == oldStop {
		t.Fatal("重启应创建新的停止通道")
	}
	select {
	case <-s.stopCh:
		t.Fatal("新一轮停止通道被误关闭")
	default:
	}
	s.Stop()
}

func TestSchedulerImmediateAndConcurrentStop(t *testing.T) {
	logger.Get()
	s := NewNewsScheduler()
	for range 100 {
		var exited atomic.Int32
		s.start(func() {
			<-s.stopCh
			exited.Add(1)
		})
		var stops sync.WaitGroup
		for range 4 {
			stops.Add(1)
			go func() {
				defer stops.Done()
				s.Stop()
			}()
		}
		stops.Wait()
		if exited.Load() != 1 {
			t.Fatal("Stop 未等待本轮任务退出")
		}
	}
}

func TestSchedulerConcurrentStartStop(t *testing.T) {
	logger.Get()
	s := NewNewsScheduler()
	var calls sync.WaitGroup
	for range 20 {
		calls.Add(1)
		go func() {
			defer calls.Done()
			for range 10 {
				s.start(func() { <-s.stopCh })
				s.Stop()
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		calls.Wait()
		close(done)
	}()
	awaitSchedulerSignal(t, done)
	s.Stop()
}

func TestSchedulerStopWaitsForClassifierPool(t *testing.T) {
	logger.Get()
	s := NewNewsScheduler()
	// 仅启动空队列工作池，避免测试触发网络或数据库访问。
	s.classifierOnce.Do(func() {
		s.classifier = NewAINewsClassifier(config.AIConfig{APIKey: "test-only"})
	})
	for range 100 {
		s.start(s.classifyWorker)
		s.Stop()
	}
}
