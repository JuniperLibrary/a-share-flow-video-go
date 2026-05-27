package logger

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

var routeNames = map[string]string{
	"GET /api/dates":                      "查询可用日期 [MarketPage/TickPage/DataPage]",
	"GET /api/data/:date":                 "查询板块数据 [MarketPage]",
	"GET /api/export-all/:date":           "导出全量数据 [DataPage]",
	"GET /api/export-all/status/:task_id": "查询导出状态 [DataPage]",
	"GET /api/export-all/file/:task_id":   "下载导出文件 [DataPage]",
	"GET /api/export-hot-sectors/:date":   "导出热门板块 [DataPage]",
	"POST /api/sectors-all/save/:date":    "保存全量板块 [DataPage]",
	"GET /api/sectors-all/save/status/:task_id": "查询保存状态 [DataPage]",
	"GET /api/sectors-all/:date":          "查询全量板块 [DataPage]",
	"GET /api/sectors-all/names":          "查询板块名称列表 [DataPage]",
	"GET /api/sectors-all/dates":          "查询板块日期列表 [DataPage]",
	"GET /api/sectors-all/range":          "查询板块区间数据 [DataPage]",
	"GET /api/notes":                      "查询笔记列表 [NotesPage]",
	"POST /api/notes":                     "创建笔记 [NotesPage]",
	"PUT /api/notes/:id":                  "更新笔记 [NotesPage]",
	"DELETE /api/notes/:id":               "删除笔记 [NotesPage]",
	"POST /api/generate-multiday":         "生成多日视频 [GeneratePage]",
	"GET /api/config":                     "查询配置 [ConfigPage]",
	"POST /api/config":                    "保存配置 [ConfigPage]",
	"POST /api/optimize-copy":             "优化文案 [GeneratePage]",
	"GET /api/files/:date":                "查询视频文件 [PreviewPage]",
	"GET /api/tick/status":                "查询Tick采集状态 [TickPage]",
	"POST /api/tick/start":                "启动Tick采集 [TickPage]",
	"POST /api/tick/force-collect":        "强制采集Tick [TickPage]",
	"POST /api/tick/stop":                 "停止Tick采集 [TickPage]",
	"POST /api/tick/enable":               "启用Tick采集 [TickPage]",
	"GET /api/tick/interval":              "查询采集间隔 [TickPage]",
	"POST /api/tick/interval":             "设置采集间隔 [TickPage]",
	"GET /api/tick-data/:date":            "查询Tick数据 [TickPage]",
	"POST /api/generate-tick":             "生成Tick视频 [TickPage]",
	"GET /api/tick/stream":                "Tick SSE流 [TickPage]",
	"GET /api/dashboard":                  "仪表盘数据 [DashboardPage]",
	"GET /api/news":                       "新闻列表 [NewsPage]",
	"GET /api/news/search":                "新闻搜索 [NewsPage]",
	"GET /api/news/date":                  "按日期查新闻 [NewsPage]",
	"GET /api/news/status":                "新闻轮询状态 [NewsPage]",
	"POST /api/news/start":                "启动新闻轮询 [NewsPage]",
	"POST /api/news/stop":                 "停止新闻轮询 [NewsPage]",
	"POST /api/news/replay":               "新闻回放 [NewsPage]",
	"GET /output/:date/:file":             "视频文件服务 [PreviewPage]",
	"GET /api/docs":                       "API文档 [DocsPage]",
}

func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		fullPath := c.FullPath()

		key := method + " " + fullPath
		name, ok := routeNames[key]
		if !ok {
			name = c.Request.URL.Path
		}

		msg := fmt.Sprintf("%s (%d, %s)", name, statusCode, latency.Round(time.Microsecond))

		errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()
		if errorMessage != "" {
			msg += " " + errorMessage
		}

		if statusCode >= 500 {
			Error(msg)
		} else if statusCode >= 400 {
			Warn(msg)
		} else {
			Info(msg)
		}
	}
}
