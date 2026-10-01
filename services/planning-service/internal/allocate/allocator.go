package allocate

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/constraints"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/schedule"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/travel"
)

type Assignment struct {
	Order        domain.Order
	VehicleID    string
	TripNumber   int
	Arrival      time.Time
	ServiceStart time.Time
	Departure    time.Time
}

type Output struct {
	Assignments []Assignment
	Unallocated []domain.ConstraintFailure
}

type Input struct {
	Date                    string
	Orders                  []domain.Order
	Vehicles                []domain.Vehicle
	Outlets                 map[string]domain.Outlet
	Est                     travel.Estimator
	FairnessSignalAvailable bool
	FuelLedgerAvailable     bool
	Policy                  domain.PlanningPolicy
	PolicySignalAvailable   bool
}

const (
	DeferralFairnessWeightDays = 14
	MaxFairnessDeferrals       = 12
	MaxDaysSinceLastServed     = 365
)

func FairnessPolicy(signalAvailable bool) string {
	return FairnessPolicyWithPolicy(signalAvailable, domain.PlanningPolicy{})
}

func FairnessPolicyWithPolicy(signalAvailable bool, policy domain.PlanningPolicy) string {
	if !signalAvailable {
		return "Delivery history unavailable; priority uses prior deferrals only."
	}
	if policy.DeferralWeightPoints <= 0 {
		policy.DeferralWeightPoints = DeferralFairnessWeightDays
	}
	if policy.MaxDeferralCount <= 0 {
		policy.MaxDeferralCount = MaxFairnessDeferrals
	}
	if policy.MaxUnservedDays <= 0 {
		policy.MaxUnservedDays = MaxDaysSinceLastServed
	}
	return fmt.Sprintf("Priority = %d × prior deferrals (capped at %d) + days since last successful delivery (capped at %d) + %d for chilled stock + %d for Fresh outlets. Hard constraints always take precedence.", policy.DeferralWeightPoints, policy.MaxDeferralCount, policy.MaxUnservedDays, PerishabilityPoints, BrandCriticalityPoints)
}

// FairnessScore makes the policy explainable: each prior deferral contributes
// 14 priority points, while an outlet gains one point per unserved day up to
// one year. Hard vehicle, depot, cooling, time-window, capacity, and fuel rules
// remain authoritative after this queue-ordering step.
func FairnessScore(order domain.Order) int {
	return FairnessScoreWithPolicy(order, domain.PlanningPolicy{})
}

func FairnessScoreWithPolicy(order domain.Order, policy domain.PlanningPolicy) int {
	if policy.DeferralWeightPoints <= 0 {
		policy.DeferralWeightPoints = DeferralFairnessWeightDays
	}
	if policy.MaxDeferralCount <= 0 {
		policy.MaxDeferralCount = MaxFairnessDeferrals
	}
	if policy.MaxUnservedDays <= 0 {
		policy.MaxUnservedDays = MaxDaysSinceLastServed
	}
	deferrals := order.OutletDeferralCount
	if deferrals < 0 {
		deferrals = 0
	}
	if deferrals > policy.MaxDeferralCount {
		deferrals = policy.MaxDeferralCount
	}
	days := order.DaysSinceLastServed
	if days < 0 {
		days = 0
	}
	if days > policy.MaxUnservedDays {
		days = policy.MaxUnservedDays
	}
	return deferrals*policy.DeferralWeightPoints + days
}

// Deferral-debt consequence weights. They are deliberately small next to the
// deferral weight (14 points) and a day of waiting (1 point), so they only break
// near-ties in favour of orders whose lateness costs the most (perishable chilled
// stock, Fresh morning sales). They never override hard vehicle, access, window,
// capacity or fuel rules, which are checked after this queue-ordering step.
// Team policy, not an official competition rule.
const (
	PerishabilityPoints    = 3
	BrandCriticalityPoints = 2
)

// ConsequencePoints is the perishability and brand-criticality part of the
// deferral-debt score.
func ConsequencePoints(order domain.Order) int {
	points := 0
	if strings.EqualFold(order.Temp, "chilled") {
		points += PerishabilityPoints
	}
	if strings.EqualFold(order.Outlet.Brand, "fresh") {
		points += BrandCriticalityPoints
	}
	return points
}

// DeferralDebtScore is the queue-ordering priority: fairness (prior deferrals and
// days unserved) plus the consequence of deferring this particular order.
func DeferralDebtScore(order domain.Order, policy domain.PlanningPolicy) int {
	return FairnessScoreWithPolicy(order, policy) + ConsequencePoints(order)
}

func DaysSinceLastServed(asOf, lastServed time.Time) int {
	location := asOf.Location()
	served := lastServed.In(location)
	asOfDay := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, location)
	servedDay := time.Date(served.Year(), served.Month(), served.Day(), 0, 0, 0, 0, location)
	days := int(asOfDay.Sub(servedDay).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// Fairness score first, then chilled, van-only, earlier window, order_ref.
func SortOrders(orders []domain.Order) []domain.Order {
	return SortOrdersWithPolicy(orders, domain.PlanningPolicy{})
}

func SortOrdersWithPolicy(orders []domain.Order, policy domain.PlanningPolicy) []domain.Order {
	out := append([]domain.Order{}, orders...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if da, db := DeferralDebtScore(a, policy), DeferralDebtScore(b, policy); da != db {
			return da > db
		}
		ac, bc := strings.EqualFold(a.Temp, "chilled"), strings.EqualFold(b.Temp, "chilled")
		if ac != bc {
			return ac
		}
		av, bv := strings.EqualFold(a.Outlet.ParkingConstraint, "van_only"), strings.EqualFold(b.Outlet.ParkingConstraint, "van_only")
		if av != bv {
			return av
		}
		if a.Outlet.WindowClose != b.Outlet.WindowClose {
			return a.Outlet.WindowClose < b.Outlet.WindowClose
		}
		return a.OrderRef < b.OrderRef
	})
	return out
}

func Generate(in Input) Output {
	orders := SortOrdersWithPolicy(in.Orders, in.Policy)
	vehicles := append([]domain.Vehicle{}, in.Vehicles...)
	sort.Slice(vehicles, func(i, j int) bool { return vehicles[i].ID < vehicles[j].ID })
	trips := map[string]domain.TripState{} // vehicle|trip
	usedTrips := map[string]map[int]bool{}
	outlets := in.Outlets
	if outlets == nil {
		outlets = map[string]domain.Outlet{}
	}
	var assigned []Assignment
	var unalloc []domain.ConstraintFailure
	maxTrips := in.Policy.MaxTripsPerVehicle
	if maxTrips <= 0 || maxTrips > 2 {
		maxTrips = 2
	}
	for _, order := range orders {
		if order.Outlet.ID == "" {
			order.Outlet = outlets[order.OutletID]
		}
		outlets[order.OutletID] = order.Outlet
		placed := false
		var attempts [][]domain.Result
		for _, veh := range vehicles {
			for tripNo := 1; tripNo <= maxTrips; tripNo++ {
				key := veh.ID + "|" + itoa(tripNo)
				state := trips[key]
				state.VehicleID = veh.ID
				state.TripNumber = tripNo
				if used := usedTrips[veh.ID]; used != nil && !used[tripNo] && len(used) >= maxTrips {
					attempts = append(attempts, []domain.Result{constraints.TripLimit(len(used))})
					continue
				}
				otherFuel := otherTripFuel(veh, tripNo, trips, outlets, in.Est)
				fails := constraints.All(order, veh, state, in.Date, in.Est, outlets, otherFuel)
				if len(fails) > 0 {
					attempts = append(attempts, fails)
					continue
				}
				day := schedule.PlanDate(in.Date)
				next, _, _, _ := schedule.AppendStop(state, order, veh, day, in.Est, outlets)
				planned := next.Stops[len(next.Stops)-1]
				trips[key] = next
				if usedTrips[veh.ID] == nil {
					usedTrips[veh.ID] = map[int]bool{}
				}
				usedTrips[veh.ID][tripNo] = true
				assigned = append(assigned, Assignment{Order: order, VehicleID: veh.ID, TripNumber: tripNo, Arrival: planned.Arrival, ServiceStart: planned.ServiceStart, Departure: planned.Depart})
				placed = true
				break
			}
			if placed {
				break
			}
		}
		if !placed {
			code, details := ExplainUnallocated(attempts)
			unalloc = append(unalloc, domain.ConstraintFailure{OrderID: order.ID, ReasonCode: code, Details: details})
		}
	}
	return Output{Assignments: assigned, Unallocated: unalloc}
}

// ExplainUnallocated turns every failed vehicle/trip attempt for one order into a
// single explanation. The primary reason comes from the attempt that was closest
// to feasible (fewest failed rules; ties go to the earliest vehicle/trip), and the
// remaining rules that blocked other vehicles are listed as limiting factors with
// how many vehicle-trips each one ruled out. Pure function of its input, so the
// result stays deterministic.
func ExplainUnallocated(attempts [][]domain.Result) (string, map[string]any) {
	if len(attempts) == 0 {
		return domain.ReasonNoEligibleVehicle, map[string]any{}
	}
	best := 0
	counts := map[string]int{}
	for i, fails := range attempts {
		if len(fails) < len(attempts[best]) {
			best = i
		}
		seen := map[string]bool{}
		for _, f := range fails {
			if !seen[f.ReasonCode] {
				seen[f.ReasonCode] = true
				counts[f.ReasonCode]++
			}
		}
	}
	primary := attempts[best][0]
	details := map[string]any{}
	for k, v := range primary.Details {
		details[k] = v
	}
	type factor struct {
		code  string
		count int
	}
	var others []factor
	for code, n := range counts {
		if code != primary.ReasonCode {
			others = append(others, factor{code, n})
		}
	}
	sort.Slice(others, func(i, j int) bool {
		if others[i].count != others[j].count {
			return others[i].count > others[j].count
		}
		return others[i].code < others[j].code
	})
	if len(others) > 0 {
		list := make([]map[string]any, 0, len(others))
		for _, o := range others {
			list = append(list, map[string]any{"reasonCode": o.code, "vehicleTripsBlocked": o.count})
		}
		details["otherLimitingFactors"] = list
	}
	details["vehicleTripsEvaluated"] = len(attempts)
	details["primaryBlockedVehicleTrips"] = counts[primary.ReasonCode]
	return primary.ReasonCode, details
}

func otherTripFuel(veh domain.Vehicle, skip int, trips map[string]domain.TripState, outlets map[string]domain.Outlet, est travel.Estimator) float64 {
	sum := 0.0
	for n := 1; n <= 2; n++ {
		if n == skip {
			continue
		}
		st := trips[veh.ID+"|"+itoa(n)]
		if len(st.Stops) == 0 {
			continue
		}
		sum += schedule.FuelLiters(schedule.TripDistanceKm(st, veh, outlets, est), veh.KmPerL)
	}
	return sum
}

func itoa(n int) string {
	if n == 1 {
		return "1"
	}
	return "2"
}
