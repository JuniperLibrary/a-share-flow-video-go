package fetcher

import (
	"fmt"
	"testing"
)

func TestFetchFinanceReport(t *testing.T) {
	report, err := FetchFinanceReport("600519")
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	fmt.Println(FormatFinanceReport(report))
}
