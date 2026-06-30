package storage

import (
	"path/filepath"
	"testing"
)

func TestLoadPendingCLSNews_ReturnsOnlyUnclassifiedRecords(t *testing.T) {
	tmp := t.TempDir()
	db, err := New(filepath.Join(tmp, "pending.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	_, err = db.SaveCLSNews([]CLSNewsRecord{
		{
			ID:             1,
			Title:          "待分类空数组",
			Content:        "content-1",
			Brief:          "brief-1",
			Level:          "A",
			ReadingNum:     10,
			CTime:          "2026-06-29 09:31:00",
			ShareURL:       "https://example.com/1",
			Sectors:        "[]",
			ClassifyStatus: "pending",
		},
		{
			ID:             2,
			Title:          "已分类",
			Content:        "content-2",
			Brief:          "brief-2",
			Level:          "B",
			ReadingNum:     11,
			CTime:          "2026-06-29 09:32:00",
			ShareURL:       "https://example.com/2",
			Sectors:        `["半导体"]`,
			ClassifyStatus: "classified",
		},
		{
			ID:             3,
			Title:          "待分类空字符串",
			Content:        "content-3",
			Brief:          "brief-3",
			Level:          "C",
			ReadingNum:     12,
			CTime:          "2026-06-29 09:33:00",
			ShareURL:       "https://example.com/3",
			Sectors:        "",
			ClassifyStatus: "retrying",
		},
	})
	if err != nil {
		t.Fatalf("SaveCLSNews: %v", err)
	}

	records, err := db.LoadPendingCLSNews(10)
	if err != nil {
		t.Fatalf("LoadPendingCLSNews: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 pending records, got %d", len(records))
	}
	if records[0].ID != 3 || records[1].ID != 1 {
		t.Fatalf("unexpected pending order: %+v", records)
	}
}

func TestCLSNewsClassificationStatsAndRetryLifecycle(t *testing.T) {
	tmp := t.TempDir()
	db, err := New(filepath.Join(tmp, "stats.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	_, err = db.SaveCLSNews([]CLSNewsRecord{
		{ID: 1, Title: "pending", Content: "a", Brief: "a", Level: "A", CTime: "2026-06-29 09:31:00", ShareURL: "https://example.com/1", Sectors: "[]", ClassifyStatus: "pending"},
		{ID: 2, Title: "classified", Content: "b", Brief: "b", Level: "A", CTime: "2026-06-29 09:32:00", ShareURL: "https://example.com/2", Sectors: `["半导体"]`, ClassifyStatus: "classified"},
		{ID: 3, Title: "skipped", Content: "c", Brief: "c", Level: "A", CTime: "2026-06-29 09:33:00", ShareURL: "https://example.com/3", Sectors: "[]", ClassifyStatus: "pending"},
	})
	if err != nil {
		t.Fatalf("SaveCLSNews: %v", err)
	}

	err = db.RecordCLSNewsRetry(1, "AI 未返回标签", 5)
	if err != nil {
		t.Fatalf("RecordCLSNewsRetry: %v", err)
	}
	err = db.MarkCLSNewsSkipped(3, "内容过短")
	if err != nil {
		t.Fatalf("MarkCLSNewsSkipped: %v", err)
	}
	stats, err := db.GetCLSNewsClassificationStats()
	if err != nil {
		t.Fatalf("GetCLSNewsClassificationStats: %v", err)
	}
	if stats.RetryingCount != 1 || stats.ClassifiedCount != 1 || stats.SkippedCount != 1 {
		t.Fatalf("unexpected stats after first pass: %+v", stats)
	}
	if stats.PendingCount != 0 || stats.FailedCount != 0 {
		t.Fatalf("unexpected pending/failed counts: %+v", stats)
	}
	if stats.LastRetryAt == "" {
		t.Fatalf("expected last_retry_at to be populated")
	}

	err = db.RecordCLSNewsRetry(1, "连续失败", 2)
	if err != nil {
		t.Fatalf("RecordCLSNewsRetry with limit: %v", err)
	}
	stats, err = db.GetCLSNewsClassificationStats()
	if err != nil {
		t.Fatalf("GetCLSNewsClassificationStats second pass: %v", err)
	}
	if stats.FailedCount != 1 || stats.RetryingCount != 0 {
		t.Fatalf("unexpected stats after failover: %+v", stats)
	}
}

func TestCLSNewsStatusFiltersAndRequeue(t *testing.T) {
	tmp := t.TempDir()
	db, err := New(filepath.Join(tmp, "filters.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	_, err = db.SaveCLSNews([]CLSNewsRecord{
		{ID: 1, Title: "pending", Content: "a", Brief: "a", Level: "A", CTime: "2026-06-29 09:31:00", ShareURL: "https://example.com/1", Sectors: "[]", ClassifyStatus: "pending"},
		{ID: 2, Title: "failed", Content: "b", Brief: "b", Level: "A", CTime: "2026-06-29 09:32:00", ShareURL: "https://example.com/2", Sectors: "[]", ClassifyStatus: "failed", RetryCount: 5, LastError: "连续失败"},
		{ID: 3, Title: "classified", Content: "c", Brief: "c", Level: "A", CTime: "2026-06-29 09:33:00", ShareURL: "https://example.com/3", Sectors: `["半导体"]`, ClassifyStatus: "classified"},
	})
	if err != nil {
		t.Fatalf("SaveCLSNews: %v", err)
	}

	failed, err := db.LoadLatestNewsByStatus(10, 0, "failed")
	if err != nil {
		t.Fatalf("LoadLatestNewsByStatus: %v", err)
	}
	if len(failed) != 1 || failed[0].ID != 2 {
		t.Fatalf("unexpected failed records: %+v", failed)
	}

	found, total, err := db.SearchCLSNewsByStatus("fail", 10, 0, "failed")
	if err != nil {
		t.Fatalf("SearchCLSNewsByStatus: %v", err)
	}
	if total != 1 || len(found) != 1 || found[0].ID != 2 {
		t.Fatalf("unexpected search result: total=%d records=%+v", total, found)
	}

	err = db.RequeueCLSNews(2)
	if err != nil {
		t.Fatalf("RequeueCLSNews: %v", err)
	}
	requeued, err := db.GetCLSNewsByID(2)
	if err != nil {
		t.Fatalf("GetCLSNewsByID: %v", err)
	}
	if requeued.ClassifyStatus != "pending" || requeued.LastError != "" {
		t.Fatalf("unexpected requeued record: %+v", requeued)
	}
}
