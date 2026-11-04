package web

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/a-share-flow-video-go/internal/config"
	"github.com/a-share-flow-video-go/internal/fetcher"
	"github.com/a-share-flow-video-go/internal/storage"
)

func TestWriteSectorsAllCSV(t *testing.T) {
	dateDir := t.TempDir()
	sectors := []storage.SectorAll{
		{Date: "2026-05-11", Code: "BK1000", Name: "人工智能", Net: 12.345, Rate: 6.78, Category: "concept"},
		{Date: "2026-05-11", Code: "BK2000", Name: "银行", Net: -3.21, Rate: -1.23, Category: "industry"},
	}

	path, err := writeSectorsAllCSV(dateDir, "2026-05-11", sectors)
	if err != nil {
		t.Fatalf("writeSectorsAllCSV returned error: %v", err)
	}
	wantPath := filepath.Join(dateDir, "板块全量_2026-05-11.csv")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open exported CSV: %v", err)
	}
	records, err := csv.NewReader(file).ReadAll()
	file.Close()
	if err != nil {
		t.Fatalf("read exported CSV: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected header plus two rows, got %d records", len(records))
	}
	if records[0][0] != "name" || records[0][1] != "net" || records[0][2] != "rate" {
		t.Fatalf("unexpected header prefix: %v", records[0][:3])
	}
	if records[1][0] != "人工智能" || records[1][1] != "12.35" || records[1][3] != "2026-05-11" {
		t.Fatalf("unexpected first data row: %v", records[1])
	}

	// Keep the first three columns compatible with the existing historical loader.
	loaded, err := fetcher.LoadFullSectorCSV(path)
	if err != nil {
		t.Fatalf("LoadFullSectorCSV could not read export: %v", err)
	}
	if len(loaded) != 2 || loaded[0].Name != "人工智能" || loaded[0].Net != 12.35 || loaded[0].Rate != 6.78 {
		t.Fatalf("unexpected loader result: %+v", loaded)
	}

	temporaryFiles, err := filepath.Glob(filepath.Join(dateDir, ".sectors-all-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary files left behind: %v", temporaryFiles)
	}
}

func TestWriteSectorsAllCSVRejectsEmptyData(t *testing.T) {
	if _, err := writeSectorsAllCSV(t.TempDir(), "2026-05-11", nil); err == nil {
		t.Fatal("expected empty data to be rejected")
	}
}

func TestRunExportTaskHistoricalUsesStoredData(t *testing.T) {
	root := t.TempDir()
	oldRoot := config.GetProjectRoot()
	config.SetProjectRoot(root)
	storage.Reset()
	t.Cleanup(func() {
		storage.Reset()
		config.SetProjectRoot(oldRoot)
	})

	db, err := storage.Get()
	if err != nil {
		t.Fatalf("storage.Get: %v", err)
	}
	stored := []storage.SectorAll{{
		Date: "2000-01-01", Code: "BK1000", Name: "历史板块", Net: 8.75, Rate: 3.21, Category: "industry",
	}}
	if err := db.SaveSectorsAll(stored); err != nil {
		t.Fatalf("SaveSectorsAll: %v", err)
	}

	task := &ExportTask{ID: "history-test", Date: "2000-01-01", Status: "pending"}
	runExportTask(task)
	if task.Status != "done" {
		t.Fatalf("task status = %s, error = %s", task.Status, task.Err)
	}
	if task.FilePath == "" {
		t.Fatal("completed task did not record its CSV path")
	}
	records, err := os.ReadFile(task.FilePath)
	if err != nil {
		t.Fatalf("read task CSV: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("task CSV is empty")
	}

	loaded, err := db.LoadSectorsAll("2000-01-01")
	if err != nil || len(loaded) != 1 || loaded[0].Net != 8.75 {
		t.Fatalf("historical snapshot changed: rows=%+v, err=%v", loaded, err)
	}
}
