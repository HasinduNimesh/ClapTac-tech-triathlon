package service

import (
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/cutoff"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/store"
)

type testOutlets struct{}

func (testOutlets) Resolve(string, string) (string, []string, []string, error) {
	return "USR001", []string{authorization.RoleStoreManager}, []string{"OUT001"}, nil
}
func (testOutlets) Outlet(string, string) (domain.Outlet, error) {
	return domain.Outlet{ID: "OUT001", Brand: "Fresh"}, nil
}

type testPolicy string

func (p testPolicy) CutoffLocalTime() (string, error) { return string(p), nil }

type testCalendar []domain.OperatingDay

func (c testCalendar) OperatingDays(string, string) ([]domain.OperatingDay, error) { return c, nil }

func TestOrderCreationUsesActiveCutoffAndOperatingCalendar(t *testing.T) {
	loc, _ := time.LoadLocation(cutoff.Zone)
	now := time.Date(2026, 9, 26, 15, 31, 0, 0, loc)
	days := testCalendar{{Date: "2026-09-26", IsOperating: true}, {Date: "2026-09-27", IsOperating: false}, {Date: "2026-09-28", IsOperating: false}, {Date: "2026-09-29", IsOperating: true}}
	s := Service{Repo: store.NewMemory(), Outlets: testOutlets{}, Cutoff: cutoff.Load(nil), Policy: testPolicy("15:30"), Calendar: days, Now: func() time.Time { return now }}
	profile := &authorization.Profile{UserID: "USR001", Roles: []string{authorization.RoleStoreManager}, OutletIDs: []string{"OUT001"}}
	created, err := s.Create(profile, "Bearer test", domain.CreateRequest{RequestedDeliveryDate: "2026-09-26", OrderUnits: 1, OrderWeightKg: 1, OrderVolumeM3: 1, TemperatureRequirement: domain.TempAmbient})
	if err != nil {
		t.Fatal(err)
	}
	if created.RequestedDeliveryDate != "2026-09-29" {
		t.Fatalf("master calendar did not move delivery to next operating date: %+v", created)
	}
}
