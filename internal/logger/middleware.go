package logger

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RequestLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		fields := []zap.Field{
			zap.Int("status", statusCode),
			zap.String("method", method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", clientIP),
			zap.Duration("latency", latency),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()
		if errorMessage != "" {
			fields = append(fields, zap.String("error", errorMessage))
		}

		if statusCode >= 500 {
			Error("HTTP request", fields...)
		} else if statusCode >= 400 {
			Warn("HTTP request", fields...)
		} else {
			Info("HTTP request", fields...)
		}
	}
}
