package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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

	var allSectors []storage.SectorAll
	allFS := []struct {
		fs       string
		category string
	}{{"m:90+t:2", "industry"}, {"m:90+t:3", "concept"}}
	for _, item := range allFS {
		for pn := 1; ; pn++ {
			task.mu.Lock()
			task.Progress = fmt.Sprintf("获取第%d页(%s)...", pn, item.category)
			task.mu.Unlock()

			url := fmt.Sprintf("https://emdatah5.eastmoney.com/dc/ZJLX/getZDYLBData?fields=f12,f14,f3,f5,f6,f62,f66,f69,f72,f75,f184&pn=%d&pz=500&fid=f62&po=1&fs=%s&ut=b2884a39ad64002292a3e90d46a5", pn, item.fs)
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				task.mu.Lock()
				task.Status = "error"
				task.Err = err.Error()
				task.mu.Unlock()
				logger.Error("全板块数据API请求创建失败",
					zap.String("date", task.Date),
					zap.String("category", item.category),
					zap.Int("page", pn),
					zap.Error(err),
				)
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			req.Header.Set("Referer", "https://emdatah5.eastmoney.com/dc/zjlx/index")

			resp, err := exportHTTPClient.Do(req)
			if err != nil {
				task.mu.Lock()
				task.Status = "error"
				task.Err = fmt.Sprintf("API请求失败: %v", err)
				task.mu.Unlock()
				logger.Error("全板块数据API请求失败",
					zap.String("date", task.Date),
					zap.String("category", item.category),
					zap.Int("page", pn),
					zap.Error(err),
				)
				return
			}

			var result struct {
				Data struct {
					Diff []map[string]any `json:"diff"`
				} `json:"data"`
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			json.Unmarshal(b, &result)

			pageItemCount := len(result.Data.Diff)
			logger.Info("全板块数据页获取成功",
				zap.String("date", task.Date),
				zap.String("category", item.category),
				zap.Int("page", pn),
				zap.Int("items", pageItemCount),
			)

			for _, d := range result.Data.Diff {
				name, _ := d["f14"].(string)
				code, _ := d["f12"].(string)
				netVal := d["f62"]
				rateVal := d["f184"]
				if name == "" || netVal == nil {
					continue
				}
				if f, ok := toFloat64(netVal); ok && f != 0 {
					rateFloat, _ := toFloat64(rateVal)
					changePctFloat, _ := toFloat64(d["f3"])
					superNetFloat, _ := toFloat64(d["f66"])
					superRateFloat, _ := toFloat64(d["f69"])
					bigNetFloat, _ := toFloat64(d["f72"])
					bigRateFloat, _ := toFloat64(d["f75"])
					volumeFloat, _ := toFloat64(d["f5"])
					turnoverFloat, _ := toFloat64(d["f6"])
					turnoverRateFloat, _ := toFloat64(d["f7"])
					leadStockName, _ := d["f140"].(string)
					leadChangeFloat, _ := toFloat64(d["f127"])
					mcapFloat, _ := toFloat64(d["f20"])
					cmcapFloat, _ := toFloat64(d["f21"])
					allSectors = append(allSectors, storage.SectorAll{
						Date:                 task.Date,
						Code:                 code,
						Name:                 name,
						Net:                  roundTo2(f / 1e8),
						Rate:                 roundTo2(rateFloat),
						ChangePct:            roundTo2(changePctFloat),
						SuperNet:             roundTo2(superNetFloat / 1e8),
						SuperRate:            roundTo2(superRateFloat),
						BigNet:               roundTo2(bigNetFloat / 1e8),
						BigRate:              roundTo2(bigRateFloat),
						Volume:               roundTo2(volumeFloat),
						Turnover:             roundTo2(turnoverFloat / 1e8),
						TurnoverRate:         roundTo2(turnoverRateFloat),
						LeadStockName:        leadStockName,
						LeadStockChangePct:   roundTo2(leadChangeFloat),
						TotalMarketCap:       roundTo2(mcapFloat / 1e8),
						CirculatingMarketCap: roundTo2(cmcapFloat / 1e8),
						Category:             item.category,
					})
				}
			}

			if pageItemCount < 500 {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		logger.Info("全板块数据分类获取完成",
			zap.String("date", task.Date),
			zap.String("category", item.category),
		)
	}

	task.mu.Lock()
	task.Progress = "正在保存到数据库..."
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
