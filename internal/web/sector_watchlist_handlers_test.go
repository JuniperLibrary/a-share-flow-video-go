package web

import (
	"testing"
	"time"

	"github.com/a-share-flow-video-go/internal/config"
)

func TestIsSectorEditForbiddenAt(t *testing.T) {
	loc := webShanghaiTZ

	tcases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"before_open", time.Date(2026, 6, 29, 9, 29, 0, 0, loc), false},
		{"open_0930", time.Date(2026, 6, 29, 9, 30, 0, 0, loc), true},
		{"noon", time.Date(2026, 6, 29, 12, 0, 0, 0, loc), true},
		{"close_1500", time.Date(2026, 6, 29, 15, 0, 0, 0, loc), true},
		{"after_close", time.Date(2026, 6, 29, 15, 1, 0, 0, loc), false},
		{"weekend", time.Date(2026, 6, 28, 10, 0, 0, 0, loc), false},
	}

	for _, tc := range tcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := isSectorEditForbiddenAt(tc.t); got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestMergeWatchlistWithDefaults(t *testing.T) {
	existing := []config.SectorWatchItem{
		{BKCode: "BK1", Name: "A", Category: "industry", Enabled: false},
		{BKCode: "BK2", Name: "B", Category: "concept", Enabled: true},
	}
	defaults := []config.SectorWatchItem{
		{BKCode: "BK1", Name: "A默认", Category: "industry", Enabled: true},
		{BKCode: "BK3", Name: "C默认", Category: "region", Enabled: true},
	}
	got, changed := mergeWatchlistWithDefaults(existing, defaults)
	if !changed {
		t.Fatalf("should be changed")
	}
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	if got[0].BKCode != "BK1" || got[0].Enabled != false {
		t.Fatalf("BK1 should keep enabled=false, got %+v", got[0])
	}
	foundBK3 := false
	for _, it := range got {
		if it.BKCode == "BK3" {
			foundBK3 = true
			if !it.Enabled || it.Name == "" {
				t.Fatalf("BK3 should exist and enabled, got %+v", it)
			}
		}
	}
	if !foundBK3 {
		t.Fatalf("BK3 should be appended")
	}
}
