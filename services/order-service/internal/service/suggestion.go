package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/cutoff"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/suggest"
)

// Suggestion proposes item lines for the caller's own outlet for the date they want delivery. The date is
// adjusted exactly like a real order (cutoff, non-operating days), so the suggestion is for the day the goods
// would actually arrive. It reads only; the manager reviews and submits the order themselves.
func (s Service) Suggestion(profile *authorization.Profile, bearer, requestedDate, temperature string) (domain.Suggestion, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermOrderCreate) {
		return domain.Suggestion{}, ErrForbidden
	}
	if len(profile.OutletIDs) == 0 {
		return domain.Suggestion{}, fmt.Errorf("%w: store manager profile missing", ErrNotFound)
	}
	temperature = strings.ToLower(strings.TrimSpace(temperature))
	if temperature != "ambient" && temperature != "chilled" {
		return domain.Suggestion{}, fmt.Errorf("%w: temperatureRequirement", ErrInvalid)
	}
	requested, err := time.Parse("2006-01-02", requestedDate)
	if err != nil {
		return domain.Suggestion{}, fmt.Errorf("%w: date", ErrInvalid)
	}
	if s.Products == nil {
		return domain.Suggestion{}, fmt.Errorf("%w: product catalog", ErrUnavailable)
	}
	outletID := profile.OutletIDs[0]
	delivery, _ := s.deliveryDate(requested)
	loc, _ := time.LoadLocation(cutoff.Zone)
	if loc == nil {
		loc = time.UTC
	}
	date := delivery.In(loc)
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	catalog, err := s.Products.Range(bearer)
	if err != nil {
		return domain.Suggestion{}, fmt.Errorf("%w: product catalog", ErrUnavailable)
	}
	products := make([]suggest.Product, 0, len(catalog))
	for _, p := range catalog {
		products = append(products, suggest.Product{ID: p.ID, Name: p.Name, UnitsPerPack: p.UnitsPerPack, AvgDailySalesEach: p.AvgDailySalesEach, Temperature: p.Temperature, Season: p.Season, Brand: p.Brand})
	}
	since := date.AddDate(0, 0, -7*suggest.HistoryWeeks).Format("2006-01-02")
	past, err := s.Repo.RecentLines(outletID, temperature, since)
	if err != nil {
		return domain.Suggestion{}, err
	}
	history := make([]suggest.HistoricOrder, 0, len(past))
	for _, h := range past {
		history = append(history, suggest.HistoricOrder{DeliveryDate: h.DeliveryDate, Packs: h.Packs})
	}
	result := suggest.Suggest(suggest.Input{Date: date, Temperature: temperature, Products: products, History: history, Operating: s.operatingFunc(date)})
	return domain.Suggestion{DeliveryDate: date.Format("2006-01-02"), Result: result}, nil
}

// operatingFunc says whether the depot delivers on a date, around the delivery date, from the configured
// operating calendar. Dates the calendar does not cover fall back to the static working-day calendar, and with
// neither every day counts as operating.
func (s Service) operatingFunc(around time.Time) func(string) bool {
	known := map[string]bool{}
	if s.Calendar != nil {
		from, to := around.AddDate(0, 0, -7*8).Format("2006-01-02"), around.AddDate(0, 0, 14).Format("2006-01-02")
		if days, err := s.Calendar.OperatingDays(from, to); err == nil {
			for _, d := range days {
				known[d.Date] = d.IsOperating
			}
		}
	}
	return func(date string) bool {
		if v, ok := known[date]; ok {
			return v
		}
		if s.Cutoff != nil {
			if d, err := time.Parse("2006-01-02", date); err == nil {
				return s.Cutoff.IsWorkingDay(d)
			}
		}
		return true
	}
}
