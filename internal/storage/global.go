package storage

import (
	"sync"

	"github.com/a-share-flow-video-go/internal/config"
)

var (
	globalDB   *DB
	globalOnce sync.Once
	globalErr  error
)

// Get returns the global database instance (lazy-init).
// 根据 DATA_MODE 决定使用文件 SQLite 还是内存 SQLite:
//   - "json" → 内存 SQLite，导出仍写到 data/*.json（只出不进）
//   - "sqlite" → 文件 SQLite (默认)
func Get() (*DB, error) {
	globalOnce.Do(func() {
		if config.DataMode() == "json" {
			globalDB, globalErr = New(":memory:")
		} else {
			globalDB, globalErr = New(config.GetDBPath())
		}
	})
	return globalDB, globalErr
}

// Reset closes and clears the global instance (useful for testing).
func Reset() {
	if globalDB != nil {
		globalDB.Close()
	}
	globalDB = nil
	globalOnce = sync.Once{}
	globalErr = nil
}
