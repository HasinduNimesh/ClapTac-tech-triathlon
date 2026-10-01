package service

import (
	"errors"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
)

func TestApplyFairnessHistoryCombinesInputsAndClampsUnknownServiceAge(t *testing.T) {
	loc := time.FixedZone("Asia/Colombo", 5*60*60+30*60)
	asOf := time.Date(2026, time.September, 30, 0, 0, 0, 0, loc)
	lastServedAt := time.Date(2026, time.September, 28, 16, 0, 0, 0, time.UTC)
	orders := []domain.Order{{ID: "recent", OutletID: "OUT-1"}, {ID: "new", OutletID: "OUT-2"}}
	policy := domain.PlanningPolicy{DeferralWeightPoints: 10, MaxDeferralCount: 3, MaxUnservedDays: 45}

	available, err := applyFairnessHistory(orders, map[string]int{"OUT-1": 2}, nil, map[string]time.Time{"OUT-1": lastServedAt}, nil, asOf, policy)
	if err != nil || !available {
		t.Fatalf("history should be available: available=%v err=%v", available, err)
	}
	if orders[0].OutletDeferralCount != 2 || orders[0].DaysSinceLastServed != 2 || orders[0].FairnessScore != 22 || orders[0].LastServedAt == nil {
		t.Fatalf("known service history was not applied: %+v", orders[0])
	}
	if orders[1].DaysSinceLastServed != 45 || orders[1].FairnessScore != 45 || orders[1].LastServedAt != nil {
		t.Fatalf("first service priority should use the capped age without inventing a service date: %+v", orders[1])
	}
}

func TestApplyFairnessHistoryFallsBackExplicitlyWhenDeliveryHistoryFails(t *testing.T) {
	orders := []domain.Order{{ID: "deferred", OutletID: "OUT-1"}}
	policy := domain.PlanningPolicy{DeferralWeightPoints: 10, MaxDeferralCount: 3, MaxUnservedDays: 45}
	available, err := applyFairnessHistory(orders, map[string]int{"OUT-1": 2}, nil, nil, errors.New("delivery history offline"), time.Time{}, policy)
	if err != nil || available {
		t.Fatalf("delivery history failure should use explicit deferral-only fallback: available=%v err=%v", available, err)
	}
	if orders[0].FairnessScore != 20 || orders[0].DaysSinceLastServed != 0 {
		t.Fatalf("fallback should not fabricate service age: %+v", orders[0])
	}
}

func TestApplyFairnessHistoryRejectsMissingDeferralSignal(t *testing.T) {
	_, err := applyFairnessHistory([]domain.Order{{OutletID: "OUT-1"}}, nil, errors.New("deferral table unavailable"), nil, nil, time.Time{}, domain.PlanningPolicy{})
	if err == nil || err.Error() != "load outlet deferral history: deferral table unavailable" {
		t.Fatalf("unexpected deferral history error: %v", err)
	}
}
