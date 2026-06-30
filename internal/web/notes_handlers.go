package web

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
)

func registerNotesRoutes(r *gin.Engine) {
	r.GET("/api/notes", handleListNotes)
	r.POST("/api/notes", handleCreateNote)
	r.PUT("/api/notes/:id", handleUpdateNote)
	r.DELETE("/api/notes/:id", handleDeleteNote)
}

func handleListNotes(c *gin.Context) {
	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	notes, err := db.ListNotes()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}
	if notes == nil {
		notes = []storage.Note{}
	}

	c.JSON(200, gin.H{"notes": notes})
}

func handleCreateNote(c *gin.Context) {
	var body struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if bindErr := c.ShouldBindJSON(&body); bindErr != nil {
		logger.BadRequest(c, bindErr.Error())
		return
	}
	if body.Content == "" {
		logger.BadRequest(c, "内容不能为空")
		return
	}
	body.Type = normalizeNoteType(body.Type)

	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	id, err := db.SaveNote(storage.Note{Type: body.Type, Content: body.Content})
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	c.JSON(200, gin.H{"ok": true, "id": id})
}

func handleUpdateNote(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logger.BadRequest(c, "无效的 id")
		return
	}

	var body struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if bindErr := c.ShouldBindJSON(&body); bindErr != nil {
		logger.BadRequest(c, bindErr.Error())
		return
	}
	if body.Content == "" {
		logger.BadRequest(c, "内容不能为空")
		return
	}
	body.Type = normalizeNoteType(body.Type)

	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	if err := db.UpdateNote(id, body.Type, body.Content); err != nil {
		logger.InternalError(c, "更新笔记失败", err)
		return
	}

	c.JSON(200, gin.H{"ok": true})
}

func handleDeleteNote(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logger.BadRequest(c, "无效的 id")
		return
	}

	db, err := storage.Get()
	if err != nil {
		logger.InternalError(c, "操作失败", err)
		return
	}

	if err := db.DeleteNote(id); err != nil {
		logger.InternalError(c, "删除笔记失败", err)
		return
	}

	c.JSON(200, gin.H{"ok": true})
}

func normalizeNoteType(noteType string) string {
	if noteType != "completed" && noteType != "planned" {
		return "planned"
	}
	return noteType
}
