package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func assessCheckout(orders []domain.LoadingOrder, confirmed []string) ([]string, error) {
	if len(orders) == 0 { return nil, fmt.Errorf("conflict: current load list is empty") }
	known := make(map[string]bool, len(orders))
	for _, order := range orders {
		if order.OrderID == "" || known[order.OrderID] { return nil, fmt.Errorf("conflict: invalid current load list") }
		known[order.OrderID] = true
	}
	seen := make(map[string]bool, len(confirmed))
	for _, id := range confirmed {
		if !known[id] || seen[id] { return nil, fmt.Errorf("invalid: confirmed orders must match the current load list") }
		seen[id] = true
	}
	missing := make([]string, 0)
	for _, order := range orders {
		if !seen[order.OrderID] || !strings.EqualFold(order.LoadingStatus, "loaded") || len(order.ShortfallSummary) > 0 {
			missing = append(missing, order.OrderID)
		}
	}
	return missing, nil
}

func (s Service) Checkout(ctx context.Context, profile *authorization.Profile, tripID string, planVersion int, confirmed []string) (domain.Checkout, error) {
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil { return domain.Checkout{}, fmt.Errorf("not found: trip") }
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryStart) {
		return domain.Checkout{}, fmt.Errorf("forbidden: driver required")
	}
	if err := s.guardVehicle(profile, run.VehicleID); err != nil { return domain.Checkout{}, err }
	if run.Status != domain.RunPrepared { return domain.Checkout{}, fmt.Errorf("conflict: trip is not prepared") }
	trip, err := s.Peers.ReadyTrip(ctx, tripID)
	if err != nil { return domain.Checkout{}, fmt.Errorf("conflict: current load list unavailable") }
	if trip.PlanVersion != run.PlanVersion || planVersion != run.PlanVersion || !strings.EqualFold(trip.LoadingStatus, "ready") {
		return domain.Checkout{}, fmt.Errorf("conflict: plan or loading status changed; refresh trip")
	}
	if confirmed == nil { confirmed = []string{} }
	missing, err := assessCheckout(trip.Orders, confirmed)
	if err != nil { return domain.Checkout{}, err }
	confirmedSet := make(map[string]bool, len(confirmed))
	for _, id := range confirmed { confirmedSet[id] = true }
	orderedConfirmed := make([]string, 0, len(confirmed))
	for _, order := range trip.Orders {
		if confirmedSet[order.OrderID] { orderedConfirmed = append(orderedConfirmed, order.OrderID) }
	}
	report, changed, err := s.Repo.RecordCheckout(ctx, run.ID, run.PlanVersion, orderedConfirmed, missing, actor(profile))
	if err != nil { return domain.Checkout{}, err }
	if !changed { return report, nil }
	if len(report.MissingOrderIDs) > 0 {
		s.Peers.Publish(ctx, audit.ActionDeliveryCheckoutBlocked, actor(profile), "TRIP", tripID, map[string]any{
			"planVersion": report.PlanVersion, "missingOrderIds": report.MissingOrderIDs, "depot": run.Depot,
		})
	} else {
		s.Peers.Publish(ctx, audit.ActionDeliveryCheckoutConfirmed, actor(profile), "TRIP", tripID, map[string]any{
			"planVersion": report.PlanVersion, "checkedAt": report.CheckedAt,
		})
	}
	return report, nil
}

func (s Service) CheckoutStatus(ctx context.Context, profile *authorization.Profile, tripID string) (*domain.Checkout, error) {
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil { return nil, fmt.Errorf("not found: trip") }
	if profile == nil { return nil, fmt.Errorf("forbidden: profile required") }
	if authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) {
		return s.Repo.Checkout(ctx, run.ID)
	}
	if authorization.HasPermission(profile.Roles, authorization.PermLoadingView) {
		if profile.Depot == "" || profile.Depot != run.Depot { return nil, fmt.Errorf("forbidden: loader depot mismatch") }
		return s.Repo.Checkout(ctx, run.ID)
	}
	if authorization.HasPermission(profile.Roles, authorization.PermDeliveryStart) {
		if err := s.guardVehicle(profile, run.VehicleID); err != nil { return nil, err }
		return s.Repo.Checkout(ctx, run.ID)
	}
	return nil, fmt.Errorf("forbidden: checkout access denied")
}
