package store

import (
	"testing"
	"time"
)

func TestMemoryForecastUsesEmptyArraysForCollectionFields(t *testing.T) {
	got, err := (&Memory{}).Forecast(time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.InputDrift == nil || got.Weekly == nil || got.ServiceTimeEvaluation == nil || got.Capacity == nil {
		t.Fatalf("forecast collection fields must serialize as empty arrays, got inputDrift=%v weekly=%v serviceTimeEvaluation=%v capacity=%v", got.InputDrift, got.Weekly, got.ServiceTimeEvaluation, got.Capacity)
	}
}
