package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidDateFormat(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"2026-06-24", true},
		{"2026-01-01", true},
		{"2026-12-31", true},
		{"", false},
		{"2026/06/24", false},
		{"2026-6-24", false},
		{"24-06-2026", false},
		{"20260624", false},
		{"2026-13-01", false},  // invalid month
		{"2026-00-01", false},  // zero month
		{"2026-01-32", false},  // invalid day
		{"abc", false},
	}
	for _, tc := range tests {
		got := validDateFormat(tc.input)
		assert.Equal(t, tc.want, got, "validDateFormat(%q)", tc.input)
	}
}

func TestExtractReportMetrics_Empty(t *testing.T) {
	m := extractReportMetrics("")
	assert.Equal(t, reportMetrics{}, m)
}

func TestExtractReportMetrics_Full(t *testing.T) {
	json := `{
		"netTotal": 123.45,
		"inflowCount": 12,
		"outflowCount": 9,
		"superNetTotal": 67.89,
		"bigNetTotal": 55.56,
		"structureDesc": "超大单主导"
	}`
	m := extractReportMetrics(json)
	assert.Equal(t, 123.45, m.NetTotal)
	assert.Equal(t, 12, m.InflowCount)
	assert.Equal(t, 9, m.OutflowCount)
	assert.Equal(t, 67.89, m.SuperNetTotal)
	assert.Equal(t, 55.56, m.BigNetTotal)
	assert.Equal(t, "超大单主导", m.StructureDesc)
}

func TestExtractReportMetrics_Partial(t *testing.T) {
	json := `{"netTotal": 42.0}`
	m := extractReportMetrics(json)
	assert.Equal(t, 42.0, m.NetTotal)
	assert.Equal(t, 0, m.InflowCount)       // zero value
	assert.Equal(t, "", m.StructureDesc)     // zero value
}

func TestExtractReportMetrics_Malformed(t *testing.T) {
	m := extractReportMetrics("{bad json")
	assert.Equal(t, reportMetrics{}, m)
}

func TestDailyReportResponse(t *testing.T) {
	resp := dailyReportResponse("2026-06-24", "full", "今日总结", "明日展望", `{"netTotal":10.5,"inflowCount":5}`, true)

	assert.Equal(t, "2026-06-24", resp["date"])
	assert.Equal(t, "full", resp["session"])
	assert.Equal(t, "今日总结", resp["summary"])
	assert.Equal(t, "明日展望", resp["outlook"])
	assert.Equal(t, true, resp["generated"])
	assert.Equal(t, 10.5, resp["netTotal"])
	assert.Equal(t, 5, resp["inflowCount"])
	assert.Equal(t, 0, resp["outflowCount"])
}

func TestDailyReportResponse_NoGenerated(t *testing.T) {
	resp := dailyReportResponse("2026-06-24", "full", "", "", "", false)
	_, exists := resp["generated"]
	assert.False(t, exists, "generated key should not be present when false")
}
