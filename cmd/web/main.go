package main

import (
	"fmt"
	"log"
	"os"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/scheduler"
	"github.com/a-share-flow-video-go/internal/tickscheduler"
	"github.com/a-share-flow-video-go/internal/web"
)

func main() {
	if err := config.LoadEnv(); err != nil {
		log.Printf("warning: failed to load .env: %v", err)
	}

	sched := scheduler.NewScheduler()
	sched.Start()
	defer sched.Stop()

	tickSched := tickscheduler.New()
	tickSched.Start()
	defer tickSched.Stop()

	r := web.SetupRouter(sched, tickSched)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	fmt.Printf("A股情绪流动可视化 Web 服务启动: http://127.0.0.1:%s\n", port)
	if err := r.Run("127.0.0.1:" + port); err != nil {
		log.Fatalf("Web 服务启动失败: %v", err)
	}
}
