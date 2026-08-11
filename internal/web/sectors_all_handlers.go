package web

import (
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
)

type SaveAllTask struct {
	mu       sync.Mutex
	Date     string    `json:"date"`
	Status   string    `json:"status"`
	Progress string    `json:"progress"`
	Count    int       `json:"count"`
	Err      string    `json:"error,omitempty"`
	Created  time.Time `json:"created"`
}

var (
	saveAllTasks       = make(map[string]*SaveAllTask)
	saveAllTasksMu     sync.Mutex
	saveAllTaskCounter int
)

func registerSectorsAllRoutes(r *gin.Engine) {
	r.POST("/api/sectors-all/save/:date", handleSaveAllSectors)
	r.GET("/api/sectors-all/save/status/:task_id", handleSaveAllStatus)
	r.GET("/api/sectors-all/:date", handleGetAllSectors)
	r.GET("/api/sectors-all/names", handleGetSectorsAllNames)
	r.GET("/api/sectors-all/dates", handleGetSectorsAllDates)
	r.GET("/api/sectors-all/range", handleGetSectorsAllRange)
}

func handleSaveAllSectors(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	saveAllTasksMu.Lock()
	saveAllTaskCounter++
	taskID := fmt.Sprintf("save_%s_%d", dateStr, saveAllTaskCounter)
	task := &SaveAllTask{
		Date:    dateStr,
		Status:  "pending",
		Created: time.Now(),
	}
	saveAllTasks[taskID] = task
	saveAllTasksMu.Unlock()

	logger.Info("全板块数据获取任务已创建",
		zap.String("date", dateStr),
		zap.String("task_id", taskID),
	)

	go runSaveAllTask(task)

	c.JSON(200, gin.H{
		"task_id": taskID,
		"message": "已启动后台获取任务",
	})
}

func handleSaveAllStatus(c *gin.Context) {
	taskID := c.Param("task_id")
	saveAllTasksMu.Lock()
	task, ok := saveAllTasks[taskID]
	saveAllTasksMu.Unlock()

	if !ok {
		logger.Warn("全板块数据任务状态查询失败，任务不存在", zap.String("task_id", taskID))
		logger.NotFound(c, "任务不存在")
		return
	}

	task.mu.Lock()
	dateStr := task.Date
	status := task.Status
	progress := task.Progress
	count := task.Count
	errMsg := task.Err
	resp := gin.H{
		"task_id":  taskID,
		"date":     dateStr,
		"status":   status,
		"progress": progress,
		"count":    count,
	}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	task.mu.Unlock()

	logger.Info("全板块数据任务状态",
		zap.String("task_id", taskID),
		zap.String("date", dateStr),
		zap.String("status", status),
		zap.String("progress", progress),
		zap.Int("count", count),
	)

	c.JSON(200, resp)
}

func runSaveAllTask(task *SaveAllTask) {
	task.mu.Lock()
	task.Status = "running"
	task.Progress = "开始获取板块数据..."
	task.mu.Unlock()

	logger.Info("全板块数据获取任务开始执行",
		zap.String("date", task.Date),
	)

	allSectors, fetchErr := fetcher.FetchSectorsAllDaily(task.Date)
	if fetchErr != nil {
		task.mu.Lock()
		task.Status = "error"
		task.Err = fmt.Sprintf("抓取全板块失败: %v", fetchErr)
		task.mu.Unlock()
		logger.Error("全板块数据抓取失败",
			zap.String("date", task.Date),
			zap.Error(fetchErr),
		)
		return
	}

	task.mu.Lock()
	task.Progress = fmt.Sprintf("抓取完成: %d 条，正在保存到数据库...", len(allSectors))
	task.Count = len(allSectors)
	task.mu.Unlock()

	db, err := storage.Get()
	if err != nil {
		task.mu.Lock()
		task.Status = "error"
		task.Err = fmt.Sprintf("数据库连接失败: %v", err)
		task.mu.Unlock()
		logger.Error("全板块数据保存失败-数据库连接失败",
			zap.String("date", task.Date),
			zap.Error(err),
		)
		return
	}

	if err := db.SaveSectorsAll(allSectors); err != nil {
		task.mu.Lock()
		task.Status = "error"
		task.Err = fmt.Sprintf("保存数据库失败: %v", err)
		task.mu.Unlock()
		logger.Error("全板块数据保存失败-写入数据库失败",
			zap.String("date", task.Date),
			zap.Int("count", len(allSectors)),
			zap.Error(err),
		)
		return
	}

	task.mu.Lock()
	task.Status = "done"
	task.Progress = fmt.Sprintf("完成: 已保存 %d 个板块数据", len(allSectors))
	task.Count = len(allSectors)
	task.mu.Unlock()

	logger.Info("全板块数据获取任务完成",
		zap.String("date", task.Date),
		zap.Int("total_count", len(allSectors)),
	)
}

func handleGetAllSectors(c *gin.Context) {
	dateStr := c.Param("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	sectors, err := db.LoadSectorsAll(dateStr)
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	c.JSON(200, gin.H{
		"date":    dateStr,
		"sectors": sectors,
	})
}

func handleGetSectorsAllRange(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate == "" || endDate == "" {
		logger.BadRequest(c, "缺少参数: start_date, end_date")
		return
	}

	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	sectors, err := db.LoadSectorsAllRange(startDate, endDate)
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	c.JSON(200, gin.H{
		"sectors": sectors,
	})
}

func handleGetSectorsAllNames(c *gin.Context) {
	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	names, err := db.ListSectorsAllNames()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	c.JSON(200, gin.H{
		"names": names,
	})
}

func handleGetSectorsAllDates(c *gin.Context) {
	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	dates, err := db.ListSectorsAllDates()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	c.JSON(200, gin.H{
		"dates": dates,
	})
}
