package tick

import (
	"reflect"
	"testing"
)

func TestBuildSectorTimeSeriesAlignsMissingTicks(t *testing.T) {
	points := []TickPoint{
		{Time: "09:35", Name: "A", Net: 1},
		{Time: "09:35", Name: "C", Net: -2},
		{Time: "09:40", Name: "B", Net: 2},
		{Time: "09:45", Name: "A", Net: 4},
		{Time: "09:45", Name: "B", Net: 1},
	}
	timeOrder := []string{"09:35", "09:40", "09:45"}

	got := buildSectorTimeSeries(points, timeOrder)
	want := map[string][]float64{
		"A": {1, 0, 3},
		"B": {0, 2, -1},
		"C": {-2, 0, 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("series must stay aligned to the global time order:\n got: %#v\nwant: %#v", got, want)
	}

	ticks, gotTimes := buildSectorTicks(points)
	if !reflect.DeepEqual(gotTimes, timeOrder) {
		t.Fatalf("chart times = %v, want %v", gotTimes, timeOrder)
	}
	if len(ticks) != len(want) {
		t.Fatalf("chart sector count = %d, want %d", len(ticks), len(want))
	}
	for _, tick := range ticks {
		if !reflect.DeepEqual(tick.Data, want[tick.Name]) {
			t.Errorf("chart data for %s = %v, want %v", tick.Name, tick.Data, want[tick.Name])
		}
	}
}

func TestTimeMinutesToFrameUsesCompressedSessionSpan(t *testing.T) {
	tests := []struct {
		name    string
		timeStr string
		session string
		want    int
	}{
		{name: "full open", timeStr: "09:30", session: "full", want: 0},
		{name: "full morning close", timeStr: "11:30", session: "full", want: 50},
		{name: "full afternoon open", timeStr: "13:00", session: "full", want: 50},
		{name: "full close", timeStr: "15:00", session: "full", want: 99},
		{name: "morning close", timeStr: "11:30", session: "morning", want: 99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := timeMinutesToFrame(minutesOf(tt.timeStr), 100, tt.session); got != tt.want {
				t.Fatalf("timeMinutesToFrame(%s, %s) = %d, want %d", tt.timeStr, tt.session, got, tt.want)
			}
		})
	}
}

func TestTickDataDrivenGeneratePlacesAfternoonEventOnCompressedTimeline(t *testing.T) {
	points := []TickPoint{
		{Time: "09:30", Name: "半导体", Net: 0},
		{Time: "13:00", Name: "半导体", Net: 10},
	}
	events, _, _ := TickDataDrivenGenerate(points, "full")
	for _, event := range events {
		if event.Text == "半导体资金加速涌入" {
			if event.Frame != 50 {
				t.Fatalf("13:00 event frame = %d, want 50", event.Frame)
			}
			return
		}
	}
	t.Fatalf("afternoon spike event not found: %+v", events)
}
