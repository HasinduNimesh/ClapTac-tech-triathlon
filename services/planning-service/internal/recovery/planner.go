// Package recovery builds bounded, deterministic depot-handover recovery plans.
// It deliberately never inserts into an active route: no reliable driver/handover
// telemetry exists for that operation yet. All candidate vehicles must be idle.
package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/constraints"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/schedule"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/travel"
)

type Incident struct {
	ID          string `json:"id"`
	VehicleID   string `json:"vehicleId"`
	Date        string `json:"date"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type DeliveryState struct {
	TripID      string     `json:"tripId"`
	Outcome     string     `json:"outcome"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type Input struct {
	Plan        domain.Plan              `json:"plan"`
	Incident    Incident                 `json:"incident"`
	ReadyAt     time.Time                `json:"readyAt"`
	Orders      []domain.Order           `json:"orders"`
	Vehicles    []domain.Vehicle         `json:"vehicles"`
	Allocations []domain.Allocation      `json:"allocations"`
	Trips       []domain.Trip            `json:"trips"`
	Deliveries  map[string]DeliveryState `json:"deliveries"`
	Estimator   travel.Estimator         `json:"estimator"`
}

type Move struct {
	AllocationID string    `json:"allocationId"`
	OrderID      string    `json:"orderId"`
	OrderRef     string    `json:"orderRef"`
	OutletID     string    `json:"outletId"`
	OutletName   string    `json:"outletName"`
	SourceTripID string    `json:"sourceTripId"`
	VehicleID    string    `json:"vehicleId"`
	Chilled      bool      `json:"chilled"`
	Arrival      time.Time `json:"arrival"`
	ServiceStart time.Time `json:"serviceStart"`
	Departure    time.Time `json:"departure"`
	DelayMinutes int       `json:"delayMinutes"`
	Deferred     bool      `json:"deferred"`
	Reason       string    `json:"reason,omitempty"`
}

type Candidate struct {
	ID              string  `json:"id"`
	Strategy        string  `json:"strategy"`
	Recommended     bool    `json:"recommended"`
	Moves           []Move  `json:"moves"`
	Recovered       int     `json:"recovered"`
	Deferred        int     `json:"deferred"`
	ChilledDeferred int     `json:"chilledDeferred"`
	Vehicles        int     `json:"vehicles"`
	DelayMinutes    int     `json:"delayMinutes"`
	DistanceKm      float64 `json:"distanceKm"`
}

type Preference struct {
	OutletID    string    `json:"outletId"`
	CandidateID string    `json:"candidateId"`
	Note        string    `json:"note"`
	Actor       string    `json:"actor"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Case struct {
	ID                 string       `json:"id"`
	PlanID             string       `json:"planId"`
	PlanVersion        int          `json:"planVersion"`
	Incident           Incident     `json:"incident"`
	Depot              string       `json:"depot"`
	ReadyAt            time.Time    `json:"readyAt"`
	CreatedAt          time.Time    `json:"createdAt"`
	ExpiresAt          time.Time    `json:"expiresAt"`
	Status             string       `json:"status"`
	Snapshot           string       `json:"-"`
	AllocationSnapshot string       `json:"-"`
	Candidates         []Candidate  `json:"candidates"`
	Preferences        []Preference `json:"preferences"`
	SelectedID         string       `json:"selectedId,omitempty"`
	ApprovedBy         string       `json:"approvedBy,omitempty"`
	ApprovalNote       string       `json:"approvalNote,omitempty"`
	PublishedVersion   int          `json:"publishedVersion,omitempty"`
}

func Digest(value any) string {
	b, _ := json.Marshal(value)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Build ranks feasible alternatives lexicographically: preserve chilled goods,
// recover more orders, reduce total delay, use fewer vehicles, then distance.
// This is a bounded greedy search, not a claim of global optimality.
func Build(in Input) ([]Candidate, error) {
	if in.ReadyAt.IsZero() || in.ReadyAt.In(schedule.Location()).Format("2006-01-02") != in.Plan.DeliveryDate {
		return nil, fmt.Errorf("invalid: handover must be on the delivery date")
	}
	orders := map[string]domain.Order{}
	for _, o := range in.Orders {
		orders[o.ID] = o
	}
	busy := map[string]bool{}
	for _, tr := range in.Trips {
		busy[tr.VehicleID] = true
	}
	var source domain.Vehicle
	for _, v := range in.Vehicles {
		if v.ID == in.Incident.VehicleID {
			source = v
		}
	}
	if source.HomeDepot == "" {
		return nil, fmt.Errorf("conflict: source depot unavailable")
	}
	var affected []domain.Allocation
	for _, a := range in.Allocations {
		if a.VehicleID != source.ID {
			continue
		}
		d, ok := in.Deliveries[a.OrderID]
		if !ok {
			return nil, fmt.Errorf("conflict: delivery progress unavailable")
		}
		if d.Outcome != "" || d.CompletedAt != nil {
			continue
		}
		if d.TripID != "" && d.TripID != a.TripID {
			return nil, fmt.Errorf("conflict: delivery ownership changed")
		}
		o, ok := orders[a.OrderID]
		if !ok || o.Outlet.ID == "" || o.WeightKg <= 0 || o.VolumeM3 <= 0 || o.Outlet.WindowClose == "" {
			return nil, fmt.Errorf("conflict: affected order data incomplete")
		}
		affected = append(affected, a)
	}
	if len(affected) == 0 {
		return nil, fmt.Errorf("conflict: no remaining deliveries for this vehicle")
	}
	if len(affected) > 100 {
		return nil, fmt.Errorf("conflict: recovery supports up to 100 remaining orders")
	}
	sort.Slice(affected, func(i, j int) bool {
		a, b := orders[affected[i].OrderID], orders[affected[j].OrderID]
		ac, bc := strings.EqualFold(a.Temp, "chilled"), strings.EqualFold(b.Temp, "chilled")
		if ac != bc {
			return ac
		}
		if a.Outlet.WindowClose != b.Outlet.WindowClose {
			return a.Outlet.WindowClose < b.Outlet.WindowClose
		}
		return a.ID < b.ID
	})
	vehicles := []domain.Vehicle{}
	for _, v := range in.Vehicles {
		if v.ID != source.ID && !busy[v.ID] && strings.EqualFold(v.Status, "available") && strings.EqualFold(v.HomeDepot, source.HomeDepot) && v.KmPerL > 0 {
			vehicles = append(vehicles, v)
		}
	}
	sort.Slice(vehicles, func(i, j int) bool { return vehicles[i].ID < vehicles[j].ID })
	if len(vehicles) > 100 {
		return nil, fmt.Errorf("conflict: recovery supports up to 100 idle vehicles")
	}
	all := []Candidate{}
	// Try every single vehicle, then fleet-wide strategies that choose earliest
	// arrival or consolidate cargo. Deduplicate identical allocations.
	for _, v := range vehicles {
		all = append(all, build(in, affected, orders, []domain.Vehicle{v}, false))
	}
	all = append(all, build(in, affected, orders, vehicles, false), build(in, affected, orders, vehicles, true))
	unique := []Candidate{}
	seen := map[string]bool{}
	for _, c := range all {
		c.ID = Digest(c.Moves)[:16]
		if !seen[c.ID] {
			unique = append(unique, c)
			seen[c.ID] = true
		}
	}
	sort.Slice(unique, func(i, j int) bool {
		a, b := unique[i], unique[j]
		if a.ChilledDeferred != b.ChilledDeferred {
			return a.ChilledDeferred < b.ChilledDeferred
		}
		if a.Deferred != b.Deferred {
			return a.Deferred < b.Deferred
		}
		if a.DelayMinutes != b.DelayMinutes {
			return a.DelayMinutes < b.DelayMinutes
		}
		if a.Vehicles != b.Vehicles {
			return a.Vehicles < b.Vehicles
		}
		if a.DistanceKm != b.DistanceKm {
			return a.DistanceKm < b.DistanceKm
		}
		return a.ID < b.ID
	})
	if len(unique) > 3 {
		unique = unique[:3]
	}
	unique[0].Recommended = true
	return unique, nil
}

func build(in Input, affected []domain.Allocation, orders map[string]domain.Order, vehicles []domain.Vehicle, consolidate bool) Candidate {
	c := Candidate{Moves: []Move{}, Strategy: "single"}
	states := map[string]domain.TripState{}
	outlets := map[string]domain.Outlet{}
	for _, o := range in.Orders {
		outlets[o.OutletID] = o.Outlet
	}
	for _, a := range affected {
		o := orders[a.OrderID]
		move := Move{AllocationID: a.ID, OrderID: o.ID, OrderRef: o.OrderRef, OutletID: o.OutletID, OutletName: o.Outlet.Name, SourceTripID: a.TripID, Chilled: strings.EqualFold(o.Temp, "chilled")}
		best := -1
		var next domain.TripState
		for i, v := range vehicles {
			s, ok := appendRecovery(states[v.ID], o, v, in.ReadyAt, in.Estimator, outlets)
			if !ok {
				continue
			}
			if best < 0 || (!consolidate && s.Stops[len(s.Stops)-1].ServiceStart.Before(next.Stops[len(next.Stops)-1].ServiceStart)) {
				best, next = i, s
			}
			if consolidate {
				break
			}
		}
		if best < 0 {
			move.Deferred, move.Reason = true, "No idle vehicle satisfies capacity, refrigeration, access, fuel and delivery window checks."
			c.Deferred++
			if move.Chilled {
				c.ChilledDeferred++
			}
		} else {
			v := vehicles[best]
			states[v.ID] = next
			stop := next.Stops[len(next.Stops)-1]
			move.VehicleID, move.Arrival, move.ServiceStart, move.Departure = v.ID, stop.Arrival, stop.ServiceStart, stop.Depart
			if a.PlannedServiceStartAt != nil {
				move.DelayMinutes = int(math.Max(0, math.Ceil(stop.ServiceStart.Sub(*a.PlannedServiceStartAt).Minutes())))
			}
			c.DelayMinutes += move.DelayMinutes
			c.Recovered++
		}
		c.Moves = append(c.Moves, move)
	}
	for _, v := range vehicles {
		if s := states[v.ID]; len(s.Stops) > 0 {
			c.Vehicles++
			c.DistanceKm += schedule.TripDistanceKm(s, v, outlets, in.Estimator)
		}
	}
	c.DistanceKm = math.Round(c.DistanceKm*10) / 10
	if c.Vehicles > 1 {
		c.Strategy = "split"
	}
	if c.Deferred > 0 {
		c.Strategy = "partial"
	}
	if c.Recovered == 0 {
		c.Strategy = "defer"
	}
	return c
}

func appendRecovery(state domain.TripState, o domain.Order, v domain.Vehicle, ready time.Time, est travel.Estimator, outlets map[string]domain.Outlet) (domain.TripState, bool) {
	for _, check := range []domain.Result{constraints.Temperature(o, v), constraints.VanOnly(o, v), constraints.Depot(o, v), constraints.Weight(o, state, v), constraints.Volume(o, state, v)} {
		if !check.Valid {
			return state, false
		}
	}
	ready = ready.In(schedule.Location())
	from, depart := v.HomeDepot, ready
	if len(state.Stops) > 0 {
		last := state.Stops[len(state.Stops)-1]
		from, depart = outlets[last.OutletID].District, last.Depart
	}
	_, minutes := est.Estimate(from, o.Outlet.District)
	arrival := depart.Add(time.Duration(minutes) * time.Minute)
	start := arrival
	if open := schedule.ParseClock(ready, o.Outlet.WindowOpen); open.After(start) {
		start = open
	}
	if start.After(schedule.ParseClock(ready, o.Outlet.WindowClose)) {
		return state, false
	}
	next := domain.TripState{VehicleID: v.ID, TripNumber: 1, Stops: append([]domain.Stop{}, state.Stops...)}
	next.Stops = append(next.Stops, domain.Stop{OrderID: o.ID, OutletID: o.OutletID, Arrival: arrival, ServiceStart: start, Depart: start.Add(time.Duration(est.ServiceMinutes(o.Outlet.MallWindow)) * time.Minute), WeightKg: o.WeightKg, VolumeM3: o.VolumeM3})
	fuel := schedule.FuelLiters(schedule.TripDistanceKm(next, v, outlets, est), v.KmPerL)
	return next, v.WeekFuelUsedL+fuel <= v.WeeklyQuotaL+1e-9
}
