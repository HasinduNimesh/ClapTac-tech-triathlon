package service

import (
	"errors"
	"testing"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/order-service/internal/domain"
)

func TestTrackingLocationScopedToStoreOrder(t *testing.T) {
	svc, _, _, owner := receiptService("")
	svc.Delivery = deliveryReaderStub{value: domain.DeliveryTracking{
		RunStatus: "in_progress", TripID: "trip-1",
		Location: &domain.Location{TripID: "trip-1", VehicleID: "VEH001", Latitude: 6.9271, Longitude: 79.8612, Timestamp: time.Now()},
	}}
	tracking, err := svc.Tracking(owner, "order-1")
	if err != nil || tracking.Delivery == nil || tracking.Delivery.Location == nil {
		t.Fatalf("owner tracking missing location: %+v %v", tracking, err)
	}
	otherStore := &authorization.Profile{Roles: []string{authorization.RoleStoreManager}, OutletIDs: []string{"OUT021"}}
	if _, err := svc.Tracking(otherStore, "order-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other store retrieved location: %v", err)
	}
}
