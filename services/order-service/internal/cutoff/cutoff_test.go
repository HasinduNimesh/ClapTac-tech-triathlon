package cutoff

import (
	"testing"
	"time"
)

func TestSaturdayAfterCutoffSkipsSunday(t *testing.T) {
	loc, err := time.LoadLocation(Zone)
	if err != nil {
		t.Fatal(err)
	}
	cal := Load([]Day{
		{Date: date(2026, 9, 26, loc), IsOperating: true},  // Sat
		{Date: date(2026, 9, 27, loc), IsOperating: false}, // Sun
		{Date: date(2026, 9, 28, loc), IsOperating: true},  // Mon
	})
	now := time.Date(2026, 9, 26, 16, 30, 0, 0, loc)
	got := cal.Adjust(date(2026, 9, 26, loc), now)
	want := date(2026, 9, 28, loc)
	if !got.Equal(want) {
		t.Fatalf("got %s want %s", got.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestBeforeCutoffKeepsSameOperatingDay(t *testing.T) {
	loc, _ := time.LoadLocation(Zone)
	cal := Load([]Day{
		{Date: date(2026, 9, 28, loc), IsOperating: true},
	})
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, loc)
	got := cal.Adjust(date(2026, 9, 28, loc), now)
	if !got.Equal(date(2026, 9, 28, loc)) {
		t.Fatalf("got %s", got)
	}
}

func TestConfiguredCutoffIsAppliedWithoutChangingOperatingConstraints(t *testing.T) {
	loc, _ := time.LoadLocation(Zone)
	cal := Load([]Day{{Date: date(2026, 9, 28, loc), IsOperating: true}, {Date: date(2026, 9, 29, loc), IsOperating: true}})
	before := time.Date(2026, 9, 28, 15, 29, 0, 0, loc)
	after := time.Date(2026, 9, 28, 15, 31, 0, 0, loc)
	if got := cal.AdjustWithCutoff(date(2026, 9, 28, loc), before, "15:30"); !got.Equal(date(2026, 9, 28, loc)) {
		t.Fatalf("before configured cutoff moved date: %s", got)
	}
	if got := cal.AdjustWithCutoff(date(2026, 9, 28, loc), after, "15:30"); !got.Equal(date(2026, 9, 29, loc)) {
		t.Fatalf("after configured cutoff kept date: %s", got)
	}
}

func date(y, m, d int, loc *time.Location) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, loc)
}
