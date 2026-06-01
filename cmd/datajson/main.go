package main

import (
	"fmt"
	"os"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

func main() {
	if err := logger.Init("info", "console", "stdout"); err != nil {
		panic(err)
	}
	defer logger.Sync()

	if err := config.LoadEnv(); err != nil {
		logger.Warn("no .env", zap.Error(err))
	}

	if len(os.Args) < 2 {
		fmt.Println("用法: go run ./cmd/datajson/ <export|import>")
		return
	}

	db, err := storage.Get()
	if err != nil {
		logger.Fatal("数据库连接失败", zap.Error(err))
	}

	switch os.Args[1] {
	case "export":
		db.ExportJSON()
	case "import":
		fmt.Println("import 已废弃：JSON 读取逻辑已移除，SQLite 是唯一数据源。")
		fmt.Println("如需从历史 JSON 恢复数据，请使用 export 重新生成 SQLite 后再迁移。")
	default:
		fmt.Printf("未知命令: %s (可用: export)\n", os.Args[1])
	}
}
