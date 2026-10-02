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
func applyFairnessHistory(orders []domain.Order, deferralCounts map[string]int, deferralErr error, lastServed map[string]time.Time, lastServedErr error, asOf time.Time, policy domain.PlanningPolicy, lastDeferralByOutlet map[string]string, lastAttempted map[string]time.Time) (bool, error) {
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
			supersededByAttempt := false
			if attemptedAt, ok := lastAttempted[orders[i].OutletID]; ok {
				if deferralDate, err := time.ParseInLocation("2006-01-02", date, asOf.Location()); err == nil {
					attemptedLocal := attemptedAt.In(asOf.Location())
					attemptedDay := time.Date(attemptedLocal.Year(), attemptedLocal.Month(), attemptedLocal.Day(), 0, 0, 0, 0, asOf.Location())
					if attemptedDay.After(deferralDate) {
						supersededByAttempt = true
					}
				}
			}
			if !supersededByAttempt {
				orders[i].DeferredLastRun = true
				orders[i].LastDeferralDate = date
			}
		}
		orders[i].FairnessScore = allocate.FairnessScoreWithPolicy(orders[i], policy)
	}
	return lastServedErr == nil, nil
}
