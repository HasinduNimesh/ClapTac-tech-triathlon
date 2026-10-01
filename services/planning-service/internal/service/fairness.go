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
func applyFairnessHistory(orders []domain.Order, deferralCounts map[string]int, deferralErr error, lastServed map[string]time.Time, lastServedErr error, asOf time.Time, policy domain.PlanningPolicy) (bool, error) {
	if deferralErr != nil {
		return false, fmt.Errorf("load outlet deferral history: %w", deferralErr)
	}
	for i := range orders {
		orders[i].OutletDeferralCount = deferralCounts[orders[i].OutletID]
		if lastServedErr == nil {
			if servedAt, ok := lastServed[orders[i].OutletID]; ok {
				orders[i].LastServedAt = &servedAt
				orders[i].DaysSinceLastServed = allocate.DaysSinceLastServed(asOf, servedAt)
			} else {
				// No successful delivery exists for this outlet yet; treat it as
				// unserved for the configured maximum age.
				orders[i].DaysSinceLastServed = policy.MaxUnservedDays
			}
		}
		orders[i].FairnessScore = allocate.FairnessScoreWithPolicy(orders[i], policy)
	}
	return lastServedErr == nil, nil
}
