package store

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func (p Postgres) SaveLocation(ctx context.Context, tripID, vehicleID string, latitude, longitude float64, timestamp time.Time) (domain.Location, error) {
    tx, err := p.Pool.Begin(ctx)
    if err != nil { return domain.Location{}, err }
    defer func() { _ = tx.Rollback(ctx) }()
    var runID, assignedVehicle, status string
    err = tx.QueryRow(ctx, "SELECT id::text,vehicle_id,status FROM delivery.runs WHERE trip_id=$1 FOR UPDATE", tripID).Scan(&runID, &assignedVehicle, &status)
    if err == pgx.ErrNoRows { return domain.Location{}, fmt.Errorf("not found: trip") }
    if err != nil { return domain.Location{}, err }
    if assignedVehicle != vehicleID { return domain.Location{}, fmt.Errorf("forbidden: vehicle mismatch") }
    if status != domain.RunInProgress { return domain.Location{}, fmt.Errorf("conflict: trip is not active") }
    var point domain.Location
    point.TripID = tripID
    err = tx.QueryRow(ctx, 
        "INSERT INTO delivery.run_locations(run_id,vehicle_id,latitude,longitude,recorded_at) VALUES($1::uuid,$2,$3,$4,$5) "+
        "ON CONFLICT(run_id) DO UPDATE SET latitude=EXCLUDED.latitude,longitude=EXCLUDED.longitude,recorded_at=EXCLUDED.recorded_at,received_at=now() "+
        "WHERE delivery.run_locations.recorded_at <= EXCLUDED.recorded_at "+
        "RETURNING vehicle_id,latitude,longitude,recorded_at",
        runID, vehicleID, latitude, longitude, timestamp,
    ).Scan(&point.VehicleID, &point.Latitude, &point.Longitude, &point.Timestamp)
    if err == pgx.ErrNoRows {
        err = tx.QueryRow(ctx, "SELECT vehicle_id,latitude,longitude,recorded_at FROM delivery.run_locations WHERE run_id=$1::uuid", runID).Scan(&point.VehicleID, &point.Latitude, &point.Longitude, &point.Timestamp)
    }
    if err != nil { return domain.Location{}, err }
    if err = tx.Commit(ctx); err != nil { return domain.Location{}, err }
    return point, nil
}

// ActiveLocation hides the retained last point as soon as the run completes.
func (p Postgres) ActiveLocation(ctx context.Context, tripID string) (*domain.Location, error) {
    var point domain.Location
    err := p.Pool.QueryRow(ctx,
        "SELECT r.trip_id,l.vehicle_id,l.latitude,l.longitude,l.recorded_at "+
        "FROM delivery.run_locations l JOIN delivery.runs r ON r.id=l.run_id "+
        "WHERE r.trip_id=$1 AND r.status=$2", tripID, domain.RunInProgress,
    ).Scan(&point.TripID, &point.VehicleID, &point.Latitude, &point.Longitude, &point.Timestamp)
    if err == pgx.ErrNoRows { return nil, nil }
    if err != nil { return nil, err }
    return &point, nil
}
