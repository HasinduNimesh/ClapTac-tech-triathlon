package service

import (
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
)

func TestBreakdownTextsOnlyStoresDelayedThirtyMinutesOrMore(t *testing.T) {
	at := func(hhmm string) *time.Time {
		v, _ := time.Parse("15:04", hhmm)
		return &v
	}
	before := []domain.Allocation{
		{OrderID: "late", PlannedArrivalAt: at("08:00")},
		{OrderID: "edge", PlannedArrivalAt: at("09:00")},
		{OrderID: "small", PlannedArrivalAt: at("10:00")},
		{OrderID: "earlier", PlannedArrivalAt: at("11:00")},
		{OrderID: "untimed"},
	}
	moved := []domain.Allocation{
		{OrderID: "late", PlannedArrivalAt: at("09:15")},
		{OrderID: "edge", PlannedArrivalAt: at("09:30")},
		{OrderID: "small", PlannedArrivalAt: at("10:29")},
		{OrderID: "earlier", PlannedArrivalAt: at("10:00")},
		{OrderID: "untimed", PlannedArrivalAt: at("12:00")},
	}
	got := breakdownDelays(before, moved)
	if len(got) != 2 || got[0] != (breakdownDelay{OrderID: "late", Minutes: 75}) || got[1] != (breakdownDelay{OrderID: "edge", Minutes: 30}) {
		t.Fatalf("critical texts after a breakdown: %+v", got)
	}
}
