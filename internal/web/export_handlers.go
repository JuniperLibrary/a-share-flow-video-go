package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
)

// ExportTask 异步全量下载任务
type ExportTask struct {
	ID       string
	Date     string
	Status   string
	Progress string
	Page     int
	Data     []storage.SectorAll
	Err      string
	mu       sync.Mutex
}

var exportTasks = make(map[string]*ExportTask)
var exportMu sync.Mutex
var exportHTTPClient = &http.Client{Timeout: 15 * time.Second}

func registerExportRoutes(r *gin.Engine) {
	r.GET("/api/export-all/:date", handleExportAll)
	r.GET("/api/export-all/status/:task_id", handleExportStatus)
	r.GET("/api/export-all/file/:task_id", handleExportFile)
	r.GET("/api/export-hot-sectors/:date", handleExportHotSectors)
}

func getTask(dateStr string) (*ExportTask, bool) {
	exportMu.Lock()
	defer exportMu.Unlock()
	for _, t := range exportTasks {
		if t.Date == dateStr && (t.Status == "pending" || t.Status == "running") {
			return t, true
		}
	}
	return nil, false
}

func runExportTask(task *ExportTask) {
	task.mu.Lock()
	task.Status = "running"
	task.Progress = "开始获取板块数据..."
	task.mu.Unlock()

	dateDir := filepath.Join(config.GetDataDir(), task.Date)
	os.MkdirAll(dateDir, 0755)
	checkpointPath := filepath.Join(dateDir, ".export_progress.json")

	if cp, err := os.ReadFile(checkpointPath); err == nil {
		var cpData struct {
			Page int `json:"page"`
		}
		if json.Unmarshal(cp, &cpData) == nil {
			task.Page = cpData.Page
		}
	}

	saveCheckpoint := func() {
		data, _ := json.Marshal(map[string]int{"page": task.Page})
		os.WriteFile(checkpointPath, data, 0644)
	}

	fetchPage := func(fs string, pn int) ([]storage.SectorAll, bool, error) {
		url := fmt.Sprintf("https://emdatah5.eastmoney.com/dc/ZJLX/getZDYLBData?fields=f12,f14,f3,f5,f6,f62,f66,f69,f72,f75,f184&pn=%d&pz=500&fid=f62&po=1&fs=%s&ut=b2884a393a59ad64002292a3e90d46a5", pn, fs)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, false, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Referer", "https://emdatah5.eastmoney.com/dc/zjlx/index")

		resp, err := exportHTTPClient.Do(req)
		if err != nil {
			return nil, false, err
		}
		defer resp.Body.Close()

		var result struct {
			Data struct {
				Diff []map[string]any `json:"diff"`
			} `json:"data"`
		}
		b, _ := io.ReadAll(resp.Body)
		json.Unmarshal(b, &result)

		var page []storage.SectorAll
		for _, item := range result.Data.Diff {
			name, _ := item["f14"].(string)
			code, _ := item["f12"].(string)
			netVal := item["f62"]
			rateVal := item["f184"]
			if name == "" || netVal == nil {
				continue
			}
			if f, ok := toFloat64(netVal); ok && f != 0 {
				rateFloat, _ := toFloat64(rateVal)
				changePctFloat, _ := toFloat64(item["f3"])
				superNetFloat, _ := toFloat64(item["f66"])
				superRateFloat, _ := toFloat64(item["f69"])
				bigNetFloat, _ := toFloat64(item["f72"])
				bigRateFloat, _ := toFloat64(item["f75"])
				volumeFloat, _ := toFloat64(item["f5"])
				turnoverFloat, _ := toFloat64(item["f6"])
				page = append(page, storage.SectorAll{
					Date:      task.Date,
					Code:      code,
					Name:      name,
					Net:       roundTo2(f / 1e8),
					Rate:      roundTo2(rateFloat),
					ChangePct: roundTo2(changePctFloat),
					SuperNet:  roundTo2(superNetFloat / 1e8),
					SuperRate: roundTo2(superRateFloat),
					BigNet:    roundTo2(bigNetFloat / 1e8),
					BigRate:   roundTo2(bigRateFloat),
					Volume:    roundTo2(volumeFloat),
					Turnover:  roundTo2(turnoverFloat / 1e8),
				})
			}
		}
		return page, len(result.Data.Diff) < 500, nil
	}

	allFS := []string{"m:90+t:2", "m:90+t:3"}
	for _, fs := range allFS {
		for pn := task.Page + 1; ; pn++ {
			task.mu.Lock()
			task.Progress = fmt.Sprintf("第%d页...", pn)
			task.mu.Unlock()

			page, done, err := fetchPage(fs, pn)
			if err != nil {
				task.mu.Lock()
				task.Status = "error"
				task.Err = err.Error()
				task.mu.Unlock()
				return
			}
			task.mu.Lock()
			task.Data = append(task.Data, page...)
			task.Page = pn
			task.mu.Unlock()
			saveCheckpoint()
			time.Sleep(200 * time.Millisecond)

			if done {
				break
			}
		}
		task.Page = 0
	}

	if db, err := storage.Get(); err == nil {
		task.mu.Lock()
		task.Progress = "正在保存到数据库..."
		task.mu.Unlock()

		if err := db.SaveSectorsAll(task.Data); err != nil {
			task.mu.Lock()
			task.Status = "error"
			task.Err = fmt.Sprintf("保存数据库失败: %v", err)
			task.mu.Unlock()
			return
		}
	}

	os.Remove(checkpointPath)

	task.mu.Lock()
	task.Status = "done"
	task.Progress = fmt.Sprintf("完成: %d 个板块", len(task.Data))
	task.mu.Unlock()
}

func handleExportAll(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	if task, exists := getTask(dateStr); exists {
		c.JSON(200, gin.H{"task_id": task.ID, "status": task.Status, "progress": task.Progress})
		return
	}

	taskID := fmt.Sprintf("exp_%s_%d", dateStr, time.Now().Unix())
	task := &ExportTask{ID: taskID, Date: dateStr, Status: "pending", Progress: "等待启动..."}
	exportMu.Lock()
	exportTasks[taskID] = task
	exportMu.Unlock()

	go runExportTask(task)
	c.JSON(200, gin.H{"task_id": taskID, "status": "pending"})
}

func handleExportStatus(c *gin.Context) {
	taskID := c.Param("task_id")
	exportMu.Lock()
	task, ok := exportTasks[taskID]
	exportMu.Unlock()

	if !ok {
		logger.NotFound(c, "任务不存在")
		return
	}

	task.mu.Lock()
	resp := gin.H{
		"task_id":  task.ID,
		"status":   task.Status,
		"progress": task.Progress,
	}
	if task.Status == "error" {
		resp["error"] = task.Err
	}
	task.mu.Unlock()
	c.JSON(200, resp)
}

func handleExportFile(c *gin.Context) {
	taskID := c.Param("task_id")
	exportMu.Lock()
	task, ok := exportTasks[taskID]
	exportMu.Unlock()

	if !ok || task.Status != "done" {
		logger.NotFound(c, "文件未就绪")
		return
	}

	dateDir := config.GetDataDir()
	filename := fmt.Sprintf("板块全量_%s.csv", task.Date)
	c.File(filepath.Join(dateDir, task.Date, filename))
}

func handleExportHotSectors(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	sectors, err := fetcher.FetchTop21HotSectors()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	c.JSON(200, gin.H{
		"date":    dateStr,
		"sectors": sectors,
	})
}
