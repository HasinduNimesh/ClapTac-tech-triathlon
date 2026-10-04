package service

import (
	"sort"
	"strings"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
)

// rankedStop is one stranded stop in urgency order.
type rankedStop struct {
	Alloc       domain.Allocation
	Order       domain.Order
	Chilled     bool
	WindowClose string
}

// rankStrandedStops orders stops left behind by a breakdown: chilled goods
// first, then the earliest delivery-window close, then the soonest planned
// arrival, then the original stop sequence so the result is deterministic.
func rankStrandedStops(moving []domain.Allocation, orders map[string]domain.Order, outlets map[string]domain.Outlet) []rankedStop {
	out := make([]rankedStop, 0, len(moving))
	for _, a := range moving {
		o := orders[a.OrderID]
		closeAt := outlets[o.OutletID].WindowClose
		if closeAt == "" {
			closeAt = o.Outlet.WindowClose
		}
		out = append(out, rankedStop{Alloc: a, Order: o, Chilled: strings.EqualFold(o.Temp, "chilled"), WindowClose: closeAt})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Chilled != b.Chilled {
			return a.Chilled
		}
		if a.WindowClose != b.WindowClose {
			if a.WindowClose == "" {
				return false
			}
			if b.WindowClose == "" {
				return true
			}
			return a.WindowClose < b.WindowClose
		}
		ai, bi := a.Alloc.PlannedArrivalAt, b.Alloc.PlannedArrivalAt
		if ai != nil && bi != nil && !ai.Equal(*bi) {
			return ai.Before(*bi)
		}
		return a.Alloc.Sequence < b.Alloc.Sequence
	})
	return out
}
