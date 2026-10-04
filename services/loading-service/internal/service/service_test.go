package service

import (
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/sequence"
)

func TestPendingDetailIncludesPublishedVersionAndLoaderAcknowledgement(t *testing.T) {
	profile := &authorization.Profile{UserID: "USR004", Roles: []string{authorization.RoleLoader}}
	trip := domain.PlanningTrip{
		TripID: "trip-1", PlanID: "plan-1", PlanRef: "PLAN000001", PlanVersion: 3,
		PlanAcknowledgements: []domain.PlanAcknowledgement{{ActorID: "USR004", ActorRole: authorization.RoleLoader}},
		Allocations:          []domain.PlanningAlloc{{OrderID: "order-1", StopSequence: 1}},
	}

	detail := pendingDetail(trip, sequence.ReverseLastOut{}, profile)
	if got := detail["planVersion"]; got != 3 {
		t.Fatalf("planVersion = %v, want 3", got)
	}
	if got := detail["acknowledgedVersion"]; got != 3 {
		t.Fatalf("acknowledgedVersion = %v, want 3", got)
	}
	if got := detail["pendingCount"]; got != 1 {
		t.Fatalf("pendingCount = %v, want 1", got)
	}
}

func TestPlannedArrivalByAllocationPreservesOnlyPublishedETAs(t *testing.T) {
	planned := time.Date(2026, 10, 1, 8, 30, 0, 0, time.FixedZone("Sri Lanka", 5*60*60+30*60))
	got := plannedArrivalByAllocation([]domain.PlanningAlloc{
		{AllocationID: "alloc-1", PlannedArrivalAt: &planned},
		{AllocationID: "alloc-2"},
		{PlannedArrivalAt: &planned},
	})
	if len(got) != 1 || got["alloc-1"] == nil || !got["alloc-1"].Equal(planned) {
		t.Fatalf("published ETA snapshot mapping = %#v, want only alloc-1 at %s", got, planned)
	}
}

func TestPendingDetailRequiresThisLoaderToAcknowledge(t *testing.T) {
	profile := &authorization.Profile{UserID: "USR005", Roles: []string{authorization.RoleLoader}}
	trip := domain.PlanningTrip{
		TripID: "trip-1", PlanVersion: 3,
		PlanAcknowledgements: []domain.PlanAcknowledgement{{ActorID: "USR004", ActorRole: authorization.RoleLoader}},
	}

	detail := pendingDetail(trip, sequence.ReverseLastOut{}, profile)
	if got := detail["planVersion"]; got != 3 {
		t.Fatalf("planVersion = %v, want 3", got)
	}
	if got := detail["acknowledgedVersion"]; got != 0 {
		t.Fatalf("acknowledgedVersion = %v, want 0 for another loader", got)
	}
}

func TestMoveToNextRunDoesNotReleaseTheTripUntilThePlanDropsTheOrder(t *testing.T) {
	loads := []domain.OrderLoad{{ID: "l1", OrderID: "o1", Status: domain.LoadShortfall}}
	moved := map[string][]domain.Issue{"l1": {{Decision: domain.DecisionMoveToNextRun}}}

	// Planning has not published a plan without the order yet (the calls after
	// the decision failed, or are still running): the line is still on the trip.
	pending, undecided, unpublished := departureBlockers(loads, moved, map[string]bool{"o1": true})
	if len(pending) != 0 || len(undecided) != 0 || len(unpublished) != 1 {
		t.Fatalf("move with the order still planned: pending=%v undecided=%v unpublished=%v", pending, undecided, unpublished)
	}

	// The confirmed plan no longer carries the order: the trip may leave.
	pending, undecided, unpublished = departureBlockers(loads, moved, map[string]bool{})
	if len(pending)+len(undecided)+len(unpublished) != 0 {
		t.Fatalf("move after the plan dropped the order: %v %v %v", pending, undecided, unpublished)
	}
}

func TestPartialLoadReleasesTheTripAndHoldOrNoDecisionDoNot(t *testing.T) {
	loads := []domain.OrderLoad{{ID: "l1", OrderID: "o1", Status: domain.LoadShortfall}}
	for decision, wantBlocked := range map[string]bool{domain.DecisionPartialLoad: false, domain.DecisionHold: true, "": true} {
		_, undecided, _ := departureBlockers(loads, map[string][]domain.Issue{"l1": {{Decision: decision}}}, map[string]bool{"o1": true})
		if (len(undecided) > 0) != wantBlocked {
			t.Fatalf("decision %q: blocked=%v want %v", decision, len(undecided) > 0, wantBlocked)
		}
	}
}
