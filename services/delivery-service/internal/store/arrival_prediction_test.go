package store

import (
    "testing"
    "time"
)

func TestArrivalChangeThreshold(t *testing.T) {
    old := time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)
    for _, tc := range []struct {
        name string
        delta time.Duration
        want bool
    }{
        {"unchanged", 0, false},
        {"under later", 29*time.Minute + 59*time.Second, false},
        {"under earlier", -(29*time.Minute + 59*time.Second), false},
        {"exact later", 30*time.Minute, true},
        {"exact earlier", -30*time.Minute, true},
        {"over later", 31*time.Minute, true},
    } {
        t.Run(tc.name, func(t *testing.T) {
            if got := arrivalChange(old, old.Add(tc.delta)); got != tc.want {
                t.Fatalf("arrivalChange(%s) = %v, want %v", tc.delta, got, tc.want)
            }
        })
    }
}
