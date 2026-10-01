package travel

import (
	"strconv"
	"strings"
)

// Estimator is the M3 travel/service-time abstraction.
// When district_travel.csv is loaded, table lookups win. Otherwise the
// documented temporary heuristic is used (not a competition rule).
type Estimator struct {
	Legs           map[string]Leg
	DefaultService int
	MallService    int
}

type Leg struct {
	Km      float64
	Minutes int
}

func New(legs map[string]Leg, defaultService, mallService int) Estimator {
	if defaultService <= 0 {
		defaultService = 15
	}
	if mallService <= 0 {
		mallService = 20
	}
	if legs == nil {
		legs = map[string]Leg{}
	}
	return Estimator{Legs: legs, DefaultService: defaultService, MallService: mallService}
}

func Key(from, to string) string { return strings.ToUpper(from) + "|" + strings.ToUpper(to) }

func (e Estimator) Estimate(from, to string) (km float64, minutes int) {
	if from == "" {
		from = to
	}
	if to == "" {
		to = from
	}
	if leg, ok := e.Legs[Key(from, to)]; ok {
		return leg.Km, leg.Minutes
	}
	// Milestone 3 temporary deterministic travel assumption; replaceable through TravelEstimator.
	if strings.EqualFold(from, to) {
		return 8, 20
	}
	return 25, 45
}

func (e Estimator) ServiceMinutes(mall bool) int {
	if mall {
		return e.MallService
	}
	return e.DefaultService
}

func ParseKm(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func ParseMin(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
