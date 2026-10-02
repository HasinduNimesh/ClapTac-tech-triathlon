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
func applyFairnessHistory(orders []domain.Order, deferralCounts map[string]int, deferralErr error, lastServed map[string]time.Time, lastServedErr error, asOf time.Time, policy domain.PlanningPolicy, lastDeferralByOutlet map[string]string) (bool, error) {
	if deferralErr != nil {
		return false, fmt.Errorf("load outlet deferral history: %w", deferralErr)
	}
	for i := range orders {
		orders[i].OutletDeferralCount = deferralCounts[orders[i].OutletID]
		var servedAt time.Time
		var served bool
		if lastServedErr == nil {
			if at, ok := lastServed[orders[i].OutletID]; ok {
				servedAt, served = at, true
				orders[i].LastServedAt = &at
				orders[i].DaysSinceLastServed = allocate.DaysSinceLastServed(asOf, at)
			} else {
				// No successful delivery exists for this outlet yet; treat it as
				// unserved for the configured maximum age.
				orders[i].DaysSinceLastServed = policy.MaxUnservedDays
			}
		}
		if date, ok := lastDeferralByOutlet[orders[i].OutletID]; ok {
			// A later successful delivery means this outlet was actually
			// served since that deferral, so it was not deferred "last run" -
			// without this check, a single historical deferral would warn on
			// every later plan forever, even after the outlet was served.
			supersededByService := false
			if served {
				if deferralDate, err := time.ParseInLocation("2006-01-02", date, asOf.Location()); err == nil {
					servedLocal := servedAt.In(asOf.Location())
					servedDay := time.Date(servedLocal.Year(), servedLocal.Month(), servedLocal.Day(), 0, 0, 0, 0, asOf.Location())
					if servedDay.After(deferralDate) {
						supersededByService = true
					}
				}
			}
			if !supersededByService {
				orders[i].DeferredLastRun = true
				orders[i].LastDeferralDate = date
			}
		}
		orders[i].FairnessScore = allocate.FairnessScoreWithPolicy(orders[i], policy)
	}
	return lastServedErr == nil, nil
}
