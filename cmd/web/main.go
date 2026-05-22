package main

import (
	"os"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tickscheduler"
	"github.com/a-share-flow-video-go/internal/web"
	"go.uber.org/zap"
)

func main() {
	if err := logger.Init("info", "console", "stdout"); err != nil {
		panic(err)
	}
	defer logger.Sync()

	if err := config.LoadEnv(); err != nil {
		logger.Warn("failed to load .env", zap.Error(err))
	}

	if _, err := storage.Get(); err != nil {
		logger.Fatal("SQLite 初始化失败", zap.Error(err))
	}

	tickSched := tickscheduler.New()
	tickSched.Start()
	defer tickSched.Shutdown()

	r := web.SetupRouter(tickSched)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	logger.Info("A-share flow video web server started", zap.String("port", port))
	if err := r.Run("127.0.0.1:" + port); err != nil {
		logger.Fatal("server failed", zap.Error(err))
	}
}
