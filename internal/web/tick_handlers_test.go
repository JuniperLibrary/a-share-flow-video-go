package web

import (
	"testing"
	"time"
)

func TestValidateForceCollectTime(t *testing.T) {
	now := time.Date(2026, 5, 11, 9, 30, 30, 0, webShanghaiTZ)
	tests := []struct {
		name    string
		date    string
		timeStr string
		now     time.Time
		wantErr bool
	}{
		{name: "current trading minute", date: "2026-05-11", timeStr: "09:30", now: now},
		{name: "historical date", date: "2026-05-08", timeStr: "09:30", now: now, wantErr: true},
		{name: "future date", date: "2026-05-12", timeStr: "09:30", now: now, wantErr: true},
		{name: "future minute", date: "2026-05-11", timeStr: "09:35", now: now, wantErr: true},
		{name: "past minute", date: "2026-05-11", timeStr: "09:29", now: now, wantErr: true},
		{name: "invalid time", date: "2026-05-11", timeStr: "bad", now: now, wantErr: true},
		{
			name: "lunch break",
			date: "2026-05-11", timeStr: "12:00",
			now: time.Date(2026, 5, 11, 12, 0, 0, 0, webShanghaiTZ), wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateForceCollectTime(tt.date, tt.timeStr, tt.now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateForceCollectTime() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}
