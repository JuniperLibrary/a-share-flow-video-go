package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSectorWatchlist_ReadWrite(t *testing.T) {
	dir := t.TempDir()
	SetProjectRoot(dir)

	wl := SectorWatchlist{
		Items: []SectorWatchItem{
			{BKCode: "BK0001", Name: "测试行业", Category: "industry", Enabled: true},
			{BKCode: "BK0002", Name: "测试概念", Category: "concept", Enabled: false},
		},
	}
	if err := SaveSectorWatchlist(wl); err != nil {
		t.Fatalf("SaveSectorWatchlist: %v", err)
	}

	p := filepath.Join(dir, ".sector-watchlist.json")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("watchlist file not exists: %v", err)
	}

	got := LoadSectorWatchlist()
	if len(got.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(got.Items))
	}
	if got.Items[0].BKCode == "" || got.Items[1].BKCode == "" {
		t.Fatalf("bk_code should not be empty")
	}
}
