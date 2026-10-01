package domain

import (
	"math"
	"testing"
)

func TestApplyArrivalRangeRequiresHistoryAndAcceptableHoldoutCoverage(t *testing.T) {
	base := EstimateLatenessProbability("DEPOT_NORTH", "Fresh", 12, 3)
	p10, p90, good, poor := -8.26, 16.76, 0.82, 0.94
	tests := []struct {
		name, want        string
		samples, holdouts int
		coverage          *float64
	}{
		{name: "too little history", samples: ArrivalRangeMinSamples - 1, holdouts: 15, coverage: &good, want: "INSUFFICIENT_HISTORY"},
		{name: "too few holdouts", samples: 24, holdouts: ArrivalRangeMinHoldout - 1, coverage: &good, want: "INSUFFICIENT_HOLDOUT"},
		{name: "poor coverage", samples: 24, holdouts: 15, coverage: &poor, want: "POOR_COVERAGE"},
		{name: "calibrated", samples: 24, holdouts: 15, coverage: &good, want: "CALIBRATED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyArrivalRange(base, tt.samples, tt.holdouts, tt.coverage, &p10, &p90)
			if got.ArrivalRangeStatus != tt.want {
				t.Fatalf("status=%q want %q", got.ArrivalRangeStatus, tt.want)
			}
			if tt.want == "CALIBRATED" {
				if got.ArrivalOffsetP10 == nil || *got.ArrivalOffsetP10 != -8.3 || got.ArrivalOffsetP90 == nil || *got.ArrivalOffsetP90 != 16.8 {
					t.Fatalf("interval offsets=%v..%v, want -8.3..16.8", got.ArrivalOffsetP10, got.ArrivalOffsetP90)
				}
			} else if got.ArrivalOffsetP10 != nil || got.ArrivalOffsetP90 != nil {
				t.Fatalf("unqualified interval must be withheld: %v..%v", got.ArrivalOffsetP10, got.ArrivalOffsetP90)
			}
		})
	}
}

func TestEstimateLatenessProbability(t *testing.T) {
	tests := []struct {
		name        string
		samples     int
		late        int
		status      string
		probability *float64
	}{
		{name: "insufficient history is withheld", samples: 9, late: 9, status: "INSUFFICIENT_HISTORY"},
		{name: "smoothed empirical rate", samples: 10, late: 4, status: "ESTIMATED", probability: floatPointer(5.0 / 12.0)},
		{name: "zero late arrivals avoids false certainty", samples: 10, late: 0, status: "ESTIMATED", probability: floatPointer(1.0 / 12.0)},
		{name: "invalid aggregate is withheld", samples: 10, late: 11, status: "INSUFFICIENT_HISTORY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateLatenessProbability("DEPOT_NORTH", "Fresh", tt.samples, tt.late)
			if got.Status != tt.status || (got.Probability == nil) != (tt.probability == nil) {
				t.Fatalf("got status=%q probability=%v, want status=%q probability=%v", got.Status, got.Probability, tt.status, tt.probability)
			}
			if got.Probability != nil && math.Abs(*got.Probability-*tt.probability) > 0.0001 {
				t.Fatalf("probability=%v, want %v", *got.Probability, *tt.probability)
			}
			if got.ModelVersion != LatenessModelVersion || got.HistoryDays != 90 {
				t.Fatalf("model metadata = %q/%d", got.ModelVersion, got.HistoryDays)
			}
		})
	}
}

func TestApplyLatenessCalibrationWithholdsWeakHoldouts(t *testing.T) {
	score := 0.23456
	invalid := 1.2
	tests := []struct {
		name      string
		samples   int
		brier     *float64
		status    string
		wantScore *float64
	}{
		{name: "too few predictions", samples: 9, brier: &score, status: "INSUFFICIENT_HOLDOUT"},
		{name: "missing score", samples: 12, status: "INSUFFICIENT_HOLDOUT"},
		{name: "valid rolling holdout", samples: 12, brier: &score, status: "EVALUATED", wantScore: floatPointer(0.2346)},
		{name: "out-of-range score", samples: 12, brier: &invalid, status: "INSUFFICIENT_HOLDOUT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyLatenessCalibration(EstimateLatenessProbability("DEPOT_NORTH", "Fresh", 12, 3), tt.samples, tt.brier)
			if got.CalibrationStatus != tt.status || (got.BrierScore == nil) != (tt.wantScore == nil) {
				t.Fatalf("status=%q score=%v, want status=%q score=%v", got.CalibrationStatus, got.BrierScore, tt.status, tt.wantScore)
			}
			if got.BrierScore != nil && *got.BrierScore != *tt.wantScore {
				t.Fatalf("score=%v, want %v", *got.BrierScore, *tt.wantScore)
			}
			if got.CalibrationVersion != LatenessCalibrationVersion {
				t.Fatalf("calibration version=%q", got.CalibrationVersion)
			}
		})
	}
}

func floatPointer(value float64) *float64 { return &value }
