package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/loading-service/internal/domain"
)

type Postgres struct {
	Pool *pgxpool.Pool
}

func (p Postgres) GetByTrip(ctx context.Context, tripID string) (domain.Session, error) {
	row := p.Pool.QueryRow(ctx, `
		SELECT id::text, trip_id, plan_id, plan_ref, delivery_date::text, vehicle_id, depot, status,
			started_by, started_at, COALESCE(ready_by,''), ready_at,
			COALESCE(trip_number,0), COALESCE(vehicle_type,''), COALESCE(vehicle_temperature_capability,''), plan_version
		FROM sessions WHERE trip_id = $1`, tripID)
	return scanSession(row)
}

func (p Postgres) StartTx(ctx context.Context, sess domain.Session, loads []domain.OrderLoad) (domain.Session, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(ctx, `
		INSERT INTO sessions (trip_id, plan_id, plan_ref, delivery_date, vehicle_id, depot, status, started_by, started_at, trip_number, vehicle_type, vehicle_temperature_capability,plan_version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now(),$9,$10,$11,$12)
		RETURNING id::text, trip_id, plan_id, plan_ref, delivery_date::text, vehicle_id, depot, status,
			started_by, started_at, COALESCE(ready_by,''), ready_at,
			COALESCE(trip_number,0), COALESCE(vehicle_type,''), COALESCE(vehicle_temperature_capability,''),plan_version
	`, sess.TripID, sess.PlanID, sess.PlanRef, sess.DeliveryDate, sess.VehicleID, sess.Depot, domain.SessionInProgress, sess.StartedBy, sess.TripNumber, sess.VehicleType, sess.VehicleTemperatureCapability, sess.PlanVersion)
	created, err := scanSession(row)
	if err != nil {
		return domain.Session{}, err
	}
	for _, l := range loads {
		_, err = tx.Exec(ctx, `
			INSERT INTO order_loads (session_id, allocation_id, order_id, stop_sequence, suggested_load_sequence, expected_units, status, updated_by, order_ref, outlet_id, brand, temperature_requirement)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		`, created.ID, l.AllocationID, l.OrderID, l.StopSequence, l.SuggestedLoadSequence, l.ExpectedUnits, domain.LoadPending, sess.StartedBy, l.OrderRef, l.OutletID, l.Brand, l.TemperatureRequirement)
		if err != nil {
			return domain.Session{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Session{}, err
	}
	return created, nil
}

func (p Postgres) ListLoads(ctx context.Context, sessionID string) ([]domain.OrderLoad, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT id::text, session_id::text, allocation_id, order_id, stop_sequence, suggested_load_sequence, expected_units, status, updated_by,
			COALESCE(order_ref,''), COALESCE(outlet_id,''), COALESCE(brand,''), COALESCE(temperature_requirement,'')
		FROM order_loads WHERE session_id::text = $1 ORDER BY suggested_load_sequence
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.OrderLoad
	for rows.Next() {
		var l domain.OrderLoad
		if err := rows.Scan(&l.ID, &l.SessionID, &l.AllocationID, &l.OrderID, &l.StopSequence, &l.SuggestedLoadSequence, &l.ExpectedUnits, &l.Status, &l.UpdatedBy, &l.OrderRef, &l.OutletID, &l.Brand, &l.TemperatureRequirement); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if out == nil {
		out = []domain.OrderLoad{}
	}
	return out, rows.Err()
}

func (p Postgres) GetLoad(ctx context.Context, sessionID, orderID string) (domain.OrderLoad, error) {
	row := p.Pool.QueryRow(ctx, `
		SELECT id::text, session_id::text, allocation_id, order_id, stop_sequence, suggested_load_sequence, expected_units, status, updated_by,
			COALESCE(order_ref,''), COALESCE(outlet_id,''), COALESCE(brand,''), COALESCE(temperature_requirement,'')
		FROM order_loads WHERE session_id::text = $1 AND order_id = $2
	`, sessionID, orderID)
	var l domain.OrderLoad
	err := row.Scan(&l.ID, &l.SessionID, &l.AllocationID, &l.OrderID, &l.StopSequence, &l.SuggestedLoadSequence, &l.ExpectedUnits, &l.Status, &l.UpdatedBy, &l.OrderRef, &l.OutletID, &l.Brand, &l.TemperatureRequirement)
	if err == pgx.ErrNoRows {
		return l, fmt.Errorf("not found")
	}
	return l, err
}

func (p Postgres) SetLoadStatus(ctx context.Context, loadID, status, actor string) error {
	_, err := p.Pool.Exec(ctx, `UPDATE order_loads SET status = $2, updated_by = $3, updated_at = now() WHERE id::text = $1`, loadID, status, actor)
	return err
}

func (p Postgres) ListIssues(ctx context.Context, loadID string) ([]domain.Issue, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT id::text, order_load_id::text, issue_type, affected_units, COALESCE(note,''), reported_by, idempotency_key, COALESCE(decision,''), COALESCE(decision_note,''), COALESCE(decided_by,''), decided_at
		FROM issues WHERE order_load_id::text = $1 ORDER BY created_at
	`, loadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Issue
	for rows.Next() {
		var i domain.Issue
		if err := rows.Scan(&i.ID, &i.OrderLoadID, &i.IssueType, &i.AffectedUnits, &i.Note, &i.ReportedBy, &i.IdempotencyKey, &i.Decision, &i.DecisionNote, &i.DecidedBy, &i.DecidedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	if out == nil {
		out = []domain.Issue{}
	}
	return out, rows.Err()
}

func (p Postgres) GetIssueByKey(ctx context.Context, key string) (domain.Issue, error) {
	row := p.Pool.QueryRow(ctx, `
		SELECT id::text, order_load_id::text, issue_type, affected_units, COALESCE(note,''), reported_by, idempotency_key, COALESCE(decision,''), COALESCE(decision_note,''), COALESCE(decided_by,''), decided_at
		FROM issues WHERE idempotency_key = $1
	`, key)
	var i domain.Issue
	err := row.Scan(&i.ID, &i.OrderLoadID, &i.IssueType, &i.AffectedUnits, &i.Note, &i.ReportedBy, &i.IdempotencyKey, &i.Decision, &i.DecisionNote, &i.DecidedBy, &i.DecidedAt)
	if err == pgx.ErrNoRows {
		return i, fmt.Errorf("not found")
	}
	return i, err
}

func (p Postgres) InsertIssue(ctx context.Context, iss domain.Issue) (domain.Issue, error) {
	row := p.Pool.QueryRow(ctx, `
		INSERT INTO issues (order_load_id, issue_type, affected_units, note, reported_by, idempotency_key)
		VALUES ($1::uuid,$2,$3,$4,$5,$6)
		RETURNING id::text, order_load_id::text, issue_type, affected_units, COALESCE(note,''), reported_by, idempotency_key, COALESCE(decision,''), COALESCE(decision_note,''), COALESCE(decided_by,''), decided_at
	`, iss.OrderLoadID, iss.IssueType, iss.AffectedUnits, iss.Note, iss.ReportedBy, iss.IdempotencyKey)
	err := row.Scan(&iss.ID, &iss.OrderLoadID, &iss.IssueType, &iss.AffectedUnits, &iss.Note, &iss.ReportedBy, &iss.IdempotencyKey, &iss.Decision, &iss.DecisionNote, &iss.DecidedBy, &iss.DecidedAt)
	return iss, err
}

func (p Postgres) UpdateIssue(ctx context.Context, id string, units int, note, typ string) error {
	_, err := p.Pool.Exec(ctx, `UPDATE issues SET affected_units = $2, note = $3, issue_type = $4, decision = NULL, decision_note = NULL, decided_by = NULL, decided_at = NULL, updated_at = now() WHERE id::text = $1`, id, units, note, typ)
	return err
}

// DecideIssue records the dispatcher's decision on a shortfall.
func (p Postgres) DecideIssue(ctx context.Context, id, decision, note, actor string) error {
	tag, err := p.Pool.Exec(ctx, `UPDATE issues SET decision = $2, decision_note = NULLIF($3, ''), decided_by = $4, decided_at = now(), updated_at = now() WHERE id::text = $1`, id, decision, note, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not found")
	}
	return nil
}

func (p Postgres) DeleteIssue(ctx context.Context, id string) error {
	tag, err := p.Pool.Exec(ctx, `DELETE FROM issues WHERE id::text = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not found")
	}
	return nil
}

func (p Postgres) GetIssue(ctx context.Context, id string) (domain.Issue, error) {
	row := p.Pool.QueryRow(ctx, `
		SELECT id::text, order_load_id::text, issue_type, affected_units, COALESCE(note,''), reported_by, idempotency_key, COALESCE(decision,''), COALESCE(decision_note,''), COALESCE(decided_by,''), decided_at
		FROM issues WHERE id::text = $1
	`, id)
	var i domain.Issue
	err := row.Scan(&i.ID, &i.OrderLoadID, &i.IssueType, &i.AffectedUnits, &i.Note, &i.ReportedBy, &i.IdempotencyKey, &i.Decision, &i.DecisionNote, &i.DecidedBy, &i.DecidedAt)
	if err == pgx.ErrNoRows {
		return i, fmt.Errorf("not found")
	}
	return i, err
}

func (p Postgres) MarkReady(ctx context.Context, sessionID, actor string) (domain.Session, error) {
	row := p.Pool.QueryRow(ctx, `
		UPDATE sessions SET status = 'ready', ready_by = $2, ready_at = now(), updated_at = now()
		WHERE id::text = $1 AND status = 'in_progress'
		RETURNING id::text, trip_id, plan_id, plan_ref, delivery_date::text, vehicle_id, depot, status,
			started_by, started_at, COALESCE(ready_by,''), ready_at,
			COALESCE(trip_number,0), COALESCE(vehicle_type,''), COALESCE(vehicle_temperature_capability,''),plan_version
	`, sessionID, actor)
	return scanSession(row)
}

func (p Postgres) ListReady(ctx context.Context, date, vehicleID string) ([]domain.Session, error) {
	q := `
		SELECT id::text, trip_id, plan_id, plan_ref, delivery_date::text, vehicle_id, depot, status,
			started_by, started_at, COALESCE(ready_by,''), ready_at,
			COALESCE(trip_number,0), COALESCE(vehicle_type,''), COALESCE(vehicle_temperature_capability,''),plan_version
		FROM sessions WHERE status = 'ready' AND delivery_date::text = $1`
	args := []any{date}
	if vehicleID != "" {
		q += ` AND vehicle_id = $2`
		args = append(args, vehicleID)
	}
	q += ` ORDER BY trip_number, trip_id`
	rows, err := p.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if out == nil {
		out = []domain.Session{}
	}
	return out, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanSession(row scanner) (domain.Session, error) {
	var s domain.Session
	var readyAt *time.Time
	err := row.Scan(&s.ID, &s.TripID, &s.PlanID, &s.PlanRef, &s.DeliveryDate, &s.VehicleID, &s.Depot, &s.Status, &s.StartedBy, &s.StartedAt, &s.ReadyBy, &readyAt, &s.TripNumber, &s.VehicleType, &s.VehicleTemperatureCapability, &s.PlanVersion)
	if err == pgx.ErrNoRows {
		return s, fmt.Errorf("not found")
	}
	s.ReadyAt = readyAt
	return s, err
}
