package web

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/copy"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/hotnews"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tick"
)

type tickGenerateTask struct {
	ID         string
	Key        string
	Date       string
	Session    string
	CopyMode   string
	Format     string
	Status     string
	Progress   string
	Logs       []string
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	MobilePath string
	TVPath     string
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

type tickGenerateRequest struct {
	Date     string `json:"date"`
	Session  string `json:"session"`
	CopyMode string `json:"copy_mode"`
	Format   string `json:"format"`
}

var (
	tickGenerateTasks     = make(map[string]*tickGenerateTask)
	tickGenerateActiveKey = make(map[string]string)
	tickGenerateTasksMu   sync.RWMutex
)

func normalizeTickGenerateRequest(body *tickGenerateRequest) {
	if body.Date == "" {
		body.Date = time.Now().Format("2006-01-02")
	}
	if body.Session == "" {
		body.Session = "full"
	}
	if body.CopyMode == "" {
		body.CopyMode = "ai"
	}
	if body.Format == "" {
		body.Format = "mobile"
	}
}

func validateTickGenerateRequest(body tickGenerateRequest) error {
	if _, ok := config.SessionConfigs[body.Session]; !ok {
		return fmt.Errorf("不支持的 session: %s", body.Session)
	}
	switch body.CopyMode {
	case "ai", "template":
	default:
		return fmt.Errorf("不支持的 copy_mode: %s", body.CopyMode)
	}
	switch body.Format {
	case "mobile", "tv", "all":
	default:
		return fmt.Errorf("不支持的视频格式: %s", body.Format)
	}
	return nil
}

func buildTickGenerateTaskKey(body tickGenerateRequest) string {
	return strings.Join([]string{body.Date, body.Session, body.CopyMode, body.Format}, "|")
}

func newTickGenerateTask(body tickGenerateRequest) *tickGenerateTask {
	now := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	return &tickGenerateTask{
		ID:        fmt.Sprintf("tick_%s_%d", strings.ReplaceAll(body.Date, "-", ""), now.UnixNano()),
		Key:       buildTickGenerateTaskKey(body),
		Date:      body.Date,
		Session:   body.Session,
		CopyMode:  body.CopyMode,
		Format:    body.Format,
		Status:    "pending",
		Progress:  "等待启动...",
		Logs:      []string{},
		CreatedAt: now,
		UpdatedAt: now,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func startOrReuseTickGenerateTask(body tickGenerateRequest) (*tickGenerateTask, bool) {
	key := buildTickGenerateTaskKey(body)

	tickGenerateTasksMu.Lock()
	defer tickGenerateTasksMu.Unlock()

	if taskID, ok := tickGenerateActiveKey[key]; ok {
		if task, exists := tickGenerateTasks[taskID]; exists && task != nil && task.isActive() {
			return task, true
		}
		delete(tickGenerateActiveKey, key)
	}

	task := newTickGenerateTask(body)
	tickGenerateTasks[task.ID] = task
	tickGenerateActiveKey[key] = task.ID
	go runTickGenerateTask(task)
	return task, false
}

func getTickGenerateTask(taskID string) (*tickGenerateTask, bool) {
	tickGenerateTasksMu.RLock()
	defer tickGenerateTasksMu.RUnlock()
	task, ok := tickGenerateTasks[taskID]
	return task, ok
}

func releaseTickGenerateTaskKey(task *tickGenerateTask) {
	tickGenerateTasksMu.Lock()
	defer tickGenerateTasksMu.Unlock()
	if taskID, ok := tickGenerateActiveKey[task.Key]; ok && taskID == task.ID {
		delete(tickGenerateActiveKey, task.Key)
	}
}

func (t *tickGenerateTask) isActive() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Status == "pending" || t.Status == "running"
}

func (t *tickGenerateTask) setStatus(status, progress string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Status = status
	t.Progress = progress
	t.UpdatedAt = time.Now()
}

func (t *tickGenerateTask) appendLog(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Logs = append(t.Logs, text)
	t.UpdatedAt = time.Now()
}

func (t *tickGenerateTask) fail(err error) {
	msg := err.Error()
	t.mu.Lock()
	t.Status = "error"
	t.Progress = msg
	t.Error = msg
	t.Logs = append(t.Logs, "❌ "+msg)
	t.UpdatedAt = time.Now()
	if t.cancel != nil {
		t.cancel()
	}
	t.mu.Unlock()
	releaseTickGenerateTaskKey(t)
}

func (t *tickGenerateTask) complete(progress string) {
	t.mu.Lock()
	t.Status = "done"
	t.Progress = progress
	t.Error = ""
	t.UpdatedAt = time.Now()
	if t.cancel != nil {
		t.cancel()
	}
	t.mu.Unlock()
	releaseTickGenerateTaskKey(t)
}

// Cancel 中断正在进行的渲染。若任务已结束则返回错误。
func (t *tickGenerateTask) Cancel() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch t.Status {
	case "done", "error", "cancelled":
		return fmt.Errorf("任务已结束，当前状态=%s", t.Status)
	}
	t.Status = "cancelled"
	t.Progress = "用户已取消"
	t.Logs = append(t.Logs, "🛑 用户取消生成")
	t.UpdatedAt = time.Now()
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

func (t *tickGenerateTask) response(existing bool) map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	logs := append([]string(nil), t.Logs...)
	cancellable := t.Status == "pending" || t.Status == "running"
	resp := map[string]any{
		"task_id":     t.ID,
		"date":        t.Date,
		"session":     t.Session,
		"copy_mode":   t.CopyMode,
		"format":      t.Format,
		"status":      t.Status,
		"progress":    t.Progress,
		"logs":        logs,
		"existing":    existing,
		"cancellable": cancellable,
	}
	if t.Error != "" {
		resp["error"] = t.Error
	}
	if t.MobilePath != "" {
		resp["mobile_path"] = t.MobilePath
		resp["mobile_url"] = "/output/" + t.Date + "/" + filepath.Base(t.MobilePath)
	}
	if t.TVPath != "" {
		resp["tv_path"] = t.TVPath
		resp["tv_url"] = "/output/" + t.Date + "/" + filepath.Base(t.TVPath)
	}
	return resp
}

func isCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func runTickGenerateTask(task *tickGenerateTask) {
	task.setStatus("running", "准备生成...")
	defer func() {
		// 保证任何 return 路径都会释放 key（fail/complete 已释放；这里只处理异常路径）
		if task.cancel != nil {
			task.cancel()
		}
	}()

	sessCfg := config.SessionConfigs[task.Session]
	task.appendLog(fmt.Sprintf("📅 日期: %s, 时段: %s, 文案: %s",
		task.Date, sessCfg.TitleSuffix,
		map[string]string{"ai": "AI生成", "template": "模板"}[task.CopyMode]))

	if pts, err := tick.LoadTickCSV(task.Date, task.Session); err == nil {
		timeSet := make(map[string]bool)
		sectorSet := make(map[string]bool)
		for _, p := range pts {
			timeSet[p.Time] = true
			sectorSet[p.Name] = true
		}
		task.appendLog(fmt.Sprintf("📊 数据预览: %d条记录, %d个时间点, %d个板块", len(pts), len(timeSet), len(sectorSet)))
	} else {
		task.appendLog(fmt.Sprintf("⚠️ 数据预览失败: %v", err))
	}

	if isCancelled(task.ctx) {
		task.appendLog("🛑 生成已取消（数据准备后）")
		releaseTickGenerateTaskKey(task)
		return
	}

	points, pErr := tick.LoadTickCSV(task.Date, task.Session)
	var copywriteText string
	cwType := "template_tick"
	if pErr != nil || len(points) == 0 {
		task.appendLog("⚠️ 无 tick 数据，跳过文案生成")
	} else {
		sectors := tick.PointsToSectors(points)

		if daily, fetchErr := fetcher.FetchSectorsAllDaily(task.Date); fetchErr == nil && len(daily) > 0 {
			sectors = fetcher.MergeSectorsWithDaily(sectors, daily, 5)
			task.appendLog(fmt.Sprintf("📊 合并全板块行情(实时抓取 15:00 收盘快照)：tick=%d，日线=%d，合并后=%d",
				len(tick.PointsToSectors(points)), len(daily), len(sectors)))
		} else if db, dbErr := storage.Get(); dbErr == nil {
			if daily, loadErr := db.LoadSectorsAll(task.Date); loadErr == nil && len(daily) > 0 {
				sectors = fetcher.MergeSectorsWithDaily(sectors, daily, 5)
				task.appendLog(fmt.Sprintf("📊 在线抓取失败，降级 sectors_all 表(可能非收盘)：tick=%d，日线=%d，合并后=%d",
					len(tick.PointsToSectors(points)), len(daily), len(sectors)))
			}
		}
		task.setStatus("running", "生成文案中...")
		if task.CopyMode == "ai" {
			task.appendLog("🤖 AI 文案生成中...")
			prevPrediction := loadPrevCopywriting(task.Date, task.Session)
			var aiErr error
			copywriteText, aiErr = copy.GenerateCopywritingAI(sectors, task.Date, task.Session, prevPrediction)
			if aiErr != nil {
				task.appendLog(fmt.Sprintf("⚠️ AI 生成失败，降级模板: %v", aiErr))
				copywriteText = copy.GenerateCopywriting(sectors, task.Date, task.Session)
			} else {
				cwType = "ai_tick"
			}
		} else {
			task.appendLog("📝 模板文案生成中...")
			copywriteText = copy.GenerateCopywriting(sectors, task.Date, task.Session)
		}
	}

	if isCancelled(task.ctx) {
		task.appendLog("🛑 生成已取消（文案阶段）")
		releaseTickGenerateTaskKey(task)
		return
	}

	renderMobile := task.Format == "all" || task.Format == "mobile"
	renderTV := task.Format == "all" || task.Format == "tv"

	var newsPagesMobile, newsPagesTV []hotnews.NewsPage
	if renderMobile {
		pages, err := hotnews.LoadForVideo(task.Date, "mobile")
		if err != nil {
			task.appendLog(fmt.Sprintf("⚠️ 新闻加载失败 (mobile): %v", err))
		} else if len(pages) > 0 {
			newsPagesMobile = pages
			task.appendLog(fmt.Sprintf("📰 新闻加载完成 (mobile): %d页", len(pages)))
		}
	}
	if renderTV {
		pages, err := hotnews.LoadForVideo(task.Date, "tv")
		if err != nil {
			task.appendLog(fmt.Sprintf("⚠️ 新闻加载失败 (tv): %v", err))
		} else if len(pages) > 0 {
			newsPagesTV = pages
			task.appendLog(fmt.Sprintf("📰 新闻加载完成 (tv): %d页", len(pages)))
		}
	}

	if renderMobile {
		if isCancelled(task.ctx) {
			task.appendLog("🛑 生成已取消（渲染前）")
			releaseTickGenerateTaskKey(task)
			return
		}
		task.appendLog("🎬 开始渲染 Tick 曲线视频 (Mobile 9:16)...")
		task.setStatus("running", "渲染 Mobile 版本...")
		outPathMobile := filepath.Join(config.GetOutputDir(), task.Date, fmt.Sprintf("%s_tick_mobile.mp4", sessCfg.FilenameSuffix))
		outMobile, rErr := tick.RenderTickVideo(task.ctx, task.Date, outPathMobile, "mobile", task.Session, nil, nil, nil, copywriteText, newsPagesMobile)
		if rErr != nil {
			if isCancelled(task.ctx) {
				task.appendLog("🛑 Mobile 渲染已取消")
				releaseTickGenerateTaskKey(task)
				return
			}
			task.appendLog(fmt.Sprintf("⚠️ Mobile 渲染失败: %v", rErr))
		} else {
			task.mu.Lock()
			task.MobilePath = outMobile
			task.mu.Unlock()
			var fileInfo string
			if fi, err := os.Stat(outMobile); err == nil {
				fileInfo = fmt.Sprintf("%.1fMB", float64(fi.Size())/1024/1024)
			}
			task.appendLog(fmt.Sprintf("✅ Tick Mobile 渲染完成 (%s) %s", outMobile, fileInfo))
		}
	}

	if renderTV {
		if isCancelled(task.ctx) {
			task.appendLog("🛑 生成已取消（TV 渲染前）")
			releaseTickGenerateTaskKey(task)
			return
		}
		task.appendLog("🎬 开始渲染 Tick 曲线视频 (TV 16:9)...")
		task.setStatus("running", "渲染 TV 版本...")
		outPathTV := filepath.Join(config.GetOutputDir(), task.Date, fmt.Sprintf("%s_tick_tv.mp4", sessCfg.FilenameSuffix))
		outTV, rErr := tick.RenderTickVideo(task.ctx, task.Date, outPathTV, "tv", task.Session, nil, nil, nil, copywriteText, newsPagesTV)
		if rErr != nil {
			if isCancelled(task.ctx) {
				task.appendLog("🛑 TV 渲染已取消")
				releaseTickGenerateTaskKey(task)
				return
			}
			task.fail(fmt.Errorf("TV 渲染失败: %w", rErr))
			return
		}

		task.mu.Lock()
		task.TVPath = outTV
		task.mu.Unlock()
		var fileInfo string
		if fi, err := os.Stat(outTV); err == nil {
			fileInfo = fmt.Sprintf("%.1fMB", float64(fi.Size())/1024/1024)
		}
		task.appendLog(fmt.Sprintf("✅ Tick TV 渲染完成 (%s) %s", outTV, fileInfo))
	}

	if copywriteText != "" {
		if db, err := storage.Get(); err == nil {
			_ = db.SaveCopywriting(storage.Copywriting{
				Date:    task.Date,
				Session: task.Session,
				Type:    cwType,
				Content: copywriteText,
			})
		}
		task.appendLog("✅ 文案已保存")
	}

	task.mu.RLock()
	hasMobile := task.MobilePath != ""
	hasTV := task.TVPath != ""
	task.mu.RUnlock()

	if !hasMobile && !hasTV {
		task.fail(fmt.Errorf("所有格式渲染都失败了"))
		return
	}

	progress := "生成完毕"
	if hasMobile && hasTV {
		progress = "Mobile 9:16 + TV 16:9 全部生成完毕"
	} else if hasMobile {
		progress = "Mobile 9:16 生成完毕"
	} else if hasTV {
		progress = "TV 16:9 生成完毕"
	}
	task.appendLog("✅ Tick 视频生成完成")
	task.complete(progress)
}
