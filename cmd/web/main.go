package main

import (
	"os"

	"github.com/a-share-flow-video-go/internal/clsnews"
	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"github.com/a-share-flow-video-go/internal/tick"
	"github.com/a-share-flow-video-go/internal/web"
	"go.uber.org/zap"
)

func main() {
	if err := logger.InitFromEnv(); err != nil {
		panic(err)
	}
	defer logger.Sync()

	if err := config.LoadEnv(); err != nil {
		logger.Warn("failed to load .env", zap.Error(err))
	}

	if _, err := storage.Get(); err != nil {
		logger.Fatal("SQLite 初始化失败", zap.Error(err))
	}

	tickSched := tick.NewScheduler()
	tickSched.Start()
	defer tickSched.Shutdown()

	newsSched := clsnews.NewNewsScheduler()
	newsSched.Start()
	defer newsSched.Stop()

	r := web.SetupRouter(tickSched, newsSched)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	logger.Info("A-share flow video web server started", zap.String("port", port))
	if err := r.Run("127.0.0.1:" + port); err != nil {
		logger.Fatal("server failed", zap.Error(err))
	}
}
