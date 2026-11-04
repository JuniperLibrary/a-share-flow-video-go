package fetcher

import (
	"strings"
	"testing"
)

func TestFetchSectorsAllDailyRejectsHistoricalDateBeforeFetching(t *testing.T) {
	_, err := FetchSectorsAllDaily("2000-01-01")
	if err == nil {
		t.Fatal("expected historical date to be rejected")
	}
	if !strings.Contains(err.Error(), "历史日期") {
		t.Fatalf("expected historical-data guidance, got %v", err)
	}
}
