package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/audit"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/fleet-service/internal/domain"
)

type Store struct {
	Pool *pgxpool.Pool
}

func (s Store) RecordFuel(ctx context.Context, entry domain.FuelEntry, key string) (domain.FuelEntry, error) {
	var saved domain.FuelEntry
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO fuel_entries (vehicle_id, entry_date, liters, receipt_ref, note, recorded_by, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id::text, vehicle_id, entry_date::text, liters, receipt_ref, note, recorded_by, created_at::text
	`, entry.VehicleID, entry.Date, entry.Liters, entry.ReceiptRef, entry.Note, entry.RecordedBy, key).Scan(
		&saved.ID, &saved.VehicleID, &saved.Date, &saved.Liters, &saved.ReceiptRef, &saved.Note, &saved.RecordedBy, &saved.CreatedAt,
	)
	if err == nil {
		return saved, nil
	}
	if err != pgx.ErrNoRows {
		return domain.FuelEntry{}, err
	}
	if err := s.Pool.QueryRow(ctx, `
		SELECT id::text, vehicle_id, entry_date::text, liters, receipt_ref, note, recorded_by, created_at::text
		FROM fuel_entries WHERE idempotency_key = $1
	`, key).Scan(&saved.ID, &saved.VehicleID, &saved.Date, &saved.Liters, &saved.ReceiptRef, &saved.Note, &saved.RecordedBy, &saved.CreatedAt); err != nil {
		return domain.FuelEntry{}, err
	}
	if saved.VehicleID != entry.VehicleID || saved.Date != entry.Date || math.Abs(saved.Liters-entry.Liters) >= 0.0005 || saved.ReceiptRef != entry.ReceiptRef || saved.Note != entry.Note || saved.RecordedBy != entry.RecordedBy {
		return domain.FuelEntry{}, fmt.Errorf("conflict: idempotency key belongs to a different fuel entry")
	}
	return saved, nil
}

func (s Store) FuelLedger(ctx context.Context, weekOf, weekStart, weekEnd string) (domain.FuelLedger, error) {
	ledger := domain.FuelLedger{WeekOf: weekOf, WeekStart: weekStart, WeekEnd: weekEnd, Items: []domain.FuelLedgerItem{}, Entries: []domain.FuelEntry{}}
	rows, err := s.Pool.Query(ctx, `
		SELECT v.vehicle_id, v.weekly_fuel_quota_l, COALESCE(SUM(f.liters), 0)
		FROM vehicles v
		LEFT JOIN fuel_entries f ON f.vehicle_id = v.vehicle_id AND f.entry_date BETWEEN $1 AND $2
		GROUP BY v.vehicle_id, v.weekly_fuel_quota_l
		ORDER BY v.vehicle_id
	`, weekStart, weekEnd)
	if err != nil {
		return ledger, err
	}
	for rows.Next() {
		var item domain.FuelLedgerItem
		if err := rows.Scan(&item.VehicleID, &item.WeeklyQuotaL, &item.ActualLitersL); err != nil {
			rows.Close()
			return ledger, err
		}
		ledger.Items = append(ledger.Items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ledger, err
	}
	rows.Close()
	entries, err := s.Pool.Query(ctx, `
		SELECT id::text, vehicle_id, entry_date::text, liters, receipt_ref, note, recorded_by, created_at::text
		FROM fuel_entries WHERE entry_date BETWEEN $1 AND $2
		ORDER BY entry_date DESC, created_at DESC, id
	`, weekStart, weekEnd)
	if err != nil {
		return ledger, err
	}
	defer entries.Close()
	for entries.Next() {
		var item domain.FuelEntry
		if err := entries.Scan(&item.ID, &item.VehicleID, &item.Date, &item.Liters, &item.ReceiptRef, &item.Note, &item.RecordedBy, &item.CreatedAt); err != nil {
			return ledger, err
		}
		ledger.Entries = append(ledger.Entries, item)
	}
	return ledger, entries.Err()
}

func (s Store) List(ctx context.Context) ([]domain.Vehicle, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, home_depot, version
		FROM vehicles ORDER BY vehicle_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Vehicle
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if out == nil {
		out = []domain.Vehicle{}
	}
	return out, rows.Err()
}

func (s Store) Get(ctx context.Context, id string) (domain.Vehicle, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT vehicle_id, type, temp, weight_cap_kg, volume_cap_m3, fuel_type, km_per_l, weekly_fuel_quota_l, home_depot, version
		FROM vehicles WHERE vehicle_id = $1`, id)
	v, err := scanVehicle(row)
	if err == pgx.ErrNoRows {
		return domain.Vehicle{}, fmt.Errorf("not found")
	}
	return v, err
}

func (s Store) UpdateMasterData(ctx context.Context, v domain.Vehicle, expected int, actorID string) (domain.Vehicle, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Vehicle{}, err
	}
	defer tx.Rollback(ctx)
	var before domain.Vehicle
	err = tx.QueryRow(ctx, `SELECT vehicle_id,type,temp,weight_cap_kg,volume_cap_m3,fuel_type,km_per_l,weekly_fuel_quota_l,home_depot,version FROM vehicles WHERE vehicle_id=$1 FOR UPDATE`, v.ID).
		Scan(&before.ID, &before.Type, &before.Temp, &before.WeightCapacityKg, &before.VolumeCapacityM3, &before.FuelType, &before.KmPerL, &before.WeeklyFuelQuotaL, &before.HomeDepot, &before.Version)
	if err != nil {
		return domain.Vehicle{}, err
	}
	if before.Version != expected {
		return domain.Vehicle{}, fmt.Errorf("conflict: vehicle version changed")
	}
	err = tx.QueryRow(ctx, `UPDATE vehicles SET type=$2,temp=$3,weight_cap_kg=$4,volume_cap_m3=$5,fuel_type=$6,km_per_l=$7,weekly_fuel_quota_l=$8,home_depot=$9,version=version+1,updated_at=now()
		WHERE vehicle_id=$1 RETURNING vehicle_id,type,temp,weight_cap_kg,volume_cap_m3,fuel_type,km_per_l,weekly_fuel_quota_l,home_depot,version`,
		v.ID, v.Type, v.Temp, v.WeightCapacityKg, v.VolumeCapacityM3, v.FuelType, v.KmPerL, v.WeeklyFuelQuotaL, v.HomeDepot).
		Scan(&v.ID, &v.Type, &v.Temp, &v.WeightCapacityKg, &v.VolumeCapacityM3, &v.FuelType, &v.KmPerL, &v.WeeklyFuelQuotaL, &v.HomeDepot, &v.Version)
	if err != nil {
		return domain.Vehicle{}, err
	}
	eventID := fmt.Sprintf("fleet-vehicle-%s-v%d", v.ID, v.Version)
	ev := audit.Event{EventID: eventID, ActorID: actorID, ActorType: "human", Action: "MASTER_DATA_VEHICLE_UPDATED", ResourceType: "VEHICLE", ResourceID: v.ID, PreviousState: map[string]any{"vehicle": before}, NewState: map[string]any{"vehicle": v}, Source: "fleet-service", Timestamp: time.Now().UTC()}
	payload, err := json.Marshal(ev)
	if err != nil {
		return domain.Vehicle{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_outbox(event_id,payload) VALUES($1,$2)`, eventID, payload); err != nil {
		return domain.Vehicle{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Vehicle{}, err
	}
	return v, nil
}

// PendingAudit is a read-only diagnostic view. Workers must use
// ClaimPendingAudit to prevent concurrent replicas from publishing duplicates.
func (s Store) PendingAudit(ctx context.Context, limit int) ([]domain.AuditOutbox, error) {
	rows, err := s.Pool.Query(ctx, `SELECT event_id,payload FROM audit_outbox WHERE delivered_at IS NULL AND next_attempt_at<=now() ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.AuditOutbox{}
	for rows.Next() {
		var item domain.AuditOutbox
		if err := rows.Scan(&item.EventID, &item.Payload); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ClaimPendingAudit atomically leases eligible rows so multiple Fleet replicas
// do not publish the same outbox event at the same time. A crashed worker's
// lease expires and the event becomes eligible for at-least-once retry.
func (s Store) ClaimPendingAudit(ctx context.Context, limit int) ([]domain.AuditOutbox, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("audit outbox claim limit must be between 1 and 100")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		WITH candidates AS (
			SELECT event_id FROM audit_outbox
			WHERE delivered_at IS NULL AND next_attempt_at<=now()
			ORDER BY created_at,event_id
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		), claimed AS (
			UPDATE audit_outbox AS outbox
			SET next_attempt_at=now()+interval '60 seconds'
			FROM candidates
			WHERE outbox.event_id=candidates.event_id
			RETURNING outbox.event_id,outbox.payload
		)
		SELECT event_id,payload FROM claimed
	`, limit)
	if err != nil {
		return nil, err
	}
	items := make([]domain.AuditOutbox, 0, limit)
	for rows.Next() {
		var item domain.AuditOutbox
		if err := rows.Scan(&item.EventID, &item.Payload); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (s Store) AuditOutboxHealth(ctx context.Context) (int, float64, error) {
	var pending int
	var oldestSeconds float64
	err := s.Pool.QueryRow(ctx, `SELECT count(*)::int, COALESCE(EXTRACT(EPOCH FROM now()-min(created_at)),0)::float8 FROM audit_outbox WHERE delivered_at IS NULL`).Scan(&pending, &oldestSeconds)
	return pending, oldestSeconds, err
}

func (s Store) MarkAudit(ctx context.Context, eventID, errorText string) error {
	if errorText == "" {
		_, err := s.Pool.Exec(ctx, `UPDATE audit_outbox SET delivered_at=now(),attempts=attempts+1,last_error='' WHERE event_id=$1`, eventID)
		return err
	}
	if len(errorText) > 1000 {
		errorText = errorText[:1000]
	}
	_, err := s.Pool.Exec(ctx, `UPDATE audit_outbox SET attempts=attempts+1,last_error=$2,next_attempt_at=now()+make_interval(secs=>LEAST(300,(2^LEAST(attempts,8))::int)) WHERE event_id=$1`, eventID, errorText)
	return err
}

func (s Store) Availability(ctx context.Context, date string) ([]domain.Availability, error) {
	vehicles, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT vehicle_id, date::text, status, COALESCE(reason,'') FROM vehicle_availability WHERE date = $1`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]domain.Availability{}
	for rows.Next() {
		var a domain.Availability
		if err := rows.Scan(&a.VehicleID, &a.Date, &a.Status, &a.Reason); err != nil {
			return nil, err
		}
		byID[a.VehicleID] = a
	}
	out := make([]domain.Availability, 0, len(vehicles))
	for _, v := range vehicles {
		if a, ok := byID[v.ID]; ok {
			out = append(out, a)
			continue
		}
		out = append(out, domain.Availability{Date: date, VehicleID: v.ID, Status: "available"})
	}
	return out, nil
}

func (s Store) UpsertAvailability(ctx context.Context, a domain.Availability) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO vehicle_availability (vehicle_id, date, status, reason)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (vehicle_id, date) DO UPDATE SET status = EXCLUDED.status, reason = EXCLUDED.reason
	`, a.VehicleID, a.Date, a.Status, a.Reason)
	return err
}

func (s Store) CreateIncident(ctx context.Context, in domain.VehicleIncident) (domain.VehicleIncident, error) {
	stops, _ := json.Marshal(in.AffectedStops)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return in, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO vehicle_incidents(vehicle_id,incident_date,trip_id,incident_type,description,affected_stops,reported_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text,status,reported_at::text`, in.VehicleID, in.Date, in.TripID, in.Type, in.Description, stops, in.ReportedBy).Scan(&in.ID, &in.Status, &in.ReportedAt)
	if err != nil {
		return in, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO vehicle_availability(vehicle_id,date,status,reason) VALUES($1,$2,'unavailable',$3) ON CONFLICT(vehicle_id,date) DO UPDATE SET status='unavailable',reason=EXCLUDED.reason`, in.VehicleID, in.Date, "Open incident: "+in.ID)
	if err != nil {
		return in, err
	}
	id := fmt.Sprintf("fleet-incident-%s", in.ID)
	payload, err := json.Marshal(audit.Event{EventID: id, ActorID: in.ReportedBy, ActorType: "human", Action: "VEHICLE_INCIDENT_REPORTED", ResourceType: "VEHICLE_INCIDENT", ResourceID: in.ID, NewState: map[string]any{"incident": in, "availability": "unavailable"}, Source: "fleet-service", Timestamp: time.Now().UTC()})
	if err != nil {
		return in, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_outbox(event_id,payload) VALUES($1,$2)`, id, payload); err != nil {
		return in, err
	}
	return in, tx.Commit(ctx)
}

func (s Store) Incidents(ctx context.Context, openOnly bool) ([]domain.VehicleIncident, error) {
	q := `SELECT id::text,vehicle_id,incident_date::text,COALESCE(trip_id,''),incident_type,description,affected_stops,status,reported_by,reported_at::text FROM vehicle_incidents`
	if openOnly {
		q += ` WHERE status='open'`
	}
	q += ` ORDER BY reported_at DESC LIMIT 200`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.VehicleIncident{}
	for rows.Next() {
		var in domain.VehicleIncident
		var stops []byte
		if err := rows.Scan(&in.ID, &in.VehicleID, &in.Date, &in.TripID, &in.Type, &in.Description, &stops, &in.Status, &in.ReportedBy, &in.ReportedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(stops, &in.AffectedStops)
		if in.AffectedStops == nil {
			in.AffectedStops = []string{}
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanVehicle(row scanner) (domain.Vehicle, error) {
	var v domain.Vehicle
	err := row.Scan(&v.ID, &v.Type, &v.Temp, &v.WeightCapacityKg, &v.VolumeCapacityM3, &v.FuelType, &v.KmPerL, &v.WeeklyFuelQuotaL, &v.HomeDepot, &v.Version)
	return v, err
}
