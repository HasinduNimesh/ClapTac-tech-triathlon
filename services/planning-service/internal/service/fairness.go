package service

import (
	"fmt"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/allocate"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
)

// applyFairnessHistory applies the two inputs used by the explainable priority
// policy. Missing delivery history degrades to deferral-only scoring, but a
// failed deferral query is fatal because otherwise we would hide a required
// fairness signal while claiming the score is complete.
func applyFairnessHistory(orders []domain.Order, deferralCounts map[string]int, deferralErr error, lastServed map[string]time.Time, lastServedErr error, asOf time.Time, policy domain.PlanningPolicy, lastDeferralByOutlet, earlierDeferralByOutlet map[string]string, lastAttempted map[string]time.Time) (bool, error) {
	if deferralErr != nil {
		return false, fmt.Errorf("load outlet deferral history: %w", deferralErr)
	}
	for i := range orders {
		orders[i].OutletDeferralCount = deferralCounts[orders[i].OutletID]
		if lastServedErr == nil {
			if at, ok := lastServed[orders[i].OutletID]; ok {
				orders[i].LastServedAt = &at
				orders[i].DaysSinceLastServed = allocate.DaysSinceLastServed(asOf, at)
			} else {
				// No successful delivery exists for this outlet yet; treat it as
				// unserved for the configured maximum age.
				orders[i].DaysSinceLastServed = policy.MaxUnservedDays
			}
		}
		if date, ok := lastDeferralByOutlet[orders[i].OutletID]; ok {
			// A later delivery attempt - successful or not - means this
			// outlet was actually run since that deferral, so it was not
			// deferred "last run". This must check every terminal outcome,
			// not just successful ones: an outlet deferred, then allocated
			// and attempted but NOT_DELIVERED/REFUSED, was still run on that
			// later date, just unsuccessfully - a different problem from
			// being skipped entirely, which is what this warning is about.
			supersededByAttempt := attemptedAfter(lastAttempted, orders[i].OutletID, date, asOf.Location())
			if !supersededByAttempt {
				orders[i].DeferredLastRun = true
				orders[i].LastDeferralDate = date
				// Priority is carried across plans from persisted history: if
				// the deferral just before this plan was itself a repeat (the
				// outlet was deferred on an earlier run too, with no delivery
				// attempt in between) the outlet is flagged priority here,
				// whether or not this plan defers it again.
				if earlier, ok := earlierDeferralByOutlet[orders[i].OutletID]; ok && !attemptedAfter(lastAttempted, orders[i].OutletID, earlier, asOf.Location()) {
					orders[i].PriorityNextPlan = true
				}
			}
		}
		orders[i].FairnessScore = allocate.FairnessScoreWithPolicy(orders[i], policy)
	}
	return lastServedErr == nil, nil
}

// attemptedAfter reports whether the outlet had a delivery attempt on a day
// strictly after the given deferral date.
func attemptedAfter(lastAttempted map[string]time.Time, outletID, deferralDate string, loc *time.Location) bool {
	attemptedAt, ok := lastAttempted[outletID]
	if !ok {
		return false
	}
	deferralDay, err := time.ParseInLocation("2006-01-02", deferralDate, loc)
	if err != nil {
		return false
	}
	attemptedLocal := attemptedAt.In(loc)
	attemptedDay := time.Date(attemptedLocal.Year(), attemptedLocal.Month(), attemptedLocal.Day(), 0, 0, 0, 0, loc)
	return attemptedDay.After(deferralDay)
}

// markPriorityNextPlan completes the priorityNextPlan flag.
//
// Definition: an order's outlet is priorityNextPlan on plan P when it has been
// deferred on two consecutive runs - the same "consecutive" notion as
// deferredLastRun (no delivery attempt between the deferrals) - and
//  1. the second deferral is on an earlier plan: derived from persisted
//     deferral history by applyFairnessHistory, so the flag is still present on
//     the next plan (and any later plan until the outlet is attempted); or
//  2. P itself records the second deferral: the outlet was deferred last run
//     (DeferredLastRun) and has a deferral on P.
//
// It is therefore derived from history across plans, not from the plan being
// viewed, and it is never cleared by viewing a different plan.
func markPriorityNextPlan(orders []domain.Order, deferredOrderIDs map[string]bool) {
	for i := range orders {
		orders[i].PriorityNextPlan = orders[i].PriorityNextPlan || (deferredOrderIDs[orders[i].ID] && orders[i].DeferredLastRun)
	}
}
