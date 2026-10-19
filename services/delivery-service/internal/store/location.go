package store

import (
	"context"
	"fmt"
	"time"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (p Postgres) SaveLocation(ctx context.Context, tripID, vehicleID string, latitude, longitude float64, timestamp time.Time) (domain.Location, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.Location{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var runID, assignedVehicle, status string
	err = tx.QueryRow(ctx, "SELECT id::text,vehicle_id,status FROM delivery.runs WHERE trip_id=$1 FOR UPDATE", tripID).Scan(&runID, &assignedVehicle, &status)
	if err == pgx.ErrNoRows {
		return domain.Location{}, fmt.Errorf("not found: trip")
	}
	if err != nil {
		return domain.Location{}, err
	}
	if assignedVehicle != vehicleID {
		return domain.Location{}, fmt.Errorf("forbidden: vehicle mismatch")
	}
	if status != domain.RunInProgress {
		return domain.Location{}, fmt.Errorf("conflict: trip is not active")
	}
	var point domain.Location
	point.TripID = tripID
	var prevLat, prevLon, distance float64
	var prevAt time.Time
	var fixes int
	err = tx.QueryRow(ctx, "SELECT latitude,longitude,recorded_at,distance_m,fixes FROM delivery.run_locations WHERE run_id=$1::uuid", runID).Scan(&prevLat, &prevLon, &prevAt, &distance, &fixes)
	switch {
	case err == pgx.ErrNoRows:
		_, err = tx.Exec(ctx, "INSERT INTO delivery.run_locations(run_id,vehicle_id,latitude,longitude,recorded_at,distance_m,fixes) VALUES($1::uuid,$2,$3,$4,$5,0,1)", runID, vehicleID, latitude, longitude, timestamp)
		if err != nil {
			return domain.Location{}, err
		}
		distance, fixes = 0, 1
	case err != nil:
		return domain.Location{}, err
	case !timestamp.Before(prevAt):
		// Newer or equal: move the point and add the step. An older point (reports can arrive out of order) is
		// ignored and the stored newer point is returned.
		distance += stepDistance(prevLat, prevLon, prevAt, latitude, longitude, timestamp)
		fixes++
		_, err = tx.Exec(ctx, "UPDATE delivery.run_locations SET vehicle_id=$2,latitude=$3,longitude=$4,recorded_at=$5,distance_m=$6,fixes=$7,received_at=now() WHERE run_id=$1::uuid", runID, vehicleID, latitude, longitude, timestamp, distance, fixes)
		if err != nil {
			return domain.Location{}, err
		}
	}
	err = tx.QueryRow(ctx, "SELECT vehicle_id,latitude,longitude,recorded_at,distance_m,fixes FROM delivery.run_locations WHERE run_id=$1::uuid", runID).Scan(&point.VehicleID, &point.Latitude, &point.Longitude, &point.Timestamp, &point.DistanceM, &point.Fixes)
	if err != nil {
		return domain.Location{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Location{}, err
	}
	return point, nil
}

// ActiveLocation hides the retained last point as soon as the run completes.
func (p Postgres) ActiveLocation(ctx context.Context, tripID string) (*domain.Location, error) {
	var point domain.Location
	err := p.Pool.QueryRow(ctx,
		"SELECT r.trip_id,l.vehicle_id,l.latitude,l.longitude,l.recorded_at,l.distance_m,l.fixes "+
			"FROM delivery.run_locations l JOIN delivery.runs r ON r.id=l.run_id "+
			"WHERE r.trip_id=$1 AND r.status=$2", tripID, domain.RunInProgress,
	).Scan(&point.TripID, &point.VehicleID, &point.Latitude, &point.Longitude, &point.Timestamp, &point.DistanceM, &point.Fixes)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &point, nil
}

// RunDistance is the distance driven on a run so far (or in total once it has ended). It is kept after the run
// completes, unlike the position, which is hidden as soon as the run ends.
func (p Postgres) RunDistance(ctx context.Context, tripID string) (*domain.RunDistance, error) {
	var out domain.RunDistance
	out.TripID = tripID
	err := p.Pool.QueryRow(ctx,
		"SELECT l.vehicle_id,l.distance_m,l.fixes,l.recorded_at,r.status FROM delivery.run_locations l JOIN delivery.runs r ON r.id=l.run_id WHERE r.trip_id=$1", tripID,
	).Scan(&out.VehicleID, &out.DistanceM, &out.Fixes, &out.LastRecordedAt, &out.RunStatus)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}
