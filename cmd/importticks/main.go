package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/storage"
)

func main() {
	dateStr := "2026-05-19"
	if len(os.Args) > 1 {
		dateStr = os.Args[1]
	}

	csvPath := filepath.Join(config.GetDataDir(), dateStr, "ticks.csv")
	f, err := os.Open(csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开文件失败: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 CSV 失败: %v\n", err)
		os.Exit(1)
	}

	if len(records) < 2 {
		fmt.Println("CSV 无数据行")
		os.Exit(0)
	}

	db, err := storage.Get()
	if err != nil {
		fmt.Fprintf(os.Stderr, "数据库初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	inputDate := time.Now().Format("2006-01-02 15:04:05")
	var count int
	for _, rec := range records[1:] {
		if len(rec) < 3 {
			continue
		}
		timeStr := normalizeTime(rec[0])
		name := rec[1]
		net, err := strconv.ParseFloat(rec[2], 64)
		if err != nil {
			continue
		}

		datetime := storage.DateToDatetimeTick(dateStr, timeStr)
		if err := db.SaveSectors([]storage.Sector{
			{Datetime: datetime, Name: name, Net: net, InputDate: inputDate},
		}); err != nil {
			fmt.Fprintf(os.Stderr, "写入失败 [%s %s]: %v\n", datetime, name, err)
			continue
		}
		count++
	}

	fmt.Printf("成功导入 %d 条 tick 记录到 %s\n", count, dateStr)
}

func normalizeTime(t string) string {
	t = strings.TrimSpace(t)
	if !strings.Contains(t, ":") {
		return t
	}
	parts := strings.Split(t, ":")
	if len(parts) == 2 {
		return fmt.Sprintf("%02s:%02s", parts[0], parts[1])
	}
	return t
}
