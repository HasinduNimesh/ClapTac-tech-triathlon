package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HasinduNimesh/ClapTac-tech-triathlon/services/planning-service/internal/domain"
)

type Postgres struct {
	Pool *pgxpool.Pool
}

func (p Postgres) ListDisruptionRisks(ctx context.Context, date string) ([]domain.DisruptionRisk, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT r.id::text,r.delivery_date::text,r.scope,r.scope_key,r.risk_type,r.severity,r.summary,r.source,r.source_reference,
			r.confidence::float8,r.created_by,r.created_at,COALESCE(o.decision,''),COALESCE(o.severity_override,''),
			COALESCE(o.reason,''),COALESCE(o.actor_id,''),o.created_at
		FROM planning.disruption_risks r
		LEFT JOIN LATERAL (
			SELECT decision,severity_override,reason,actor_id,created_at
			FROM planning.disruption_risk_overrides WHERE risk_id=r.id
			ORDER BY created_at DESC,id DESC LIMIT 1
		) o ON TRUE
		WHERE r.delivery_date=$1::date
		ORDER BY CASE r.severity WHEN 'HIGH' THEN 0 WHEN 'MEDIUM' THEN 1 ELSE 2 END,r.scope,r.scope_key,r.created_at DESC`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DisruptionRisk{}
	for rows.Next() {
		item, err := scanDisruptionRisk(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p Postgres) CreateDisruptionRisk(ctx context.Context, risk domain.DisruptionRisk) (domain.DisruptionRisk, error) {
	err := p.Pool.QueryRow(ctx, `
		INSERT INTO planning.disruption_risks(delivery_date,scope,scope_key,risk_type,severity,summary,source,source_reference,confidence,created_by)
		VALUES($1::date,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id::text,created_at`, risk.DeliveryDate, risk.Scope, risk.ScopeKey, risk.RiskType, risk.Severity,
		risk.Summary, risk.Source, risk.SourceReference, risk.Confidence, risk.CreatedBy).Scan(&risk.ID, &risk.CreatedAt)
	return risk, err
}

func (p Postgres) AppendDisruptionRiskOverride(ctx context.Context, riskID, decision, severity, reason, actor string) (domain.DisruptionRisk, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.DisruptionRisk{}, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM planning.disruption_risks WHERE id=$1::uuid)`, riskID).Scan(&exists); err != nil {
		return domain.DisruptionRisk{}, err
	}
	if !exists {
		return domain.DisruptionRisk{}, fmt.Errorf("not found: disruption risk")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO planning.disruption_risk_overrides(risk_id,decision,severity_override,reason,actor_id) VALUES($1::uuid,$2,NULLIF($3,''),$4,$5)`, riskID, decision, severity, reason, actor); err != nil {
		return domain.DisruptionRisk{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DisruptionRisk{}, err
	}
	return p.GetDisruptionRisk(ctx, riskID)
}

func (p Postgres) GetDisruptionRisk(ctx context.Context, riskID string) (domain.DisruptionRisk, error) {
	row := p.Pool.QueryRow(ctx, `
		SELECT r.id::text,r.delivery_date::text,r.scope,r.scope_key,r.risk_type,r.severity,r.summary,r.source,r.source_reference,
			r.confidence::float8,r.created_by,r.created_at,COALESCE(o.decision,''),COALESCE(o.severity_override,''),
			COALESCE(o.reason,''),COALESCE(o.actor_id,''),o.created_at
		FROM planning.disruption_risks r
		LEFT JOIN LATERAL (
			SELECT decision,severity_override,reason,actor_id,created_at
			FROM planning.disruption_risk_overrides WHERE risk_id=r.id
			ORDER BY created_at DESC,id DESC LIMIT 1
		) o ON TRUE
		WHERE r.id=$1::uuid`, riskID)
	return scanDisruptionRisk(row)
}

type rowScanner interface{ Scan(dest ...any) error }

func scanDisruptionRisk(row rowScanner) (domain.DisruptionRisk, error) {
	var risk domain.DisruptionRisk
	var overriddenAt *time.Time
	err := row.Scan(&risk.ID, &risk.DeliveryDate, &risk.Scope, &risk.ScopeKey, &risk.RiskType, &risk.Severity, &risk.Summary,
		&risk.Source, &risk.SourceReference, &risk.Confidence, &risk.CreatedBy, &risk.CreatedAt, &risk.OverrideDecision,
		&risk.OverrideSeverity, &risk.OverrideReason, &risk.OverriddenBy, &overriddenAt)
	if err == pgx.ErrNoRows {
		return risk, fmt.Errorf("not found: disruption risk")
	}
	risk.OverriddenAt = overriddenAt
	return risk, err
}

func (p Postgres) Publish(ctx context.Context, planID, actor, contentHash string) (domain.Publication, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return domain.Publication{}, err
	}
	defer tx.Rollback(ctx)
	var out domain.Publication
	err = tx.QueryRow(ctx, `SELECT pp.version, pp.content_hash, pp.published_by, pp.published_at FROM planning.plans p JOIN planning.plan_publications pp ON pp.plan_id=p.id AND pp.version=p.current_version WHERE p.id=$1::uuid AND pp.content_hash=$2 FOR UPDATE OF p`, planID, contentHash).Scan(&out.Version, &out.ContentHash, &out.PublishedBy, &out.PublishedAt)
	if err == nil {
		if _, err = tx.Exec(ctx, `UPDATE planning.plans SET status='confirmed', current_version=$2, published_at=$3, updated_at=now() WHERE id=$1::uuid`, planID, out.Version, out.PublishedAt); err != nil {
			return domain.Publication{}, err
		}
	} else if err == pgx.ErrNoRows {
		if err = tx.QueryRow(ctx, `SELECT current_version+1 FROM planning.plans WHERE id=$1::uuid FOR UPDATE`, planID).Scan(&out.Version); err != nil {
			return domain.Publication{}, err
		}
		out.ContentHash, out.PublishedBy = contentHash, actor
		if err = tx.QueryRow(ctx, `INSERT INTO planning.plan_publications(plan_id,version,content_hash,published_by) VALUES($1::uuid,$2,$3,$4) RETURNING published_at`, planID, out.Version, contentHash, actor).Scan(&out.PublishedAt); err != nil {
			return domain.Publication{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE planning.plans SET status='confirmed', current_version=$2, published_at=$3, updated_at=now() WHERE id=$1::uuid`, planID, out.Version, out.PublishedAt); err != nil {
			return domain.Publication{}, err
		}
	} else {
		return domain.Publication{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Publication{}, err
	}
	return out, nil
}

func (p Postgres) Publication(ctx context.Context, planID string) (domain.Publication, error) {
	var out domain.Publication
	if err := p.Pool.QueryRow(ctx, `SELECT version,content_hash,published_by,published_at FROM planning.plan_publications WHERE plan_id=$1::uuid ORDER BY version DESC LIMIT 1`, planID).Scan(&out.Version, &out.ContentHash, &out.PublishedBy, &out.PublishedAt); err != nil {
		return out, err
	}
	rows, err := p.Pool.Query(ctx, `SELECT actor_id,actor_role,acknowledged_at FROM planning.plan_acknowledgements WHERE plan_id=$1::uuid AND version=$2 ORDER BY actor_role,actor_id`, planID, out.Version)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var a domain.PlanAcknowledgement
		if err := rows.Scan(&a.ActorID, &a.ActorRole, &a.AcknowledgedAt); err != nil {
			return out, err
		}
		out.Acknowledgements = append(out.Acknowledgements, a)
	}
	if out.Acknowledgements == nil {
		out.Acknowledgements = []domain.PlanAcknowledgement{}
	}
	return out, rows.Err()
}

func (p Postgres) Acknowledge(ctx context.Context, planID string, version int, actor, role string) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current int
	if err := tx.QueryRow(ctx, `SELECT current_version FROM planning.plans WHERE id=$1::uuid FOR UPDATE`, planID).Scan(&current); err != nil {
		return err
	}
	if version != current || current == 0 {
		return fmt.Errorf("stale_version")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO planning.plan_acknowledgements(plan_id,version,actor_id,actor_role) VALUES($1::uuid,$2,$3,$4) ON CONFLICT DO NOTHING`, planID, version, actor, role); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p Postgres) ListConfirmedByDate(ctx context.Context, date string) ([]domain.Plan, error) {
	q := `SELECT id::text, plan_ref, delivery_date::text, status, created_by, generated_at, current_version, published_at FROM plans WHERE status = 'confirmed'`
	args := []any{}
	if date != "" {
		q += ` AND delivery_date = $1`
		args = append(args, date)
	}
	q += ` ORDER BY delivery_date`
	rows, err := p.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Plan
	for rows.Next() {
		pl, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pl)
	}
	if out == nil {
		out = []domain.Plan{}
	}
	return out, rows.Err()
}

func (p Postgres) GetTrip(ctx context.Context, id string) (domain.Trip, error) {
	row := p.Pool.QueryRow(ctx, `SELECT id::text, plan_id::text, vehicle_id, trip_number, status FROM trips WHERE id::text = $1`, id)
	var t domain.Trip
	err := row.Scan(&t.ID, &t.PlanID, &t.VehicleID, &t.TripNumber, &t.Status)
	if err == pgx.ErrNoRows {
		return t, fmt.Errorf("not found")
	}
	return t, err
}

func (p Postgres) GetByDate(ctx context.Context, date string) (domain.Plan, error) {
	row := p.Pool.QueryRow(ctx, `SELECT id::text, plan_ref, delivery_date::text, status, created_by, generated_at, current_version, published_at FROM plans WHERE delivery_date = $1`, date)
	return scanPlan(row)
}

func (p Postgres) Get(ctx context.Context, id string) (domain.Plan, error) {
	row := p.Pool.QueryRow(ctx, `SELECT id::text, plan_ref, delivery_date::text, status, created_by, generated_at, current_version, published_at FROM plans WHERE id::text = $1 OR plan_ref = $1`, id)
	return scanPlan(row)
}

func (p Postgres) GetOrderTracking(ctx context.Context, orderID string) (domain.OrderTracking, error) {
	var out domain.OrderTracking
	var state, planStatus string
	var detail []byte
	err := p.Pool.QueryRow(ctx, `
		SELECT x.state,x.plan_id,x.plan_ref,x.trip_id,x.vehicle_id,x.stop_sequence,x.reason_code,x.reason_comment,x.reason_detail,
			x.planned_arrival_at,x.planned_service_start_at,x.plan_status
		FROM (
			SELECT 'PLANNED'::text state,p.id::text plan_id,p.plan_ref,t.id::text trip_id,t.vehicle_id,a.sequence stop_sequence,
				''::text reason_code,''::text reason_comment,'{}'::jsonb reason_detail,a.planned_arrival_at,a.planned_service_start_at,p.status plan_status,p.delivery_date created
			FROM allocations a JOIN plans p ON p.id=a.plan_id JOIN trips t ON t.id=a.trip_id WHERE a.order_id=$1
			UNION ALL
			SELECT 'DEFERRED'::text,p.id::text,p.plan_ref,''::text,''::text,0,d.reason_code,COALESCE(d.comment,''),COALESCE(d.reason_detail,'{}'::jsonb),NULL::timestamptz,NULL::timestamptz,p.status,p.delivery_date
			FROM deferrals d JOIN plans p ON p.id=d.plan_id WHERE d.order_id=$1
		) x ORDER BY x.created DESC LIMIT 1`, orderID).Scan(&state, &out.PlanID, &out.PlanRef, &out.TripID, &out.VehicleID, &out.StopSequence, &out.ReasonCode, &out.ReasonComment, &detail, &out.PlannedArrivalAt, &out.PlannedServiceStartAt, &planStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, fmt.Errorf("not found")
		}
		return out, err
	}
	out.State = state
	_ = json.Unmarshal(detail, &out.ReasonDetail)
	if state == "PLANNED" && planStatus != "confirmed" {
		out.State = "CONFIRMED"
	}
	return out, nil
}

func (p Postgres) Create(ctx context.Context, date, actor string) (domain.Plan, error) {
	row := p.Pool.QueryRow(ctx, `
		INSERT INTO plans (plan_ref, delivery_date, status, created_by)
		VALUES ('PLAN' || lpad(nextval('planning.plan_ref_seq')::text, 6, '0'), $1, 'draft', $2)
		RETURNING id::text, plan_ref, delivery_date::text, status, created_by, generated_at, current_version, published_at
	`, date, actor)
	return scanPlan(row)
}

// ClaimGeneration serializes generation attempts before any allocations are
// written. The generated_at marker is also the durable indication that a run
// began, so a partial failure cannot be mistaken for a clean draft.
func (p Postgres) ClaimGeneration(ctx context.Context, id string) error {
	tag, err := p.Pool.Exec(ctx, `
		UPDATE planning.plans p SET generated_at=now(),updated_at=now()
		WHERE (p.id::text=$1 OR p.plan_ref=$1) AND p.status<>'confirmed'
		  AND p.generated_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM planning.allocations a WHERE a.plan_id=p.id)
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("conflict: generate already applied; reset first")
	}
	return nil
}

func (p Postgres) ReleaseGeneration(ctx context.Context, id string) error {
	_, err := p.Pool.Exec(ctx, `
		UPDATE planning.plans p SET generated_at=NULL,updated_at=now()
		WHERE (p.id::text=$1 OR p.plan_ref=$1) AND NOT EXISTS (
			SELECT 1 FROM planning.allocations a WHERE a.plan_id=p.id
		)
	`, id)
	return err
}

func (p Postgres) SetStatus(ctx context.Context, id, status string) error {
	_, err := p.Pool.Exec(ctx, `UPDATE plans SET status = $2, updated_at = now() WHERE (id::text = $1 OR plan_ref = $1) AND status <> 'confirmed'`, id, status)
	return err
}

func (p Postgres) Revise(ctx context.Context, id string) error {
	tag, err := p.Pool.Exec(ctx, `UPDATE planning.plans SET status='validated',updated_at=now() WHERE (id::text=$1 OR plan_ref=$1) AND status='confirmed'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("conflict: only a published plan can be revised")
	}
	return nil
}

func (p Postgres) Reset(ctx context.Context, planID string) error {
	_, err := p.Pool.Exec(ctx, `DELETE FROM deferrals WHERE plan_id::text = $1`, planID)
	if err != nil {
		return err
	}
	_, err = p.Pool.Exec(ctx, `DELETE FROM allocations WHERE plan_id::text = $1`, planID)
	if err != nil {
		return err
	}
	_, err = p.Pool.Exec(ctx, `DELETE FROM trips WHERE plan_id::text = $1`, planID)
	if err != nil {
		return err
	}
	_, err = p.Pool.Exec(ctx, `UPDATE plans SET generated_at = NULL, status = 'draft', updated_at = now() WHERE id::text = $1`, planID)
	return err
}

func (p Postgres) CountAllocations(ctx context.Context, planID string) (int, error) {
	var n int
	err := p.Pool.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE plan_id::text = $1`, planID).Scan(&n)
	return n, err
}

func (p Postgres) EnsureTrip(ctx context.Context, planID, vehicleID string, tripNo int) (domain.Trip, error) {
	row := p.Pool.QueryRow(ctx, `
		INSERT INTO trips (plan_id, vehicle_id, trip_number, status)
		VALUES ($1::uuid, $2, $3, 'draft')
		ON CONFLICT (plan_id, vehicle_id, trip_number) DO UPDATE SET updated_at = now()
		RETURNING id::text, plan_id::text, vehicle_id, trip_number, status
	`, planID, vehicleID, tripNo)
	var t domain.Trip
	err := row.Scan(&t.ID, &t.PlanID, &t.VehicleID, &t.TripNumber, &t.Status)
	return t, err
}

func (p Postgres) NextSeq(ctx context.Context, tripID string) (int, error) {
	var n int
	err := p.Pool.QueryRow(ctx, `SELECT COALESCE(max(sequence),0)+1 FROM allocations WHERE trip_id::text = $1`, tripID).Scan(&n)
	return n, err
}

func (p Postgres) InsertAllocation(ctx context.Context, a domain.Allocation) (domain.Allocation, error) {
	row := p.Pool.QueryRow(ctx, `
		INSERT INTO allocations (plan_id, order_id, trip_id, vehicle_id, sequence, planned_arrival_at, planned_service_start_at, planned_departure_at)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8)
		RETURNING id::text, plan_id::text, order_id, trip_id::text, vehicle_id, sequence, planned_arrival_at, planned_service_start_at, planned_departure_at
	`, a.PlanID, a.OrderID, a.TripID, a.VehicleID, a.Sequence, a.PlannedArrivalAt, a.PlannedServiceStartAt, a.PlannedDepartureAt)
	err := row.Scan(&a.ID, &a.PlanID, &a.OrderID, &a.TripID, &a.VehicleID, &a.Sequence, &a.PlannedArrivalAt, &a.PlannedServiceStartAt, &a.PlannedDepartureAt)
	return a, err
}

func (p Postgres) MoveTripAllocations(ctx context.Context, planID, sourceVehicle, targetVehicle string, tripNo int, moved []domain.Allocation) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var sourceTrip string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM planning.trips WHERE plan_id=$1::uuid AND vehicle_id=$2 AND trip_number=$3 FOR UPDATE`, planID, sourceVehicle, tripNo).Scan(&sourceTrip); err != nil {
		return err
	}
	var targetTrip string
	if err = tx.QueryRow(ctx, `INSERT INTO planning.trips(plan_id,vehicle_id,trip_number,status) VALUES($1::uuid,$2,$3,'draft') ON CONFLICT(plan_id,vehicle_id,trip_number) DO UPDATE SET updated_at=now() RETURNING id::text`, planID, targetVehicle, tripNo).Scan(&targetTrip); err != nil {
		return err
	}
	for i, a := range moved {
		tag, e := tx.Exec(ctx, `UPDATE planning.allocations SET trip_id=$3::uuid,vehicle_id=$4,sequence=$5,planned_arrival_at=$6,planned_service_start_at=$7,planned_departure_at=$8,updated_at=now() WHERE plan_id=$1::uuid AND id::text=$2 AND trip_id::text=$9 AND vehicle_id=$10`, planID, a.ID, targetTrip, targetVehicle, i+1, a.PlannedArrivalAt, a.PlannedServiceStartAt, a.PlannedDepartureAt, sourceTrip, sourceVehicle)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("conflict: affected allocation changed during reassignment")
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM planning.trips t WHERE t.id::text=$1 AND NOT EXISTS(SELECT 1 FROM planning.allocations a WHERE a.trip_id=t.id)`, sourceTrip); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p Postgres) GetAllocation(ctx context.Context, planID, allocID string) (domain.Allocation, error) {
	row := p.Pool.QueryRow(ctx, `
		SELECT id::text, plan_id::text, order_id, trip_id::text, vehicle_id, sequence, planned_arrival_at, planned_service_start_at, planned_departure_at
		FROM allocations WHERE plan_id::text = $1 AND id::text = $2
	`, planID, allocID)
	var a domain.Allocation
	err := row.Scan(&a.ID, &a.PlanID, &a.OrderID, &a.TripID, &a.VehicleID, &a.Sequence, &a.PlannedArrivalAt, &a.PlannedServiceStartAt, &a.PlannedDepartureAt)
	if err == pgx.ErrNoRows {
		return a, fmt.Errorf("not found")
	}
	return a, err
}

func (p Postgres) DeleteAllocation(ctx context.Context, planID, allocID string) error {
	tag, err := p.Pool.Exec(ctx, `DELETE FROM allocations WHERE plan_id::text = $1 AND id::text = $2`, planID, allocID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not found")
	}
	return nil
}

func (p Postgres) ListAllocations(ctx context.Context, planID string) ([]domain.Allocation, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT id::text, plan_id::text, order_id, trip_id::text, vehicle_id, sequence, planned_arrival_at, planned_service_start_at, planned_departure_at
		FROM allocations WHERE plan_id::text = $1 ORDER BY vehicle_id, sequence
	`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Allocation
	for rows.Next() {
		var a domain.Allocation
		if err := rows.Scan(&a.ID, &a.PlanID, &a.OrderID, &a.TripID, &a.VehicleID, &a.Sequence, &a.PlannedArrivalAt, &a.PlannedServiceStartAt, &a.PlannedDepartureAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if out == nil {
		out = []domain.Allocation{}
	}
	return out, rows.Err()
}

func (p Postgres) ListTrips(ctx context.Context, planID string) ([]domain.Trip, error) {
	rows, err := p.Pool.Query(ctx, `SELECT id::text, plan_id::text, vehicle_id, trip_number, status FROM trips WHERE plan_id::text = $1 ORDER BY vehicle_id, trip_number`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Trip
	for rows.Next() {
		var t domain.Trip
		if err := rows.Scan(&t.ID, &t.PlanID, &t.VehicleID, &t.TripNumber, &t.Status); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if out == nil {
		out = []domain.Trip{}
	}
	return out, rows.Err()
}

func (p Postgres) InsertDeferral(ctx context.Context, d domain.Deferral) error {
	detail, _ := json.Marshal(d.ReasonDetail)
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO deferrals (plan_id, order_id, outlet_id, reason_code, reason_detail, comment, deferred_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (plan_id, order_id) DO UPDATE SET reason_code = EXCLUDED.reason_code, reason_detail = EXCLUDED.reason_detail, comment = EXCLUDED.comment
	`, d.PlanID, d.OrderID, d.OutletID, d.ReasonCode, detail, d.Comment, d.DeferredBy)
	return err
}

func (p Postgres) ListDeferrals(ctx context.Context, planID string) ([]domain.Deferral, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT id::text, plan_id::text, order_id, COALESCE(outlet_id,''), reason_code, COALESCE(reason_detail,'{}'), COALESCE(comment,''), deferred_by
		FROM deferrals WHERE plan_id::text = $1
	`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Deferral
	for rows.Next() {
		var d domain.Deferral
		var raw []byte
		if err := rows.Scan(&d.ID, &d.PlanID, &d.OrderID, &d.OutletID, &d.ReasonCode, &raw, &d.Comment, &d.DeferredBy); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &d.ReasonDetail)
		out = append(out, d)
	}
	if out == nil {
		out = []domain.Deferral{}
	}
	return out, rows.Err()
}

func (p Postgres) OutletDeferralCounts(ctx context.Context) (map[string]int, error) {
	rows, err := p.Pool.Query(ctx, `SELECT outlet_id, count(*) FROM deferrals WHERE outlet_id IS NOT NULL AND outlet_id <> '' GROUP BY outlet_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

func (p Postgres) WeekFuelByVehicle(ctx context.Context, weekStart, weekEnd, excludePlan string) (map[string]float64, error) {
	// Distance is recomputed at validation time; store a conservative 0 here
	// and let the service add confirmed-plan projected fuel when it rebuilds trips.
	return map[string]float64{}, nil
}

func (p Postgres) ConfirmedPlansInWeek(ctx context.Context, from, to, exclude string) ([]domain.Plan, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT id::text, plan_ref, delivery_date::text, status, created_by, generated_at, current_version, published_at
		FROM plans WHERE status = 'confirmed' AND delivery_date >= $1 AND delivery_date <= $2 AND id::text <> $3
	`, from, to, exclude)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Plan
	for rows.Next() {
		pl, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, pl)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanPlan(row scanner) (domain.Plan, error) {
	var pl domain.Plan
	var gen *time.Time
	err := row.Scan(&pl.ID, &pl.PlanRef, &pl.DeliveryDate, &pl.Status, &pl.CreatedBy, &gen, &pl.CurrentVersion, &pl.PublishedAt)
	if err == pgx.ErrNoRows {
		return pl, fmt.Errorf("not found")
	}
	pl.GeneratedAt = gen
	return pl, err
}
