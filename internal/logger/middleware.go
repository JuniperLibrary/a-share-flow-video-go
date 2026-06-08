package logger

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var routeNames = map[string]string{
	"GET /api/dates":                      "查询可用日期",
	"GET /api/data/:date":                 "查询板块数据",
	"GET /api/export-all/:date":           "导出全量数据",
	"GET /api/export-all/status/:task_id": "查询导出状态",
	"GET /api/export-all/file/:task_id":   "下载导出文件",
	"GET /api/export-hot-sectors/:date":   "导出热门板块",
	"POST /api/sectors-all/save/:date":    "保存全量板块",
	"GET /api/sectors-all/save/status/:task_id": "查询保存状态",
	"GET /api/sectors-all/:date":          "查询全量板块",
	"GET /api/sectors-all/names":          "查询板块名称列表",
	"GET /api/sectors-all/dates":          "查询板块日期列表",
	"GET /api/sectors-all/range":          "查询板块区间数据",
	"GET /api/notes":                      "查询笔记列表",
	"POST /api/notes":                     "创建笔记",
	"PUT /api/notes/:id":                  "更新笔记",
	"DELETE /api/notes/:id":               "删除笔记",
	"POST /api/generate-multiday":         "生成多日视频",
	"GET /api/config":                     "查询配置",
	"POST /api/config":                    "保存配置",
	"POST /api/optimize-copy":             "优化文案",
	"GET /api/files/:date":                "查询视频文件",
	"GET /api/tick/status":                "查询Tick采集状态",
	"POST /api/tick/start":                "启动Tick采集",
	"POST /api/tick/force-collect":        "强制采集Tick",
	"POST /api/tick/stop":                 "停止Tick采集",
	"POST /api/tick/enable":               "启用Tick采集",
	"GET /api/tick/interval":              "查询采集间隔",
	"POST /api/tick/interval":             "设置采集间隔",
	"GET /api/tick-data/:date":            "查询Tick数据",
	"POST /api/generate-tick":             "生成Tick视频",
	"GET /api/tick/stream":                "Tick SSE流",
	"GET /api/dashboard":                  "仪表盘数据",
	"GET /api/news":                       "新闻列表",
	"GET /api/news/search":                "新闻搜索",
	"GET /api/news/date":                  "按日期查新闻",
	"GET /api/news/status":                "新闻轮询状态",
	"POST /api/news/start":                "启动新闻轮询",
	"POST /api/news/stop":                 "停止新闻轮询",
	"POST /api/news/replay":               "新闻回放",
	"GET /output/:date/:file":             "视频文件服务",
	"GET /api/docs":                       "API文档",
}

// RequestLoggerMiddleware HTTP 请求日志中间件。
// 记录每个请求的方法、路径、状态码、耗时和客户端信息。
// 成功（<400）记录为 Debug，客户端错误（4xx）记录为 Warn，服务端错误（5xx）记录为 Error。
func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method

		key := method + " " + c.FullPath()
		apiName, ok := routeNames[key]
		if !ok {
			apiName = path
		}

		fields := []zap.Field{
			zap.Int("status", statusCode),
			zap.String("method", method),
			zap.String("path", path),
			zap.String("api", apiName),
			zap.Duration("latency", latency),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		if query != "" {
			fields = append(fields, zap.String("query", query))
		}

		errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()
		if errorMessage != "" {
			fields = append(fields, zap.String("error", errorMessage))
		}

		msg := fmt.Sprintf("HTTP %s %s", method, path)
		if statusCode >= 500 {
			Error(msg, fields...)
		} else if statusCode >= 400 {
			Warn(msg, fields...)
		} else {
			Debug(msg, fields...)
		}
	}
}
