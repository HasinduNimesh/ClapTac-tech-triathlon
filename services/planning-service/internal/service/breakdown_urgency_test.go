package service

import (
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
)

func TestRankStrandedStopsChilledThenWindowThenArrival(t *testing.T) {
	t0 := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	later := t0.Add(time.Hour)
	orders := map[string]domain.Order{
		"a": {ID: "a", OutletID: "O1", Temp: "ambient"},
		"b": {ID: "b", OutletID: "O2", Temp: "ambient"},
		"c": {ID: "c", OutletID: "O3", Temp: "chilled"},
		"d": {ID: "d", OutletID: "O4", Temp: "ambient"},
	}
	outlets := map[string]domain.Outlet{
		"O1": {WindowClose: "17:00"}, "O2": {WindowClose: "12:00"}, "O3": {WindowClose: "18:00"}, "O4": {WindowClose: "12:00"},
	}
	moving := []domain.Allocation{
		{ID: "1", OrderID: "a", Sequence: 1, PlannedArrivalAt: &t0},
		{ID: "2", OrderID: "b", Sequence: 2, PlannedArrivalAt: &later},
		{ID: "3", OrderID: "c", Sequence: 3, PlannedArrivalAt: &later},
		{ID: "4", OrderID: "d", Sequence: 4, PlannedArrivalAt: &t0},
	}
	got := rankStrandedStops(moving, orders, outlets)
	want := []string{"c", "d", "b", "a"}
	for i, w := range want {
		if got[i].Order.ID != w {
			t.Fatalf("rank %d = %s want %s (%v)", i+1, got[i].Order.ID, w, got)
		}
	}
	if !got[0].Chilled || got[1].Chilled {
		t.Fatal("chilled flag wrong")
	}
}
