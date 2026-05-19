package main

import (
	"fmt"
	"os"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/storage"
)

func main() {
	dbPath := config.GetDBPath()

	if _, err := os.Stat(dbPath); err == nil {
		fmt.Printf("数据库已存在: %s\n", dbPath)
		fmt.Println("如需重建，请先删除现有文件。")
		os.Exit(0)
	}

	db, err := storage.Get()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Printf("SQLite 数据库已初始化: %s\n", dbPath)
	fmt.Println("表结构:")
	fmt.Println("  - sectors      (datetime, name, net)  PK: datetime+name")
	fmt.Println("  - copywriting  (date, session, type, content)  PK: date+session+type")
}
