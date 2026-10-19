package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func validateLocation(profile *authorization.Profile, run domain.Run, latitude, longitude float64, timestamp time.Time, now time.Time) error {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryUpdate) || profile.VehicleID == "" || profile.VehicleID != run.VehicleID {
		return fmt.Errorf("forbidden: driver is not assigned to this vehicle")
	}
	if run.Status != domain.RunInProgress {
		return fmt.Errorf("conflict: trip is not active")
	}
	if math.IsNaN(latitude) || math.IsInf(latitude, 0) || latitude < -90 || latitude > 90 ||
		math.IsNaN(longitude) || math.IsInf(longitude, 0) || longitude < -180 || longitude > 180 ||
		timestamp.IsZero() || timestamp.Before(now.Add(-5*time.Minute)) || timestamp.After(now.Add(2*time.Minute)) {
		return fmt.Errorf("invalid: location coordinates or timestamp")
	}
	return nil
}

func (s Service) UpdateLocation(ctx context.Context, profile *authorization.Profile, tripID string, latitude, longitude float64, timestamp time.Time) (domain.Location, error) {
	run, err := s.Repo.GetByTrip(ctx, tripID)
	if err != nil {
		return domain.Location{}, fmt.Errorf("not found: trip")
	}
	if err := validateLocation(profile, run, latitude, longitude, timestamp, time.Now()); err != nil {
		return domain.Location{}, err
	}
	return s.Repo.SaveLocation(ctx, tripID, profile.VehicleID, latitude, longitude, timestamp)
}

func (s Service) TripLocation(ctx context.Context, profile *authorization.Profile, tripID string) (*domain.Location, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) {
		return nil, fmt.Errorf("forbidden: dispatcher permission required")
	}
	if _, err := s.Repo.GetByTrip(ctx, tripID); err != nil {
		return nil, fmt.Errorf("not found: trip")
	}
	return s.Repo.ActiveLocation(ctx, tripID)
}

// RunDistance is the distance driven on a run, for dispatchers. It stays available after the run completes.
func (s Service) RunDistance(ctx context.Context, profile *authorization.Profile, tripID string) (*domain.RunDistance, error) {
	if profile == nil || !authorization.HasPermission(profile.Roles, authorization.PermDeliveryViewAll) {
		return nil, fmt.Errorf("forbidden: dispatcher permission required")
	}
	if _, err := s.Repo.GetByTrip(ctx, tripID); err != nil {
		return nil, fmt.Errorf("not found: trip")
	}
	return s.Repo.RunDistance(ctx, tripID)
}
