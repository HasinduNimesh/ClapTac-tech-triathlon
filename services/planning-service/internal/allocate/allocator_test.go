package allocate

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/travel"
)

func TestDeterministicAllocation(t *testing.T) {
	est := travel.New(nil, 15, 15)
	in := Input{
		Date: "2026-09-29",
		Est:  est,
		Orders: []domain.Order{
			{ID: "1", OrderRef: "ORD001", OutletID: "A", Temp: "ambient", WeightKg: 10, VolumeM3: 1, Outlet: domain.Outlet{ID: "A", Brand: "Style", District: "Colombo", Depot: "D1", WindowOpen: "09:00", WindowClose: "17:00"}},
			{ID: "2", OrderRef: "ORD002", OutletID: "B", Temp: "chilled", WeightKg: 10, VolumeM3: 1, Outlet: domain.Outlet{ID: "B", Brand: "Fresh", District: "Colombo", Depot: "D1", WindowOpen: "06:30", WindowClose: "08:00"}},
			{ID: "3", OrderRef: "ORD003", OutletID: "C", Temp: "ambient", WeightKg: 10, VolumeM3: 1, Outlet: domain.Outlet{ID: "C", Brand: "Tech", District: "Colombo", Depot: "D1", ParkingConstraint: "van_only", WindowOpen: "10:00", WindowClose: "16:00"}},
		},
		Vehicles: []domain.Vehicle{
			{ID: "VEH001", Type: "truck", Temp: "ambient", HomeDepot: "D1", Status: "available", WeightCap: 500, VolumeCap: 20, KmPerL: 8, WeeklyQuotaL: 200},
			{ID: "VEH002", Type: "truck", Temp: "reefer", HomeDepot: "D1", Status: "available", WeightCap: 500, VolumeCap: 20, KmPerL: 8, WeeklyQuotaL: 200},
			{ID: "VEH003", Type: "van", Temp: "ambient", HomeDepot: "D1", Status: "available", WeightCap: 200, VolumeCap: 8, KmPerL: 12, WeeklyQuotaL: 80},
		},
	}
	a := Generate(in)
	b := Generate(in)
	if len(a.Assignments) != 3 || len(b.Assignments) != 3 {
		t.Fatalf("assigned %d / %d unalloc %#v", len(a.Assignments), len(b.Assignments), a.Unallocated)
	}
	for i := range a.Assignments {
		if a.Assignments[i].VehicleID != b.Assignments[i].VehicleID || a.Assignments[i].Order.ID != b.Assignments[i].Order.ID {
			t.Fatalf("nondeterministic %#v vs %#v", a.Assignments, b.Assignments)
		}
		planned := a.Assignments[i]
		if planned.Arrival.IsZero() || planned.ServiceStart.Before(planned.Arrival) || planned.Departure.Before(planned.ServiceStart) {
			t.Fatalf("invalid persisted timing inputs for order %s: %#v", planned.Order.ID, planned)
		}
	}
	byOrder := map[string]string{}
	for _, as := range a.Assignments {
		byOrder[as.Order.ID] = as.VehicleID
	}
	if byOrder["2"] != "VEH002" {
		t.Fatalf("chilled want VEH002 got %s", byOrder["2"])
	}
	if byOrder["3"] != "VEH003" {
		t.Fatalf("van-only want VEH003 got %s", byOrder["3"])
	}
}

func BenchmarkGenerate250Orders(b *testing.B) {
	in := benchmarkInput250()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Generate(in)
	}
}

func BenchmarkGenerate250OrdersParallel(b *testing.B) {
	in := benchmarkInput250()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = Generate(in)
		}
	})
}

func BenchmarkGenerateWaypointNetworkScale(b *testing.B) {
	in := benchmarkInputWaypointNetworkScale()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Generate(in)
	}
}

func TestWaypointScaleAccountsForOrdersWithinVehicleLimits(t *testing.T) {
	in := benchmarkInputWaypointNetworkScale()
	out := Generate(in)
	vehicles := make(map[string]domain.Vehicle, len(in.Vehicles))
	orders := make(map[string]domain.Order, len(in.Orders))
	for _, vehicle := range in.Vehicles {
		vehicles[vehicle.ID] = vehicle
	}
	for _, order := range in.Orders {
		orders[order.ID] = order
	}
	seen := make(map[string]bool, len(in.Orders))
	type load struct{ weight, volume float64 }
	loads := map[string]load{}

	for _, assignment := range out.Assignments {
		order := assignment.Order
		vehicle, ok := vehicles[assignment.VehicleID]
		if !ok {
			t.Fatalf("assignment references unknown vehicle %s", assignment.VehicleID)
		}
		if _, ok := orders[order.ID]; !ok || seen[order.ID] {
			t.Fatalf("unknown or duplicate assignment for order %s", order.ID)
		}
		seen[order.ID] = true
		if assignment.TripNumber < 1 || assignment.TripNumber > 2 {
			t.Fatalf("vehicle %s has out-of-range trip number %d", vehicle.ID, assignment.TripNumber)
		}
		if vehicle.HomeDepot != order.Outlet.Depot {
			t.Fatalf("order %s crossed depot from %s to %s", order.ID, vehicle.HomeDepot, order.Outlet.Depot)
		}
		if strings.EqualFold(order.Temp, "chilled") && !strings.EqualFold(vehicle.Temp, "reefer") {
			t.Fatalf("chilled order %s assigned to non-reefer %s", order.ID, vehicle.ID)
		}
		if strings.EqualFold(order.Outlet.ParkingConstraint, "van_only") && !strings.EqualFold(vehicle.Type, "van") {
			t.Fatalf("van-only outlet order %s assigned to %s", order.ID, vehicle.Type)
		}
		if assignment.Arrival.IsZero() || assignment.ServiceStart.Before(assignment.Arrival) || assignment.Departure.Before(assignment.ServiceStart) {
			t.Fatalf("invalid stop timing for order %s: %#v", order.ID, assignment)
		}
		key := fmt.Sprintf("%s/%d", assignment.VehicleID, assignment.TripNumber)
		current := loads[key]
		current.weight += order.WeightKg
		current.volume += order.VolumeM3
		loads[key] = current
	}
	for _, deferred := range out.Unallocated {
		if _, ok := orders[deferred.OrderID]; !ok || seen[deferred.OrderID] {
			t.Fatalf("unknown or duplicate deferral for order %s", deferred.OrderID)
		}
		if deferred.ReasonCode == "" {
			t.Fatalf("order %s has no deferral reason", deferred.OrderID)
		}
		seen[deferred.OrderID] = true
	}
	if len(seen) != len(in.Orders) {
		t.Fatalf("planner accounted for %d of %d orders", len(seen), len(in.Orders))
	}
	for key, used := range loads {
		vehicleID := strings.SplitN(key, "/", 2)[0]
		vehicle := vehicles[vehicleID]
		if used.weight > vehicle.WeightCap+1e-9 || used.volume > vehicle.VolumeCap+1e-9 {
			t.Fatalf("trip %s exceeds vehicle limits: load=%+v limits=%g kg/%g m3", key, used, vehicle.WeightCap, vehicle.VolumeCap)
		}
	}
}

func benchmarkInputWaypointNetworkScale() Input {
	est := travel.New(nil, 15, 15)
	orders := make([]domain.Order, 120)
	brands := []string{"Fresh", "Style", "Tech"}
	for i := range orders {
		depot := "Peliyagoda"
		if i%2 != 0 {
			depot = "Kandy"
		}
		brand := "Fresh"
		if i >= 80 && i < 105 {
			brand = brands[1]
		} else if i >= 105 {
			brand = brands[2]
		}
		outletID := fmt.Sprintf("OUT%03d", i+1)
		temp := "ambient"
		if brand == "Fresh" && i%5 == 0 {
			temp = "chilled"
		}
		access := "normal"
		if brand == "Style" && i%3 == 0 {
			access = "van_only"
		}
		orders[i] = domain.Order{
			ID: fmt.Sprintf("ORD%03d", i+1), OrderRef: fmt.Sprintf("ORD%06d", i+1), OutletID: outletID,
			Temp: temp, WeightKg: 25, VolumeM3: .2,
			Outlet: domain.Outlet{ID: outletID, Brand: brand, District: depot, Depot: depot, ParkingConstraint: access, WindowOpen: "07:00", WindowClose: "17:00"},
		}
	}
	vehicles := make([]domain.Vehicle, 60)
	for i := range vehicles {
		depot := "Peliyagoda"
		if i%2 != 0 {
			depot = "Kandy"
		}
		temp := "ambient"
		vehicleType := "truck"
		if i < 12 {
			temp = "reefer"
		}
		if i >= 12 && i < 16 {
			vehicleType = "van"
			temp = "reefer"
		}
		vehicles[i] = domain.Vehicle{
			ID: fmt.Sprintf("VEH%03d", i+1), Type: vehicleType, Temp: temp, HomeDepot: depot, Status: "available",
			WeightCap: 1000, VolumeCap: 20, KmPerL: 8, WeeklyQuotaL: 200,
		}
	}
	return Input{Date: "2026-09-29", Orders: orders, Vehicles: vehicles, Est: est, FairnessSignalAvailable: true, FuelLedgerAvailable: true}
}

func benchmarkInput250() Input {
	est := travel.New(nil, 15, 15)
	orders := make([]domain.Order, 250)
	for i := range orders {
		id := fmt.Sprintf("OUT%03d", i)
		orders[i] = domain.Order{ID: fmt.Sprint(i), OrderRef: fmt.Sprintf("ORD%06d", i), OutletID: id, Temp: "ambient", WeightKg: 25, VolumeM3: .2, Outlet: domain.Outlet{ID: id, Brand: "Fresh", District: "Colombo", Depot: "D1", WindowOpen: "08:00", WindowClose: "17:00"}}
	}
	vehicles := make([]domain.Vehicle, 25)
	for i := range vehicles {
		vehicles[i] = domain.Vehicle{ID: fmt.Sprintf("VEH%03d", i), Type: "truck", Temp: "reefer", HomeDepot: "D1", Status: "available", WeightCap: 1000, VolumeCap: 20, KmPerL: 8, WeeklyQuotaL: 200}
	}
	return Input{Date: "2026-09-29", Orders: orders, Vehicles: vehicles, Est: est, FairnessSignalAvailable: true, FuelLedgerAvailable: true}
}

func TestConcurrent250OrderGenerationIsDeterministic(t *testing.T) {
	in := benchmarkInput250()
	want := Generate(in)
	if len(want.Assignments) == 0 {
		t.Fatal("benchmark fixture unexpectedly generated no allocations")
	}
	wantSignature := allocationSignature(want)
	const workers = 16
	const iterations = 8
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				got := Generate(in)
				if signature := allocationSignature(got); signature != wantSignature {
					errs <- fmt.Sprintf("allocation changed under contention at iteration %d", iteration)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func allocationSignature(out Output) string {
	var signature strings.Builder
	for _, assignment := range out.Assignments {
		fmt.Fprintf(&signature, "%s=%s/%d;", assignment.Order.ID, assignment.VehicleID, assignment.TripNumber)
	}
	for _, unallocated := range out.Unallocated {
		fmt.Fprintf(&signature, "!%s=%s;", unallocated.OrderID, unallocated.ReasonCode)
	}
	return signature.String()
}

func TestFairnessScoreCombinesDeferralsAndServiceAge(t *testing.T) {
	deferred := domain.Order{OutletDeferralCount: 1, DaysSinceLastServed: 10}
	older := domain.Order{DaysSinceLastServed: 25}
	if got := FairnessScore(deferred); got != 24 {
		t.Fatalf("one deferral plus 10 unserved days = 24, got %d", got)
	}
	if FairnessScore(older) <= FairnessScore(deferred) {
		t.Fatalf("25 unserved days should outrank one deferral plus 10 days")
	}
	if got := FairnessScore(domain.Order{OutletDeferralCount: 99, DaysSinceLastServed: 999}); got != MaxFairnessDeferrals*DeferralFairnessWeightDays+MaxDaysSinceLastServed {
		t.Fatalf("fairness score caps not applied: %d", got)
	}
	if got := FairnessScore(domain.Order{OutletDeferralCount: -1, DaysSinceLastServed: -4}); got != 0 {
		t.Fatalf("negative history must be ignored, got %d", got)
	}
}

func TestPolicyVersionChangesFairnessWeightAndTripLimit(t *testing.T) {
	policy := domain.PlanningPolicy{DeferralWeightPoints: 5, MaxDeferralCount: 3, MaxUnservedDays: 40, MaxTripsPerVehicle: 1}
	if got := FairnessScoreWithPolicy(domain.Order{OutletDeferralCount: 4, DaysSinceLastServed: 60}, policy); got != 55 {
		t.Fatalf("configured fairness score=%d", got)
	}
	orders := []domain.Order{
		{ID: "one", OrderRef: "A", OutletID: "A", Temp: "ambient", WeightKg: 10, VolumeM3: 1, Outlet: domain.Outlet{ID: "A", Brand: "Fresh", District: "Colombo", Depot: "D1", WindowOpen: "06:00", WindowClose: "20:00"}},
		{ID: "two", OrderRef: "B", OutletID: "B", Temp: "ambient", WeightKg: 10, VolumeM3: 1, Outlet: domain.Outlet{ID: "B", Brand: "Fresh", District: "Colombo", Depot: "D1", WindowOpen: "06:00", WindowClose: "20:00"}},
	}
	in := Input{Date: "2026-10-01", Orders: orders, Vehicles: []domain.Vehicle{{ID: "V1", Type: "truck", Temp: "ambient", Status: "available", HomeDepot: "D1", WeightCap: 10, VolumeCap: 5, KmPerL: 10, WeeklyQuotaL: 100}}, Est: travel.New(nil, 15, 15), Policy: policy}
	out := Generate(in)
	if len(out.Assignments) != 1 || len(out.Unallocated) != 1 {
		t.Fatalf("one-trip policy must leave one stop unallocated: %+v", out)
	}
}

func TestSortOrdersUsesFairnessScoreBeforeOperationalTieBreakers(t *testing.T) {
	orders := []domain.Order{
		{ID: "served-recently", OrderRef: "ORD-A", DaysSinceLastServed: 2, Temp: "chilled"},
		{ID: "deferred", OrderRef: "ORD-B", OutletDeferralCount: 1, DaysSinceLastServed: 0},
		{ID: "oldest", OrderRef: "ORD-C", DaysSinceLastServed: 20},
	}
	sorted := SortOrders(orders)
	if sorted[0].ID != "oldest" || sorted[1].ID != "deferred" || sorted[2].ID != "served-recently" {
		t.Fatalf("unexpected fairness order: %s, %s, %s", sorted[0].ID, sorted[1].ID, sorted[2].ID)
	}
}

func TestGenerateUsesFairnessPriorityWhenEligibleOutletsCompeteForCapacity(t *testing.T) {
	orders := []domain.Order{
		{ID: "recent", OrderRef: "ORD-RECENT", OutletID: "OUT-RECENT", Temp: "ambient", WeightKg: 10, VolumeM3: 1,
			Outlet: domain.Outlet{ID: "OUT-RECENT", Brand: "Fresh", District: "Colombo", Depot: "D1", WindowOpen: "06:00", WindowClose: "20:00"}},
		{ID: "deferred", OrderRef: "ORD-DEFERRED", OutletID: "OUT-DEFERRED", OutletDeferralCount: 1, Temp: "ambient", WeightKg: 10, VolumeM3: 1,
			Outlet: domain.Outlet{ID: "OUT-DEFERRED", Brand: "Fresh", District: "Colombo", Depot: "D1", WindowOpen: "06:00", WindowClose: "20:00"}},
	}
	in := Input{Date: "2026-10-01", Orders: orders, Vehicles: []domain.Vehicle{{ID: "V1", Type: "truck", Temp: "ambient", Status: "available", HomeDepot: "D1", WeightCap: 10, VolumeCap: 5, KmPerL: 10, WeeklyQuotaL: 100}}, Est: travel.New(nil, 15, 15), Policy: domain.PlanningPolicy{MaxTripsPerVehicle: 1}}
	out := Generate(in)
	if len(out.Assignments) != 1 || out.Assignments[0].Order.ID != "deferred" {
		t.Fatalf("higher-priority eligible outlet should receive the constrained capacity first: %+v", out)
	}
	if len(out.Unallocated) != 1 || out.Unallocated[0].OrderID != "recent" {
		t.Fatalf("lower-priority outlet should remain explicitly unallocated: %+v", out.Unallocated)
	}
}

func TestDaysSinceLastServedUsesSriLankaCalendarDays(t *testing.T) {
	loc := time.FixedZone("Asia/Colombo", 5*60*60+30*60)
	asOf := time.Date(2026, time.September, 30, 0, 0, 0, 0, loc)
	served := time.Date(2026, time.September, 29, 23, 59, 0, 0, loc)
	if got := DaysSinceLastServed(asOf, served); got != 1 {
		t.Fatalf("expected previous local calendar date to be one day, got %d", got)
	}
	if got := DaysSinceLastServed(asOf, asOf.Add(time.Hour)); got != 0 {
		t.Fatalf("future service date should clamp to 0, got %d", got)
	}
}

func TestNoReeferVanUnallocated(t *testing.T) {
	est := travel.New(nil, 15, 15)
	in := Input{
		Date: "2026-09-29",
		Est:  est,
		Orders: []domain.Order{{
			ID: "4", OrderRef: "ORD004", OutletID: "D", Temp: "chilled", WeightKg: 10, VolumeM3: 1,
			Outlet: domain.Outlet{ID: "D", Brand: "Fresh", District: "Colombo", Depot: "D1", ParkingConstraint: "van_only", WindowOpen: "06:30", WindowClose: "08:00"},
		}},
		Vehicles: []domain.Vehicle{
			{ID: "VEH001", Type: "truck", Temp: "reefer", HomeDepot: "D1", Status: "available", WeightCap: 500, VolumeCap: 20, KmPerL: 8, WeeklyQuotaL: 200},
			{ID: "VEH003", Type: "van", Temp: "ambient", HomeDepot: "D1", Status: "available", WeightCap: 200, VolumeCap: 8, KmPerL: 12, WeeklyQuotaL: 80},
		},
	}
	out := Generate(in)
	if len(out.Unallocated) != 1 {
		t.Fatalf("want unallocated got %#v %#v", out.Assignments, out.Unallocated)
	}
}

func TestExplainUnallocatedPrimaryAndOtherFactors(t *testing.T) {
	attempts := [][]domain.Result{
		{{ReasonCode: domain.ReasonRefrigeration}, {ReasonCode: domain.ReasonDepotMismatch}},
		{{ReasonCode: domain.ReasonWeightExceeded, Details: map[string]any{"capKg": 100.0}}},
		{{ReasonCode: domain.ReasonRefrigeration}, {ReasonCode: domain.ReasonWeightExceeded}},
	}
	code, details := ExplainUnallocated(attempts)
	if code != domain.ReasonWeightExceeded {
		t.Fatalf("closest attempt should decide primary reason, got %s", code)
	}
	if details["capKg"] != 100.0 || details["vehicleTripsEvaluated"] != 3 || details["primaryBlockedVehicleTrips"] != 2 {
		t.Fatalf("unexpected details %#v", details)
	}
	others, ok := details["otherLimitingFactors"].([]map[string]any)
	if !ok || len(others) != 2 || others[0]["reasonCode"] != domain.ReasonRefrigeration || others[0]["vehicleTripsBlocked"] != 2 || others[1]["reasonCode"] != domain.ReasonDepotMismatch {
		t.Fatalf("unexpected other factors %#v", details["otherLimitingFactors"])
	}
	again, againDetails := ExplainUnallocated(attempts)
	if again != code || len(againDetails) != len(details) {
		t.Fatal("explanation must be deterministic")
	}
	if c, d := ExplainUnallocated(nil); c != domain.ReasonNoEligibleVehicle || len(d) != 0 {
		t.Fatalf("no attempts should map to NO_ELIGIBLE_VEHICLE, got %s %#v", c, d)
	}
}

func TestDeferralDebtConsequenceBreaksTiesWithoutOverridingFairness(t *testing.T) {
	fresh := domain.Order{ID: "fresh-chilled", OrderRef: "B", Temp: "chilled", Outlet: domain.Outlet{Brand: "Fresh"}, DaysSinceLastServed: 5}
	plain := domain.Order{ID: "plain", OrderRef: "A", Temp: "ambient", Outlet: domain.Outlet{Brand: "Style"}, DaysSinceLastServed: 5}
	if got := ConsequencePoints(fresh); got != PerishabilityPoints+BrandCriticalityPoints {
		t.Fatalf("chilled Fresh consequence=%d", got)
	}
	if ConsequencePoints(plain) != 0 {
		t.Fatal("ambient non-Fresh order should add no consequence points")
	}
	sorted := SortOrders([]domain.Order{plain, fresh})
	if sorted[0].ID != "fresh-chilled" {
		t.Fatalf("equal fairness: costlier deferral should be queued first, got %s", sorted[0].ID)
	}
	deferred := domain.Order{ID: "deferred", OrderRef: "C", OutletDeferralCount: 1, Outlet: domain.Outlet{Brand: "Style"}, Temp: "ambient"}
	if got := SortOrders([]domain.Order{fresh, deferred}); got[0].ID != "deferred" {
		t.Fatalf("a prior deferral must outrank consequence points, got %s", got[0].ID)
	}
	if got := FairnessScore(fresh); got != 5 {
		t.Fatalf("fairness score itself must stay unchanged, got %d", got)
	}
}
