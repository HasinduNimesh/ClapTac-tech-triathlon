package recovery

import (
	"encoding/json"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/schedule"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/travel"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func exampleInput() Input {
	ready := time.Date(2026, 10, 26, 10, 0, 0, 0, schedule.Location())
	in := Input{Plan: domain.Plan{ID: "example-plan", DeliveryDate: "2026-10-26", CurrentVersion: 3}, Incident: Incident{ID: "incident-1", VehicleID: "WP_CAA_4821", Date: "2026-10-26", Type: "breakdown", Status: "open", Description: "Engine failure after the first delivery. Remaining cargo needs a depot handover."}, ReadyAt: ready, Estimator: travel.New(nil, 15, 20), Deliveries: map[string]DeliveryState{}, Trips: []domain.Trip{{ID: "source-trip", VehicleID: "WP_CAA_4821", TripNumber: 1}}}
	for _, id := range []string{"WP_CAA_4821", "WP_CAB_2416", "WP_CAC_8102", "WP_CAD_3907"} {
		in.Vehicles = append(in.Vehicles, domain.Vehicle{ID: id, Status: "available", Type: "van", Temp: "reefer", HomeDepot: "Kelaniya", WeightCap: 900, VolumeCap: 12, KmPerL: 8, WeeklyQuotaL: 200})
	}
	for i, name := range []string{"Fresh · Colombo 03", "Fresh · Rajagiriya", "Style · Nugegoda", "Tech · Borella"} {
		id := []string{"ORD-1042", "ORD-1048", "ORD-1051", "ORD-1056"}[i]
		o := domain.Order{ID: id, OrderRef: id, OutletID: []string{"OUT001", "OUT002", "OUT003", "OUT004"}[i], Temp: "ambient", WeightKg: 180, VolumeM3: 2, Brand: "Fresh"}
		if i < 2 {
			o.Temp = "chilled"
		}
		o.Outlet = domain.Outlet{ID: o.OutletID, Name: name, Depot: "Kelaniya", District: "Colombo", WindowOpen: "08:00", WindowClose: "16:00"}
		in.Orders = append(in.Orders, o)
		baseline := ready.Add(30 * time.Minute)
		in.Allocations = append(in.Allocations, domain.Allocation{ID: "allocation-" + id, OrderID: id, TripID: "source-trip", VehicleID: in.Incident.VehicleID, Sequence: i + 1, PlannedServiceStartAt: &baseline})
		in.Deliveries[id] = DeliveryState{}
	}
	return in
}

func TestRecoveryExamples(t *testing.T) {
	for _, scenario := range []string{"full", "partial", "no-capacity"} {
		t.Run(scenario, func(t *testing.T) {
			in := exampleInput()
			if scenario == "partial" {
				in.Vehicles = in.Vehicles[:2]
				in.Vehicles[1].WeightCap = 400
			}
			if scenario == "no-capacity" {
				in.Vehicles = in.Vehicles[:1]
			}
			options, err := Build(in)
			if err != nil {
				t.Fatal(err)
			}
			best := options[0]
			t.Logf("%s: recommended %s; recovered=%d deferred=%d vehicles=%d delay=%d min distance=%.1f km", scenario, best.Strategy, best.Recovered, best.Deferred, best.Vehicles, best.DelayMinutes, best.DistanceKm)
			if scenario == "full" && (best.Recovered != 4 || best.Strategy != "split") {
				t.Fatalf("expected full split recovery: %+v", best)
			}
			if scenario == "partial" && (best.Recovered != 2 || best.Deferred != 2 || best.ChilledDeferred != 0) {
				t.Fatalf("expected chilled-first partial recovery: %+v", best)
			}
			if scenario == "no-capacity" && (best.Recovered != 0 || best.Deferred != 4) {
				t.Fatalf("expected explicit deferral: %+v", best)
			}
			if !best.Recommended {
				t.Fatal("best must be recommended")
			}
			again, _ := Build(in)
			if Digest(options) != Digest(again) {
				t.Fatal("non-deterministic results")
			}
			for _, c := range options {
				seen := map[string]bool{}
				for _, m := range c.Moves {
					if seen[m.OrderID] {
						t.Fatal("duplicate order")
					}
					seen[m.OrderID] = true
					if !m.Deferred && m.Arrival.Before(in.ReadyAt) {
						t.Fatal("ETA before handover")
					}
				}
			}
			// Optional reproducible fixtures for the screenshot harness: real planner
			// output with synthetic inputs, never fabricated candidate arithmetic.
			if dir := os.Getenv("RECOVERY_EXAMPLE_DIR"); dir != "" {
				c := Case{ID: "example-" + scenario, PlanID: in.Plan.ID, PlanVersion: 3, Incident: in.Incident, Depot: "Kelaniya", ReadyAt: in.ReadyAt, CreatedAt: in.ReadyAt.Add(-30 * time.Minute), ExpiresAt: in.ReadyAt.Add(-15 * time.Minute), Status: "pending", Candidates: options, Preferences: []Preference{}}
				b, _ := json.MarshalIndent(c, "", "  ")
				if err := os.WriteFile(filepath.Join(dir, scenario+".json"), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRecoveryConstraintsAndProgress(t *testing.T) {
	for _, scenario := range []string{"completed", "busy", "cooling", "weight", "volume", "fuel", "depot", "access", "window", "unknown-progress", "utc-handover"} {
		t.Run(scenario, func(t *testing.T) {
			in := exampleInput()
			in.Vehicles = in.Vehicles[:2]
			in.Orders = in.Orders[:1]
			in.Allocations = in.Allocations[:1]
			switch scenario {
			case "completed":
				in.Deliveries[in.Orders[0].ID] = DeliveryState{Outcome: "PARTIAL"}
			case "busy":
				in.Trips = append(in.Trips, domain.Trip{VehicleID: in.Vehicles[1].ID})
			case "cooling":
				in.Vehicles[1].Temp = "ambient"
			case "weight":
				in.Vehicles[1].WeightCap = 100
			case "volume":
				in.Vehicles[1].VolumeCap = 1
			case "fuel":
				in.Vehicles[1].WeeklyQuotaL = 1
			case "depot":
				in.Vehicles[1].HomeDepot = "Ratmalana"
			case "access":
				in.Vehicles[1].Type = "truck"
				in.Orders[0].Outlet.ParkingConstraint = "van_only"
			case "window":
				in.Orders[0].Outlet.WindowClose = "10:30"
			case "unknown-progress":
				delete(in.Deliveries, in.Orders[0].ID)
			case "utc-handover":
				in.ReadyAt = in.ReadyAt.UTC()
				in.Orders[0].Outlet.WindowClose = "11:00"
			}
			options, err := Build(in)
			if scenario == "completed" || scenario == "unknown-progress" {
				if err == nil {
					t.Fatal("must not plan completed or unknown work")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "utc-handover" {
				if options[0].Recovered != 1 || options[0].Moves[0].Arrival.In(schedule.Location()).Hour() != 10 {
					t.Fatalf("handover timezone incorrect: %+v", options[0])
				}
				return
			}
			if options[0].Recovered != 0 {
				t.Fatalf("unsafe candidate: %+v", options[0])
			}
		})
	}
}

func TestCompletedStopsNeverMove(t *testing.T) {
	in := exampleInput()
	in.Deliveries[in.Orders[0].ID] = DeliveryState{Outcome: "DELIVERED"}
	options, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range options {
		for _, m := range c.Moves {
			if m.OrderID == in.Orders[0].ID {
				t.Fatal("completed order moved")
			}
		}
	}
}
