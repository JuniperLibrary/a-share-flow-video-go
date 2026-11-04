package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/a-share-flow-video-go/internal/config"
)

// ImportJSON restores the static snapshot into a fresh database. Missing files
// are normal on first run; malformed or unreadable history is an error so it
// cannot be silently replaced by an empty export.
func (db *DB) ImportJSON() error {
	dir := config.GetDataDir()
	db.mu.Lock()
	defer db.mu.Unlock()

	for _, spec := range exportQueries {
		if err := db.importJSONFileLocked(dir, spec.File, spec.Table, spec.Query); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) importJSONFileLocked(dir, filename, table, selectQuery string) error {
	path := filepath.Join(dir, filename)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取历史 JSON %s: %w", path, err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return fmt.Errorf("解析历史 JSON %s: %w", path, err)
	}
	if len(rows) == 0 {
		return nil
	}
	columns, err := columnsFromSelect(selectQuery)
	if err != nil {
		return err
	}

	tx, err := db.db.Begin()
	if err != nil {
		return fmt.Errorf("开启历史导入事务 %s: %w", filename, err)
	}
	defer tx.Rollback()

	statements := make(map[string]*sql.Stmt)
	defer func() {
		for _, stmt := range statements {
			_ = stmt.Close()
		}
	}()

	for i, row := range rows {
		// Old snapshots may not contain newly added columns. Insert only the
		// fields present in each row so SQLite defaults still apply.
		names := make([]string, 0, len(columns))
		values := make([]any, 0, len(columns))
		for _, column := range columns {
			if value, ok := row[column]; ok {
				names = append(names, column)
				values = append(values, value)
			}
		}
		if len(names) == 0 {
			return fmt.Errorf("历史 JSON %s 第 %d 行没有可导入字段", filename, i+1)
		}

		key := strings.Join(names, ",")
		stmt := statements[key]
		if stmt == nil {
			placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
			query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, key, placeholders)
			stmt, err = tx.Prepare(query)
			if err != nil {
				return fmt.Errorf("准备历史导入 %s: %w", filename, err)
			}
			statements[key] = stmt
		}
		if _, err := stmt.Exec(values...); err != nil {
			return fmt.Errorf("导入历史 JSON %s 第 %d 行: %w", filename, i+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交历史导入 %s: %w", filename, err)
	}
	return nil
}

func columnsFromSelect(query string) ([]string, error) {
	upper := strings.ToUpper(query)
	fromIndex := strings.Index(upper, " FROM ")
	if !strings.HasPrefix(upper, "SELECT ") || fromIndex < 0 {
		return nil, fmt.Errorf("无法从查询解析导出字段")
	}
	parts := strings.Split(query[len("SELECT "):fromIndex], ",")
	columns := make([]string, 0, len(parts))
	for _, part := range parts {
		column := strings.TrimSpace(part)
		if column == "" {
			return nil, fmt.Errorf("导出查询包含空字段")
		}
		columns = append(columns, column)
	}
	return columns, nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Chmod(perm); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
