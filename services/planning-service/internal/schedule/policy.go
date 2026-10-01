package schedule

import (
	"strings"
	"time"

	_ "time/tzdata"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/travel"
)

const Zone = "Asia/Colombo"

// DepotDeparture is the only place that decides when a trip leaves the depot.
// Fresh leaves early enough for morning store openings; Style/Tech leave to
// arrive at the first window open.
func DepotDeparture(brand string, windowOpen, planDate time.Time, travelToFirst time.Duration) time.Time {
	target := windowOpen
	if strings.EqualFold(brand, "Fresh") {
		capArrive := time.Date(planDate.Year(), planDate.Month(), planDate.Day(), 7, 30, 0, 0, planDate.Location())
		if target.After(capArrive) {
			target = capArrive
		}
	}
	return target.Add(-travelToFirst)
}

func ParseClock(day time.Time, hhmm string) time.Time {
	if hhmm == "" {
		return time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, day.Location())
	}
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		t, err = time.Parse("15:04:05", hhmm)
		if err != nil {
			return time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, day.Location())
		}
	}
	return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), t.Second(), 0, day.Location())
}

func Location() *time.Location {
	loc, err := time.LoadLocation(Zone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func PlanDate(date string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", date, Location())
	if err != nil {
		return time.Now().In(Location())
	}
	return t
}

func district(id string, outlets map[string]domain.Outlet) string {
	if o, ok := outlets[id]; ok && o.District != "" {
		return o.District
	}
	return id
}

// AppendStop applies wait-then-serve: service_start = max(arrival, window_open).
func AppendStop(trip domain.TripState, order domain.Order, vehicle domain.Vehicle, day time.Time, est travel.Estimator, outlets map[string]domain.Outlet) (domain.TripState, time.Time, time.Time, error) {
	if outlets == nil {
		outlets = map[string]domain.Outlet{}
	}
	outlets[order.OutletID] = order.Outlet
	open := ParseClock(day, order.Outlet.WindowOpen)
	closeT := ParseClock(day, order.Outlet.WindowClose)
	from := vehicle.HomeDepot
	var arrival time.Time
	to := district(order.OutletID, outlets)
	if len(trip.Stops) == 0 {
		_, minutes := est.Estimate(from, to)
		travelDur := time.Duration(minutes) * time.Minute
		leave := DepotDeparture(order.Brand, open, day, travelDur)
		arrival = leave.Add(travelDur)
	} else {
		prev := trip.Stops[len(trip.Stops)-1]
		from = district(prev.OutletID, outlets)
		_, minutes := est.Estimate(from, to)
		arrival = prev.Depart.Add(time.Duration(minutes) * time.Minute)
	}
	serviceStart := arrival
	if open.After(arrival) {
		serviceStart = open
	}
	depart := serviceStart.Add(time.Duration(est.ServiceMinutes(order.Outlet.MallWindow)) * time.Minute)
	trip.Stops = append(append([]domain.Stop{}, trip.Stops...), domain.Stop{
		OrderID: order.ID, OutletID: order.OutletID,
		Arrival: arrival, ServiceStart: serviceStart, Depart: depart,
		WeightKg: order.WeightKg, VolumeM3: order.VolumeM3,
	})
	if serviceStart.After(closeT) {
		return trip, serviceStart, closeT, errWindow
	}
	return trip, serviceStart, closeT, nil
}

var errWindow = errString("service starts after window close")

type errString string

func (e errString) Error() string { return string(e) }

func TripDistanceKm(trip domain.TripState, vehicle domain.Vehicle, outlets map[string]domain.Outlet, est travel.Estimator) float64 {
	if len(trip.Stops) == 0 {
		return 0
	}
	total := 0.0
	km, _ := est.Estimate(vehicle.HomeDepot, district(trip.Stops[0].OutletID, outlets))
	total += km
	for i := 1; i < len(trip.Stops); i++ {
		k, _ := est.Estimate(district(trip.Stops[i-1].OutletID, outlets), district(trip.Stops[i].OutletID, outlets))
		total += k
	}
	back, _ := est.Estimate(district(trip.Stops[len(trip.Stops)-1].OutletID, outlets), vehicle.HomeDepot)
	return total + back
}

func FuelLiters(km, kmPerL float64) float64 {
	if kmPerL <= 0 {
		return km
	}
	return km / kmPerL
}
