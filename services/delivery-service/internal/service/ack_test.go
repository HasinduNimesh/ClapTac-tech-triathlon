package service

import (
	"testing"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func TestDriverAcknowledgementIsPerTrip(t *testing.T) {
	driver := &authorization.Profile{UserID: "USR010", Roles: []string{authorization.RoleDriver}}
	acks := []domain.PlanAcknowledgement{{ActorID: "USR010", ActorRole: authorization.RoleDriver, TripID: "trip-1"}}
	if !driverAcknowledged(acks, driver, "trip-1") {
		t.Fatal("driver acknowledged trip-1 but is not accepted for it")
	}
	if driverAcknowledged(acks, driver, "trip-2") {
		t.Fatal("an acknowledgement of trip-1 must not let the driver start trip-2")
	}
	if driverAcknowledged(acks, &authorization.Profile{UserID: "USR011", Roles: []string{authorization.RoleDriver}}, "trip-1") {
		t.Fatal("another driver's acknowledgement must not count")
	}
	loaderAck := []domain.PlanAcknowledgement{{ActorID: "USR010", ActorRole: authorization.RoleLoader, TripID: "trip-1"}}
	if driverAcknowledged(loaderAck, driver, "trip-1") {
		t.Fatal("a loader receipt must not count as a driver acknowledgement")
	}
}

func TestDriverLegacyPlanLevelAcknowledgementStillGatesStart(t *testing.T) {
	driver := &authorization.Profile{UserID: "USR010", Roles: []string{authorization.RoleDriver}}
	legacy := []domain.PlanAcknowledgement{{ActorID: "USR010", ActorRole: authorization.RoleDriver}}
	if !driverAcknowledged(legacy, driver, "trip-1") {
		t.Fatal("receipts recorded before trip identity existed must keep working for in-flight runs")
	}
}
