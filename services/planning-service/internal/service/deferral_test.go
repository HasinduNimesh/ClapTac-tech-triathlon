package service

import (
	"strings"
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
)

func TestValidateDeferralRequiresReasonDeciderAndNextRun(t *testing.T) {
	ok := domain.ReasonManualDeferral
	if got, err := validateDeferral("u-1", ok, " 2026-10-07 ", "2026-10-06"); err != nil || got != "2026-10-07" {
		t.Fatalf("valid deferral rejected: %q %v", got, err)
	}
	for name, c := range map[string][4]string{
		"missing reason":          {"u-1", "", "2026-10-07", "2026-10-06"},
		"unknown reason":          {"u-1", "BECAUSE", "2026-10-07", "2026-10-06"},
		"missing decider":         {" ", ok, "2026-10-07", "2026-10-06"},
		"missing next run":        {"u-1", ok, "", "2026-10-06"},
		"bad next run":            {"u-1", ok, "tomorrow", "2026-10-06"},
		"next run not after plan": {"u-1", ok, "2026-10-06", "2026-10-06"},
	} {
		if _, err := validateDeferral(c[0], c[1], c[2], c[3]); err == nil || !strings.HasPrefix(err.Error(), "invalid:") {
			t.Errorf("%s: expected invalid error, got %v", name, err)
		}
	}
}

func TestMarkPriorityNextPlanOnSecondConsecutiveDeferral(t *testing.T) {
	orders := []domain.Order{
		{ID: "a", DeferredLastRun: true},  // deferred now and last run -> priority
		{ID: "b", DeferredLastRun: false}, // first deferral
		{ID: "c", DeferredLastRun: true},  // not deferred on this plan
	}
	markPriorityNextPlan(orders, map[string]bool{"a": true, "b": true})
	if !orders[0].PriorityNextPlan || orders[1].PriorityNextPlan || orders[2].PriorityNextPlan {
		t.Fatalf("priority flags: %+v", orders)
	}
}
