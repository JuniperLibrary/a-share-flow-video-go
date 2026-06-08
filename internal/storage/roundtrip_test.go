package storage

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadTickSectors_VolumeTurnover_Roundtrip(t *testing.T) {
	tmp := t.TempDir()
	db, err := New(filepath.Join(tmp, "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	original := []Sector{
		{
			Datetime:  "2026-06-03 09:30",
			Name:      "半导体",
			Net:       1.23,
			Rate:      5.4,
			ChangePct: 2.1,
			SuperNet:  0.8,
			SuperRate: 3.5,
			BigNet:    0.4,
			BigRate:   1.9,
			Volume:    12345.0,
			Turnover:  1.85,
		},
	}

	if err := db.SaveSectors(original); err != nil {
		t.Fatalf("SaveSectors: %v", err)
	}

	loaded, err := db.LoadTickSectors("2026-06-03")
	if err != nil {
		t.Fatalf("LoadTickSectors: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 row, got %d", len(loaded))
	}

	got := loaded[0]
	if got.Volume != original[0].Volume {
		t.Errorf("Volume: got %v, want %v", got.Volume, original[0].Volume)
	}
	if got.Turnover != original[0].Turnover {
		t.Errorf("Turnover: got %v, want %v", got.Turnover, original[0].Turnover)
	}
	if got.Name != "半导体" {
		t.Errorf("Name: got %q, want 半导体", got.Name)
	}
}

func TestSaveLoadSectorsAll_VolumeTurnover_Roundtrip(t *testing.T) {
	tmp := t.TempDir()
	db, err := New(filepath.Join(tmp, "test2.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	original := []SectorAll{
		{
			Date:      "2026-06-03",
			Code:      "BK1234",
			Name:      "AI应用",
			Net:       2.5,
			Rate:      8.0,
			ChangePct: 3.2,
			Volume:    56789.0,
			Turnover:  3.21,
			Category:  "industry",
		},
	}

	if err := db.SaveSectorsAll(original); err != nil {
		t.Fatalf("SaveSectorsAll: %v", err)
	}

	loaded, err := db.LoadSectorsAll("2026-06-03")
	if err != nil {
		t.Fatalf("LoadSectorsAll: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 row, got %d", len(loaded))
	}

	got := loaded[0]
	if got.Volume != original[0].Volume {
		t.Errorf("Volume: got %v, want %v", got.Volume, original[0].Volume)
	}
	if got.Turnover != original[0].Turnover {
		t.Errorf("Turnover: got %v, want %v", got.Turnover, original[0].Turnover)
	}
}
