package service

import (
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/store"
)

func TestDeliveryFollowupUsesNextOperatingDayAndOneOrderPerStop(t *testing.T) {
	repo := store.NewMemory()
	original, err := repo.Create(domain.Order{OutletID: "OUT001", Brand: "Fresh",
		RequestedDeliveryDate: "2026-10-03", OrderUnits: 10, OrderWeightKg: 20,
		OrderVolumeM3: 1, TemperatureRequirement: domain.TempAmbient, Status: domain.StatusConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	service := Service{Repo: repo, Now: func() time.Time { return time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC) }, Calendar: testCalendar{
		{Date: "2026-10-04", IsOperating: false},
		{Date: "2026-10-05", IsOperating: true},
	}}
	first, err := service.CreateDeliveryFollowup(original.ID, "stop-1", "2026-10-03", 2, "NEXT_RUN")
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestedDeliveryDate != "2026-10-05" || first.OrderUnits != 2 ||
		first.OrderWeightKg != 4 || first.SourceSystem != "delivery-reattempt" {
		t.Fatalf("follow-up = %+v", first)
	}
	service.Now = func() time.Time { return time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC) }
	replay, err := service.CreateDeliveryFollowup(original.ID, "stop-1", "2026-10-03", 2, "NEXT_RUN")
	if err != nil || replay.ID != first.ID {
		t.Fatalf("duplicate follow-up = %+v, %v", replay, err)
	}
	service.Now = func() time.Time { return time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC) }
	deferred, err := service.CreateDeliveryFollowup(original.ID, "stop-2", "2026-10-03", 1, "REQUEST_DEFERRAL")
	if err != nil || deferred.SourceSystem != "delivery-deferral-request" {
		t.Fatalf("deferral handoff = %+v, %v", deferred, err)
	}
	if _, err := service.CreateDeliveryFollowup(original.ID, "stop-3", "2026-10-03", 11, "NEXT_RUN"); err == nil {
		t.Fatal("quantity above original order must fail")
	}
}

func TestDeliveryFollowupWithoutCalendarRowsUsesNextDay(t *testing.T) {
	repo := store.NewMemory()
	original, err := repo.Create(domain.Order{OutletID: "OUT001", Brand: "Fresh",
		RequestedDeliveryDate: "2026-10-03", OrderUnits: 10, OrderWeightKg: 20,
		OrderVolumeM3: 1, TemperatureRequirement: domain.TempAmbient, Status: domain.StatusConfirmed})
	if err != nil {
		t.Fatal(err)
	}
	service := Service{Repo: repo, Now: func() time.Time { return time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC) }, Calendar: testCalendar{}}
	got, err := service.CreateDeliveryFollowup(original.ID, "stop-1", "2026-10-03", 2, "NEXT_RUN")
	if err != nil || got.RequestedDeliveryDate != "2026-10-04" {
		t.Fatalf("unmaintained calendar must fall back to the next day: %+v, %v", got, err)
	}
	closed := Service{Repo: repo, Now: service.Now, Calendar: testCalendar{{Date: "2026-10-04", IsOperating: false}}}
	if _, err := closed.CreateDeliveryFollowup(original.ID, "stop-2", "2026-10-03", 1, "NEXT_RUN"); err == nil {
		t.Fatal("a calendar whose rows are all closed must still block")
	}
}
