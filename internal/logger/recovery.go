package logger

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				fullPath := c.FullPath()
				key := c.Request.Method + " " + fullPath
				name, ok := routeNames[key]
				if !ok {
					name = c.Request.URL.Path
				}

				Error(fmt.Sprintf("panic recovered: %s", name),
					zap.Any("error", err),
					zap.String("stack", string(debug.Stack())),
				)

				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
				c.Abort()
			}
		}()

		c.Next()
	}
}
