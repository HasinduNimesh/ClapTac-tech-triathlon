package service

import "testing"

func TestISOWeekRangeColomboYearBoundary(t *testing.T) {
	start, end, err := ISOWeekRange("2026-01-04") // Sunday, ISO week 1.
	if err != nil {
		t.Fatal(err)
	}
	if start != "2025-12-29" || end != "2026-01-04" {
		t.Fatalf("wrong week range: %s through %s", start, end)
	}
}

func TestISOWeekRangeRejectsInvalidDate(t *testing.T) {
	if _, _, err := ISOWeekRange("not-a-date"); err == nil {
		t.Fatal("expected invalid date to be rejected")
	}
}
