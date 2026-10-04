package service

import (
	"fmt"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/store"
)

// planDiff is how a plan version differs from the lines a loading session is
// working from. What the loader is shown ("what changed") and what syncing then
// applies to the session both come from this one comparison, so the banner can
// never disagree with the change: if the criteria for a removed or moved line
// change, they change here for both.
type planDiff struct {
	// Added are allocations on the plan that the session has no line for yet.
	Added []domain.PlanningAlloc
	// Removed are session lines whose order is no longer on the plan.
	Removed []domain.OrderLoad
	// Moved are lines whose stop changed; a loaded one must be rechecked.
	Moved []changedLine
	// Resequenced are lines that keep their stop but sit elsewhere in the load order.
	Resequenced []changedLine
}

type changedLine struct {
	Load  domain.OrderLoad
	Alloc domain.PlanningAlloc
	// SuggestedLoadSequence is the line's place in the new load order.
	SuggestedLoadSequence int
}

// diffPlan compares the session's lines with a plan's allocations. suggested maps
// order IDs to their new load sequence; pass nil when only the stop changes matter.
func diffPlan(loads []domain.OrderLoad, allocs []domain.PlanningAlloc, suggested map[string]int) planDiff {
	byOrder := make(map[string]domain.PlanningAlloc, len(allocs))
	for _, a := range allocs {
		byOrder[a.OrderID] = a
	}
	have := make(map[string]bool, len(loads))
	var d planDiff
	for _, l := range loads {
		have[l.OrderID] = true
		a, onPlan := byOrder[l.OrderID]
		switch {
		case !onPlan:
			d.Removed = append(d.Removed, l)
		case a.StopSequence != l.StopSequence:
			d.Moved = append(d.Moved, changedLine{Load: l, Alloc: a, SuggestedLoadSequence: suggested[l.OrderID]})
		case suggested != nil && suggested[l.OrderID] != l.SuggestedLoadSequence:
			d.Resequenced = append(d.Resequenced, changedLine{Load: l, Alloc: a, SuggestedLoadSequence: suggested[l.OrderID]})
		}
	}
	for _, a := range allocs {
		if !have[a.OrderID] {
			d.Added = append(d.Added, a)
		}
	}
	return d
}

// describe is the "what changed" list the loader app shows.
func (d planDiff) describe() []map[string]any {
	out := []map[string]any{}
	for _, l := range d.Removed {
		out = append(out, map[string]any{"kind": "REMOVED", "orderId": l.OrderID, "orderRef": l.OrderRef, "outletId": l.OutletID, "fromStop": l.StopSequence, "loaded": l.Status == domain.LoadLoaded})
	}
	for _, m := range d.Moved {
		out = append(out, map[string]any{"kind": "MOVED", "orderId": m.Load.OrderID, "orderRef": m.Load.OrderRef, "outletId": m.Load.OutletID, "fromStop": m.Load.StopSequence, "toStop": m.Alloc.StopSequence, "loaded": m.Load.Status == domain.LoadLoaded})
	}
	for _, a := range d.Added {
		out = append(out, map[string]any{"kind": "ADDED", "orderId": a.OrderID, "orderRef": a.OrderRef, "outletId": a.OutletID, "toStop": a.StopSequence})
	}
	return out
}

// removals are the session lines to delete when syncing.
func (d planDiff) removals() []string {
	ids := make([]string, 0, len(d.Removed))
	for _, l := range d.Removed {
		ids = append(ids, l.ID)
	}
	return ids
}

// loadChanges are the line updates to apply when syncing. A moved line that was
// already loaded goes back to pending so the loader rechecks where it sits.
func (d planDiff) loadChanges() []store.LoadChange {
	out := make([]store.LoadChange, 0, len(d.Moved)+len(d.Resequenced))
	for _, m := range d.Moved {
		out = append(out, store.LoadChange{
			LoadID: m.Load.ID, StopSequence: m.Alloc.StopSequence, SuggestedLoadSequence: m.SuggestedLoadSequence,
			Note: fmt.Sprintf("Moved from Stop %d", m.Load.StopSequence), ResetToPending: m.Load.Status == domain.LoadLoaded,
		})
	}
	for _, r := range d.Resequenced {
		out = append(out, store.LoadChange{
			LoadID: r.Load.ID, StopSequence: r.Alloc.StopSequence, SuggestedLoadSequence: r.SuggestedLoadSequence, Note: r.Load.ChangeNote,
		})
	}
	return out
}
