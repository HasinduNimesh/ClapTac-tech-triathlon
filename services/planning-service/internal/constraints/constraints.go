package constraints

import (
	"strings"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/schedule"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/travel"
)

func ok() domain.Result { return domain.Result{Valid: true} }

func fail(code string, details map[string]any) domain.Result {
	return domain.Result{Valid: false, ReasonCode: code, Details: details}
}

func Weight(order domain.Order, trip domain.TripState, vehicle domain.Vehicle) domain.Result {
	sum := order.WeightKg
	for _, s := range trip.Stops {
		sum += s.WeightKg
	}
	if sum <= vehicle.WeightCap+1e-9 {
		return ok()
	}
	return fail(domain.ReasonWeightExceeded, map[string]any{"sumKg": sum, "capKg": vehicle.WeightCap})
}

func Volume(order domain.Order, trip domain.TripState, vehicle domain.Vehicle) domain.Result {
	sum := order.VolumeM3
	for _, s := range trip.Stops {
		sum += s.VolumeM3
	}
	if sum <= vehicle.VolumeCap+1e-9 {
		return ok()
	}
	return fail(domain.ReasonVolumeExceeded, map[string]any{"sumM3": sum, "capM3": vehicle.VolumeCap})
}

func Temperature(order domain.Order, vehicle domain.Vehicle) domain.Result {
	if strings.EqualFold(order.Temp, "chilled") && !strings.EqualFold(vehicle.Temp, "reefer") {
		return fail(domain.ReasonRefrigeration, map[string]any{"temperatureRequirement": order.Temp, "vehicleTemp": vehicle.Temp})
	}
	return ok()
}

func VanOnly(order domain.Order, vehicle domain.Vehicle) domain.Result {
	if strings.EqualFold(order.Outlet.ParkingConstraint, "van_only") && !strings.EqualFold(vehicle.Type, "van") {
		return fail(domain.ReasonVanRequired, map[string]any{"parkingConstraint": order.Outlet.ParkingConstraint, "vehicleType": vehicle.Type})
	}
	return ok()
}

func Depot(order domain.Order, vehicle domain.Vehicle) domain.Result {
	if order.Outlet.Depot != "" && !strings.EqualFold(order.Outlet.Depot, vehicle.HomeDepot) {
		return fail(domain.ReasonDepotMismatch, map[string]any{"outletDepot": order.Outlet.Depot, "vehicleDepot": vehicle.HomeDepot})
	}
	return ok()
}

func VehicleAvailable(vehicle domain.Vehicle) domain.Result {
	if vehicle.Status != "" && !strings.EqualFold(vehicle.Status, "available") {
		return fail(domain.ReasonVehicleUnavailable, map[string]any{"status": vehicle.Status})
	}
	return ok()
}

func TripLimit(existingTrips int) domain.Result {
	if existingTrips >= 2 {
		return fail(domain.ReasonTripLimit, map[string]any{"trips": existingTrips})
	}
	return ok()
}

func DeliveryWindow(order domain.Order, vehicle domain.Vehicle, trip domain.TripState, day string, est travel.Estimator, outlets map[string]domain.Outlet) domain.Result {
	planDay := schedule.PlanDate(day)
	_, serviceStart, closeT, err := schedule.AppendStop(trip, order, vehicle, planDay, est, outlets)
	if err != nil || serviceStart.After(closeT) {
		return fail(domain.ReasonDeliveryWindow, map[string]any{"serviceStart": serviceStart.Format("15:04"), "windowClose": closeT.Format("15:04")})
	}
	return ok()
}

func FuelQuota(order domain.Order, vehicle domain.Vehicle, trip domain.TripState, outlets map[string]domain.Outlet, est travel.Estimator, otherTripFuel float64) domain.Result {
	planDay := schedule.PlanDate("2000-01-01")
	next, _, _, _ := schedule.AppendStop(trip, order, vehicle, planDay, est, outlets)
	if outlets == nil {
		outlets = map[string]domain.Outlet{}
	}
	outlets[order.OutletID] = order.Outlet
	km := schedule.TripDistanceKm(next, vehicle, outlets, est)
	liters := schedule.FuelLiters(km, vehicle.KmPerL)
	total := vehicle.WeekFuelUsedL + otherTripFuel + liters
	if total <= vehicle.WeeklyQuotaL+1e-9 {
		return ok()
	}
	return fail(domain.ReasonFuelExceeded, map[string]any{"projectedL": total, "quotaL": vehicle.WeeklyQuotaL})
}

func All(order domain.Order, vehicle domain.Vehicle, trip domain.TripState, day string, est travel.Estimator, outlets map[string]domain.Outlet, otherTripFuel float64) []domain.Result {
	checks := []domain.Result{
		VehicleAvailable(vehicle),
		Temperature(order, vehicle),
		VanOnly(order, vehicle),
		Depot(order, vehicle),
		Weight(order, trip, vehicle),
		Volume(order, trip, vehicle),
		DeliveryWindow(order, vehicle, trip, day, est, outlets),
		FuelQuota(order, vehicle, trip, outlets, est, otherTripFuel),
	}
	var bad []domain.Result
	for _, r := range checks {
		if !r.Valid {
			bad = append(bad, r)
		}
	}
	return bad
}
