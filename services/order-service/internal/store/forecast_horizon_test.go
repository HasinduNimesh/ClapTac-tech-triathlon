package store

import (
	"testing"
	"time"
)

func TestForecastHorizonIsTenWeeks(t *testing.T) {
	if ForecastHorizonWeeks != 10 || ForecastHorizonDays != 70 {
		t.Fatalf("horizon must be ten weeks (70 days), got %d weeks / %d days", ForecastHorizonWeeks, ForecastHorizonDays)
	}
}

func TestProjectWeeklyReturnsTenOrderedWeeks(t *testing.T) {
	weekStart := time.Date(2026, time.September, 28, 0, 0, 0, 0, time.UTC)
	groups := []forecastGroup{
		{depot: "DEPOT_NORTH", brand: "Fresh", chilled: 10, ambient: 6, weight: 80, volume: 8},
		{depot: "DEPOT_SOUTH", brand: "Tech", chilled: 0, ambient: 3, weight: 10, volume: 1},
	}
	got := projectWeekly(weekStart, groups)
	if len(got) != ForecastHorizonWeeks*len(groups) {
		t.Fatalf("want %d buckets, got %d", ForecastHorizonWeeks*len(groups), len(got))
	}
	for w := 0; w < ForecastHorizonWeeks; w++ {
		wantDate := weekStart.AddDate(0, 0, 7*w).Format("2006-01-02")
		for gi, g := range groups {
			b := got[w*len(groups)+gi]
			if b.WeekStarting != wantDate || b.HorizonWeek != w+1 || b.Depot != g.depot || b.Brand != g.brand {
				t.Fatalf("bucket %d/%d out of order: %+v (want week %s)", w, gi, b, wantDate)
			}
			if !b.Estimate {
				t.Fatalf("bucket %+v must be flagged as an estimate", b)
			}
		}
	}
	if last := got[len(got)-1].WeekStarting; last != "2026-11-30" {
		t.Fatalf("tenth week should start 63 days after the first (2026-11-30), got %s", last)
	}
	first := got[0]
	if first.ChilledOrders != 3 || first.AmbientOrders != 2 || first.EstimatedWeightKg != 20 || first.EstimatedVolumeM3 != 2 {
		t.Fatalf("baseline should be the four-week mean, got %+v", first)
	}
	// The baseline is stationary: weeks beyond the first carry the same estimate.
	tenth := got[(ForecastHorizonWeeks-1)*len(groups)]
	if tenth.EstimatedWeightKg != first.EstimatedWeightKg || tenth.ChilledOrders != first.ChilledOrders {
		t.Fatalf("flat baseline expected across the horizon: %+v vs %+v", first, tenth)
	}
}

func TestProjectWeeklyRangeWidensWithDistance(t *testing.T) {
	got := projectWeekly(time.Date(2026, time.September, 28, 0, 0, 0, 0, time.UTC), []forecastGroup{{depot: "D", brand: "Fresh", ambient: 4, weight: 4, volume: 4}})
	if got[0].RangePercent != 10 {
		t.Fatalf("first week range should be the 10%% base, got %v", got[0].RangePercent)
	}
	for i := 1; i < len(got); i++ {
		if got[i].RangePercent <= got[i-1].RangePercent {
			t.Fatalf("range must widen week on week: week %d %v <= week %d %v", i+1, got[i].RangePercent, i, got[i-1].RangePercent)
		}
	}
	if got[9].RangePercent != 28 {
		t.Fatalf("tenth week range should be 28%%, got %v", got[9].RangePercent)
	}
}

func TestProjectWeeklyWithoutHistoryProducesNoBuckets(t *testing.T) {
	if got := projectWeekly(time.Now(), nil); len(got) != 0 {
		t.Fatalf("no history means no estimate, got %d buckets", len(got))
	}
}

func TestMemoryForecastReportsTheHorizon(t *testing.T) {
	got, err := (&Memory{}).Forecast(time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.HorizonWeeks != 10 || got.ForecastVersion != "confirmed_order_mean_v2" {
		t.Fatalf("unexpected horizon/version: %d %s", got.HorizonWeeks, got.ForecastVersion)
	}
}
