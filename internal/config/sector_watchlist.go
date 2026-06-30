package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type SectorWatchItem struct {
	BKCode   string `json:"bk_code"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
}

func (s *SectorWatchItem) UnmarshalJSON(data []byte) error {
	var raw struct {
		BKCodeSnake string `json:"bk_code"`
		BKCodeCamel string `json:"bkCode"`
		Name        string `json:"name"`
		Category    string `json:"category"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.BKCode = raw.BKCodeSnake
	if s.BKCode == "" {
		s.BKCode = raw.BKCodeCamel
	}
	s.Name = raw.Name
	s.Category = raw.Category
	if raw.Enabled != nil {
		s.Enabled = *raw.Enabled
	}
	return nil
}

type SectorWatchlist struct {
	Items []SectorWatchItem `json:"items"`
}

var watchlistMu sync.Mutex

func GetSectorWatchlistPath() string {
	return filepath.Join(GetProjectRoot(), ".sector-watchlist.json")
}

func LoadSectorWatchlist() SectorWatchlist {
	watchlistMu.Lock()
	defer watchlistMu.Unlock()

	var wl SectorWatchlist
	f, err := os.Open(GetSectorWatchlistPath())
	if err != nil {
		return wl
	}
	defer f.Close()

	_ = json.NewDecoder(f).Decode(&wl)
	wl.Items = normalizeWatchlistItems(wl.Items)
	return wl
}

func SaveSectorWatchlist(wl SectorWatchlist) error {
	watchlistMu.Lock()
	defer watchlistMu.Unlock()

	wl.Items = normalizeWatchlistItems(wl.Items)
	f, err := os.Create(GetSectorWatchlistPath())
	if err != nil {
		return fmt.Errorf("create sector watchlist: %w", err)
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(wl)
}

func normalizeWatchlistItems(items []SectorWatchItem) []SectorWatchItem {
	seen := make(map[string]bool)
	out := make([]SectorWatchItem, 0, len(items))
	for _, it := range items {
		if it.BKCode == "" {
			continue
		}
		key := it.BKCode
		if seen[key] {
			continue
		}
		seen[key] = true
		if it.Category == "" {
			it.Category = "industry"
		}
		out = append(out, it)
	}
	return out
}
