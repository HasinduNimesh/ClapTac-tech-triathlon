package constraints

import (
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/schedule"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/travel"
)

func truck() domain.Vehicle {
	return domain.Vehicle{ID: "VEH001", Type: "truck", Temp: "ambient", HomeDepot: "DEPOT_NORTH", Status: "available", WeightCap: 100, VolumeCap: 10, KmPerL: 10, WeeklyQuotaL: 100}
}

func reeferVan() domain.Vehicle {
	v := truck()
	v.ID, v.Type, v.Temp = "VEH002", "van", "reefer"
	return v
}

func order(temp, parking, depot string, w, vol float64) domain.Order {
	return domain.Order{ID: "o1", OrderRef: "ORD1", OutletID: "OUT1", Temp: temp, WeightKg: w, VolumeM3: vol,
		Outlet: domain.Outlet{ID: "OUT1", Brand: "Style", District: "Colombo", Depot: depot, ParkingConstraint: parking, WindowOpen: "09:00", WindowClose: "17:00"}}
}

func TestWeightExactAllowed(t *testing.T) {
	if !Weight(order("ambient", "normal", "DEPOT_NORTH", 100, 1), domain.TripState{}, truck()).Valid {
		t.Fatal("exact weight")
	}
}

func TestWeightOverRejected(t *testing.T) {
	if Weight(order("ambient", "normal", "DEPOT_NORTH", 101, 1), domain.TripState{}, truck()).Valid {
		t.Fatal("over weight")
	}
}

func TestVolumeOverRejected(t *testing.T) {
	if Volume(order("ambient", "normal", "DEPOT_NORTH", 1, 11), domain.TripState{}, truck()).Valid {
		t.Fatal("over volume")
	}
}

func TestChilledReeferAllowed(t *testing.T) {
	if !Temperature(order("chilled", "normal", "DEPOT_NORTH", 1, 1), reeferVan()).Valid {
		t.Fatal("reefer")
	}
}

func TestChilledAmbientRejected(t *testing.T) {
	if Temperature(order("chilled", "normal", "DEPOT_NORTH", 1, 1), truck()).Valid {
		t.Fatal("ambient chilled")
	}
}

func TestVanOnlyVanAllowed(t *testing.T) {
	if !VanOnly(order("ambient", "van_only", "DEPOT_NORTH", 1, 1), reeferVan()).Valid {
		t.Fatal("van")
	}
}

func TestVanOnlyTruckRejected(t *testing.T) {
	if VanOnly(order("ambient", "van_only", "DEPOT_NORTH", 1, 1), truck()).Valid {
		t.Fatal("truck")
	}
}

func TestWrongDepotRejected(t *testing.T) {
	if Depot(order("ambient", "normal", "DEPOT_SOUTH", 1, 1), truck()).Valid {
		t.Fatal("depot")
	}
}

func TestThirdTripRejected(t *testing.T) {
	if TripLimit(2).Valid {
		t.Fatal("trip limit")
	}
}

func TestUnavailableRejected(t *testing.T) {
	v := truck()
	v.Status = "in_workshop"
	if VehicleAvailable(v).Valid {
		t.Fatal("workshop")
	}
}

func TestFuelExceeded(t *testing.T) {
	v := truck()
	v.WeeklyQuotaL = 0.01
	v.KmPerL = 1
	est := travel.New(nil, 15, 20)
	o := order("ambient", "normal", "DEPOT_NORTH", 1, 1)
	if FuelQuota(o, v, domain.TripState{}, map[string]domain.Outlet{o.OutletID: o.Outlet}, est, 0).Valid {
		t.Fatal("fuel")
	}
}

func TestWindowImpossible(t *testing.T) {
	o := order("ambient", "normal", "DEPOT_NORTH", 1, 1)
	o.Outlet.WindowOpen, o.Outlet.WindowClose = "09:00", "09:30"
	est := travel.New(nil, 15, 15)
	trip := domain.TripState{Stops: []domain.Stop{{
		OutletID: "OUTX", Depart: schedule.PlanDate("2026-09-29").Add(12 * time.Hour),
	}}}
	if DeliveryWindow(o, truck(), trip, "2026-09-29", est, map[string]domain.Outlet{o.OutletID: o.Outlet}).Valid {
		t.Fatal("window")
	}
}

func TestEarlyArrivalWaits(t *testing.T) {
	o := order("ambient", "normal", "DEPOT_NORTH", 1, 1)
	o.Outlet.WindowOpen, o.Outlet.WindowClose = "06:30", "07:15"
	est := travel.New(map[string]travel.Leg{"DEPOT_NORTH|COLOMBO": {Km: 5, Minutes: 20}}, 15, 15)
	if !DeliveryWindow(o, truck(), domain.TripState{}, "2026-09-29", est, map[string]domain.Outlet{o.OutletID: o.Outlet}).Valid {
		t.Fatal("should wait then serve")
	}
}
