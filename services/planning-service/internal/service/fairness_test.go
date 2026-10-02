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

	available, err := applyFairnessHistory(orders, map[string]int{"OUT-1": 2}, nil, map[string]time.Time{"OUT-1": lastServedAt}, nil, asOf, policy, map[string]string{"OUT-1": "2026-09-28"}, nil)
	if err != nil || !available {
		t.Fatalf("history should be available: available=%v err=%v", available, err)
	}
	if orders[0].OutletDeferralCount != 2 || orders[0].DaysSinceLastServed != 2 || orders[0].FairnessScore != 22 || orders[0].LastServedAt == nil {
		t.Fatalf("known service history was not applied: %+v", orders[0])
	}
	if !orders[0].DeferredLastRun || orders[0].LastDeferralDate != "2026-09-28" {
		t.Fatalf("FR-53: repeat-deferral warning was not applied for a known prior deferral: %+v", orders[0])
	}
	if orders[1].DaysSinceLastServed != 45 || orders[1].FairnessScore != 45 || orders[1].LastServedAt != nil {
		t.Fatalf("first service priority should use the capped age without inventing a service date: %+v", orders[1])
	}
	if orders[1].DeferredLastRun {
		t.Fatalf("FR-53: an outlet with no recorded prior deferral must not be flagged: %+v", orders[1])
	}
}

func TestApplyFairnessHistoryClearsRepeatDeferralWarningAfterLaterService(t *testing.T) {
	loc := time.FixedZone("Asia/Colombo", 5*60*60+30*60)
	asOf := time.Date(2026, time.September, 30, 0, 0, 0, 0, loc)
	// Deferred on 2026-09-20, then actually served on 2026-09-25: a plan on
	// 2026-09-30 must not warn that the outlet was deferred "last run" when
	// the last run actually succeeded.
	servedAfterDeferral := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	orders := []domain.Order{{ID: "recovered", OutletID: "OUT-1"}}
	policy := domain.PlanningPolicy{DeferralWeightPoints: 10, MaxDeferralCount: 3, MaxUnservedDays: 45}

	lastAttempted := map[string]time.Time{"OUT-1": servedAfterDeferral}
	_, err := applyFairnessHistory(orders, map[string]int{"OUT-1": 1}, nil, map[string]time.Time{"OUT-1": servedAfterDeferral}, nil, asOf, policy, map[string]string{"OUT-1": "2026-09-20"}, lastAttempted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orders[0].DeferredLastRun {
		t.Fatalf("FR-53 review fix: a deferral superseded by a later successful delivery must not warn: %+v", orders[0])
	}
}

func TestApplyFairnessHistoryClearsRepeatDeferralWarningAfterLaterFailedAttempt(t *testing.T) {
	loc := time.FixedZone("Asia/Colombo", 5*60*60+30*60)
	asOf := time.Date(2026, time.September, 30, 0, 0, 0, 0, loc)
	// Review follow-up on #4/#11: deferred on 2026-09-20, then allocated and
	// attempted on 2026-09-25 but NOT_DELIVERED/REFUSED (so it never appears
	// in OutletLastServed, which only covers DELIVERED/PARTIAL). The outlet
	// was still actually run on that later date - just unsuccessfully - so
	// the "deferred last run" warning must still clear; a stale deferral
	// from five days earlier is not the right explanation for a later,
	// distinct delivery failure.
	attemptedAfterDeferral := time.Date(2026, time.September, 25, 10, 0, 0, 0, time.UTC)
	orders := []domain.Order{{ID: "attempted-not-delivered", OutletID: "OUT-1"}}
	policy := domain.PlanningPolicy{DeferralWeightPoints: 10, MaxDeferralCount: 3, MaxUnservedDays: 45}

	// lastServed is empty: the later run failed, so it never reached
	// OutletLastServed. Only lastAttempted sees it.
	_, err := applyFairnessHistory(orders, map[string]int{"OUT-1": 1}, nil, nil, nil, asOf, policy,
		map[string]string{"OUT-1": "2026-09-20"}, map[string]time.Time{"OUT-1": attemptedAfterDeferral})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orders[0].DeferredLastRun {
		t.Fatalf("FR-53 review follow-up: a deferral superseded by a later attempt (even a failed one) must not warn: %+v", orders[0])
	}
}

func TestApplyFairnessHistoryFallsBackExplicitlyWhenDeliveryHistoryFails(t *testing.T) {
	orders := []domain.Order{{ID: "deferred", OutletID: "OUT-1"}}
	policy := domain.PlanningPolicy{DeferralWeightPoints: 10, MaxDeferralCount: 3, MaxUnservedDays: 45}
	available, err := applyFairnessHistory(orders, map[string]int{"OUT-1": 2}, nil, nil, errors.New("delivery history offline"), time.Time{}, policy, nil, nil)
	if err != nil || available {
		t.Fatalf("delivery history failure should use explicit deferral-only fallback: available=%v err=%v", available, err)
	}
	if orders[0].FairnessScore != 20 || orders[0].DaysSinceLastServed != 0 {
		t.Fatalf("fallback should not fabricate service age: %+v", orders[0])
	}
}

func TestApplyFairnessHistoryRejectsMissingDeferralSignal(t *testing.T) {
	_, err := applyFairnessHistory([]domain.Order{{OutletID: "OUT-1"}}, nil, errors.New("deferral table unavailable"), nil, nil, time.Time{}, domain.PlanningPolicy{}, nil, nil)
	if err == nil || err.Error() != "load outlet deferral history: deferral table unavailable" {
		t.Fatalf("unexpected deferral history error: %v", err)
	}
}
