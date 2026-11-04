package rendercheck

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CollectVideos 展开输入路径：目录递归收集 mp4，忽略备份文件。
func CollectVideos(paths []string) ([]string, error) {
	var files []string
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("解析路径 %s: %w", p, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("访问路径 %s: %w", p, err)
		}
		if !info.IsDir() {
			if isVideo(abs) {
				files = append(files, abs)
			}
			continue
		}
		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !isVideo(path) {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("遍历目录 %s: %w", p, err)
		}
	}
	sort.Strings(files)
	return files, nil
}

func isVideo(path string) bool {
	lower := strings.ToLower(path)
	if !strings.HasSuffix(lower, ".mp4") {
		return false
	}
	// output/ 下存在 .mp4.bak_xxx 之类的历史备份，不能当成成片体检。
	return !strings.Contains(lower, ".bak")
}
