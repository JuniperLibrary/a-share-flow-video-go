package main

import (
	"fmt"

	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/logger"
	"github.com/a-share-flow-video-go/internal/storage"
	"go.uber.org/zap"
)

func runIngestSectorsAll(dateStr string) error {
	db, err := storage.Get()
	if err != nil {
		return fmt.Errorf("db init: %w", err)
	}
	rows, err := fetcher.FetchSectorsAllDaily(dateStr)
	if err != nil {
		return fmt.Errorf("fetch sectors_all: %w", err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("fetched empty sectors_all rows for %s", dateStr)
	}
	neg := 0
	pos := 0
	for _, r := range rows {
		if r.Net < 0 {
			neg++
		} else if r.Net > 0 {
			pos++
		}
	}
	if err := db.SaveSectorsAll(rows); err != nil {
		return fmt.Errorf("save sectors_all: %w", err)
	}
	logger.Info("写入 sectors_all 成功",
		zap.String("date", dateStr),
		zap.Int("total", len(rows)),
		zap.Int("net_inflow", pos),
		zap.Int("net_outflow", neg))
	return nil
}
