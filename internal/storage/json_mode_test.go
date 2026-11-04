package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-share-flow-video-go/internal/config"
)

func useStorageTestRoot(t *testing.T, root string) {
	t.Helper()
	oldRoot := config.GetProjectRoot()
	config.SetProjectRoot(root)
	t.Cleanup(func() { config.SetProjectRoot(oldRoot) })
}

func TestJSONModeRestoresHistoryAcrossRestart(t *testing.T) {
	root := t.TempDir()
	useStorageTestRoot(t, root)
	t.Setenv("DATA_MODE", "json")
	Reset()
	t.Cleanup(Reset)

	dataDir := config.GetDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	history := `[{"date":"2026-05-10","session":"full","type":"template","content":"历史文案"}]`
	if err := os.WriteFile(filepath.Join(dataDir, "copywriting.json"), []byte(history), 0644); err != nil {
		t.Fatal(err)
	}
	// Deliberately omit recently added classification fields to verify that
	// old snapshots receive schema defaults instead of failing the import.
	oldNews := `[{"id":123,"title":"历史新闻","content":"","brief":"","level":"A","reading_num":10,"ctime":"2026-05-10 12:00:00","shareurl":"","sectors":"[]","created_at":"2026-05-10 12:00:00"}]`
	if err := os.WriteFile(filepath.Join(dataDir, "cls_news.json"), []byte(oldNews), 0644); err != nil {
		t.Fatal(err)
	}

	db, err := Get()
	if err != nil {
		t.Fatalf("Get with existing history: %v", err)
	}
	copyRows, err := db.LoadCopywriting("2026-05-10")
	if err != nil || len(copyRows) != 1 || copyRows[0].Content != "历史文案" {
		t.Fatalf("history was not restored: rows=%+v, err=%v", copyRows, err)
	}
	newsRows, err := db.RawRows("SELECT id, classify_status FROM cls_news WHERE id = 123")
	if err != nil || len(newsRows) != 1 || newsRows[0]["classify_status"] != "pending" {
		t.Fatalf("old news defaults were not applied: rows=%+v, err=%v", newsRows, err)
	}

	if err := db.SaveCopywriting(Copywriting{Date: "2026-05-10", Session: "full", Type: "ai", Content: "新增文案"}); err != nil {
		t.Fatalf("SaveCopywriting: %v", err)
	}
	if err := db.ExportJSON(); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	emptySectors, err := os.ReadFile(filepath.Join(dataDir, "sectors.json"))
	if err != nil || strings.TrimSpace(string(emptySectors)) != "[]" {
		t.Fatalf("empty tables must export as [] rather than null: %q, err=%v", emptySectors, err)
	}

	Reset()
	db, err = Get()
	if err != nil {
		t.Fatalf("restart with exported history: %v", err)
	}
	copyRows, err = db.LoadCopywriting("2026-05-10")
	if err != nil || len(copyRows) != 2 {
		t.Fatalf("restart lost copywriting: rows=%+v, err=%v", copyRows, err)
	}
}

func TestJSONModeRejectsMalformedHistory(t *testing.T) {
	root := t.TempDir()
	useStorageTestRoot(t, root)
	t.Setenv("DATA_MODE", "json")
	Reset()
	t.Cleanup(Reset)

	if err := os.MkdirAll(config.GetDataDir(), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config.GetDataDir(), "copywriting.json")
	if err := os.WriteFile(path, []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
	if db, err := Get(); err == nil || db != nil {
		t.Fatalf("malformed history must fail initialization: db=%v, err=%v", db, err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "{" {
		t.Fatalf("malformed history was modified: %q, err=%v", content, err)
	}
}

func TestExportJSONReturnsWriteErrors(t *testing.T) {
	root := t.TempDir()
	useStorageTestRoot(t, root)
	if err := os.MkdirAll(filepath.Join(config.GetDataDir(), "copywriting.json"), 0755); err != nil {
		t.Fatal(err)
	}
	db, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.ExportJSON(); err == nil || !strings.Contains(err.Error(), "copywriting.json") {
		t.Fatalf("expected copywriting write error, got %v", err)
	}
}
