package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dataDir := "data"

	// 查找所有 板块全量_*.csv
	var files []string
	filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(info.Name(), "板块全量_") && strings.HasSuffix(info.Name(), ".csv") {
			files = append(files, path)
		}
		return nil
	})

	if len(files) == 0 {
		fmt.Println("未找到 板块全量_*.csv 文件")
		return
	}

	fmt.Printf("找到 %d 个文件需要处理\n\n", len(files))

	for _, file := range files {
		dedupFile(file)
	}
}

func dedupFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("[跳过] %s: %v\n", path, err)
		return
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		fmt.Printf("[错误] %s: 读取失败: %v\n", path, err)
		return
	}

	if len(records) < 2 {
		fmt.Printf("[跳过] %s: 数据不足\n", path)
		return
	}

	header := records[0]
	rows := records[1:]

	// 按板块名称去重，保留第一条
	seen := make(map[string]bool)
	var deduped [][]string
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		name := strings.TrimSpace(row[0])
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		deduped = append(deduped, row)
	}

	originalCount := len(rows)
	newCount := len(deduped)

	if originalCount == newCount {
		fmt.Printf("[正常] %s: %d 条，无重复\n", filepath.Base(path), newCount)
		return
	}

	// 写回
	out, err := os.Create(path)
	if err != nil {
		fmt.Printf("[错误] %s: 写入失败: %v\n", path, err)
		return
	}
	defer out.Close()

	w := csv.NewWriter(out)
	w.Write(header)
	w.WriteAll(deduped)
	w.Flush()

	fmt.Printf("[去重] %s: %d → %d 条 (删除 %d 条重复)\n",
		filepath.Base(path), originalCount, newCount, originalCount-newCount)
}
