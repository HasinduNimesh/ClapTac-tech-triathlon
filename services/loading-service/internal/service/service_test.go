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
