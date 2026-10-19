package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/cutoff"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/store"
)

var rangeProducts = []domain.Product{
	{ID: "FR-MILK-1L", Brand: "Fresh", Name: "Fresh milk 1 L", Pack: "crate", UnitsPerPack: 12, PackWeightKg: 12.6, PackVolumeM3: 0.021, Temperature: "chilled", AvgDailySalesEach: 36},
	{ID: "FR-RICE-5KG", Brand: "Fresh", Name: "Samba rice 5 kg", Pack: "bag", UnitsPerPack: 1, PackWeightKg: 5.1, PackVolumeM3: 0.009, Temperature: "ambient", AvgDailySalesEach: 4},
}

func suggestionService(calendar domain.CalendarReader) (Service, *authorization.Profile) {
	s, profile := linesService(&testProducts{items: rangeProducts})
	s.Calendar = calendar
	return s, profile
}

func TestSuggestionStartsFromTheStoresSalesRateAndKeepsToTheGoodsType(t *testing.T) {
	s, profile := suggestionService(nil)
	got, err := s.Suggestion(profile, "Bearer t", "2026-10-07", "chilled")
	if err != nil {
		t.Fatal(err)
	}
	if got.DeliveryDate != "2026-10-07" || got.Basis != "sales" || len(got.Lines) != 1 || got.Lines[0].ProductID != "FR-MILK-1L" || got.Lines[0].Packs != 3 {
		t.Fatalf("%+v", got)
	}
}

func TestSuggestionLearnsFromPlacedOrdersAndUsesTheCalendarForLongClosures(t *testing.T) {
	// Depot closed Sat-Mon: an order for Friday 9 Oct has to last four days.
	days := testCalendar{
		{Date: "2026-10-09", IsOperating: true}, {Date: "2026-10-10", IsOperating: false},
		{Date: "2026-10-11", IsOperating: false}, {Date: "2026-10-12", IsOperating: false}, {Date: "2026-10-13", IsOperating: true},
	}
	s, profile := suggestionService(days)
	// Three earlier orders of 6 crates for one day each (the calendar says every other date operates).
	for _, date := range []string{"2026-09-16", "2026-09-23", "2026-09-30"} {
		order := domain.Order{OutletID: "OUT001", Brand: "Fresh", RequestedDeliveryDate: date, OrderUnits: 6, OrderWeightKg: 75.6, OrderVolumeM3: 0.126, TemperatureRequirement: domain.TempChilled, Status: domain.StatusConfirmed,
			Lines: []domain.OrderLine{{LineNo: 1, ProductID: "FR-MILK-1L", PackQty: 6}}}
		if _, err := s.Repo.Create(order); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Suggestion(profile, "Bearer t", "2026-10-09", "chilled")
	if err != nil {
		t.Fatal(err)
	}
	if got.Basis != "history" || got.CoverDays != 4 || len(got.Lines) != 1 || got.Lines[0].Packs != 24 {
		t.Fatalf("6 crates a day for 4 days: %+v", got)
	}
	if !strings.Contains(got.Lines[0].Reason, "for 4 days") || len(got.Notes) == 0 {
		t.Fatalf("the manager is told why: %+v", got)
	}
}

func TestSuggestionSeesOnlyTheCallersOwnOutlet(t *testing.T) {
	s, profile := suggestionService(nil)
	other := domain.Order{OutletID: "OUT999", Brand: "Fresh", RequestedDeliveryDate: "2026-09-30", OrderUnits: 90, TemperatureRequirement: domain.TempChilled, Lines: []domain.OrderLine{{LineNo: 1, ProductID: "FR-MILK-1L", PackQty: 90}}}
	for i := 0; i < 4; i++ {
		if _, err := s.Repo.Create(other); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Suggestion(profile, "Bearer t", "2026-10-07", "chilled")
	if err != nil || got.Basis != "sales" || got.Lines[0].Packs != 3 {
		t.Fatalf("another outlet's orders must not leak into this suggestion: %+v %v", got, err)
	}
}

func TestSuggestionRefusesBadInputAndOutages(t *testing.T) {
	s, profile := suggestionService(nil)
	for name, args := range map[string][2]string{"bad date": {"07/10/2026", "chilled"}, "bad goods": {"2026-10-07", "frozen"}, "no goods": {"2026-10-07", ""}} {
		if _, err := s.Suggestion(profile, "Bearer t", args[0], args[1]); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := s.Suggestion(&authorization.Profile{UserID: "d", Roles: []string{authorization.RoleDispatcher}}, "Bearer t", "2026-10-07", "chilled"); !errors.Is(err, ErrForbidden) {
		t.Errorf("only a store manager asks for a suggestion: %v", err)
	}
	if _, err := s.Suggestion(nil, "Bearer t", "2026-10-07", "chilled"); !errors.Is(err, ErrForbidden) {
		t.Errorf("no profile: %v", err)
	}
	down := &Service{Repo: store.NewMemory(), Outlets: testOutlets{}, Cutoff: cutoff.Load(nil), Now: func() time.Time { return time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC) }, Products: &testProducts{err: errors.New("down")}}
	if _, err := down.Suggestion(profile, "Bearer t", "2026-10-07", "chilled"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("catalog down: %v", err)
	}
}
