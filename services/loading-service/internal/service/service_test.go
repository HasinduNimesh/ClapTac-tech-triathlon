package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/client"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/sequence"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/store"
)

func TestPendingDetailIncludesPublishedVersionAndLoaderAcknowledgement(t *testing.T) {
	profile := &authorization.Profile{UserID: "USR004", Roles: []string{authorization.RoleLoader}}
	trip := domain.PlanningTrip{
		TripID: "trip-1", PlanID: "plan-1", PlanRef: "PLAN000001", PlanVersion: 3,
		PlanAcknowledgements: []domain.PlanAcknowledgement{{ActorID: "USR004", ActorRole: authorization.RoleLoader}},
		Allocations:          []domain.PlanningAlloc{{OrderID: "order-1", StopSequence: 1}},
	}

	detail := Service{Sequence: sequence.ReverseLastOut{}}.pendingDetail(context.Background(), trip, profile)
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

func TestPlannedDepartureByAllocationPreservesOnlyPublishedTimes(t *testing.T) {
	planned := time.Date(2026, 10, 1, 8, 50, 0, 0, time.FixedZone("Sri Lanka", 5*60*60+30*60))
	got := plannedDepartureByAllocation([]domain.PlanningAlloc{
		{AllocationID: "alloc-1", PlannedDepartureAt: &planned},
		{AllocationID: "alloc-2"},
		{PlannedDepartureAt: &planned},
	})
	if len(got) != 1 || got["alloc-1"] == nil || !got["alloc-1"].Equal(planned) {
		t.Fatalf("planned departure mapping = %#v, want only alloc-1 at %s", got, planned)
	}
}

func TestPendingDetailRequiresThisLoaderToAcknowledge(t *testing.T) {
	profile := &authorization.Profile{UserID: "USR005", Roles: []string{authorization.RoleLoader}}
	trip := domain.PlanningTrip{
		TripID: "trip-1", PlanVersion: 3,
		PlanAcknowledgements: []domain.PlanAcknowledgement{{ActorID: "USR004", ActorRole: authorization.RoleLoader}},
	}

	detail := Service{Sequence: sequence.ReverseLastOut{}}.pendingDetail(context.Background(), trip, profile)
	if got := detail["planVersion"]; got != 3 {
		t.Fatalf("planVersion = %v, want 3", got)
	}
	if got := detail["acknowledgedVersion"]; got != 0 {
		t.Fatalf("acknowledgedVersion = %v, want 0 for another loader", got)
	}
}

func TestPlanChangesListsAddedRemovedAndMovedOrders(t *testing.T) {
	loads := []domain.OrderLoad{
		{OrderID: "o1", OrderRef: "ORD1", StopSequence: 1, Status: domain.LoadLoaded},
		{OrderID: "o2", OrderRef: "ORD2", StopSequence: 4, Status: domain.LoadLoaded},
		{OrderID: "o3", OrderRef: "ORD3", StopSequence: 3},
	}
	trip := domain.PlanningTrip{PlanVersion: 3, Allocations: []domain.PlanningAlloc{
		{OrderID: "o1", StopSequence: 1}, {OrderID: "o2", StopSequence: 2}, {OrderID: "o4", OrderRef: "ORD4", StopSequence: 3},
	}}
	got := map[string]map[string]any{}
	for _, c := range planChanges(loads, trip) {
		got[c["orderId"].(string)] = c
	}
	if len(got) != 3 {
		t.Fatalf("changes = %v, want 3", got)
	}
	if got["o2"]["kind"] != "MOVED" || got["o2"]["fromStop"] != 4 || got["o2"]["toStop"] != 2 || got["o2"]["loaded"] != true {
		t.Fatalf("moved = %v", got["o2"])
	}
	if got["o3"]["kind"] != "REMOVED" || got["o4"]["kind"] != "ADDED" {
		t.Fatalf("removed/added = %v / %v", got["o3"], got["o4"])
	}
}

func TestPlanningInfoTotalsAndFreshTripTime(t *testing.T) {
	leave := time.Date(2026, 10, 1, 3, 30, 0, 0, time.UTC)
	lastOut := leave.Add(214 * time.Minute)
	trip := domain.PlanningTrip{
		VehicleWeightCapacityKg: 4000, VehicleVolumeCapacityM3: 28, PlannedDepartureAt: &leave,
		Allocations: []domain.PlanningAlloc{
			{Brand: "Fresh", WeightKg: 1000, VolumeM3: 10, Temperature: "chilled", District: "Colombo"},
			{Brand: "Fresh", WeightKg: 840.04, VolumeM3: 4.9, Temperature: "ambient", District: "Colombo", PlannedDepartureAt: &lastOut},
		},
	}
	info := planningInfo(trip)
	if info["totalWeightKg"] != 1840.0 || info["totalVolumeM3"] != 14.9 || info["chilledVolumeM3"] != 10.0 {
		t.Fatalf("totals = %v %v %v", info["totalWeightKg"], info["totalVolumeM3"], info["chilledVolumeM3"])
	}
	if info["tripMinutes"] != 214 || info["freshBudgetMinutes"] != domain.FreshTripBudgetMinutes {
		t.Fatalf("trip time = %v / %v", info["tripMinutes"], info["freshBudgetMinutes"])
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

func ptr(v float64) *float64 { return &v }

func TestRefrigeratedVehicleNeedsTheReadingAndTheSealToDepart(t *testing.T) {
	cases := []struct {
		name   string
		checks ReadyChecks
		want   string // error prefix, "" for accepted
	}{
		{"nothing at all", ReadyChecks{}, "invalid"},
		{"reading only", ReadyChecks{ChilledTemperatureC: ptr(3)}, "invalid"},
		{"seal only", ReadyChecks{SealNumber: "WP-1"}, "invalid"},
		{"blank seal", ReadyChecks{ChilledTemperatureC: ptr(3), SealNumber: "  "}, "invalid"},
		{"too warm", ReadyChecks{ChilledTemperatureC: ptr(4.5), SealNumber: "WP-1"}, "conflict"},
		{"too cold", ReadyChecks{ChilledTemperatureC: ptr(1.5), SealNumber: "WP-1"}, "conflict"},
		{"nonsense reading", ReadyChecks{ChilledTemperatureC: ptr(99), SealNumber: "WP-1"}, "invalid"},
		{"seal too long", ReadyChecks{ChilledTemperatureC: ptr(3), SealNumber: strings.Repeat("x", 41)}, "invalid"},
		{"in range with a seal", ReadyChecks{ChilledTemperatureC: ptr(3), SealNumber: " WP-1 "}, ""},
		{"edge of the range", ReadyChecks{ChilledTemperatureC: ptr(2), SealNumber: "WP-1"}, ""},
	}
	for _, capability := range []string{"reefer", "refrigerated", "multi-temp"} {
		for _, c := range cases {
			got, err := validateReadyChecks(capability, c.checks)
			switch {
			case c.want == "" && err != nil:
				t.Errorf("%s/%s: unexpected error %v", capability, c.name, err)
			case c.want != "" && (err == nil || !strings.HasPrefix(err.Error(), c.want)):
				t.Errorf("%s/%s: want %q error, got %v", capability, c.name, c.want, err)
			case c.want == "" && got.SealNumber != "WP-1":
				t.Errorf("%s/%s: seal not trimmed: %q", capability, c.name, got.SealNumber)
			}
		}
	}
}

func TestAmbientVehicleDoesNotNeedTheChecksButTheyAreStillValidated(t *testing.T) {
	if _, err := validateReadyChecks("ambient", ReadyChecks{}); err != nil {
		t.Fatalf("ambient vehicle without checks: %v", err)
	}
	if _, err := validateReadyChecks("ambient", ReadyChecks{ChilledTemperatureC: ptr(99)}); err == nil {
		t.Fatal("an impossible reading must still be refused")
	}
	if _, err := validateReadyChecks("ambient", ReadyChecks{SealNumber: strings.Repeat("x", 41)}); err == nil {
		t.Fatal("an over-long seal must still be refused")
	}
}

func TestPlanDiffIsTheSingleSourceForWhatTheLoaderSeesAndWhatSyncApplies(t *testing.T) {
	loads := []domain.OrderLoad{
		{ID: "l1", OrderID: "o1", OrderRef: "ORD1", StopSequence: 1, SuggestedLoadSequence: 3, Status: domain.LoadLoaded},
		{ID: "l2", OrderID: "o2", OrderRef: "ORD2", StopSequence: 4, SuggestedLoadSequence: 1, Status: domain.LoadLoaded},
		{ID: "l3", OrderID: "o3", OrderRef: "ORD3", StopSequence: 3, SuggestedLoadSequence: 2, Status: domain.LoadPending},
		{ID: "l5", OrderID: "o5", OrderRef: "ORD5", StopSequence: 2, SuggestedLoadSequence: 2, Status: domain.LoadPending, ChangeNote: "Added in v2"},
	}
	allocs := []domain.PlanningAlloc{
		{OrderID: "o1", StopSequence: 1}, {OrderID: "o2", StopSequence: 2}, {OrderID: "o4", OrderRef: "ORD4", StopSequence: 3}, {OrderID: "o5", StopSequence: 2},
	}
	suggested := map[string]int{"o1": 4, "o2": 3, "o4": 2, "o5": 2}
	d := diffPlan(loads, allocs, suggested)

	if len(d.Removed) != 1 || d.Removed[0].ID != "l3" || len(d.Added) != 1 || d.Added[0].OrderID != "o4" {
		t.Fatalf("added/removed = %+v / %+v", d.Added, d.Removed)
	}
	if got := d.removals(); len(got) != 1 || got[0] != "l3" {
		t.Fatalf("removals = %v", got)
	}
	// A moved line that was loaded is rechecked; a line that only changed its place in
	// the load order keeps its status and note; an unchanged line is not touched.
	changes := map[string]store.LoadChange{}
	for _, c := range d.loadChanges() {
		changes[c.LoadID] = c
	}
	if c := changes["l2"]; c.StopSequence != 2 || c.SuggestedLoadSequence != 3 || !c.ResetToPending || c.Note != "Moved from Stop 4" {
		t.Fatalf("moved loaded line = %+v", c)
	}
	if c := changes["l1"]; c.StopSequence != 1 || c.SuggestedLoadSequence != 4 || c.ResetToPending || c.Note != "" {
		t.Fatalf("resequenced line = %+v", c)
	}
	if _, touched := changes["l5"]; touched {
		t.Fatalf("an unchanged line must not be touched: %+v", changes["l5"])
	}

	// What the loader is told is exactly the lines sync moves, removes or adds.
	shown := map[string]string{}
	for _, c := range d.describe() {
		shown[c["orderId"].(string)] = c["kind"].(string)
	}
	if len(shown) != 3 || shown["o2"] != "MOVED" || shown["o3"] != "REMOVED" || shown["o4"] != "ADDED" {
		t.Fatalf("described = %v", shown)
	}
	if _, listed := shown["o1"]; listed {
		t.Fatal("a pure load-order change is not a stop change and is not announced")
	}
}

func TestOrdersAreReadConcurrentlyButNeverMoreThanTheLimit(t *testing.T) {
	var inFlight, peak, calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/orders/")
		_, _ = w.Write([]byte(`{"order":{"id":"` + id + `","orderRef":"REF-` + id + `","orderUnits":5}}`))
	}))
	defer srv.Close()
	svc := Service{Peers: client.Peers{OrdersURL: srv.URL}}
	ids := []string{}
	for i := 0; i < 30; i++ {
		ids = append(ids, fmt.Sprintf("o%d", i))
	}
	ids = append(ids, "o3", "o4", "") // duplicates and blanks are not fetched again
	got, err := svc.orderDetails(context.Background(), ids)
	if err != nil || len(got) != 30 || got["o7"].OrderRef != "REF-o7" {
		t.Fatalf("details = %d err=%v", len(got), err)
	}
	if calls.Load() != 30 {
		t.Fatalf("each distinct order is read once, got %d calls", calls.Load())
	}
	if peak.Load() < 2 || peak.Load() > orderFetchConcurrency {
		t.Fatalf("expected between 2 and %d reads at once, saw %d", orderFetchConcurrency, peak.Load())
	}
}

func TestOrderDetailsReturnsWhatItCouldReadAndTheFirstError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/bad") {
			http.Error(w, "nope", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"order":{"id":"ok","orderUnits":1}}`))
	}))
	defer srv.Close()
	got, err := Service{Peers: client.Peers{OrdersURL: srv.URL}}.orderDetails(context.Background(), []string{"ok", "bad"})
	if err == nil || len(got) != 1 {
		t.Fatalf("got %d details, err=%v; want the readable one and an error", len(got), err)
	}
}
