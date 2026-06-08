package debate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"go.uber.org/zap"
)

type RenderTask struct {
	ID         string
	Status     string
	Progress   string
	Err        string
	OutputPath string
	Probe      *ProbeResult
	mu         sync.Mutex
	cancel     context.CancelFunc
}

type RenderManager struct {
	mu         sync.RWMutex
	tasks      map[string]*RenderTask
	OnComplete func(taskID string, probe *ProbeResult)
}

var GlobalRenderManager = &RenderManager{
	tasks: make(map[string]*RenderTask),
}

func (m *RenderManager) Start(taskID string, script Script, audioTurns []AudioTurn, format string) {
	outputDir := filepath.Join(config.GetOutputDir(), "debate")
	outputPath := filepath.Join(outputDir, taskID+".mp4")

	task := &RenderTask{
		ID:         taskID,
		Status:     "pending",
		Progress:   "准备中...",
		OutputPath: outputPath,
	}

	ctx, cancel := context.WithCancel(context.Background())
	task.cancel = cancel

	m.mu.Lock()
	m.tasks[taskID] = task
	m.mu.Unlock()

	go m.runRender(ctx, task, script, audioTurns, format)
}

func (m *RenderManager) runRender(ctx context.Context, task *RenderTask, script Script, audioTurns []AudioTurn, format string) {
	select {
	case <-ctx.Done():
		task.mu.Lock()
		task.Status = "cancelled"
		task.Progress = "已取消"
		task.mu.Unlock()
		return
	default:
	}

	task.mu.Lock()
	task.Status = "rendering"
	task.Progress = "渲染中..."
	task.mu.Unlock()

	type renderResult struct {
		path  string
		probe *ProbeResult
		err   error
	}

	resultCh := make(chan renderResult, 1)
	go func() {
		path, probe, err := Render(task.ID, script, audioTurns, task.OutputPath, format)
		resultCh <- renderResult{path, probe, err}
	}()

	select {
	case <-ctx.Done():
		task.mu.Lock()
		task.Status = "cancelled"
		task.Progress = "已取消"
		task.mu.Unlock()
		_ = os.Remove(task.OutputPath)
		logger.Info("debate 渲染已取消", zap.String("taskId", task.ID))
	case res := <-resultCh:
		if res.err != nil {
			task.mu.Lock()
			task.Status = "error"
			task.Err = res.err.Error()
			task.mu.Unlock()
			return
		}
		task.mu.Lock()
		task.Status = "done"
		task.Progress = "渲染完成"
		task.Probe = res.probe
		task.mu.Unlock()
		logger.Info("debate 异步渲染完成", zap.String("taskId", task.ID), zap.String("output", task.OutputPath))
		if m.OnComplete != nil {
			m.OnComplete(task.ID, res.probe)
		}
	}
}

func (m *RenderManager) GetStatus(taskID string) *RenderTask {
	m.mu.RLock()
	task, ok := m.tasks[taskID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	task.mu.Lock()
	defer task.mu.Unlock()
	return &RenderTask{
		ID:         task.ID,
		Status:     task.Status,
		Progress:   task.Progress,
		Err:        task.Err,
		OutputPath: task.OutputPath,
		Probe:      task.Probe,
	}
}

func (m *RenderManager) Cancel(taskID string) error {
	m.mu.RLock()
	task, ok := m.tasks[taskID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("任务不存在")
	}
	task.mu.Lock()
	defer task.mu.Unlock()
	if task.Status == "done" || task.Status == "cancelled" {
		return nil
	}
	if task.cancel != nil {
		task.cancel()
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		task.mu.Lock()
		if task.Status != "done" {
			task.Status = "cancelled"
			task.Progress = "已取消"
		}
		task.mu.Unlock()
	}()
	return nil
}
