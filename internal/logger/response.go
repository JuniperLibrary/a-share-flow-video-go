package logger

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func ErrorResponse(c *gin.Context, statusCode int, msg string, err error) {
	if err != nil {
		c.Error(err)
	}
	c.JSON(statusCode, gin.H{"error": msg})
}

func ErrorResponsef(c *gin.Context, statusCode int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ErrorResponse(c, statusCode, msg, nil)
}

func InternalError(c *gin.Context, msg string, err error) {
	ErrorResponse(c, http.StatusInternalServerError, msg, err)
}

func InternalErrorf(c *gin.Context, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ErrorResponse(c, http.StatusInternalServerError, msg, nil)
}

func BadRequest(c *gin.Context, msg string) {
	ErrorResponse(c, http.StatusBadRequest, msg, nil)
}

func BadRequestf(c *gin.Context, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ErrorResponse(c, http.StatusBadRequest, msg, nil)
}

func NotFound(c *gin.Context, msg string) {
	ErrorResponse(c, http.StatusNotFound, msg, nil)
}

func NotFoundf(c *gin.Context, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	ErrorResponse(c, http.StatusNotFound, msg, nil)
}

func Success(c *gin.Context, data gin.H) {
	c.JSON(http.StatusOK, data)
}
