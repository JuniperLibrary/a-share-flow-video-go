package web

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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
	FilePath string
	Err      string
	mu       sync.Mutex
}

var exportTasks = make(map[string]*ExportTask)
var exportMu sync.Mutex

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
		t.mu.Lock()
		active := t.Date == dateStr && (t.Status == "pending" || t.Status == "running")
		t.mu.Unlock()
		if active {
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

	fail := func(err error) {
		task.mu.Lock()
		task.Status = "error"
		task.Err = err.Error()
		task.mu.Unlock()
		logger.Error("全量导出任务失败", zap.String("date", task.Date), zap.Error(err))
	}

	today := time.Now().In(webShanghaiTZ).Format("2006-01-02")
	var sectors []storage.SectorAll
	var err error
	if task.Date == today {
		task.mu.Lock()
		task.Progress = "正在抓取当前全量行情..."
		task.mu.Unlock()
		sectors, err = fetcher.FetchSectorsAllDaily(task.Date)
	} else {
		task.mu.Lock()
		task.Progress = "正在读取历史行情..."
		task.mu.Unlock()
		var db *storage.DB
		db, err = storage.Get()
		if err == nil {
			sectors, err = db.LoadSectorsAll(task.Date)
		}
	}
	if err != nil {
		fail(fmt.Errorf("获取导出数据失败: %w", err))
		return
	}
	if len(sectors) == 0 {
		fail(fmt.Errorf("日期 %s 没有可导出的板块数据", task.Date))
		return
	}

	// Only the live export refreshes SQLite. Historical exports are read-only
	// and must never replace a stored snapshot with current prices.
	if task.Date == today {
		db, dbErr := storage.Get()
		if dbErr != nil {
			fail(fmt.Errorf("数据库初始化失败: %w", dbErr))
			return
		}
		task.mu.Lock()
		task.Progress = "正在保存当前行情..."
		task.mu.Unlock()
		if err := db.SaveSectorsAll(sectors); err != nil {
			fail(fmt.Errorf("保存数据库失败: %w", err))
			return
		}
	}

	task.mu.Lock()
	task.Progress = "正在生成 CSV 文件..."
	task.mu.Unlock()
	dateDir := filepath.Join(config.GetDataDir(), task.Date)
	filePath, err := writeSectorsAllCSV(dateDir, task.Date, sectors)
	if err != nil {
		fail(fmt.Errorf("生成 CSV 失败: %w", err))
		return
	}

	task.mu.Lock()
	task.FilePath = filePath
	task.Status = "done"
	task.Progress = fmt.Sprintf("完成: %d 个板块", len(sectors))
	task.Err = ""
	task.mu.Unlock()
}

// writeSectorsAllCSV writes a complete snapshot and atomically replaces the
// previous file only after the CSV has been flushed and closed successfully.
func writeSectorsAllCSV(dateDir, dateStr string, sectors []storage.SectorAll) (string, error) {
	if len(sectors) == 0 {
		return "", fmt.Errorf("板块数据为空")
	}
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return "", err
	}

	file, err := os.CreateTemp(dateDir, ".sectors-all-*.tmp")
	if err != nil {
		return "", err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{
		"name", "net", "rate", "date", "code", "change_pct",
		"super_net", "super_rate", "big_net", "big_rate", "volume", "turnover",
		"turnover_rate", "lead_stock_name", "lead_stock_change_pct",
		"total_market_cap", "circulating_market_cap", "category",
	}); err != nil {
		return "", err
	}
	for _, s := range sectors {
		if err := writer.Write([]string{
			s.Name,
			strconv.FormatFloat(s.Net, 'f', 2, 64),
			strconv.FormatFloat(s.Rate, 'f', 2, 64),
			dateStr,
			s.Code,
			strconv.FormatFloat(s.ChangePct, 'f', 2, 64),
			strconv.FormatFloat(s.SuperNet, 'f', 2, 64),
			strconv.FormatFloat(s.SuperRate, 'f', 2, 64),
			strconv.FormatFloat(s.BigNet, 'f', 2, 64),
			strconv.FormatFloat(s.BigRate, 'f', 2, 64),
			strconv.FormatFloat(s.Volume, 'f', 2, 64),
			strconv.FormatFloat(s.Turnover, 'f', 2, 64),
			strconv.FormatFloat(s.TurnoverRate, 'f', 2, 64),
			s.LeadStockName,
			strconv.FormatFloat(s.LeadStockChangePct, 'f', 2, 64),
			strconv.FormatFloat(s.TotalMarketCap, 'f', 2, 64),
			strconv.FormatFloat(s.CirculatingMarketCap, 'f', 2, 64),
			s.Category,
		}); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	if err := file.Chmod(0644); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}

	finalPath := filepath.Join(dateDir, fmt.Sprintf("板块全量_%s.csv", dateStr))
	if err := os.Rename(tempPath, finalPath); err != nil {
		return "", err
	}
	return finalPath, nil
}

func handleExportAll(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		dateStr = time.Now().In(webShanghaiTZ).Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		logger.BadRequest(c, "日期格式必须为 YYYY-MM-DD")
		return
	}
	today := time.Now().In(webShanghaiTZ).Format("2006-01-02")
	if dateStr > today {
		logger.BadRequest(c, "不能导出未来日期")
		return
	}

	if task, exists := getTask(dateStr); exists {
		task.mu.Lock()
		status, progress := task.Status, task.Progress
		task.mu.Unlock()
		c.JSON(200, gin.H{"task_id": task.ID, "status": status, "progress": progress})
		return
	}

	taskID := fmt.Sprintf("exp_%s_%d", dateStr, time.Now().UnixNano())
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
	if !ok {
		logger.NotFound(c, "文件未就绪")
		return
	}

	task.mu.Lock()
	status, filePath := task.Status, task.FilePath
	task.mu.Unlock()
	if status != "done" || filePath == "" {
		logger.NotFound(c, "文件未就绪")
		return
	}
	c.File(filePath)
}

func handleExportHotSectors(c *gin.Context) {
	dateStr := c.Param("date")
	today := time.Now().In(webShanghaiTZ).Format("2006-01-02")
	if dateStr == "" {
		dateStr = today
	}
	if dateStr != today {
		logger.BadRequest(c, "实时热门板块仅支持当前日期，历史日期请使用全量数据导出")
		return
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
