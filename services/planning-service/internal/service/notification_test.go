package service

import (
	"testing"
	"time"
)

func TestMajorDelayThresholdIsThirtyWholeMinutes(t *testing.T) {
	base := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		delay time.Duration
		want  int
	}{{"missing ETA", 0, 0}, {"on time", 0, 0}, {"early", -5 * time.Minute, 0}, {"below threshold", 29*time.Minute + 59*time.Second, 29}, {"threshold", 30 * time.Minute, 30}} {
		t.Run(tc.name, func(t *testing.T) {
			var next *time.Time
			if tc.name != "missing ETA" {
				v := base.Add(tc.delay)
				next = &v
			}
			if got := majorDelayMinutes(&base, next); got != tc.want {
				t.Fatalf("majorDelayMinutes()=%d want %d", got, tc.want)
			}
		})
	}
}
