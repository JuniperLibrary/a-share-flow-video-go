package fetcher

import (
	"testing"

	"github.com/a-share-flow-video-go/internal/config"
)

func TestRoundTo2(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{2.255, 2.26},
		{-2.255, -2.26},
		{0.005, 0.01},
		{-0.005, -0.01},
		{123.456, 123.46},
		{-123.456, -123.46},
		{0, 0},
		{1.00, 1},
	}
	for _, tt := range tests {
		got := roundTo2(tt.input)
		if got != tt.want {
			t.Errorf("roundTo2(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestAbsF(t *testing.T) {
	if absF(-5.3) != 5.3 {
		t.Errorf("absF(-5.3) = %v", absF(-5.3))
	}
	if absF(5.3) != 5.3 {
		t.Errorf("absF(5.3) = %v", absF(5.3))
	}
}

func TestSaveAndLoadCSV(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetProjectRoot(tmpDir)

	sectors := []Sector{
		{Name: "半导体", Net: 225.5, Color: "#00F0FF"},
		{Name: "银行", Net: -45.9, Color: "#FF6B8A"},
		{Name: "白酒", Net: 0, Color: "#FFD700"},
	}

	err := SaveDailyData(sectors, "2026-05-13")
	if err != nil {
		t.Fatalf("SaveDailyData failed: %v", err)
	}

	loaded, err := LoadCachedData("2026-05-13")
	if err != nil {
		t.Fatalf("LoadCachedData failed: %v", err)
	}

	if len(loaded) != len(sectors) {
		t.Fatalf("loaded %d sectors, want %d", len(loaded), len(sectors))
	}

	for i, want := range sectors {
		got := loaded[i]
		if got.Name != want.Name {
			t.Errorf("sector[%d].Name = %q, want %q", i, got.Name, want.Name)
		}
		if got.Net != want.Net {
			t.Errorf("sector[%d].Net = %v, want %v", i, got.Net, want.Net)
		}
		if got.Color != want.Color {
			t.Errorf("sector[%d].Color = %q, want %q", i, got.Color, want.Color)
		}
	}
}

func TestTop18HotSectors_Filtering(t *testing.T) {
	df := []Sector{
		{Name: "半导体", Net: 225.5},
		{Name: "银行", Net: 46.4},
		{Name: "白酒", Net: -18.2},
		{Name: "人工智能", Net: -45.9},
	}

	targetSet := make(map[string]bool)
	for _, t := range Top18HotSectors {
		targetSet[t] = true
	}

	var matched, unmatched []Sector
	for _, s := range df {
		if targetSet[s.Name] {
			matched = append(matched, s)
		} else {
			unmatched = append(unmatched, s)
		}
	}

	if len(matched) != 4 {
		t.Errorf("matched %d sectors, want 4", len(matched))
	}

	var inflow, outflow []Sector
	for _, s := range matched {
		if s.Net >= 0 {
			inflow = append(inflow, s)
		} else {
			outflow = append(outflow, s)
		}
	}

	if len(inflow) != 2 {
		t.Errorf("inflow %d sectors, want 2", len(inflow))
	}
	if len(outflow) != 2 {
		t.Errorf("outflow %d sectors, want 2", len(outflow))
	}
}

func TestFetchTop18HotSectors_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test")
	}

	sectors, err := FetchTop18HotSectors()
	if err != nil {
		t.Fatalf("FetchTop18HotSectors failed: %v", err)
	}

	if len(sectors) == 0 {
		t.Fatal("FetchTop18HotSectors returned empty sectors (API may be blocked or non-trading day)")
	}

	if len(sectors) < config.MinSectorCount {
		t.Errorf("got %d sectors, want at least %d", len(sectors), config.MinSectorCount)
	}

	for i, s := range sectors {
		if s.Name == "" {
			t.Errorf("sector[%d].Name is empty", i)
		}
		if s.Color == "" {
			t.Errorf("sector[%d].Color is empty", i)
		}
	}

	inflowCount := 0
	for _, s := range sectors {
		if s.Net > 0 {
			inflowCount++
		}
	}

	t.Logf("Fetched %d sectors: %d inflow, %d outflow", len(sectors), inflowCount, len(sectors)-inflowCount)
	if inflowCount > 0 {
		t.Logf("Top inflow: %s %.1f亿", sectors[0].Name, sectors[0].Net)
	}
	if inflowCount < len(sectors) {
		t.Logf("Top outflow: %s %.1f亿", sectors[inflowCount].Name, sectors[inflowCount].Net)
	}
}

func TestFetchHistoricalSectors_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test")
	}

	sectors, err := FetchHistoricalSectors("2026-05-12")
	if err != nil {
		t.Fatalf("FetchHistoricalSectors failed: %v", err)
	}

	if len(sectors) == 0 {
		t.Log("FetchHistoricalSectors returned empty (may be expected for this date)")
		return
	}

	t.Logf("Fetched %d historical sectors", len(sectors))
}

func TestSaveLoadRoundtrip_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API test")
	}

	tmpDir := t.TempDir()
	config.SetProjectRoot(tmpDir)

	sectors, err := FetchTop18HotSectors()
	if err != nil || len(sectors) == 0 {
		t.Skipf("API returned no data: %v", err)
	}

	err = SaveDailyData(sectors, "2026-05-13")
	if err != nil {
		t.Fatalf("SaveDailyData failed: %v", err)
	}

	loaded, err := LoadCachedData("2026-05-13")
	if err != nil {
		t.Fatalf("LoadCachedData failed: %v", err)
	}

	if len(loaded) != len(sectors) {
		t.Fatalf("roundtrip: loaded %d, saved %d", len(loaded), len(sectors))
	}

	for i := range sectors {
		if loaded[i].Name != sectors[i].Name {
			t.Errorf("roundtrip[%d].Name = %q, want %q", i, loaded[i].Name, sectors[i].Name)
		}
		if loaded[i].Net != sectors[i].Net {
			t.Errorf("roundtrip[%d].Net = %v, want %v", i, loaded[i].Net, sectors[i].Net)
		}
	}

	t.Logf("Roundtrip OK: %d sectors saved and loaded", len(sectors))
}

func TestNewRequest_Error(t *testing.T) {
	_, err := newRequest("GET", "://invalid-url")
	if err == nil {
		t.Error("newRequest should return error for invalid URL")
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input any
		want  float64
		ok    bool
	}{
		{float64(123.45), 123.45, true},
		{"456.78", 456.78, true},
		{nil, 0, false},
		{int(100), 0, false},
	}
	for _, tt := range tests {
		got, ok := toFloat64(tt.input)
		if ok != tt.ok {
			t.Errorf("toFloat64(%v) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("toFloat64(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestGetField(t *testing.T) {
	colIdx := map[string]int{"name": 0, "net": 1, "color": 2}
	record := []string{"半导体", "225.50", "#00F0FF"}

	if getField(record, colIdx, "name") != "半导体" {
		t.Error("getField name mismatch")
	}
	if getField(record, colIdx, "net") != "225.50" {
		t.Error("getField net mismatch")
	}
	if getField(record, colIdx, "missing") != "" {
		t.Error("getField missing should return empty")
	}
}

func TestColorPaletteAssignment(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetProjectRoot(tmpDir)

	sectors, err := FetchHistoricalSectors("2026-05-12")
	if err != nil || len(sectors) == 0 {
		t.Skipf("API returned no data: %v", err)
	}

	paletteSet := make(map[string]bool)
	for _, c := range ColorPalette {
		paletteSet[c] = true
	}

	for i, s := range sectors {
		if !paletteSet[s.Color] {
			t.Errorf("sector[%d].Color %q not in ColorPalette", i, s.Color)
		}
	}

	inflowCount := 0
	for _, s := range sectors {
		if s.Net > 0 {
			inflowCount++
		}
	}

	t.Logf("Fetched %d sectors: %d inflow, %d outflow", len(sectors), inflowCount, len(sectors)-inflowCount)
}

func TestNewRequest_Success(t *testing.T) {
	req, err := newRequest("GET", "https://example.com/test")
	if err != nil {
		t.Fatalf("newRequest failed: %v", err)
	}
	if req.Method != "GET" {
		t.Errorf("req.Method = %q, want GET", req.Method)
	}
	if req.URL.String() != "https://example.com/test" {
		t.Errorf("req.URL = %q, want https://example.com/test", req.URL.String())
	}
	if req.Header.Get("User-Agent") == "" {
		t.Error("User-Agent header not set")
	}
	if req.Header.Get("Referer") == "" {
		t.Error("Referer header not set")
	}
}
