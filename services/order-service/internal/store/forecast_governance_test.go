package store

import "testing"

func TestClassifyForecastDrift(t *testing.T) {
	tests := []struct {
		name             string
		previous, recent int
		status           string
		change           *float64
	}{
		{name: "insufficient observations are not treated as stable", previous: 4, recent: 5, status: "INSUFFICIENT_HISTORY"},
		{name: "new demand with enough recent observations", previous: 0, recent: 10, status: "NEW_DEMAND"},
		{name: "positive shift", previous: 10, recent: 15, status: "SHIFTED", change: floatPtr(50)},
		{name: "negative shift", previous: 20, recent: 9, status: "SHIFTED", change: floatPtr(-55)},
		{name: "within threshold", previous: 20, recent: 29, status: "STABLE", change: floatPtr(45)},
		{name: "invalid counts fail closed", previous: -1, recent: 15, status: "INSUFFICIENT_HISTORY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, change := classifyForecastDrift(tt.previous, tt.recent)
			if status != tt.status {
				t.Fatalf("status = %q, want %q", status, tt.status)
			}
			if (change == nil) != (tt.change == nil) {
				t.Fatalf("change = %v, want %v", change, tt.change)
			}
			if change != nil && *change != *tt.change {
				t.Fatalf("change = %v, want %v", *change, *tt.change)
			}
		})
	}
}

func TestForecastAPERequiresMeaningfulHoldoutCount(t *testing.T) {
	tests := []struct {
		name              string
		predicted, actual int
		want              *float64
	}{
		{name: "perfect estimate", predicted: 12, actual: 12, want: floatPtr(0)},
		{name: "missed half the demand", predicted: 5, actual: 10, want: floatPtr(50)},
		{name: "small holdout suppressed", predicted: 5, actual: 9},
		{name: "invalid baseline suppressed", predicted: -1, actual: 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := forecastAPE(tt.predicted, tt.actual)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("APE = %v, want %v", got, tt.want)
			}
			if got != nil && *got != *tt.want {
				t.Fatalf("APE = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func floatPtr(value float64) *float64 { return &value }
