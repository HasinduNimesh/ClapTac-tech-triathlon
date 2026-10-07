package store

import (
	"math"
	"testing"
	"time"
)

func TestHaversineKnownDistances(t *testing.T) {
	// 0.01 degrees of latitude is about 1.112 km anywhere.
	if got := haversine(6.9271, 79.8612, 6.9371, 79.8612); math.Abs(got-1111.9) > 2 {
		t.Fatalf("0.01 degrees north = %.1f m", got)
	}
	// Colombo Fort to Kandy is roughly 94 km in a straight line.
	if got := haversine(6.9344, 79.8428, 7.2906, 80.6337) / 1000; got < 90 || got > 98 {
		t.Fatalf("Colombo to Kandy = %.1f km", got)
	}
	if haversine(6.9, 79.8, 6.9, 79.8) != 0 {
		t.Fatal("the same point is zero metres away")
	}
}

func TestStepDistanceCountsDrivingAndIgnoresNoise(t *testing.T) {
	at := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	if d := stepDistance(6.9271, 79.8612, at, 6.9371, 79.8612, at.Add(60*time.Second)); d < 1100 || d > 1125 {
		t.Fatalf("1.11 km in a minute is driving: %.1f", d)
	}
	if d := stepDistance(6.9271, 79.8612, at, 6.92715, 79.86125, at.Add(30*time.Second)); d != 0 {
		t.Fatalf("a few metres is jitter: %.1f", d)
	}
	if d := stepDistance(6.9271, 79.8612, at, 7.5, 79.8612, at.Add(10*time.Second)); d != 0 {
		t.Fatalf("60 km in 10 s is a bad fix: %.1f", d)
	}
	if d := stepDistance(6.9271, 79.8612, at, 6.9371, 79.8612, at); d != 0 {
		t.Fatalf("no time passed: %.1f", d)
	}
	if d := stepDistance(6.9271, 79.8612, at, 6.9371, 79.8612, at.Add(-time.Minute)); d != 0 {
		t.Fatalf("an older report adds nothing: %.1f", d)
	}
}
