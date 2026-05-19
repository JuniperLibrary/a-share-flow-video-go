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

// Get returns the global SQLite database instance (lazy-init).
func Get() (*DB, error) {
	globalOnce.Do(func() {
		globalDB, globalErr = New(config.GetDBPath())
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
