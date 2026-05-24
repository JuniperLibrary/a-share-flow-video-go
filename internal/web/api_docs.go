package web

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed API.md
var apiDocsContent []byte

func init() {
	registerDocsRoute = func(r *gin.Engine) {
		r.GET("/api/docs", func(c *gin.Context) {
			c.Data(http.StatusOK, "text/markdown; charset=utf-8", apiDocsContent)
		})
	}
}

var registerDocsRoute func(r *gin.Engine)
